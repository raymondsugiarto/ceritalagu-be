package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/user"
)

// Users is the customer store (user.GormRepository).
type Users interface {
	GetByID(ctx context.Context, id string) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
	MarkSignedIn(ctx context.Context, id string, at time.Time) error
	Promote(ctx context.Context, id string, t user.Type) error
}

// Guests links the visitor's guest session to the user who signs in (guest.GormRepository).
type Guests interface {
	Claim(ctx context.Context, sessionID, userID string, at time.Time) error
}

// Service holds the customer sign-in rules.
type Service struct {
	repo   Repository
	users  Users
	guests Guests
	mailer mail.Sender
	keys   Keys
	now    func() time.Time
	code   func() (string, error)
}

// NewService builds the service.
func NewService(repo Repository, users Users, guests Guests, mailer mail.Sender, keys Keys) *Service {
	return &Service{repo: repo, users: users, guests: guests, mailer: mailer, keys: keys,
		now: func() time.Time { return time.Now().UTC() }, code: newCode}
}

// bcryptCost is a var so tests can lower it.
var bcryptCost = 12

// dummyHash is compared against when there is no credential, so a miss takes as long as a wrong password.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcryptCost)

var (
	errNotRegistered = apperror.NotFound("Email ini belum terdaftar. Buat lagu pertamamu dulu — emailmu akan terdaftar di langkah terakhir.")
	errSuspended     = apperror.Forbidden("Akun ini sedang dinonaktifkan. Hubungi kami untuk bantuan.")
	errBadLogin      = apperror.Unauthorized("Email atau password salah.")
	errCodeInvalid   = "Kode salah. Periksa lagi email dari kami."
	errCodeExpired   = apperror.BadRequest("Kode sudah tidak berlaku. Minta kode baru.")
	errSetupExpired  = apperror.Unauthorized("Sesi verifikasi sudah berakhir. Masuk lagi untuk menerima kode baru.")
)

// Lookup says whether email signs in with a password or an emailed code.
func (s *Service) Lookup(ctx context.Context, email string) (LookupOutput, error) {
	u, err := s.findActive(ctx, email)
	if err != nil {
		return LookupOutput{}, err
	}
	method, err := s.methodOf(ctx, u.ID)
	if err != nil {
		return LookupOutput{}, err
	}
	return LookupOutput{Email: u.Email, Method: method}, nil
}

// RequestOTP emails a login code to a user who has no password yet. One code per minute per email.
func (s *Service) RequestOTP(ctx context.Context, email string) (OTPSentOutput, error) {
	u, err := s.findActive(ctx, email)
	if err != nil {
		return OTPSentOutput{}, err
	}
	method, err := s.methodOf(ctx, u.ID)
	if err != nil {
		return OTPSentOutput{}, err
	}
	if method == MethodPassword {
		return OTPSentOutput{}, apperror.Conflict("Akun ini sudah punya password. Masuk dengan password.")
	}
	now := s.now()
	last, err := s.repo.LatestOTP(ctx, u.Email, PurposeLogin)
	switch {
	case err == nil:
		// A code was sent moments ago (double click, page reload): keep it valid and say when a new
		// one may be requested, instead of failing and leaving the user without a code screen.
		if wait := last.CreatedAt.Add(OTPResendAfter).Sub(now); wait > 0 && last.ConsumedAt == nil && last.Attempts < OTPMaxAttempts {
			return OTPSentOutput{Email: u.Email, ExpiresInSec: int(last.ExpiresAt.Sub(now).Seconds()),
				ResendAfterSec: int(wait.Seconds()) + 1}, nil
		}
	case !errors.Is(err, ErrNotFound):
		return OTPSentOutput{}, apperror.Internal(err)
	}

	code, err := s.code()
	if err != nil {
		return OTPSentOutput{}, apperror.Internal(err)
	}
	o := OTP{ID: uuid.NewString(), UserID: u.ID, Email: u.Email, Purpose: PurposeLogin,
		CodeHash: s.keys.hashOTP(u.Email, PurposeLogin, code), ExpiresAt: now.Add(OTPTTL), CreatedAt: now}
	if err := s.repo.CreateOTP(ctx, o); err != nil {
		return OTPSentOutput{}, apperror.Internal(err)
	}
	if err := s.mailer.Send(ctx, otpMessage(u.Email, code)); err != nil {
		return OTPSentOutput{}, apperror.Unavailable("Kode belum bisa dikirim. Coba lagi sebentar lagi.", err)
	}
	return OTPSentOutput{Email: u.Email, ExpiresInSec: int(OTPTTL.Seconds()), ResendAfterSec: int(OTPResendAfter.Seconds())}, nil
}

// VerifyOTP checks the latest code for email and returns a setup token for choosing a password.
// A code works once, for OTPTTL, and is locked after OTPMaxAttempts wrong guesses.
func (s *Service) VerifyOTP(ctx context.Context, in VerifyOTPInput) (OTPVerifiedOutput, error) {
	email := normalizeEmail(in.Email)
	o, err := s.repo.LatestOTP(ctx, email, PurposeLogin)
	if errors.Is(err, ErrNotFound) {
		return OTPVerifiedOutput{}, errCodeExpired
	}
	if err != nil {
		return OTPVerifiedOutput{}, apperror.Internal(err)
	}
	now := s.now()
	if o.ConsumedAt != nil || !now.Before(o.ExpiresAt) || o.Attempts >= OTPMaxAttempts {
		return OTPVerifiedOutput{}, errCodeExpired
	}
	want := s.keys.hashOTP(email, PurposeLogin, strings.TrimSpace(in.Code))
	if subtle.ConstantTimeCompare([]byte(want), []byte(o.CodeHash)) != 1 {
		n, err := s.repo.AddOTPAttempt(ctx, o.ID, now)
		if err != nil {
			return OTPVerifiedOutput{}, apperror.Internal(err)
		}
		if n >= OTPMaxAttempts {
			return OTPVerifiedOutput{}, apperror.BadRequest("Terlalu banyak kode salah. Minta kode baru.")
		}
		return OTPVerifiedOutput{}, apperror.Validation([]apperror.ErrorDetail{{Field: "code", Message: errCodeInvalid}})
	}
	if err := s.repo.ConsumeOTP(ctx, o.ID, now); err != nil {
		if errors.Is(err, ErrAlreadyUsed) {
			return OTPVerifiedOutput{}, errCodeExpired
		}
		return OTPVerifiedOutput{}, apperror.Internal(err)
	}
	if _, err := s.findActive(ctx, email); err != nil {
		return OTPVerifiedOutput{}, err
	}
	token := s.keys.issue(kindSetup, o.UserID, "", now.Add(SetupTokenTTL))
	return OTPVerifiedOutput{SetupToken: token, ExpiresInSec: int(SetupTokenTTL.Seconds())}, nil
}

// SetPassword creates the user's first credential from a setup token and signs them in.
func (s *Service) SetPassword(ctx context.Context, v guest.Visitor, in SetPasswordInput) (CustomerOutput, string, error) {
	if err := checkPassword(in.Password); err != nil {
		return CustomerOutput{}, "", err
	}
	now := s.now()
	c, err := s.keys.parse(in.SetupToken, kindSetup, now)
	if err != nil {
		return CustomerOutput{}, "", errSetupExpired.Wrap(err)
	}
	u, err := s.users.GetByID(ctx, c.Subject)
	if errors.Is(err, user.ErrNotFound) {
		return CustomerOutput{}, "", errSetupExpired.Wrap(err)
	}
	if err != nil {
		return CustomerOutput{}, "", apperror.Internal(err)
	}
	if u.Status == user.StatusSuspended {
		return CustomerOutput{}, "", errSuspended
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcryptCost)
	if err != nil {
		return CustomerOutput{}, "", apperror.Internal(fmt.Errorf("hash password: %w", err))
	}
	cred := Credential{ID: uuid.NewString(), UserID: u.ID, PasswordHash: string(hash), PasswordSetAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		if errors.Is(err, ErrCredentialed) {
			return CustomerOutput{}, "", apperror.Conflict("Password akun ini sudah diatur. Masuk dengan password.")
		}
		return CustomerOutput{}, "", apperror.Internal(err)
	}
	// A registered customer (password set) is a prospect in the funnel.
	if err := s.users.Promote(ctx, u.ID, user.TypeProspect); err != nil {
		return CustomerOutput{}, "", apperror.Internal(err)
	}
	return s.signIn(ctx, v, u, cred, now)
}

// Login checks email + password. Every failure is the same 401.
func (s *Service) Login(ctx context.Context, v guest.Visitor, in LoginInput) (CustomerOutput, string, error) {
	u, err := s.users.GetByEmail(ctx, normalizeEmail(in.Email))
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		return CustomerOutput{}, "", apperror.Internal(err)
	}
	var cred Credential
	if err == nil {
		cred, err = s.repo.CredentialByUserID(ctx, u.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return CustomerOutput{}, "", apperror.Internal(err)
		}
	}
	hash := []byte(cred.PasswordHash)
	if len(hash) == 0 {
		hash = dummyHash
	}
	ok := bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) == nil && cred.PasswordHash != ""
	if !ok {
		return CustomerOutput{}, "", errBadLogin
	}
	if u.Status == user.StatusSuspended {
		return CustomerOutput{}, "", errSuspended
	}
	return s.signIn(ctx, v, u, cred, s.now())
}

// Authenticate turns a session token into the signed-in customer.
func (s *Service) Authenticate(ctx context.Context, token string) (user.User, error) {
	unauthorized := apperror.Unauthorized("Silakan masuk terlebih dahulu.")
	c, err := s.keys.parse(token, kindSession, s.now())
	if err != nil {
		return user.User{}, unauthorized.Wrap(err)
	}
	u, err := s.users.GetByID(ctx, c.Subject)
	if errors.Is(err, user.ErrNotFound) {
		return user.User{}, unauthorized.Wrap(err)
	}
	if err != nil {
		return user.User{}, apperror.Internal(err)
	}
	cred, err := s.repo.CredentialByUserID(ctx, u.ID)
	if errors.Is(err, ErrNotFound) || (err == nil && stamp(cred.PasswordHash) != c.Stamp) {
		return user.User{}, unauthorized.Wrap(errors.New("password changed"))
	}
	if err != nil {
		return user.User{}, apperror.Internal(err)
	}
	if u.Status == user.StatusSuspended {
		return user.User{}, unauthorized.Wrap(errors.New("user suspended"))
	}
	return u, nil
}

// signIn records the sign-in, claims the visitor's guest songs and issues the session token.
func (s *Service) signIn(ctx context.Context, v guest.Visitor, u user.User, cred Credential, now time.Time) (CustomerOutput, string, error) {
	if err := s.users.MarkSignedIn(ctx, u.ID, now); err != nil {
		return CustomerOutput{}, "", apperror.Internal(err)
	}
	if v.SessionID != "" {
		if err := s.guests.Claim(ctx, v.SessionID, u.ID, now); err != nil {
			return CustomerOutput{}, "", apperror.Internal(err)
		}
	}
	if u.Status == user.StatusDraft {
		u.Status = user.StatusActive
	}
	token := s.keys.issue(kindSession, u.ID, stamp(cred.PasswordHash), now.Add(SessionTTL))
	return toCustomerOutput(u), token, nil
}

func (s *Service) findActive(ctx context.Context, email string) (user.User, error) {
	u, err := s.users.GetByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, user.ErrNotFound) {
		return user.User{}, errNotRegistered
	}
	if err != nil {
		return user.User{}, apperror.Internal(err)
	}
	if u.Status == user.StatusSuspended {
		return user.User{}, errSuspended
	}
	return u, nil
}

func (s *Service) methodOf(ctx context.Context, userID string) (Method, error) {
	_, err := s.repo.CredentialByUserID(ctx, userID)
	switch {
	case err == nil:
		return MethodPassword, nil
	case errors.Is(err, ErrNotFound):
		return MethodOTP, nil
	default:
		return "", apperror.Internal(err)
	}
}

func checkPassword(p string) error {
	var msg string
	switch {
	case len(p) < MinPasswordLength:
		msg = fmt.Sprintf("Minimal %d karakter.", MinPasswordLength)
	case len(p) > MaxPasswordLength:
		msg = fmt.Sprintf("Maksimal %d karakter.", MaxPasswordLength)
	case strings.TrimSpace(p) == "":
		msg = "Password tidak boleh hanya spasi."
	}
	if msg != "" {
		return apperror.Validation([]apperror.ErrorDetail{{Field: "password", Message: msg}})
	}
	return nil
}

func otpMessage(to, code string) mail.Message {
	return mail.Message{
		To:      to,
		Subject: "Kode masuk Lagu Untukmu: " + code,
		Text: "Halo!\n\n" +
			"Kode masukmu: " + code + "\n\n" +
			fmt.Sprintf("Kode ini berlaku %d menit dan hanya bisa dipakai sekali. ", int(OTPTTL.Minutes())) +
			"Jangan berikan kode ini kepada siapa pun, termasuk yang mengaku dari tim kami.\n\n" +
			"Kalau kamu tidak sedang masuk ke Lagu Untukmu, abaikan email ini.\n\n" +
			"— Lagu Untukmu\n",
	}
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

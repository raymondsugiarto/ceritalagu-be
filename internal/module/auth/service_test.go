package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/mail"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/user"
)

func init() { bcryptCost = 4 }

type fakeMailer struct{ sent []mail.Message }

func (m *fakeMailer) Send(_ context.Context, msg mail.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

type fakeGuests struct{ claims map[string]string }

func (g *fakeGuests) Claim(_ context.Context, sessionID, userID string, _ time.Time) error {
	g.claims[sessionID] = userID
	return nil
}

type fixture struct {
	svc    *Service
	users  *user.MemoryRepository
	mailer *fakeMailer
	guests *fakeGuests
	clock  time.Time
	codes  []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{users: user.NewMemoryRepository(), mailer: &fakeMailer{}, guests: &fakeGuests{claims: map[string]string{}},
		clock: time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)}
	f.svc = NewService(NewMemoryRepository(), f.users, f.guests, f.mailer, NewKeys("test-master-key"))
	f.svc.now = func() time.Time { return f.clock }
	f.svc.code = func() (string, error) {
		c := "123456"
		f.codes = append(f.codes, c)
		return c, nil
	}
	return f
}

func (f *fixture) addUser(t *testing.T, email string, st user.Status) user.User {
	t.Helper()
	u := user.User{ID: "u-" + email, Email: email, Status: st, CreatedAt: f.clock, UpdatedAt: f.clock}
	if err := f.users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func code(t *testing.T, err error) apperror.Code {
	t.Helper()
	var ae *apperror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *apperror.Error, got %v", err)
	}
	return ae.Code
}

func TestFirstLoginWithOTPThenPassword(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.addUser(t, "ana@example.com", user.StatusDraft)
	v := guest.Visitor{SessionID: "g-1"}

	look, err := f.svc.Lookup(ctx, " Ana@Example.com ")
	if err != nil || look.Method != MethodOTP {
		t.Fatalf("lookup = %+v, %v; want otp", look, err)
	}
	if _, err := f.svc.RequestOTP(ctx, "ana@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(f.mailer.sent) != 1 || !strings.Contains(f.mailer.sent[0].Text, "123456") {
		t.Fatalf("mail not sent with the code: %+v", f.mailer.sent)
	}

	// Wrong code: validation error on "code".
	if _, err := f.svc.VerifyOTP(ctx, VerifyOTPInput{Email: "ana@example.com", Code: "000000"}); code(t, err) != apperror.CodeValidation {
		t.Fatalf("wrong code err = %v", err)
	}
	ver, err := f.svc.VerifyOTP(ctx, VerifyOTPInput{Email: "ana@example.com", Code: "123456"})
	if err != nil {
		t.Fatal(err)
	}
	// A code works once.
	if _, err := f.svc.VerifyOTP(ctx, VerifyOTPInput{Email: "ana@example.com", Code: "123456"}); err == nil {
		t.Fatal("reused code accepted")
	}

	out, token, err := f.svc.SetPassword(ctx, v, SetPasswordInput{SetupToken: ver.SetupToken, Password: "rahasia-123"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != string(user.StatusActive) || token == "" || f.guests.claims["g-1"] != out.ID {
		t.Fatalf("set password: out=%+v token=%q claims=%v", out, token, f.guests.claims)
	}
	if u, _ := f.svc.Authenticate(ctx, token); u.ID != out.ID {
		t.Fatal("session token does not authenticate")
	}

	// Second time: lookup says password; OTP is refused; password login works.
	if look, _ := f.svc.Lookup(ctx, "ana@example.com"); look.Method != MethodPassword {
		t.Fatalf("lookup after set = %v", look.Method)
	}
	f.clock = f.clock.Add(2 * time.Minute)
	if _, err := f.svc.RequestOTP(ctx, "ana@example.com"); code(t, err) != apperror.CodeConflict {
		t.Fatalf("otp for password user err = %v", err)
	}
	if _, _, err := f.svc.Login(ctx, v, LoginInput{Email: "ana@example.com", Password: "salah-sekali"}); code(t, err) != apperror.CodeUnauthorized {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, _, err := f.svc.Login(ctx, v, LoginInput{Email: "ANA@example.com", Password: "rahasia-123"}); err != nil {
		t.Fatal(err)
	}
	// The setup token cannot set a second password.
	if _, _, err := f.svc.SetPassword(ctx, v, SetPasswordInput{SetupToken: ver.SetupToken, Password: "lain-lagi-1"}); code(t, err) != apperror.CodeConflict {
		t.Fatalf("second set password err = %v", err)
	}
}

func TestOTPRules(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.addUser(t, "budi@example.com", user.StatusDraft)
	f.addUser(t, "off@example.com", user.StatusSuspended)

	if _, err := f.svc.Lookup(ctx, "nobody@example.com"); code(t, err) != apperror.CodeNotFound {
		t.Fatalf("unknown email err = %v", err)
	}
	if _, err := f.svc.RequestOTP(ctx, "off@example.com"); code(t, err) != apperror.CodeForbidden {
		t.Fatalf("suspended err = %v", err)
	}
	if _, err := f.svc.RequestOTP(ctx, "budi@example.com"); err != nil {
		t.Fatal(err)
	}
	// Asking again within a minute keeps the first code: no new email, the wait is reported.
	again, err := f.svc.RequestOTP(ctx, "budi@example.com")
	if err != nil || again.ResendAfterSec != int(OTPResendAfter.Seconds())+1 || len(f.mailer.sent) != 1 {
		t.Fatalf("resend within a minute = %+v, %v; mails=%d", again, err, len(f.mailer.sent))
	}

	// Five wrong guesses lock the code, even the right one fails afterwards.
	for range OTPMaxAttempts {
		_, _ = f.svc.VerifyOTP(ctx, VerifyOTPInput{Email: "budi@example.com", Code: "999999"})
	}
	if _, err := f.svc.VerifyOTP(ctx, VerifyOTPInput{Email: "budi@example.com", Code: "123456"}); code(t, err) != apperror.CodeBadRequest {
		t.Fatalf("locked code err = %v", err)
	}

	// Expired code.
	f.clock = f.clock.Add(2 * time.Minute)
	if _, err := f.svc.RequestOTP(ctx, "budi@example.com"); err != nil {
		t.Fatal(err)
	}
	f.clock = f.clock.Add(OTPTTL)
	if _, err := f.svc.VerifyOTP(ctx, VerifyOTPInput{Email: "budi@example.com", Code: "123456"}); code(t, err) != apperror.CodeBadRequest {
		t.Fatalf("expired code err = %v", err)
	}
}

func TestTokensAreNotInterchangeable(t *testing.T) {
	k := NewKeys("k")
	now := time.Now()
	setup := k.issue(kindSetup, "u1", "", now.Add(time.Minute))
	if _, err := k.parse(setup, kindSession, now); err == nil {
		t.Fatal("setup token accepted as a session")
	}
	if _, err := k.parse(setup, kindSetup, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := k.parse(setup+"x", kindSetup, now); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err := NewKeys("other").parse(setup, kindSetup, now); err == nil {
		t.Fatal("token signed with another key accepted")
	}
}

func TestShortPasswordRejected(t *testing.T) {
	f := newFixture(t)
	_, _, err := f.svc.SetPassword(context.Background(), guest.Visitor{}, SetPasswordInput{SetupToken: "x", Password: "short"})
	if code(t, err) != apperror.CodeValidation {
		t.Fatalf("err = %v", err)
	}
}

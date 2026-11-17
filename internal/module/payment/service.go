package payment

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/payment/doku"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
)

// Repository is the order store (GormRepository).
type Repository interface {
	Create(ctx context.Context, o Order) error
	Update(ctx context.Context, o Order) error
	Get(ctx context.Context, id string) (Order, error)
	GetByInvoice(ctx context.Context, invoice string) (Order, error)
	InvoiceTaken(ctx context.Context, invoice string) (bool, error)
	OpenForSong(ctx context.Context, songID string, now time.Time) (Order, error)
	MarkPaid(ctx context.Context, id, method string, payload map[string]any, at time.Time) (bool, error)
	CountPaid(ctx context.Context, userID string) (int64, error)
}

// Songs reads and unlocks songs (song.Service).
type Songs interface {
	Owned(ctx context.Context, v guest.Visitor, id string) (song.Song, error)
	Unlock(ctx context.Context, id, ownerUserID string, at time.Time) error
}

// Users reads the buyer and moves them through the funnel (user.GormRepository).
type Users interface {
	GetByID(ctx context.Context, id string) (user.User, error)
	Promote(ctx context.Context, id string, t user.Type) error
}

// Gateway is DOKU Checkout (doku.Client). nil when payments are not configured.
type Gateway interface {
	CreateCheckout(ctx context.Context, r doku.CheckoutRequest) (doku.CheckoutResult, error)
	CheckStatus(ctx context.Context, invoiceNumber string) (doku.StatusResult, error)
	VerifyNotification(h http.Header, target string, body []byte) bool
}

// Settings are the payment rules from payment.yml.
type Settings struct {
	UnlockPrice   int64
	OrderTTL      time.Duration
	PublicBaseURL string // customer site; DOKU returns the buyer to <it>/songs/<song_id>?order=<id>
	NotifyPath    string // signed path of the DOKU Notification URL
}

// Service holds the checkout rules.
type Service struct {
	repo    Repository
	songs   Songs
	users   Users
	gateway Gateway
	cfg     Settings
	log     *slog.Logger
	now     func() time.Time
}

// NewService builds the service. gateway may be nil: checkout then answers 503.
func NewService(repo Repository, songs Songs, users Users, gateway Gateway, cfg Settings, log *slog.Logger) *Service {
	return &Service{repo: repo, songs: songs, users: users, gateway: gateway, cfg: cfg, log: log,
		now: func() time.Time { return time.Now().UTC() }}
}

var errDisabled = apperror.Unavailable("Pembayaran belum tersedia. Silakan coba lagi nanti.", errors.New("payment gateway not configured"))

// Pricing is shown next to the unlock button.
func (s *Service) Pricing() PricingOutput {
	return PricingOutput{UnlockPrice: s.cfg.UnlockPrice, Currency: "IDR", Enabled: s.gateway != nil}
}

// Checkout opens (or reuses) a DOKU payment page for the visitor's song.
func (s *Service) Checkout(ctx context.Context, v guest.Visitor, songID string) (CheckoutOutput, error) {
	if s.gateway == nil {
		return CheckoutOutput{}, errDisabled
	}
	sg, err := s.songs.Owned(ctx, v, songID)
	if err != nil {
		return CheckoutOutput{}, err
	}
	if sg.IsUnlocked {
		return CheckoutOutput{}, apperror.Conflict("Lagu ini sudah terbuka penuh.")
	}
	if sg.OwnerUserID == "" {
		return CheckoutOutput{}, apperror.Conflict("Isi email dan nomor HP dulu sebelum membeli lagu.")
	}
	buyer, err := s.users.GetByID(ctx, sg.OwnerUserID)
	if err != nil {
		return CheckoutOutput{}, apperror.Internal(err)
	}
	if buyer.Status == user.StatusSuspended {
		return CheckoutOutput{}, apperror.Forbidden("Akun ini dinonaktifkan.")
	}
	now := s.now()

	// A pending order still inside its payment window is reused: the customer gets the same DOKU page.
	if open, err := s.repo.OpenForSong(ctx, sg.ID, now); err == nil {
		return toCheckoutOutput(open), nil
	} else if !errors.Is(err, ErrNotFound) {
		return CheckoutOutput{}, apperror.Internal(err)
	}

	invoice, err := s.newInvoice(ctx, now)
	if err != nil {
		return CheckoutOutput{}, apperror.Internal(err)
	}
	expires := now.Add(s.cfg.OrderTTL)
	o := Order{
		ID: uuid.NewString(), InvoiceNumber: invoice, UserID: buyer.ID, SongID: sg.ID, GuestSessionID: v.SessionID,
		Gateway: GatewayDoku, Amount: s.cfg.UnlockPrice, Currency: "IDR", Status: StatusPending,
		ExpiresAt: &expires, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, o); err != nil {
		return CheckoutOutput{}, apperror.Internal(err)
	}
	// Starting a checkout is strong buying intent.
	if err := s.users.Promote(ctx, buyer.ID, user.TypeProspect); err != nil {
		s.log.Warn("promote buyer to prospect", "user_id", buyer.ID, "err", err)
	}

	res, err := s.gateway.CreateCheckout(ctx, doku.CheckoutRequest{
		InvoiceNumber: invoice, Amount: o.Amount, DueMinutes: max(int(s.cfg.OrderTTL.Minutes()), 1),
		CallbackURL: s.returnURL(sg.ID, o.ID), ItemName: "Full lagu: " + sg.Title,
		CustomerID: buyer.ID, CustomerName: displayName(buyer), CustomerEmail: buyer.Email, CustomerPhone: buyer.Phone,
	})
	if err != nil {
		o.Status, o.UpdatedAt = StatusFailed, s.now()
		o.GatewayResponse = map[string]any{"error": err.Error()}
		if saveErr := s.repo.Update(ctx, o); saveErr != nil {
			err = errors.Join(err, saveErr)
		}
		return CheckoutOutput{}, apperror.Unavailable("Halaman pembayaran tidak bisa dibuka. Coba lagi sebentar lagi.", err)
	}
	o.PaymentURL, o.GatewayRequestID, o.GatewayResponse, o.UpdatedAt = res.PaymentURL, res.RequestID, res.Raw, s.now()
	if err := s.repo.Update(ctx, o); err != nil {
		return CheckoutOutput{}, apperror.Internal(err)
	}
	return toCheckoutOutput(o), nil
}

// GetOrder returns the visitor's order. A pending order is checked with DOKU first, so the result page
// shows "paid" right away even when the notification has not arrived (or cannot reach a dev machine).
func (s *Service) GetOrder(ctx context.Context, v guest.Visitor, id string) (OrderOutput, error) {
	o, err := s.repo.Get(ctx, id)
	if errors.Is(err, ErrNotFound) || (err == nil && o.GuestSessionID != v.SessionID) {
		return OrderOutput{}, apperror.NotFound("payment order not found")
	}
	if err != nil {
		return OrderOutput{}, apperror.Internal(err)
	}
	if o.Status == StatusPending && s.gateway != nil && o.PaymentURL != "" {
		st, err := s.gateway.CheckStatus(ctx, o.InvoiceNumber)
		switch {
		case err == nil:
			if o, err = s.apply(ctx, o, st, false); err != nil {
				return OrderOutput{}, err
			}
		case errors.Is(err, doku.ErrNotFound):
			// The customer has not picked a payment method yet.
		default:
			s.log.Warn("doku status check", "order_id", o.ID, "err", err)
		}
	}
	if o.Status == StatusPending && o.ExpiresAt != nil && s.now().After(*o.ExpiresAt) {
		o.Status, o.UpdatedAt = StatusExpired, s.now()
		if err := s.repo.Update(ctx, o); err != nil {
			return OrderOutput{}, apperror.Internal(err)
		}
	}
	return toOrderOutput(o), nil
}

// HandleNotification processes a DOKU HTTP notification. Unsigned or unknown notifications are rejected.
func (s *Service) HandleNotification(ctx context.Context, h http.Header, body []byte) error {
	if s.gateway == nil {
		return errDisabled
	}
	if !s.gateway.VerifyNotification(h, s.cfg.NotifyPath, body) {
		return apperror.Unauthorized("invalid signature")
	}
	st, err := doku.ParseResult(body)
	if err != nil {
		return apperror.BadRequest("invalid notification body")
	}
	o, err := s.repo.GetByInvoice(ctx, st.InvoiceNumber)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("unknown invoice number")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	s.log.Info("doku notification", "order_id", o.ID, "invoice", o.InvoiceNumber, "status", st.Status, "channel", st.Channel)
	_, err = s.apply(ctx, o, st, true)
	return err
}

// apply moves the order to DOKU's status. On SUCCESS it is marked paid (once), the song is unlocked
// and the buyer promoted; both are idempotent, so they run again on every SUCCESS in case an earlier
// attempt failed half-way.
func (s *Service) apply(ctx context.Context, o Order, st doku.StatusResult, fromNotification bool) (Order, error) {
	now := s.now()
	switch st.Status {
	case doku.StatusSuccess:
		if st.Amount != 0 && st.Amount != o.Amount {
			s.log.Error("doku amount mismatch", "order_id", o.ID, "want", o.Amount, "got", st.Amount)
			return o, apperror.Conflict("amount mismatch")
		}
		var payload map[string]any
		if fromNotification {
			payload = st.Raw
		}
		if _, err := s.repo.MarkPaid(ctx, o.ID, st.Channel, payload, now); err != nil {
			return o, apperror.Internal(err)
		}
		if o.Status != StatusPaid {
			o.Status, o.PaidAt, o.PaymentMethod = StatusPaid, &now, st.Channel
		}
		if err := s.fulfil(ctx, o, now); err != nil {
			return o, err
		}
	case doku.StatusFailed, doku.StatusExpired, doku.StatusTimeout:
		if o.Status != StatusPending {
			return o, nil
		}
		o.Status = StatusFailed
		if st.Status != doku.StatusFailed {
			o.Status = StatusExpired
		}
		o.UpdatedAt = now
		if fromNotification {
			o.NotificationPayload = st.Raw
		}
		if err := s.repo.Update(ctx, o); err != nil {
			return o, apperror.Internal(err)
		}
	}
	return o, nil
}

// fulfil unlocks the song and moves the buyer to customer (first purchase) or repeat_customer.
func (s *Service) fulfil(ctx context.Context, o Order, at time.Time) error {
	if err := s.songs.Unlock(ctx, o.SongID, o.UserID, at); err != nil {
		return apperror.Internal(fmt.Errorf("unlock song %s: %w", o.SongID, err))
	}
	if o.UserID == "" {
		return nil
	}
	paid, err := s.repo.CountPaid(ctx, o.UserID)
	if err != nil {
		return apperror.Internal(err)
	}
	t := user.TypeCustomer
	if paid >= 2 {
		t = user.TypeRepeatCustomer
	}
	if err := s.users.Promote(ctx, o.UserID, t); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

func (s *Service) returnURL(songID, orderID string) string {
	base := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	return base + "/songs/" + url.PathEscape(songID) + "?order=" + url.QueryEscape(orderID)
}

// newInvoice returns "LU-YYMMDD-XXXXXXXX" (18 chars: under DOKU's 30-char card limit, no symbols but "-").
func (s *Service) newInvoice(ctx context.Context, now time.Time) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for range 5 {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("random invoice: %w", err)
		}
		for i := range b {
			b[i] = alphabet[int(b[i])%len(alphabet)]
		}
		inv := "LU-" + now.Format("060102") + "-" + string(b)
		taken, err := s.repo.InvoiceTaken(ctx, inv)
		if err != nil {
			return "", err
		}
		if !taken {
			return inv, nil
		}
	}
	return "", errors.New("could not mint a unique invoice number")
}

func displayName(u user.User) string {
	if u.Name != "" {
		return u.Name
	}
	name, _, _ := strings.Cut(u.Email, "@")
	return name
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

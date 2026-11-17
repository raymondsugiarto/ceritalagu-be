package payment

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/module/payment/doku"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
)

type memOrders struct {
	mu sync.Mutex
	m  map[string]Order
}

func (r *memOrders) Create(_ context.Context, o Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[o.ID] = o
	return nil
}
func (r *memOrders) Update(_ context.Context, o Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[o.ID] = o
	return nil
}
func (r *memOrders) Get(_ context.Context, id string) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.m[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	return o, nil
}
func (r *memOrders) GetByInvoice(_ context.Context, inv string) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, o := range r.m {
		if o.InvoiceNumber == inv {
			return o, nil
		}
	}
	return Order{}, ErrNotFound
}
func (r *memOrders) InvoiceTaken(ctx context.Context, inv string) (bool, error) {
	_, err := r.GetByInvoice(ctx, inv)
	return err == nil, nil
}
func (r *memOrders) OpenForSong(_ context.Context, songID string, now time.Time) (Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, o := range r.m {
		if o.SongID == songID && o.Status == StatusPending && o.PaymentURL != "" && o.ExpiresAt.After(now) {
			return o, nil
		}
	}
	return Order{}, ErrNotFound
}
func (r *memOrders) MarkPaid(_ context.Context, id, method string, _ map[string]any, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o := r.m[id]
	if o.Status == StatusPaid {
		return false, nil
	}
	o.Status, o.PaidAt, o.PaymentMethod = StatusPaid, &at, method
	r.m[id] = o
	return true, nil
}
func (r *memOrders) CountPaid(_ context.Context, userID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, o := range r.m {
		if o.UserID == userID && o.Status == StatusPaid {
			n++
		}
	}
	return n, nil
}

type fakeSongs map[string]song.Song

func (f fakeSongs) Owned(_ context.Context, v guest.Visitor, id string) (song.Song, error) {
	s, ok := f[id]
	if !ok || s.GuestSessionID != v.SessionID {
		return song.Song{}, apperror.NotFound("song not found")
	}
	return s, nil
}
func (f fakeSongs) Unlock(_ context.Context, id, _ string, at time.Time) error {
	s := f[id]
	s.IsUnlocked, s.UnlockedAt = true, &at
	f[id] = s
	return nil
}

type fakeGateway struct {
	status  string
	created int
}

func (g *fakeGateway) CreateCheckout(_ context.Context, r doku.CheckoutRequest) (doku.CheckoutResult, error) {
	g.created++
	return doku.CheckoutResult{RequestID: "req", PaymentURL: "https://pay.test/" + r.InvoiceNumber}, nil
}
func (g *fakeGateway) CheckStatus(_ context.Context, inv string) (doku.StatusResult, error) {
	if g.status == "" {
		return doku.StatusResult{}, doku.ErrNotFound
	}
	return doku.StatusResult{Status: g.status, InvoiceNumber: inv, Amount: 49000, Channel: "QRIS"}, nil
}
func (g *fakeGateway) VerifyNotification(h http.Header, _ string, _ []byte) bool {
	return h.Get("Signature") == "ok"
}

type fixture struct {
	svc   *Service
	songs fakeSongs
	users *user.MemoryRepository
	gw    *fakeGateway
	v     guest.Visitor
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	users := user.NewMemoryRepository()
	if err := users.Create(context.Background(), user.User{ID: "u1", Email: "a@b.c", Status: user.StatusDraft, Type: user.TypeLead}); err != nil {
		t.Fatal(err)
	}
	v := guest.Visitor{SessionID: "g1"}
	songs := fakeSongs{"s1": {ID: "s1", GuestSessionID: "g1", OwnerUserID: "u1", Title: "Untuk Ibu"}}
	gw := &fakeGateway{}
	svc := NewService(&memOrders{m: map[string]Order{}}, songs, users, gw,
		Settings{UnlockPrice: 49000, OrderTTL: time.Hour, PublicBaseURL: "http://site.test", NotifyPath: "/n"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return fixture{svc: svc, songs: songs, users: users, gw: gw, v: v}
}

func typeOf(t *testing.T, f fixture) user.Type {
	t.Helper()
	u, err := f.users.GetByID(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	return u.Type
}

func TestCheckoutReusesOpenOrderAndPromotesProspect(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, err := f.svc.Checkout(ctx, f.v, "s1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.Checkout(ctx, f.v, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if a.OrderID != b.OrderID || f.gw.created != 1 {
		t.Fatalf("pending order not reused: %s vs %s, %d gateway calls", a.OrderID, b.OrderID, f.gw.created)
	}
	if a.PaymentURL == "" || a.Amount != 49000 {
		t.Fatalf("checkout = %+v", a)
	}
	if got := typeOf(t, f); got != user.TypeProspect {
		t.Fatalf("type = %s, want prospect", got)
	}
}

func TestCheckoutOtherVisitorIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Checkout(context.Background(), guest.Visitor{SessionID: "someone-else"}, "s1")
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Status != http.StatusNotFound {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestStatusCheckMarksPaidUnlocksAndPromotesCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	co, err := f.svc.Checkout(ctx, f.v, "s1")
	if err != nil {
		t.Fatal(err)
	}
	f.gw.status = doku.StatusSuccess
	o, err := f.svc.GetOrder(ctx, f.v, co.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusPaid || o.PaymentMethod != "QRIS" {
		t.Fatalf("order = %+v", o)
	}
	if !f.songs["s1"].IsUnlocked {
		t.Fatal("song not unlocked")
	}
	if got := typeOf(t, f); got != user.TypeCustomer {
		t.Fatalf("type = %s, want customer", got)
	}
}

func TestNotificationRequiresSignature(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	co, _ := f.svc.Checkout(ctx, f.v, "s1")
	o, _ := f.svc.GetOrder(ctx, f.v, co.OrderID)
	body := []byte(`{"order":{"invoice_number":"` + o.InvoiceNumber + `","amount":49000},"transaction":{"status":"SUCCESS"}}`)

	if err := f.svc.HandleNotification(ctx, http.Header{}, body); err == nil {
		t.Fatal("unsigned notification accepted")
	}
	h := http.Header{}
	h.Set("Signature", "ok")
	if err := f.svc.HandleNotification(ctx, h, body); err != nil {
		t.Fatal(err)
	}
	// A repeated notification is fine.
	if err := f.svc.HandleNotification(ctx, h, body); err != nil {
		t.Fatal(err)
	}
	if !f.songs["s1"].IsUnlocked {
		t.Fatal("song not unlocked")
	}
}

func TestNotificationAmountMismatchDoesNotUnlock(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	co, _ := f.svc.Checkout(ctx, f.v, "s1")
	o, _ := f.svc.GetOrder(ctx, f.v, co.OrderID)
	h := http.Header{}
	h.Set("Signature", "ok")
	body := []byte(`{"order":{"invoice_number":"` + o.InvoiceNumber + `","amount":1000},"transaction":{"status":"SUCCESS"}}`)
	if err := f.svc.HandleNotification(ctx, h, body); err == nil {
		t.Fatal("wrong amount accepted")
	}
	if f.songs["s1"].IsUnlocked {
		t.Fatal("song unlocked on wrong amount")
	}
}

func TestCheckoutDisabledWithoutGateway(t *testing.T) {
	svc := NewService(&memOrders{m: map[string]Order{}}, fakeSongs{}, user.NewMemoryRepository(), nil,
		Settings{UnlockPrice: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.Checkout(context.Background(), guest.Visitor{}, "x"); err == nil {
		t.Fatal("checkout without gateway should fail")
	}
	if svc.Pricing().Enabled {
		t.Fatal("pricing says enabled")
	}
}

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
	"github.com/yourorg/music-api/internal/module/notification"
	"github.com/yourorg/music-api/internal/module/payment/doku"
	"github.com/yourorg/music-api/internal/module/pricing"
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

func (r *memOrders) usage(voucherID, userID string, now time.Time) (total, byUser int64) {
	for _, o := range r.m {
		live := o.Status == StatusPaid || (o.Status == StatusPending && o.ExpiresAt.After(now))
		if o.VoucherID == voucherID && live {
			total++
			if o.UserID == userID {
				byUser++
			}
		}
	}
	return total, byUser
}
func (r *memOrders) CreateWithVoucher(_ context.Context, o Order, quota int, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	total, byUser := r.usage(o.VoucherID, o.UserID, now)
	if byUser > 0 {
		return ErrVoucherUsed
	}
	if total >= int64(quota) {
		return ErrVoucherExhausted
	}
	r.m[o.ID] = o
	return nil
}
func (r *memOrders) VoucherUsage(_ context.Context, voucherID, userID string, now time.Time) (int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, u := r.usage(voucherID, userID, now)
	return t, u, nil
}

// fakePricing: prices plus vouchers by code.
type fakePricing struct {
	p        pricing.Prices
	vouchers map[string]pricing.Voucher
}

func (f *fakePricing) Prices(context.Context) (pricing.Prices, error) { return f.p, nil }
func (f *fakePricing) UsableVoucher(_ context.Context, code string, _ time.Time) (pricing.Voucher, error) {
	v, ok := f.vouchers[pricing.NormalizeCode(code)]
	if !ok {
		return pricing.Voucher{}, apperror.BadRequest("Kode voucher tidak ditemukan.")
	}
	return v, nil
}
func (f *fakePricing) PublicPromo(context.Context) (pricing.PublicVoucherOutput, error) {
	return pricing.PublicVoucherOutput{}, apperror.NotFound("Tidak ada voucher promo yang aktif.")
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
	price *fakePricing
	repo  *memOrders
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
	price := &fakePricing{p: pricing.Prices{Normal: 99000, Sale: 49000, MinPayable: 10000}, vouchers: map[string]pricing.Voucher{
		"HEMAT10": {ID: "v1", Code: "HEMAT10", DiscountAmount: 10000, Quota: 2, IsActive: true},
		"GEDE":    {ID: "v2", Code: "GEDE", DiscountAmount: 60000, Quota: 1, IsActive: true},
	}}
	repo := &memOrders{m: map[string]Order{}}
	svc := NewService(repo, songs, users, price, gw,
		Settings{OrderTTL: time.Hour, PublicBaseURL: "http://site.test", NotifyPath: "/n"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return fixture{svc: svc, price: price, repo: repo, songs: songs, users: users, gw: gw, v: v}
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
	a, err := f.svc.Checkout(ctx, f.v, "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.svc.Checkout(ctx, f.v, "s1", "")
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
	_, err := f.svc.Checkout(context.Background(), guest.Visitor{SessionID: "someone-else"}, "s1", "")
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Status != http.StatusNotFound {
		t.Fatalf("err = %v, want 404", err)
	}
}

func TestStatusCheckMarksPaidUnlocksAndPromotesCustomer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	co, err := f.svc.Checkout(ctx, f.v, "s1", "")
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
	co, _ := f.svc.Checkout(ctx, f.v, "s1", "")
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
	co, _ := f.svc.Checkout(ctx, f.v, "s1", "")
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
	svc := NewService(&memOrders{m: map[string]Order{}}, fakeSongs{}, user.NewMemoryRepository(),
		&fakePricing{p: pricing.Prices{Normal: 1, Sale: 1, MinPayable: 1}}, nil, Settings{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.Checkout(context.Background(), guest.Visitor{}, "x", ""); err == nil {
		t.Fatal("checkout without gateway should fail")
	}
	if p, _ := svc.Pricing(context.Background()); p.Enabled {
		t.Fatal("pricing says enabled")
	}
}

func addSong(t *testing.T, f fixture, songID, userID string) {
	t.Helper()
	if userID != "u1" {
		if err := f.users.Create(context.Background(), user.User{ID: userID, Email: userID + "@b.c", Status: user.StatusDraft, Type: user.TypeLead}); err != nil {
			t.Fatal(err)
		}
	}
	f.songs[songID] = song.Song{ID: songID, GuestSessionID: "g1", OwnerUserID: userID, Title: songID}
}

func TestCheckoutUsesSalePrice(t *testing.T) {
	f := newFixture(t)
	co, err := f.svc.Checkout(context.Background(), f.v, "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	o := f.repo.m[co.OrderID]
	if o.Amount != 49000 || o.NormalPrice != 99000 || o.SalePrice != 49000 || o.VoucherCode != "" {
		t.Fatalf("order = %+v", o)
	}
	p, _ := f.svc.Pricing(context.Background())
	if p.NormalPrice != 99000 || p.SalePrice != 49000 || p.UnlockPrice != 49000 {
		t.Fatalf("pricing = %+v", p)
	}
}

func TestCheckoutWithVoucherDiscounts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	q, err := f.svc.Quote(ctx, f.v, "s1", " hemat10 ")
	if err != nil {
		t.Fatal(err)
	}
	if q.Total != 39000 || q.VoucherDiscount != 10000 || q.VoucherCode != "HEMAT10" {
		t.Fatalf("quote = %+v", q)
	}
	co, err := f.svc.Checkout(ctx, f.v, "s1", "hemat10")
	if err != nil {
		t.Fatal(err)
	}
	o := f.repo.m[co.OrderID]
	if co.Amount != 39000 || o.VoucherID != "v1" || o.VoucherDiscount != 10000 {
		t.Fatalf("order = %+v", o)
	}
	// Same voucher again: the pending order is reused.
	again, err := f.svc.Checkout(ctx, f.v, "s1", "HEMAT10")
	if err != nil || again.OrderID != co.OrderID || f.gw.created != 1 {
		t.Fatalf("not reused: %+v %v (%d gateway calls)", again, err, f.gw.created)
	}
}

func TestCheckoutVoucherClampedToMin(t *testing.T) {
	f := newFixture(t)
	co, err := f.svc.Checkout(context.Background(), f.v, "s1", "GEDE")
	if err != nil {
		t.Fatal(err)
	}
	if o := f.repo.m[co.OrderID]; o.Amount != 10000 || o.VoucherDiscount != 39000 {
		t.Fatalf("order = %+v", o)
	}
}

func TestSwitchingVoucherReplacesPendingOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	plain, err := f.svc.Checkout(ctx, f.v, "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	withV, err := f.svc.Checkout(ctx, f.v, "s1", "HEMAT10")
	if err != nil {
		t.Fatal(err)
	}
	if withV.OrderID == plain.OrderID || withV.Amount != 39000 {
		t.Fatalf("voucher order = %+v", withV)
	}
	if st := f.repo.m[plain.OrderID].Status; st != StatusExpired {
		t.Fatalf("old order status = %s, want expired", st)
	}
	// Removing the voucher frees it again.
	back, err := f.svc.Checkout(ctx, f.v, "s1", "")
	if err != nil || back.Amount != 49000 {
		t.Fatalf("back = %+v %v", back, err)
	}
	if total, _, _ := f.repo.VoucherUsage(ctx, "v1", "u1", time.Now()); total != 0 {
		t.Fatalf("voucher still held by %d orders", total)
	}
}

func TestCheckoutPriceChangeReplacesPendingOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.svc.Checkout(ctx, f.v, "s1", "HEMAT10")
	f.price.p.Sale = 45000
	b, err := f.svc.Checkout(ctx, f.v, "s1", "HEMAT10")
	if err != nil {
		t.Fatal(err)
	}
	if b.OrderID == a.OrderID || b.Amount != 35000 {
		t.Fatalf("b = %+v", b)
	}
}

func TestVoucherQuotaExhausted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	addSong(t, f, "s2", "u2")
	addSong(t, f, "s3", "u3")
	if _, err := f.svc.Checkout(ctx, f.v, "s1", "HEMAT10"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, f.v, "s2", "HEMAT10"); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Checkout(ctx, f.v, "s3", "HEMAT10")
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Status != http.StatusConflict || ae.Message != "Kuota voucher ini sudah habis." {
		t.Fatalf("err = %v, want quota exhausted", err)
	}
	if _, err := f.svc.Quote(ctx, f.v, "s3", "HEMAT10"); err == nil {
		t.Fatal("quote accepted an exhausted voucher")
	}
}

func TestVoucherOncePerUser(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	addSong(t, f, "s2", "u1")
	if _, err := f.svc.Checkout(ctx, f.v, "s1", "HEMAT10"); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.Checkout(ctx, f.v, "s2", "HEMAT10")
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Message != "Voucher ini sudah kamu pakai." {
		t.Fatalf("err = %v, want already used", err)
	}
}

func TestUnknownVoucherRejected(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Checkout(context.Background(), f.v, "s1", "NOPE"); err == nil {
		t.Fatal("unknown voucher accepted")
	}
	if f.gw.created != 0 {
		t.Fatal("gateway called for a bad voucher")
	}
}

type fakeReceipts struct{ sent chan notification.PaidOrder }

func (f fakeReceipts) PaymentSucceeded(_ context.Context, o notification.PaidOrder) error {
	f.sent <- o
	return nil
}

func TestPaidOrderSendsReceiptOnce(t *testing.T) {
	f := newFixture(t)
	rc := fakeReceipts{sent: make(chan notification.PaidOrder, 4)}
	f.svc.WithReceipts(rc)
	ctx := context.Background()
	co, err := f.svc.Checkout(ctx, f.v, "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	f.gw.status = doku.StatusSuccess
	for range 2 { // the customer's status check, then a repeat (or a late notification)
		if _, err := f.svc.GetOrder(ctx, f.v, co.OrderID); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case p := <-rc.sent:
		if p.OrderID != co.OrderID || p.UserID != "u1" || p.SongID != "s1" || p.Amount != 49000 || p.PaymentMethod != "QRIS" {
			t.Fatalf("receipt = %+v", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no receipt sent")
	}
	select {
	case p := <-rc.sent:
		t.Fatalf("receipt sent twice: %+v", p)
	case <-time.After(100 * time.Millisecond):
	}
}

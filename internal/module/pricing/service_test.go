package pricing

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/audit"
)

type memStore struct {
	mu       sync.Mutex
	prices   *Prices
	vouchers map[string]Voucher
}

func (m *memStore) GetPrices(context.Context) (Prices, error) {
	if m.prices == nil {
		return Prices{}, ErrNotFound
	}
	return *m.prices, nil
}
func (m *memStore) SavePrices(_ context.Context, p Prices, _ time.Time) error {
	m.prices = &p
	return nil
}
func (m *memStore) ListVouchers(context.Context, time.Time) ([]Voucher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Voucher, 0, len(m.vouchers))
	for _, v := range m.vouchers {
		out = append(out, v)
	}
	return out, nil
}
func (m *memStore) GetVoucher(_ context.Context, id string) (Voucher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vouchers[id]
	if !ok {
		return Voucher{}, ErrNotFound
	}
	return v, nil
}
func (m *memStore) VoucherByCode(_ context.Context, code string) (Voucher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.vouchers {
		if v.Code == code {
			return v, nil
		}
	}
	return Voucher{}, ErrNotFound
}
func (m *memStore) CreateVoucher(_ context.Context, v Voucher) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vouchers[v.ID] = v
	return nil
}
func (m *memStore) UpdateVoucher(_ context.Context, v Voucher) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vouchers[v.ID] = v
	return nil
}
func (m *memStore) ArchiveVoucher(_ context.Context, id string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vouchers, id)
	return nil
}

type nopAudit struct{}

func (nopAudit) Record(context.Context, audit.Entry) error { return nil }

var now = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

func newSvc() (*Service, *memStore) {
	st := &memStore{vouchers: map[string]Voucher{}}
	s := NewService(st, nopAudit{}, Prices{Normal: 49000, Sale: 49000, MinPayable: DefaultMinPayable})
	s.now = func() time.Time { return now }
	return s, st
}

func status(err error) int {
	var ae *apperror.Error
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

func TestPricesFallback(t *testing.T) {
	s, _ := newSvc()
	p, err := s.Prices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p != (Prices{Normal: 49000, Sale: 49000, MinPayable: 10000}) {
		t.Fatalf("prices = %+v", p)
	}
}

func TestUpdatePricesValidates(t *testing.T) {
	s, _ := newSvc()
	ctx := context.Background()
	for _, in := range []UpdatePricesInput{
		{NormalPrice: 49000, SalePrice: 59000, MinPayable: 10000}, // sale > normal
		{NormalPrice: 99000, SalePrice: 5000, MinPayable: 10000},  // sale < min
		{NormalPrice: 99000, SalePrice: 49000, MinPayable: 0},     // min must be positive
	} {
		if _, err := s.UpdatePrices(ctx, "a1", in); status(err) != http.StatusUnprocessableEntity && status(err) != http.StatusBadRequest {
			t.Fatalf("UpdatePrices(%+v) err = %v, want validation error", in, err)
		}
	}
	out, err := s.UpdatePrices(ctx, "a1", UpdatePricesInput{NormalPrice: 99000, SalePrice: 49000, MinPayable: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if out.NormalPrice != 99000 || out.SalePrice != 49000 {
		t.Fatalf("out = %+v", out)
	}
	if p, _ := s.Prices(ctx); p.Normal != 99000 {
		t.Fatalf("prices not saved: %+v", p)
	}
}

func TestQuoteClampsToMinPayable(t *testing.T) {
	p := Prices{Normal: 99000, Sale: 49000, MinPayable: 10000}
	if d, tot := Quote(p, nil); d != 0 || tot != 49000 {
		t.Fatalf("no voucher: %d %d", d, tot)
	}
	if d, tot := Quote(p, &Voucher{DiscountAmount: 10000}); d != 10000 || tot != 39000 {
		t.Fatalf("10k voucher: %d %d", d, tot)
	}
	if d, tot := Quote(p, &Voucher{DiscountAmount: 49000}); d != 39000 || tot != 10000 {
		t.Fatalf("49k voucher: %d %d", d, tot)
	}
}

func TestCreateVoucherNormalisesCodeAndRejectsDuplicate(t *testing.T) {
	s, _ := newSvc()
	ctx := context.Background()
	in := CreateVoucherInput{Code: "  hemat10 ", DiscountAmount: 10000, Quota: 5, ExpiresAt: now.Add(48 * time.Hour)}
	out, err := s.CreateVoucher(ctx, "a1", in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Code != "HEMAT10" || !out.IsActive {
		t.Fatalf("out = %+v", out)
	}
	if _, err := s.CreateVoucher(ctx, "a1", in); status(err) != http.StatusConflict {
		t.Fatalf("duplicate err = %v, want 409", err)
	}
	bad := in
	bad.Code, bad.ExpiresAt = "OK-CODE", now.Add(-time.Hour)
	if _, err := s.CreateVoucher(ctx, "a1", bad); err == nil {
		t.Fatal("expired voucher accepted")
	}
	bad = in
	bad.Code = "a!"
	if _, err := s.CreateVoucher(ctx, "a1", bad); err == nil {
		t.Fatal("bad code accepted")
	}
}

func TestUsableVoucherRejectsExpiredInactiveUnknown(t *testing.T) {
	s, st := newSvc()
	ctx := context.Background()
	st.vouchers["v1"] = Voucher{ID: "v1", Code: "LIVE", DiscountAmount: 5000, Quota: 1, ExpiresAt: now.Add(time.Hour), IsActive: true}
	st.vouchers["v2"] = Voucher{ID: "v2", Code: "OLD", DiscountAmount: 5000, Quota: 1, ExpiresAt: now.Add(-time.Hour), IsActive: true}
	st.vouchers["v3"] = Voucher{ID: "v3", Code: "OFF", DiscountAmount: 5000, Quota: 1, ExpiresAt: now.Add(time.Hour), IsActive: false}

	if v, err := s.UsableVoucher(ctx, " live ", now); err != nil || v.ID != "v1" {
		t.Fatalf("live: %+v %v", v, err)
	}
	for _, code := range []string{"OLD", "OFF", "NOPE", ""} {
		if _, err := s.UsableVoucher(ctx, code, now); status(err) != http.StatusBadRequest {
			t.Fatalf("%q err = %v, want 400", code, err)
		}
	}
}

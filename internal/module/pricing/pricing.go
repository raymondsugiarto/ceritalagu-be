// Package pricing holds what a full song costs: the normal price (shown struck through), the sale
// price the customer actually pays, the minimum a checkout may charge, and discount vouchers.
// Prices live in the setting table (key song_pricing); vouchers in the voucher table.
package pricing

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/yourorg/music-api/internal/module/audit"
)

// SettingKey is the setting row that holds Prices as JSON.
const SettingKey = "song_pricing"

// DefaultMinPayable is the lowest total a checkout may charge when no setting row exists (DOKU cannot charge Rp0).
const DefaultMinPayable int64 = 10000

// ErrNotFound is returned when no live row matches.
var ErrNotFound = errors.New("pricing: not found")

// codePattern: voucher codes after trimming and uppercasing.
var codePattern = regexp.MustCompile(`^[A-Z0-9_-]{3,30}$`)

// Prices are whole IDR.
type Prices struct {
	Normal     int64 `json:"normal_price"`
	Sale       int64 `json:"sale_price"`
	MinPayable int64 `json:"min_payable"`
}

// Voucher is one discount code (table voucher).
type Voucher struct {
	ID             string
	Code           string
	DiscountAmount int64 // whole IDR off the sale price
	Quota          int
	ExpiresAt      time.Time
	IsActive       bool
	IsPublic       bool  // promoted on the public site (the landing banner)
	Used           int64 // live orders using it (filled by List only)
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Promo is the voucher promoted to the public site, with the discount it really gives: the quote
// clamps a large voucher so the total never drops below the minimum payable.
type Promo struct {
	Code           string
	DiscountAmount int64
	Total          int64
	NormalPrice    int64
	SalePrice      int64
	ExpiresAt      time.Time
}

// Quote applies v (may be nil) to p: the discount actually given and the total to charge. The total
// never drops below p.MinPayable, so a large voucher is cut down rather than making the song free.
func Quote(p Prices, v *Voucher) (discount, total int64) {
	total = p.Sale
	if v != nil {
		total = max(p.Sale-v.DiscountAmount, min(p.MinPayable, p.Sale))
	}
	return p.Sale - total, total
}

// Store is the storage the service needs (GormRepository).
type Store interface {
	GetPrices(ctx context.Context) (Prices, error) // ErrNotFound when never saved
	SavePrices(ctx context.Context, p Prices, at time.Time) error
	ListVouchers(ctx context.Context, now time.Time) ([]Voucher, error)
	// ListPublicVouchers returns the promoted live vouchers with their usage at now.
	ListPublicVouchers(ctx context.Context, now time.Time) ([]Voucher, error)
	GetVoucher(ctx context.Context, id string) (Voucher, error)
	VoucherByCode(ctx context.Context, code string) (Voucher, error)
	CreateVoucher(ctx context.Context, v Voucher) error
	UpdateVoucher(ctx context.Context, v Voucher) error
	ArchiveVoucher(ctx context.Context, id string, at time.Time) error
}

// Auditor records back-office changes (audit.GormRecorder).
type Auditor interface {
	Record(ctx context.Context, e audit.Entry) error
}

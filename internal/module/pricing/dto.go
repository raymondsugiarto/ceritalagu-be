package pricing

import "time"

// UpdatePricesInput is PUT /admin/pricing (whole IDR).
type UpdatePricesInput struct {
	NormalPrice int64 `json:"normal_price" validate:"min=0"`
	SalePrice   int64 `json:"sale_price" validate:"min=0"`
	MinPayable  int64 `json:"min_payable" validate:"min=0"`
}

// PricesOutput is the admin price form.
type PricesOutput struct {
	NormalPrice int64  `json:"normal_price"`
	SalePrice   int64  `json:"sale_price"`
	MinPayable  int64  `json:"min_payable"`
	Currency    string `json:"currency"`
}

// CreateVoucherInput is the admin "add voucher" form. Code is fixed after creation.
type CreateVoucherInput struct {
	Code           string    `json:"code" validate:"required,max=30"`
	DiscountAmount int64     `json:"discount_amount"`
	Quota          int       `json:"quota"`
	ExpiresAt      time.Time `json:"expires_at"`
	IsActive       *bool     `json:"is_active"` // default true
}

// UpdateVoucherInput changes only the fields that are set.
type UpdateVoucherInput struct {
	DiscountAmount *int64     `json:"discount_amount"`
	Quota          *int       `json:"quota"`
	ExpiresAt      *time.Time `json:"expires_at"`
	IsActive       *bool      `json:"is_active"`
}

// VoucherStatus is how the admin list labels a voucher.
type VoucherStatus string

// Voucher statuses (derived, not stored).
const (
	VoucherActive    VoucherStatus = "active"
	VoucherInactive  VoucherStatus = "inactive"
	VoucherExpired   VoucherStatus = "expired"
	VoucherExhausted VoucherStatus = "exhausted"
)

// VoucherOutput is a voucher as the admin sees it.
type VoucherOutput struct {
	ID             string        `json:"id"`
	Code           string        `json:"code"`
	DiscountAmount int64         `json:"discount_amount"`
	Quota          int           `json:"quota"`
	Used           int64         `json:"used"`
	Remaining      int64         `json:"remaining"`
	ExpiresAt      time.Time     `json:"expires_at"`
	IsActive       bool          `json:"is_active"`
	Status         VoucherStatus `json:"status"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

func toPricesOutput(p Prices) PricesOutput {
	return PricesOutput{NormalPrice: p.Normal, SalePrice: p.Sale, MinPayable: p.MinPayable, Currency: "IDR"}
}

func toVoucherOutput(v Voucher, now time.Time) VoucherOutput {
	st := VoucherActive
	switch {
	case !v.IsActive:
		st = VoucherInactive
	case !now.Before(v.ExpiresAt):
		st = VoucherExpired
	case v.Used >= int64(v.Quota):
		st = VoucherExhausted
	}
	return VoucherOutput{ID: v.ID, Code: v.Code, DiscountAmount: v.DiscountAmount, Quota: v.Quota, Used: v.Used,
		Remaining: max(int64(v.Quota)-v.Used, 0), ExpiresAt: v.ExpiresAt, IsActive: v.IsActive, Status: st,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

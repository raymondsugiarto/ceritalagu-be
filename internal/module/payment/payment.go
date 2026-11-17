// Package payment sells the full version of a song: a checkout opens a DOKU payment page, and the
// DOKU notification (or a status check when the customer comes back) marks the order paid and
// unlocks the song.
package payment

import "time"

// Status is the order state (payment_order.status).
type Status string

// Order statuses.
const (
	StatusPending Status = "pending"
	StatusPaid    Status = "paid"
	StatusFailed  Status = "failed"
	StatusExpired Status = "expired"
)

// GatewayDoku is the only gateway for now.
const GatewayDoku = "doku"

// Order is one checkout to unlock a song (table payment_order).
type Order struct {
	ID                  string
	InvoiceNumber       string
	UserID              string
	SongID              string
	GuestSessionID      string
	Gateway             string
	Amount              int64 // whole IDR actually charged: SalePrice - VoucherDiscount
	NormalPrice         int64 // shown struck through at checkout
	SalePrice           int64
	VoucherID           string
	VoucherCode         string
	VoucherDiscount     int64
	Currency            string
	Status              Status
	PaymentURL          string
	PaymentMethod       string // DOKU channel id once paid, e.g. VIRTUAL_ACCOUNT_BCA
	GatewayRequestID    string
	GatewayResponse     map[string]any
	NotificationPayload map[string]any
	ExpiresAt           *time.Time
	PaidAt              *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

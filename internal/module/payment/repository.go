package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when no live order matches.
var ErrNotFound = errors.New("payment order not found")

// Voucher limits hit by CreateWithVoucher.
var (
	ErrVoucherExhausted = errors.New("voucher quota exhausted")
	ErrVoucherUsed      = errors.New("voucher already used by this user")
)

type model struct {
	ID                  string         `gorm:"column:id;primaryKey"`
	InvoiceNumber       string         `gorm:"column:invoice_number"`
	UserID              string         `gorm:"column:user_id"`
	SongID              string         `gorm:"column:song_id"`
	GuestSessionID      string         `gorm:"column:guest_session_id"`
	Gateway             string         `gorm:"column:gateway"`
	Amount              int64          `gorm:"column:amount"`
	NormalPrice         int64          `gorm:"column:normal_price"`
	SalePrice           int64          `gorm:"column:sale_price"`
	VoucherID           *string        `gorm:"column:voucher_id"`
	VoucherCode         string         `gorm:"column:voucher_code"`
	VoucherDiscount     int64          `gorm:"column:voucher_discount"`
	Currency            string         `gorm:"column:currency"`
	Status              string         `gorm:"column:status"`
	PaymentURL          string         `gorm:"column:payment_url"`
	PaymentMethod       string         `gorm:"column:payment_method"`
	GatewayRequestID    string         `gorm:"column:gateway_request_id"`
	GatewayResponse     map[string]any `gorm:"column:gateway_response;serializer:json"`
	NotificationPayload map[string]any `gorm:"column:notification_payload;serializer:json"`
	ExpiresAt           *time.Time     `gorm:"column:expires_at"`
	PaidAt              *time.Time     `gorm:"column:paid_at"`
	CreatedAt           time.Time      `gorm:"column:created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (model) TableName() string { return "payment_order" }

// GormRepository stores orders in PostgreSQL. Soft-deleted rows are skipped by GORM.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// Create inserts o (ID, invoice number and timestamps are set by the service).
func (r *GormRepository) Create(ctx context.Context, o Order) error {
	if err := r.db.WithContext(ctx).Create(fromOrder(o)).Error; err != nil {
		return fmt.Errorf("insert payment_order: %w", err)
	}
	return nil
}

// Update saves every field of o.
func (r *GormRepository) Update(ctx context.Context, o Order) error {
	if err := r.db.WithContext(ctx).Save(fromOrder(o)).Error; err != nil {
		return fmt.Errorf("update payment_order %s: %w", o.ID, err)
	}
	return nil
}

// Get returns ErrNotFound if the order does not exist.
func (r *GormRepository) Get(ctx context.Context, id string) (Order, error) {
	return r.first(ctx, "id = ?", id)
}

// GetByInvoice returns ErrNotFound if no order has that invoice number.
func (r *GormRepository) GetByInvoice(ctx context.Context, invoice string) (Order, error) {
	return r.first(ctx, "invoice_number = ?", invoice)
}

// InvoiceTaken reports whether an order already uses invoice.
func (r *GormRepository) InvoiceTaken(ctx context.Context, invoice string) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model{}).Where("invoice_number = ?", invoice).Count(&n).Error; err != nil {
		return false, fmt.Errorf("check invoice number: %w", err)
	}
	return n > 0, nil
}

// OpenForSong returns the newest pending order of songID that has not expired at now, or ErrNotFound.
func (r *GormRepository) OpenForSong(ctx context.Context, songID string, now time.Time) (Order, error) {
	var m model
	err := r.db.WithContext(ctx).
		Where("song_id = ? AND status = ? AND payment_url <> '' AND expires_at > ?", songID, string(StatusPending), now).
		Order("created_at DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("select open order of song %s: %w", songID, err)
	}
	return m.toOrder(), nil
}

// MarkPaid sets the order paid unless it already is; it reports whether this call changed it.
// The conditional update makes a notification and a status check racing each other safe.
func (r *GormRepository) MarkPaid(ctx context.Context, id, method string, payload map[string]any, at time.Time) (bool, error) {
	updates := map[string]any{"status": string(StatusPaid), "paid_at": at, "updated_at": at, "payment_method": method}
	if payload != nil {
		updates["notification_payload"] = gorm.Expr("?::jsonb", mustJSON(payload))
	}
	res := r.db.WithContext(ctx).Model(&model{}).Where("id = ? AND status <> ?", id, string(StatusPaid)).Updates(updates)
	if res.Error != nil {
		return false, fmt.Errorf("mark payment_order %s paid: %w", id, res.Error)
	}
	return res.RowsAffected > 0, nil
}

// CountPaid counts the user's paid orders.
func (r *GormRepository) CountPaid(ctx context.Context, userID string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model{}).Where("user_id = ? AND status = ?", userID, string(StatusPaid)).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count paid orders: %w", err)
	}
	return n, nil
}

// LatestPaidForSong returns the song's most recent paid order, or ErrNotFound.
func (r *GormRepository) LatestPaidForSong(ctx context.Context, songID string) (Order, error) {
	var m model
	err := r.db.WithContext(ctx).Where("song_id = ? AND status = ?", songID, string(StatusPaid)).
		Order("paid_at DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("select paid order of song %s: %w", songID, err)
	}
	return m.toOrder(), nil
}

func (r *GormRepository) first(ctx context.Context, where string, arg any) (Order, error) {
	var m model
	err := r.db.WithContext(ctx).Where(where, arg).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("select payment_order: %w", err)
	}
	return m.toOrder(), nil
}

func fromOrder(o Order) *model {
	return &model{
		ID: o.ID, InvoiceNumber: o.InvoiceNumber, UserID: o.UserID, SongID: o.SongID, GuestSessionID: o.GuestSessionID,
		Gateway: o.Gateway, Amount: o.Amount, NormalPrice: o.NormalPrice, SalePrice: o.SalePrice,
		VoucherID: nullable(o.VoucherID), VoucherCode: o.VoucherCode, VoucherDiscount: o.VoucherDiscount,
		Currency: o.Currency, Status: string(o.Status), PaymentURL: o.PaymentURL,
		PaymentMethod: o.PaymentMethod, GatewayRequestID: o.GatewayRequestID, GatewayResponse: o.GatewayResponse,
		NotificationPayload: o.NotificationPayload, ExpiresAt: o.ExpiresAt, PaidAt: o.PaidAt,
		CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

func (m model) toOrder() Order {
	return Order{
		ID: m.ID, InvoiceNumber: m.InvoiceNumber, UserID: m.UserID, SongID: m.SongID, GuestSessionID: m.GuestSessionID,
		Gateway: m.Gateway, Amount: m.Amount, NormalPrice: m.NormalPrice, SalePrice: m.SalePrice,
		VoucherID: deref(m.VoucherID), VoucherCode: m.VoucherCode, VoucherDiscount: m.VoucherDiscount,
		Currency: m.Currency, Status: Status(m.Status), PaymentURL: m.PaymentURL,
		PaymentMethod: m.PaymentMethod, GatewayRequestID: m.GatewayRequestID, GatewayResponse: m.GatewayResponse,
		NotificationPayload: m.NotificationPayload, ExpiresAt: m.ExpiresAt, PaidAt: m.PaidAt,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// CreateWithVoucher inserts o, which uses voucher o.VoucherID, inside one transaction that locks the
// voucher row, so concurrent checkouts cannot take more than quota. Live uses are paid orders and
// pending orders still inside their payment window. A user may hold one live use of each voucher.
func (r *GormRepository) CreateWithVoucher(ctx context.Context, o Order, quota int, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked string
		if err := tx.Raw("SELECT id FROM voucher WHERE id = ? AND deleted_at IS NULL FOR UPDATE", o.VoucherID).
			Scan(&locked).Error; err != nil {
			return fmt.Errorf("lock voucher %s: %w", o.VoucherID, err)
		}
		if locked == "" {
			return ErrVoucherExhausted
		}
		total, byUser, err := voucherUsage(tx, o.VoucherID, o.UserID, now)
		if err != nil {
			return err
		}
		if byUser > 0 {
			return ErrVoucherUsed
		}
		if total >= int64(quota) {
			return ErrVoucherExhausted
		}
		if err := tx.Create(fromOrder(o)).Error; err != nil {
			return fmt.Errorf("insert payment_order: %w", err)
		}
		return nil
	})
}

// VoucherUsage counts the live uses of a voucher: in total and by userID.
func (r *GormRepository) VoucherUsage(ctx context.Context, voucherID, userID string, now time.Time) (int64, int64, error) {
	return voucherUsage(r.db.WithContext(ctx), voucherID, userID, now)
}

func voucherUsage(db *gorm.DB, voucherID, userID string, now time.Time) (total, byUser int64, err error) {
	row := struct {
		Total  int64
		ByUser int64
	}{}
	err = db.Raw(`SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE user_id = ?) AS by_user FROM payment_order
		WHERE voucher_id = ? AND deleted_at IS NULL AND (status = ? OR (status = ? AND expires_at > ?))`,
		userID, voucherID, string(StatusPaid), string(StatusPending), now).Scan(&row).Error
	if err != nil {
		return 0, 0, fmt.Errorf("count voucher %s usage: %w", voucherID, err)
	}
	return row.Total, row.ByUser, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var _ Repository = (*GormRepository)(nil)

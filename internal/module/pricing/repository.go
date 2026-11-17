package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type settingRow struct {
	ID          string          `gorm:"column:id;primaryKey"`
	SettingKey  string          `gorm:"column:setting_key"`
	Value       json.RawMessage `gorm:"column:value;type:jsonb"`
	Description string          `gorm:"column:description"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
	UpdatedAt   time.Time       `gorm:"column:updated_at"`
	DeletedAt   gorm.DeletedAt  `gorm:"column:deleted_at"`
}

func (settingRow) TableName() string { return "setting" }

type voucherRow struct {
	ID             string         `gorm:"column:id;primaryKey"`
	Code           string         `gorm:"column:code"`
	DiscountAmount int64          `gorm:"column:discount_amount"`
	Quota          int            `gorm:"column:quota"`
	ExpiresAt      time.Time      `gorm:"column:expires_at"`
	IsActive       bool           `gorm:"column:is_active"`
	Used           int64          `gorm:"column:used;->"` // read-only: computed in ListVouchers
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (voucherRow) TableName() string { return "voucher" }

// GormRepository stores prices and vouchers in PostgreSQL.
type GormRepository struct {
	db *gorm.DB
}

// NewGormRepository wraps db.
func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

// GetPrices reads the song_pricing setting.
func (r *GormRepository) GetPrices(ctx context.Context) (Prices, error) {
	var row settingRow
	err := r.db.WithContext(ctx).Where("setting_key = ?", SettingKey).Order("updated_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Prices{}, ErrNotFound
	}
	if err != nil {
		return Prices{}, fmt.Errorf("select setting %s: %w", SettingKey, err)
	}
	var p Prices
	if err := json.Unmarshal(row.Value, &p); err != nil {
		return Prices{}, fmt.Errorf("decode setting %s: %w", SettingKey, err)
	}
	return p, nil
}

// SavePrices updates the song_pricing setting, creating it the first time.
func (r *GormRepository) SavePrices(ctx context.Context, p Prices, at time.Time) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&settingRow{}).Where("setting_key = ?", SettingKey).
		Updates(map[string]any{"value": gorm.Expr("?::jsonb", string(raw)), "updated_at": at})
	if res.Error != nil {
		return fmt.Errorf("update setting %s: %w", SettingKey, res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	row := settingRow{ID: uuid.NewString(), SettingKey: SettingKey, Value: raw,
		Description: "Harga full lagu: harga normal (dicoret), harga diskon (dibayar), minimum bayar (IDR)",
		CreatedAt:   at, UpdatedAt: at}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert setting %s: %w", SettingKey, err)
	}
	return nil
}

// usedExpr counts the live orders of a voucher: paid, or pending inside the payment window.
const usedExpr = `(SELECT COUNT(*) FROM payment_order o WHERE o.voucher_id = voucher.id AND o.deleted_at IS NULL
  AND (o.status = 'paid' OR (o.status = 'pending' AND o.expires_at > ?))) AS used`

// ListVouchers returns every live voucher, newest first, with its usage at now.
func (r *GormRepository) ListVouchers(ctx context.Context, now time.Time) ([]Voucher, error) {
	var rows []voucherRow
	if err := r.db.WithContext(ctx).Model(&voucherRow{}).Select("voucher.*, "+usedExpr, now).
		Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("select vouchers: %w", err)
	}
	out := make([]Voucher, len(rows))
	for i, m := range rows {
		out[i] = m.toVoucher()
	}
	return out, nil
}

// GetVoucher returns ErrNotFound when id is unknown or archived.
func (r *GormRepository) GetVoucher(ctx context.Context, id string) (Voucher, error) {
	return r.first(ctx, "id = ?", id)
}

// VoucherByCode looks up an uppercase code.
func (r *GormRepository) VoucherByCode(ctx context.Context, code string) (Voucher, error) {
	return r.first(ctx, "code = ?", code)
}

// CreateVoucher inserts v.
func (r *GormRepository) CreateVoucher(ctx context.Context, v Voucher) error {
	if err := r.db.WithContext(ctx).Create(fromVoucher(v)).Error; err != nil {
		return fmt.Errorf("insert voucher: %w", err)
	}
	return nil
}

// UpdateVoucher saves the editable fields of v.
func (r *GormRepository) UpdateVoucher(ctx context.Context, v Voucher) error {
	res := r.db.WithContext(ctx).Model(&voucherRow{}).Where("id = ?", v.ID).Updates(map[string]any{
		"discount_amount": v.DiscountAmount, "quota": v.Quota, "expires_at": v.ExpiresAt,
		"is_active": v.IsActive, "updated_at": v.UpdatedAt,
	})
	if res.Error != nil {
		return fmt.Errorf("update voucher %s: %w", v.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ArchiveVoucher soft-deletes the voucher; orders keep their voucher_code.
func (r *GormRepository) ArchiveVoucher(ctx context.Context, id string, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&voucherRow{}).Where("id = ?", id).
		Updates(map[string]any{"deleted_at": at, "updated_at": at})
	if res.Error != nil {
		return fmt.Errorf("archive voucher %s: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) first(ctx context.Context, where string, arg any) (Voucher, error) {
	var m voucherRow
	err := r.db.WithContext(ctx).Where(where, arg).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Voucher{}, ErrNotFound
	}
	if err != nil {
		return Voucher{}, fmt.Errorf("select voucher: %w", err)
	}
	return m.toVoucher(), nil
}

func fromVoucher(v Voucher) *voucherRow {
	return &voucherRow{ID: v.ID, Code: v.Code, DiscountAmount: v.DiscountAmount, Quota: v.Quota,
		ExpiresAt: v.ExpiresAt, IsActive: v.IsActive, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

func (m voucherRow) toVoucher() Voucher {
	return Voucher{ID: m.ID, Code: m.Code, DiscountAmount: m.DiscountAmount, Quota: m.Quota,
		ExpiresAt: m.ExpiresAt, IsActive: m.IsActive, Used: m.Used, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

var _ Store = (*GormRepository)(nil)

package pricing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/audit"
)

// Service manages prices and vouchers.
type Service struct {
	store    Store
	audit    Auditor
	fallback Prices
	now      func() time.Time
}

// NewService builds the service. fallback is used until an admin saves prices (payment.unlockPrice).
func NewService(store Store, auditor Auditor, fallback Prices) *Service {
	if fallback.MinPayable <= 0 {
		fallback.MinPayable = DefaultMinPayable
	}
	return &Service{store: store, audit: auditor, fallback: fallback, now: func() time.Time { return time.Now().UTC() }}
}

// Prices returns the saved prices, or the fallback when none were saved.
func (s *Service) Prices(ctx context.Context) (Prices, error) {
	p, err := s.store.GetPrices(ctx)
	if errors.Is(err, ErrNotFound) {
		return s.fallback, nil
	}
	if err != nil {
		return Prices{}, apperror.Internal(err)
	}
	return p, nil
}

// AdminPrices is GET /admin/pricing.
func (s *Service) AdminPrices(ctx context.Context) (PricesOutput, error) {
	p, err := s.Prices(ctx)
	return toPricesOutput(p), err
}

// UpdatePrices saves the three prices. sale <= normal and min_payable <= sale.
func (s *Service) UpdatePrices(ctx context.Context, actorID string, in UpdatePricesInput) (PricesOutput, error) {
	var details []apperror.ErrorDetail
	if in.NormalPrice <= 0 {
		details = append(details, apperror.ErrorDetail{Field: "normal_price", Message: "harus lebih dari 0"})
	}
	if in.MinPayable <= 0 {
		details = append(details, apperror.ErrorDetail{Field: "min_payable", Message: "harus lebih dari 0"})
	}
	if in.SalePrice > in.NormalPrice {
		details = append(details, apperror.ErrorDetail{Field: "sale_price", Message: "tidak boleh lebih besar dari harga normal"})
	}
	if in.SalePrice < in.MinPayable {
		details = append(details, apperror.ErrorDetail{Field: "sale_price", Message: "tidak boleh lebih kecil dari minimum bayar"})
	}
	if len(details) > 0 {
		return PricesOutput{}, apperror.Validation(details)
	}
	old, err := s.Prices(ctx)
	if err != nil {
		return PricesOutput{}, err
	}
	p := Prices{Normal: in.NormalPrice, Sale: in.SalePrice, MinPayable: in.MinPayable}
	if err := s.store.SavePrices(ctx, p, s.now()); err != nil {
		return PricesOutput{}, apperror.Internal(err)
	}
	s.record(ctx, actorID, "pricing.update", "setting", SettingKey, map[string]any{"from": old, "to": p})
	return toPricesOutput(p), nil
}

// ListVouchers returns every live voucher with its usage.
func (s *Service) ListVouchers(ctx context.Context) ([]VoucherOutput, error) {
	now := s.now()
	vs, err := s.store.ListVouchers(ctx, now)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]VoucherOutput, len(vs))
	for i, v := range vs {
		out[i] = toVoucherOutput(v, now)
	}
	return out, nil
}

// CreateVoucher adds a voucher. The code is trimmed and uppercased and must be unused.
func (s *Service) CreateVoucher(ctx context.Context, actorID string, in CreateVoucherInput) (VoucherOutput, error) {
	now := s.now()
	code := NormalizeCode(in.Code)
	if !codePattern.MatchString(code) {
		return VoucherOutput{}, fieldErr("code", "3-30 karakter: huruf, angka, - atau _ (mis. HEMAT10)")
	}
	v := Voucher{ID: uuid.NewString(), Code: code, DiscountAmount: in.DiscountAmount, Quota: in.Quota,
		ExpiresAt: in.ExpiresAt.UTC(), IsActive: in.IsActive == nil || *in.IsActive, CreatedAt: now, UpdatedAt: now}
	if err := validateVoucher(v, now); err != nil {
		return VoucherOutput{}, err
	}
	if _, err := s.store.VoucherByCode(ctx, code); err == nil {
		return VoucherOutput{}, apperror.Conflict(fmt.Sprintf("kode voucher %q sudah dipakai", code))
	} else if !errors.Is(err, ErrNotFound) {
		return VoucherOutput{}, apperror.Internal(err)
	}
	if err := s.store.CreateVoucher(ctx, v); err != nil {
		return VoucherOutput{}, apperror.Internal(err)
	}
	s.record(ctx, actorID, "voucher.create", "voucher", v.ID, map[string]any{
		"code": v.Code, "discount_amount": v.DiscountAmount, "quota": v.Quota, "expires_at": v.ExpiresAt, "is_active": v.IsActive})
	return toVoucherOutput(v, now), nil
}

// UpdateVoucher changes the fields that are set (never the code).
func (s *Service) UpdateVoucher(ctx context.Context, actorID, id string, in UpdateVoucherInput) (VoucherOutput, error) {
	v, err := s.getVoucher(ctx, id)
	if err != nil {
		return VoucherOutput{}, err
	}
	now := s.now()
	changed := map[string]any{}
	if in.DiscountAmount != nil && *in.DiscountAmount != v.DiscountAmount {
		v.DiscountAmount, changed["discount_amount"] = *in.DiscountAmount, *in.DiscountAmount
	}
	if in.Quota != nil && *in.Quota != v.Quota {
		v.Quota, changed["quota"] = *in.Quota, *in.Quota
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.Equal(v.ExpiresAt) {
		v.ExpiresAt, changed["expires_at"] = in.ExpiresAt.UTC(), in.ExpiresAt.UTC()
	}
	if in.IsActive != nil && *in.IsActive != v.IsActive {
		v.IsActive, changed["is_active"] = *in.IsActive, *in.IsActive
	}
	if len(changed) == 0 {
		return toVoucherOutput(v, now), nil
	}
	if err := validateVoucher(v, time.Time{}); err != nil { // an edit may keep a past expiry (to end it early)
		return VoucherOutput{}, err
	}
	v.UpdatedAt = now
	if err := s.store.UpdateVoucher(ctx, v); err != nil {
		return VoucherOutput{}, mapErr(err)
	}
	s.record(ctx, actorID, "voucher.update", "voucher", v.ID, changed)
	return toVoucherOutput(v, now), nil
}

// ArchiveVoucher soft-deletes a voucher. Orders that used it keep the code.
func (s *Service) ArchiveVoucher(ctx context.Context, actorID, id string) error {
	v, err := s.getVoucher(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.ArchiveVoucher(ctx, id, s.now()); err != nil {
		return mapErr(err)
	}
	s.record(ctx, actorID, "voucher.archive", "voucher", id, map[string]any{"code": v.Code})
	return nil
}

// UsableVoucher returns the voucher behind code if a customer may use it at now (quota is checked by
// the payment module, which owns the orders).
func (s *Service) UsableVoucher(ctx context.Context, code string, now time.Time) (Voucher, error) {
	code = NormalizeCode(code)
	if code == "" {
		return Voucher{}, apperror.BadRequest("Masukkan kode voucher.")
	}
	v, err := s.store.VoucherByCode(ctx, code)
	if errors.Is(err, ErrNotFound) {
		return Voucher{}, apperror.BadRequest("Kode voucher tidak ditemukan.")
	}
	if err != nil {
		return Voucher{}, apperror.Internal(err)
	}
	if !v.IsActive {
		return Voucher{}, apperror.BadRequest("Voucher ini sudah tidak aktif.")
	}
	if !now.Before(v.ExpiresAt) {
		return Voucher{}, apperror.BadRequest("Voucher ini sudah kedaluwarsa.")
	}
	return v, nil
}

// NormalizeCode trims and uppercases a voucher code.
func NormalizeCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

func validateVoucher(v Voucher, now time.Time) error {
	var details []apperror.ErrorDetail
	if v.DiscountAmount <= 0 {
		details = append(details, apperror.ErrorDetail{Field: "discount_amount", Message: "harus lebih dari 0"})
	}
	if v.Quota <= 0 {
		details = append(details, apperror.ErrorDetail{Field: "quota", Message: "minimal 1"})
	}
	if v.ExpiresAt.IsZero() || (!now.IsZero() && !v.ExpiresAt.After(now)) {
		details = append(details, apperror.ErrorDetail{Field: "expires_at", Message: "harus di masa depan"})
	}
	if len(details) > 0 {
		return apperror.Validation(details)
	}
	return nil
}

func (s *Service) getVoucher(ctx context.Context, id string) (Voucher, error) {
	v, err := s.store.GetVoucher(ctx, id)
	if err != nil {
		return Voucher{}, mapErr(err)
	}
	return v, nil
}

func (s *Service) record(ctx context.Context, actorID, action, entity, id string, diff map[string]any) {
	err := s.audit.Record(ctx, audit.Entry{ActorType: "admin", ActorID: actorID, Action: action,
		EntityType: entity, EntityID: id, Diff: diff})
	if err != nil {
		slog.ErrorContext(ctx, "audit log write failed", "action", action, "entity_id", id, "error", err)
	}
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("voucher tidak ditemukan")
	}
	return apperror.Internal(err)
}

func fieldErr(field, msg string) error {
	return apperror.Validation([]apperror.ErrorDetail{{Field: field, Message: msg}})
}

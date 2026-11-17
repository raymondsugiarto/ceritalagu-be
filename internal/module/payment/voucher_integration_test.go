//go:build integration

// Runs the voucher SQL against the local PostgreSQL from config/resources inside a transaction that is
// always rolled back, so nothing is left behind: go test -tags integration ./internal/module/payment/
package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/database"
	"github.com/yourorg/music-api/internal/module/pricing"
)

var errRollback = errors.New("rollback")

func TestVoucherSQLIntegration(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("no config:", err)
	}
	ctx := context.Background()
	gdb, err := database.Open(ctx, cfg.Database.Main, "")
	if err != nil {
		t.Skip("no database:", err)
	}
	defer database.Close(gdb)

	err = gdb.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		prices := pricing.NewGormRepository(tx)
		orders := NewGormRepository(tx)

		// Prices round-trip through the setting table.
		if err := prices.SavePrices(ctx, pricing.Prices{Normal: 99000, Sale: 49000, MinPayable: 10000}, now); err != nil {
			t.Fatal(err)
		}
		if p, err := prices.GetPrices(ctx); err != nil || p.Sale != 49000 || p.Normal != 99000 {
			t.Fatalf("prices = %+v, %v", p, err)
		}
		if err := prices.SavePrices(ctx, pricing.Prices{Normal: 89000, Sale: 39000, MinPayable: 10000}, now); err != nil {
			t.Fatal(err)
		}
		if p, _ := prices.GetPrices(ctx); p.Sale != 39000 {
			t.Fatalf("update not applied: %+v", p)
		}

		v := pricing.Voucher{ID: uuid.NewString(), Code: "ITEST" + uuid.NewString()[:6], DiscountAmount: 10000, Quota: 2,
			ExpiresAt: now.Add(time.Hour), IsActive: true, CreatedAt: now, UpdatedAt: now}
		if err := prices.CreateVoucher(ctx, v); err != nil {
			t.Fatal(err)
		}
		if got, err := prices.VoucherByCode(ctx, v.Code); err != nil || got.ID != v.ID {
			t.Fatalf("by code = %+v, %v", got, err)
		}

		newOrder := func(userID string) Order {
			exp := now.Add(time.Hour)
			return Order{ID: uuid.NewString(), InvoiceNumber: "IT-" + uuid.NewString()[:8], UserID: userID,
				SongID: uuid.NewString(), Gateway: GatewayDoku, Amount: 29000, NormalPrice: 89000, SalePrice: 39000,
				VoucherID: v.ID, VoucherCode: v.Code, VoucherDiscount: 10000, Currency: "IDR", Status: StatusPending,
				ExpiresAt: &exp, CreatedAt: now, UpdatedAt: now}
		}
		u1, u2, u3 := uuid.NewString(), uuid.NewString(), uuid.NewString()
		if err := orders.CreateWithVoucher(ctx, newOrder(u1), v.Quota, now); err != nil {
			t.Fatal(err)
		}
		if err := orders.CreateWithVoucher(ctx, newOrder(u1), v.Quota, now); !errors.Is(err, ErrVoucherUsed) {
			t.Fatalf("same user again: %v, want ErrVoucherUsed", err)
		}
		second := newOrder(u2)
		if err := orders.CreateWithVoucher(ctx, second, v.Quota, now); err != nil {
			t.Fatal(err)
		}
		if err := orders.CreateWithVoucher(ctx, newOrder(u3), v.Quota, now); !errors.Is(err, ErrVoucherExhausted) {
			t.Fatalf("over quota: %v, want ErrVoucherExhausted", err)
		}
		total, byUser, err := orders.VoucherUsage(ctx, v.ID, u1, now)
		if err != nil || total != 2 || byUser != 1 {
			t.Fatalf("usage = %d/%d, %v", total, byUser, err)
		}

		// Stored order keeps the voucher fields.
		got, err := orders.Get(ctx, second.ID)
		if err != nil || got.VoucherID != v.ID || got.VoucherDiscount != 10000 || got.SalePrice != 39000 {
			t.Fatalf("order = %+v, %v", got, err)
		}

		// An expired order frees its use, and the admin list counts it.
		second.Status = StatusExpired
		if err := orders.Update(ctx, second); err != nil {
			t.Fatal(err)
		}
		list, err := prices.ListVouchers(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		var used int64 = -1
		for _, x := range list {
			if x.ID == v.ID {
				used = x.Used
			}
		}
		if used != 1 {
			t.Fatalf("list used = %d, want 1", used)
		}
		if err := orders.CreateWithVoucher(ctx, newOrder(u3), v.Quota, now); err != nil {
			t.Fatalf("freed slot not reusable: %v", err)
		}

		v.Quota, v.UpdatedAt = 5, now
		if err := prices.UpdateVoucher(ctx, v); err != nil {
			t.Fatal(err)
		}
		if err := prices.ArchiveVoucher(ctx, v.ID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := prices.GetVoucher(ctx, v.ID); !errors.Is(err, pricing.ErrNotFound) {
			t.Fatalf("archived voucher still found: %v", err)
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatal(err)
	}
}

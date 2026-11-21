package notification_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/music-api/internal/module/notification"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
)

type fakeSongs struct{ songs map[string]song.Song }

func (f fakeSongs) GetSong(_ context.Context, id string) (song.Song, error) {
	s, ok := f.songs[id]
	if !ok {
		return song.Song{}, errors.New("song not found")
	}
	return s, nil
}

func newReceipt(opts notification.ReceiptSetting) (*notification.ReceiptService, *fakeSender) {
	sender := &fakeSender{}
	users := &fakeUsers{users: map[string]user.User{"u-1": {ID: "u-1", Name: "Nadia", Email: "nadia@example.com"}}}
	songs := fakeSongs{songs: map[string]song.Song{"s-1": {ID: "s-1", Title: "Untuk Nadia", FullAudioKey: "full.mp3"}}}
	audio := &fakeAudio{files: map[string][]byte{"full.mp3": []byte("FULL")}}
	return notification.NewReceiptService(sender, users, songs, audio, opts, slog.New(slog.NewTextHandler(io.Discard, nil))), sender
}

func paidOrder() notification.PaidOrder {
	return notification.PaidOrder{
		OrderID: "o-1", InvoiceNumber: "LU-261120-ABCD2345", UserID: "u-1", SongID: "s-1",
		Amount: 39000, NormalPrice: 99000, SalePrice: 49000, VoucherCode: "HEMAT10", VoucherDiscount: 10000,
		PaymentMethod: "VIRTUAL_ACCOUNT_BCA", PaidAt: time.Date(2026, 11, 20, 3, 5, 0, 0, time.UTC),
	}
}

func TestReceiptSendsBrevoTemplateParams(t *testing.T) {
	t.Parallel()
	svc, sender := newReceipt(notification.ReceiptSetting{Enabled: true, TemplateID: 12, SiteURL: "https://ceritalagu.com/"})
	if err := svc.PaymentSucceeded(context.Background(), paidOrder()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d emails", len(sender.sent))
	}
	m := sender.sent[0]
	if m.To != "nadia@example.com" || m.TemplateID != 12 {
		t.Fatalf("to=%q template=%d", m.To, m.TemplateID)
	}
	want := map[string]string{
		"GREETING_NAME":  "Nadia",
		"SONG_TITLE":     "Untuk Nadia",
		"SONG_URL":       "https://ceritalagu.com/songs/s-1",
		"INVOICE":        "LU-261120-ABCD2345",
		"AMOUNT":         "Rp39.000",
		"NORMAL_PRICE":   "Rp99.000",
		"DISCOUNT":       "-Rp10.000",
		"VOUCHER_CODE":   "HEMAT10",
		"PAYMENT_METHOD": "Virtual Account BCA",
		"PAID_AT":        "20/11/2026 10:05 WIB",
	}
	for k, v := range want {
		if m.Params[k] != v {
			t.Errorf("%s = %v, want %q", k, m.Params[k], v)
		}
	}
	// The fallback body is filled too, for the log driver.
	if !strings.Contains(m.Text, "Rp39.000") || m.Subject == "" {
		t.Errorf("fallback body missing: subject=%q text=%q", m.Subject, m.Text)
	}
	if len(m.Files) != 0 {
		t.Error("full song attached although attachFullAudio is off")
	}
}

func TestReceiptAttachesFullSongWhenAllowed(t *testing.T) {
	t.Parallel()
	svc, sender := newReceipt(notification.ReceiptSetting{Enabled: true, AttachFullAudio: true, MaxAttachmentBytes: 1024})
	if err := svc.PaymentSucceeded(context.Background(), paidOrder()); err != nil {
		t.Fatal(err)
	}
	f := sender.sent[0].Files
	if len(f) != 1 || f[0].Name != "untuk-nadia.mp3" || string(f[0].Content) != "FULL" {
		t.Fatalf("files = %+v", f)
	}
}

func TestReceiptDisabledSendsNothing(t *testing.T) {
	t.Parallel()
	svc, sender := newReceipt(notification.ReceiptSetting{})
	if err := svc.PaymentSucceeded(context.Background(), paidOrder()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatal("disabled receipt sent an email")
	}
}

func TestReceiptWithoutBuyerIsAnError(t *testing.T) {
	t.Parallel()
	svc, _ := newReceipt(notification.ReceiptSetting{Enabled: true})
	o := paidOrder()
	o.UserID = ""
	if err := svc.PaymentSucceeded(context.Background(), o); !errors.Is(err, notification.ErrNoRecipient) {
		t.Fatalf("err = %v", err)
	}
}

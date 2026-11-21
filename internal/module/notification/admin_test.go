package notification_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/audit"
	"github.com/yourorg/music-api/internal/module/notification"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/module/user"
)

type fakeOrders struct {
	o   notification.PaidOrder
	err error
}

func (f fakeOrders) LatestPaid(context.Context, string) (notification.PaidOrder, error) { return f.o, f.err }

type fakeAudit struct{ entries []audit.Entry }

func (f *fakeAudit) Record(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

func newAdmin(orders fakeOrders, ownerID string) (*notification.AdminService, *fakeSender, *fakeAudit) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sender := &fakeSender{}
	users := &fakeUsers{users: map[string]user.User{"u-1": {ID: "u-1", Name: "Nadia", Email: "nadia@example.com"}}}
	songs := fakeSongs{songs: map[string]song.Song{"s-1": {ID: "s-1", Title: "Untuk Nadia", OwnerUserID: ownerID}}}
	audio := &fakeAudio{files: map[string][]byte{}}
	// Both automatic emails are off: the admin button must still send.
	ready := notification.NewService(sender, users, audio, notification.Setting{}, log)
	receipts := notification.NewReceiptService(sender, users, songs, audio, notification.ReceiptSetting{TemplateID: 5}, log)
	au := &fakeAudit{}
	return notification.NewAdminService(songs, ready, receipts, orders, au, log), sender, au
}

func TestAdminResendsSongReadyToCustomer(t *testing.T) {
	t.Parallel()
	svc, sender, au := newAdmin(fakeOrders{}, "u-1")
	out, err := svc.SendEmail(context.Background(), "admin-1", "s-1", notification.SendEmailInput{Kind: notification.KindSongReady})
	if err != nil {
		t.Fatal(err)
	}
	if out.To != "nadia@example.com" || out.Test || len(sender.sent) != 1 {
		t.Fatalf("out = %+v, sent %d", out, len(sender.sent))
	}
	if len(au.entries) != 1 || au.entries[0].ActorID != "admin-1" || au.entries[0].Action != "song.email_send" {
		t.Fatalf("audit = %+v", au.entries)
	}
}

func TestAdminSendsTestReceiptToOverride(t *testing.T) {
	t.Parallel()
	svc, sender, _ := newAdmin(fakeOrders{o: paidOrder()}, "u-1")
	out, err := svc.SendEmail(context.Background(), "admin-1", "s-1",
		notification.SendEmailInput{Kind: notification.KindPaymentReceipt, To: "qa@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if out.To != "qa@example.com" || !out.Test || out.TemplateID != 5 {
		t.Fatalf("out = %+v", out)
	}
	if m := sender.sent[0]; m.To != "qa@example.com" || m.Params["GREETING_NAME"] != "Nadia" {
		t.Fatalf("sent = %+v", m)
	}
}

func TestAdminReceiptWithoutPaidOrderIsConflict(t *testing.T) {
	t.Parallel()
	svc, sender, _ := newAdmin(fakeOrders{err: notification.ErrNoPaidOrder}, "u-1")
	_, err := svc.SendEmail(context.Background(), "admin-1", "s-1", notification.SendEmailInput{Kind: notification.KindPaymentReceipt})
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Status != 409 || len(sender.sent) != 0 {
		t.Fatalf("err = %v", err)
	}
}

func TestAdminGuestSongNeedsTestAddress(t *testing.T) {
	t.Parallel()
	svc, _, _ := newAdmin(fakeOrders{}, "")
	_, err := svc.SendEmail(context.Background(), "admin-1", "s-1", notification.SendEmailInput{Kind: notification.KindSongReady})
	var ae *apperror.Error
	if !errors.As(err, &ae) || ae.Status != 409 {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.SendEmail(context.Background(), "admin-1", "s-1",
		notification.SendEmailInput{Kind: notification.KindSongReady, To: "qa@example.com"}); err != nil {
		t.Fatal(err)
	}
}

package notification

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/audit"
	"github.com/yourorg/music-api/internal/module/song"
	"github.com/yourorg/music-api/internal/response"
)

// Admin "send email" button on the song library: resend a customer email, or send it to a test inbox.

// EmailKind is which email the admin sends.
type EmailKind string

// Email kinds.
const (
	KindSongReady      EmailKind = "song_ready"
	KindPaymentReceipt EmailKind = "payment_receipt"
)

// ErrNoPaidOrder is returned when a song has no paid order to write a receipt for.
var ErrNoPaidOrder = errors.New("notification: song has no paid order")

// PaidOrders finds the song's paid order (payment.PaidOrders).
type PaidOrders interface {
	LatestPaid(ctx context.Context, songID string) (PaidOrder, error)
}

// Auditor writes the admin audit log (audit.GormRecorder).
type Auditor interface {
	Record(ctx context.Context, e audit.Entry) error
}

// SendEmailInput is the body of POST /admin/songs/:id/email.
type SendEmailInput struct {
	Kind EmailKind `json:"kind" validate:"required,oneof=song_ready payment_receipt"`
	// To sends to this address instead of the customer (testing). Empty = the customer.
	To string `json:"to" validate:"omitempty,email,max=254"`
}

// SendEmailOutput says where the email went.
type SendEmailOutput struct {
	Kind EmailKind `json:"kind"`
	Sent
	Test bool `json:"test"` // sent to an override address, not the customer
}

// AdminService sends customer emails on an admin's request.
type AdminService struct {
	songs    Songs
	ready    *Service
	receipts *ReceiptService
	orders   PaidOrders
	audit    Auditor
	log      *slog.Logger
}

// NewAdminService builds the service.
func NewAdminService(songs Songs, ready *Service, receipts *ReceiptService, orders PaidOrders, auditor Auditor, log *slog.Logger) *AdminService {
	return &AdminService{songs: songs, ready: ready, receipts: receipts, orders: orders, audit: auditor, log: log}
}

// SendEmail sends kind for the song, to in.To or to the customer.
func (s *AdminService) SendEmail(ctx context.Context, actorID, songID string, in SendEmailInput) (SendEmailOutput, error) {
	sg, err := s.songs.GetSong(ctx, songID)
	if errors.Is(err, song.ErrSongNotFound) {
		return SendEmailOutput{}, apperror.NotFound("lagu tidak ditemukan")
	}
	if err != nil {
		return SendEmailOutput{}, apperror.Internal(err)
	}

	var sent Sent
	switch in.Kind {
	case KindSongReady:
		sent, err = s.ready.ResendSongReady(ctx, sg, in.To)
	case KindPaymentReceipt:
		var o PaidOrder
		o, err = s.orders.LatestPaid(ctx, sg.ID)
		if errors.Is(err, ErrNoPaidOrder) {
			return SendEmailOutput{}, apperror.Conflict("Lagu ini belum dibayar, jadi belum ada bukti pembayaran untuk dikirim.")
		}
		if err == nil {
			sent, err = s.receipts.ResendPaymentReceipt(ctx, o, in.To)
		}
	default:
		return SendEmailOutput{}, apperror.BadRequest("kind harus song_ready atau payment_receipt")
	}
	if errors.Is(err, ErrNoRecipient) {
		return SendEmailOutput{}, apperror.Conflict("Lagu ini belum punya email pelanggan. Isi alamat tujuan untuk mengirim email tes.")
	}
	if err != nil {
		s.log.Error("admin email send failed", "song_id", sg.ID, "kind", in.Kind, "err", err)
		return SendEmailOutput{}, apperror.Unavailable("Email gagal dikirim: "+err.Error(), err)
	}

	out := SendEmailOutput{Kind: in.Kind, Sent: sent, Test: in.To != ""}
	if err := s.audit.Record(ctx, audit.Entry{ActorType: "admin", ActorID: actorID, Action: "song.email_send",
		EntityType: "song", EntityID: sg.ID,
		Diff: map[string]any{"kind": in.Kind, "to": sent.To, "test": out.Test}}); err != nil {
		s.log.ErrorContext(ctx, "audit log write failed", "action", "song.email_send", "entity_id", sg.ID, "error", err)
	}
	return out, nil
}

// AdminHandler serves the admin send-email route.
type AdminHandler struct {
	svc   *AdminService
	auth  fiber.Handler
	actor func(fiber.Ctx) string
}

// NewAdminHandler builds the handler. auth guards the route; actor names the admin for the audit log.
func NewAdminHandler(svc *AdminService, auth fiber.Handler, actor func(fiber.Ctx) string) *AdminHandler {
	return &AdminHandler{svc: svc, auth: auth, actor: actor}
}

// RegisterRoutes mounts the route under /api/v1. It implements app.Module.
//
//	POST /api/v1/admin/songs/:id/email   {kind: song_ready|payment_receipt, to?: test address}
func (h *AdminHandler) RegisterRoutes(r fiber.Router) {
	r.Post("/admin/songs/:id/email", h.auth, h.send)
}

func (h *AdminHandler) send(c fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return apperror.BadRequest("id must be a valid UUID")
	}
	var in SendEmailInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.SendEmail(c.Context(), h.actor(c), id, in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

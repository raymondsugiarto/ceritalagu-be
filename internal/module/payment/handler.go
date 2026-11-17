package payment

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/response"
)

// PricingOutput is GET /payments/pricing.
type PricingOutput struct {
	UnlockPrice int64  `json:"unlock_price"`
	Currency    string `json:"currency"`
	Enabled     bool   `json:"enabled"` // false until DOKU credentials are configured
}

// CheckoutOutput is where to send the customer.
type CheckoutOutput struct {
	OrderID       string     `json:"order_id"`
	InvoiceNumber string     `json:"invoice_number"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	PaymentURL    string     `json:"payment_url"`
	ExpiresAt     *time.Time `json:"expires_at"`
}

// OrderOutput is an order as the customer sees it after returning from DOKU.
type OrderOutput struct {
	ID            string     `json:"id"`
	InvoiceNumber string     `json:"invoice_number"`
	SongID        string     `json:"song_id"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	Status        Status     `json:"status"`
	PaymentMethod string     `json:"payment_method,omitempty"`
	PaymentURL    string     `json:"payment_url,omitempty"` // only while pending (to resume paying)
	ExpiresAt     *time.Time `json:"expires_at"`
	PaidAt        *time.Time `json:"paid_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func toCheckoutOutput(o Order) CheckoutOutput {
	return CheckoutOutput{OrderID: o.ID, InvoiceNumber: o.InvoiceNumber, Amount: o.Amount, Currency: o.Currency,
		PaymentURL: o.PaymentURL, ExpiresAt: o.ExpiresAt}
}

func toOrderOutput(o Order) OrderOutput {
	out := OrderOutput{ID: o.ID, InvoiceNumber: o.InvoiceNumber, SongID: o.SongID, Amount: o.Amount, Currency: o.Currency,
		Status: o.Status, PaymentMethod: o.PaymentMethod, ExpiresAt: o.ExpiresAt, PaidAt: o.PaidAt, CreatedAt: o.CreatedAt}
	if o.Status == StatusPending {
		out.PaymentURL = o.PaymentURL
	}
	return out
}

// Handler serves the payment endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	GET  /api/v1/payments/pricing               unlock price + whether payments are on
//	POST /api/v1/songs/:id/checkout             open a DOKU payment page (returns payment_url to redirect to)
//	GET  /api/v1/payment-orders/:id             order status (checks DOKU while pending)
//	POST /api/v1/payments/doku/notifications    DOKU HTTP notification (signature-verified, no cookie)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	r.Get("/payments/pricing", h.pricing)
	r.Post("/songs/:id/checkout", h.checkout)
	r.Get("/payment-orders/:id", h.getOrder)
	r.Post("/payments/doku/notifications", h.dokuNotification)
}

func (h *Handler) pricing(c fiber.Ctx) error { return response.OK(c, h.svc.Pricing()) }

func (h *Handler) checkout(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Checkout(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *Handler) getOrder(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.GetOrder(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) dokuNotification(c fiber.Ctx) error {
	hdr := http.Header{}
	for _, k := range []string{"Client-Id", "Request-Id", "Request-Timestamp", "Signature"} {
		hdr.Set(k, c.Get(k))
	}
	if err := h.svc.HandleNotification(c.Context(), hdr, c.Body()); err != nil {
		return err
	}
	return response.Message(c, "notification received")
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

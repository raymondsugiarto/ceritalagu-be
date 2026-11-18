package contact

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves POST /contact.
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) send(c fiber.Ctx) error {
	var in SendContactInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Send(c.Context(), guest.FromCtx(c), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// sendLimiter allows 5 messages per 10 minutes per client IP (inbox flooding).
func sendLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        5,
		Expiration: 10 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			return "contact:" + c.IP()
		},
		LimitReached: func(_ fiber.Ctx) error {
			return apperror.TooMany("Terlalu banyak pesan terkirim. Coba lagi beberapa menit lagi.").WithReason(apperror.ReasonTooManyMessages)
		},
	})
}

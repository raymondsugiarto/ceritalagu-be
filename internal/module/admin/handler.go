package admin

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Login attempts allowed per client IP per window (brute-force protection).
const (
	loginMaxAttempts = 5
	loginWindow      = time.Minute
)

// Handler serves /admin/auth. Routes are registered in router.go.
type Handler struct {
	svc    *Service
	guard  *Guard
	secure bool
}

// NewHandler builds the handler. secure sets the cookie's Secure flag (true behind HTTPS).
func NewHandler(svc *Service, guard *Guard, secure bool) *Handler {
	return &Handler{svc: svc, guard: guard, secure: secure}
}

func (h *Handler) login(c fiber.Ctx) error {
	var in LoginInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, token, err := h.svc.Login(c.Context(), in)
	if err != nil {
		return err
	}
	c.Cookie(sessionCookie(token, h.svc.signer.TTL(), h.secure))
	return response.OK(c, out)
}

func (h *Handler) logout(c fiber.Ctx) error {
	c.Cookie(sessionCookie("", -time.Second, h.secure))
	return response.Message(c, "signed out")
}

func (h *Handler) me(c fiber.Ctx) error {
	return response.OK(c, toOutput(FromCtx(c)))
}

func loginLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        loginMaxAttempts,
		Expiration: loginWindow,
		KeyGenerator: func(c fiber.Ctx) string {
			return "admin-login:" + c.IP()
		},
		LimitReached: func(_ fiber.Ctx) error {
			return apperror.TooMany("terlalu banyak percobaan masuk, coba lagi sebentar lagi")
		},
	})
}

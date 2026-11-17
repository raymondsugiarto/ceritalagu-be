package auth

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/response"
)

// CookieName is the customer session cookie. HttpOnly + SameSite=Lax: never readable by scripts and
// not sent on cross-site POSTs (CSRF).
const CookieName = "user_session"

// Handler serves /auth. Routes are registered in router.go.
type Handler struct {
	svc    *Service
	secure bool
}

// NewHandler builds the handler. secure sets the cookie's Secure flag (true behind HTTPS).
func NewHandler(svc *Service, secure bool) *Handler { return &Handler{svc: svc, secure: secure} }

func (h *Handler) lookup(c fiber.Ctx) error {
	var in EmailInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Lookup(c.Context(), in.Email)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) requestOTP(c fiber.Ctx) error {
	var in EmailInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.RequestOTP(c.Context(), in.Email)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) verifyOTP(c fiber.Ctx) error {
	var in VerifyOTPInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.VerifyOTP(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) setPassword(c fiber.Ctx) error {
	var in SetPasswordInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, token, err := h.svc.SetPassword(c.Context(), guest.FromCtx(c), in)
	if err != nil {
		return err
	}
	c.Cookie(h.cookie(token, SessionTTL))
	return response.OK(c, out)
}

func (h *Handler) login(c fiber.Ctx) error {
	var in LoginInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, token, err := h.svc.Login(c.Context(), guest.FromCtx(c), in)
	if err != nil {
		return err
	}
	c.Cookie(h.cookie(token, SessionTTL))
	return response.OK(c, out)
}

func (h *Handler) logout(c fiber.Ctx) error {
	c.Cookie(h.cookie("", -time.Second))
	return response.Message(c, "signed out")
}

func (h *Handler) me(c fiber.Ctx) error {
	u, err := h.svc.Authenticate(c.Context(), c.Cookies(CookieName))
	if err != nil {
		return err
	}
	return response.OK(c, toCustomerOutput(u))
}

func (h *Handler) cookie(value string, ttl time.Duration) *fiber.Cookie {
	return &fiber.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HTTPOnly: true,
		Secure:   h.secure,
		SameSite: fiber.CookieSameSiteLaxMode,
	}
}

// perIP allows limit calls per minute per client IP to one endpoint (brute force, email flooding).
func perIP(name string, limit int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        limit,
		Expiration: time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			return "auth-" + name + ":" + c.IP()
		},
		LimitReached: func(_ fiber.Ctx) error {
			return apperror.TooMany("Terlalu banyak percobaan. Coba lagi sebentar lagi.")
		},
	})
}

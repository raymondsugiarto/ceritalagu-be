package admin

import (
	"slices"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/internal/apperror"
)

// CookieName is the admin session cookie. It is scoped to /api/v1/admin and SameSite=Strict, so
// it is never sent by cross-site requests (CSRF) or to the customer API.
const CookieName = "admin_session"

// CookiePath limits the session cookie to the admin API.
const CookiePath = "/api/v1/admin"

const localsKey = "admin.current"

// Guard builds route middleware that requires a signed-in admin.
type Guard struct {
	svc *Service
}

// NewGuard builds the guard.
func NewGuard(svc *Service) *Guard { return &Guard{svc: svc} }

// Require allows a signed-in admin with one of roles (any role when none are given).
func (g *Guard) Require(roles ...Role) fiber.Handler {
	return func(c fiber.Ctx) error {
		a, err := g.svc.Authenticate(c.Context(), c.Cookies(CookieName))
		if err != nil {
			return err
		}
		if len(roles) > 0 && !slices.Contains(roles, a.Role) {
			return apperror.Forbidden("akunmu tidak punya akses ke halaman ini")
		}
		c.Locals(localsKey, a)
		return c.Next()
	}
}

// FromCtx returns the admin set by Require (zero value if it did not run).
func FromCtx(c fiber.Ctx) Admin {
	a, _ := c.Locals(localsKey).(Admin)
	return a
}

func sessionCookie(value string, ttl time.Duration, secure bool) *fiber.Cookie {
	return &fiber.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     CookiePath,
		MaxAge:   int(ttl.Seconds()),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: fiber.CookieSameSiteStrictMode,
	}
}

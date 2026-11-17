package guest

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// CookieName is the guest session cookie.
const CookieName = "guest_session_id"

const (
	localsKey    = "guest.visitor"
	cookieMaxAge = 365 * 24 * time.Hour
	maxUALength  = 500
)

// Middleware reads (or mints) the guest cookie and stores the Visitor in the request.
// It does not touch the database: the session row is created when the guest first saves something.
type Middleware struct {
	salt   string
	secure bool
}

// NewMiddleware builds the middleware. salt seeds the IP hash; secure sets the cookie's Secure flag.
func NewMiddleware(salt string, secure bool) *Middleware {
	return &Middleware{salt: salt, secure: secure}
}

// RegisterRoutes attaches the middleware to /api/v1. Register this module first in app.New so it
// runs before every other module's routes.
func (m *Middleware) RegisterRoutes(r fiber.Router) {
	r.Use(m.handle)
}

func (m *Middleware) handle(c fiber.Ctx) error {
	id := c.Cookies(CookieName)
	if _, err := uuid.Parse(id); err != nil {
		id = uuid.NewString()
		c.Cookie(&fiber.Cookie{
			Name:     CookieName,
			Value:    id,
			Path:     "/",
			MaxAge:   int(cookieMaxAge.Seconds()),
			HTTPOnly: true,
			Secure:   m.secure,
			SameSite: fiber.CookieSameSiteLaxMode,
		})
	}
	ua := c.Get(fiber.HeaderUserAgent)
	if len(ua) > maxUALength {
		ua = ua[:maxUALength]
	}
	c.Locals(localsKey, Visitor{SessionID: id, IPHash: m.hashIP(c.IP()), UserAgent: ua})
	return c.Next()
}

// hashIP never stores the raw IP (privacy): sha256(salt || ip).
func (m *Middleware) hashIP(ip string) string {
	sum := sha256.Sum256([]byte(m.salt + ip))
	return hex.EncodeToString(sum[:])
}

// FromCtx returns the Visitor set by the middleware (zero value if it did not run).
func FromCtx(c fiber.Ctx) Visitor {
	v, _ := c.Locals(localsKey).(Visitor)
	return v
}

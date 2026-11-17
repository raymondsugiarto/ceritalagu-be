package admin

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	POST /api/v1/admin/auth/login    sign in (sets the admin_session cookie; 5 tries/min per IP)
//	POST /api/v1/admin/auth/logout   sign out (clears the cookie)
//	GET  /api/v1/admin/auth/me       the signed-in admin (401 when not signed in)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/auth")
	g.Post("/login", loginLimiter(), h.login)
	g.Post("/logout", h.logout)
	g.Get("/me", h.guard.Require(), h.me)
}

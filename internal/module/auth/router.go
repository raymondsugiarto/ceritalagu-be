package auth

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	POST /api/v1/auth/lookup       {email} → {method: otp|password}     (20/min per IP)
//	POST /api/v1/auth/otp          {email} → emails a 6-digit code      (5/min per IP, 1/min per email)
//	POST /api/v1/auth/otp/verify   {email, code} → {setup_token}        (10/min per IP, 5 tries per code)
//	POST /api/v1/auth/password     {setup_token, password} → session    (10/min per IP)
//	POST /api/v1/auth/login        {email, password} → session          (5/min per IP)
//	POST /api/v1/auth/logout       clears the user_session cookie
//	GET  /api/v1/auth/me           the signed-in customer (401 when not signed in)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/auth")
	g.Post("/lookup", perIP("lookup", 20), h.lookup)
	g.Post("/otp", perIP("otp", 5), h.requestOTP)
	g.Post("/otp/verify", perIP("verify", 10), h.verifyOTP)
	g.Post("/password", perIP("password", 10), h.setPassword)
	g.Post("/login", perIP("login", 5), h.login)
	g.Post("/logout", h.logout)
	g.Get("/me", h.me)
}

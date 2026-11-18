package contact

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	POST /api/v1/contact   send a message to the support inbox (no login)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	r.Post("/contact", sendLimiter(), h.send)
}

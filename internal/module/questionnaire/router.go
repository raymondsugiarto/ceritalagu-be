package questionnaire

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	POST /api/v1/questionnaires       submit the 7 answers (guest, no login)
//	GET  /api/v1/questionnaires/:id   the visitor's own questionnaire (review / edit)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/questionnaires")
	g.Post("/", h.create)
	g.Get("/:id", h.get)
}

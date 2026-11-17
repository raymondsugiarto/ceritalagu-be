package user

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under the given router (/api/v1). It implements app.Module.
//
//	GET    /api/v1/admin/users          list (search, ?type=lead|prospect|customer|repeat_customer, pagination)
//	GET    /api/v1/admin/users/stats    funnel counts: guests + users per type
//	POST   /api/v1/admin/users          create
//	GET    /api/v1/admin/users/:id      detail
//	PATCH  /api/v1/admin/users/:id      partial update
//	DELETE /api/v1/admin/users/:id      delete
//
// Admin only (customer emails and phones); under /admin so the admin_session cookie (Path=/api/v1/admin) is sent. Customers are created by the questionnaire contact step.
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/users", h.auth)
	g.Get("/", h.list)
	g.Get("/stats", h.stats)
	g.Post("/", h.create)
	g.Get("/:id", h.get)
	g.Patch("/:id", h.update)
	g.Delete("/:id", h.delete)
}

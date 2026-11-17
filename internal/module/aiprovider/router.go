package aiprovider

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the admin provider API under /api/v1. It implements app.Module.
// Every route needs a super_admin session (admin_session cookie).
//
//	GET    /api/v1/admin/providers/adapters        adapter catalog (types, defaults) for the form
//	GET    /api/v1/admin/providers?category=        list (lyrics_style | music)
//	POST   /api/v1/admin/providers                 create (API key encrypted; optional is_active)
//	GET    /api/v1/admin/providers/:id             detail (API key masked)
//	PATCH  /api/v1/admin/providers/:id             partial update (api_key only to replace it)
//	POST   /api/v1/admin/providers/:id/activate    switch it on (several may be active; priority decides)
//	POST   /api/v1/admin/providers/:id/deactivate  turn it off
//	POST   /api/v1/admin/providers/:id/test        test connection with the stored key
//	DELETE /api/v1/admin/providers/:id             archive (soft delete; inactive providers only)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/providers", h.auth)
	g.Get("/adapters", h.adapters)
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Get("/:id", h.get)
	g.Patch("/:id", h.update)
	g.Post("/:id/activate", h.activate)
	g.Post("/:id/deactivate", h.deactivate)
	g.Post("/:id/test", h.test)
	g.Delete("/:id", h.archive)
}

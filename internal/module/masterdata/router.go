package masterdata

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
// :kind is recipient_type | occasion | genre.
//
//	GET    /api/v1/questionnaire-options          active options for the questionnaire (public)
//	GET    /api/v1/admin/master-data/:kind        all rows, incl. inactive and prompt hints (super_admin)
//	POST   /api/v1/admin/master-data/:kind        add a row
//	PATCH  /api/v1/admin/master-data/:kind/:id    partial update (code is fixed)
//	DELETE /api/v1/admin/master-data/:kind/:id    archive (soft delete)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	r.Get("/questionnaire-options", h.options)

	g := r.Group("/admin/master-data", h.auth)
	g.Get("/:kind", h.list)
	g.Post("/:kind", h.create)
	g.Patch("/:kind/:id", h.update)
	g.Delete("/:kind/:id", h.archive)
}

package pricing

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves the admin pricing and voucher API.
type Handler struct {
	svc   *Service
	auth  fiber.Handler
	actor func(c fiber.Ctx) string
}

// NewHandler builds the handler. auth guards every route; actor returns the admin ID for the audit log.
func NewHandler(svc *Service, auth fiber.Handler, actor func(c fiber.Ctx) string) *Handler {
	return &Handler{svc: svc, auth: auth, actor: actor}
}

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	GET    /api/v1/admin/pricing            normal, sale and minimum price (super_admin)
//	PUT    /api/v1/admin/pricing            save them
//	GET    /api/v1/admin/vouchers           every voucher with usage
//	POST   /api/v1/admin/vouchers           add a voucher
//	PATCH  /api/v1/admin/vouchers/:id       partial update (code is fixed)
//	DELETE /api/v1/admin/vouchers/:id       archive (soft delete)
func (h *Handler) RegisterRoutes(r fiber.Router) {
	p := r.Group("/admin/pricing", h.auth)
	p.Get("", h.getPrices)
	p.Put("", h.updatePrices)

	v := r.Group("/admin/vouchers", h.auth)
	v.Get("", h.listVouchers)
	v.Post("", h.createVoucher)
	v.Patch("/:id", h.updateVoucher)
	v.Delete("/:id", h.archiveVoucher)
}

func (h *Handler) getPrices(c fiber.Ctx) error {
	out, err := h.svc.AdminPrices(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) updatePrices(c fiber.Ctx) error {
	var in UpdatePricesInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.UpdatePrices(c.Context(), h.actor(c), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) listVouchers(c fiber.Ctx) error {
	out, err := h.svc.ListVouchers(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) createVoucher(c fiber.Ctx) error {
	var in CreateVoucherInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.CreateVoucher(c.Context(), h.actor(c), in)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *Handler) updateVoucher(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in UpdateVoucherInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.UpdateVoucher(c.Context(), h.actor(c), id, in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) archiveVoucher(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.ArchiveVoucher(c.Context(), h.actor(c), id); err != nil {
		return err
	}
	return response.Message(c, "voucher archived")
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

package aiprovider

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves /admin/providers. Routes are registered in router.go.
type Handler struct {
	svc   *Service
	auth  fiber.Handler
	actor func(c fiber.Ctx) string
}

// NewHandler builds the handler. auth guards every route (super_admin only); actor returns the
// signed-in admin's ID for the audit log.
func NewHandler(svc *Service, auth fiber.Handler, actor func(c fiber.Ctx) string) *Handler {
	return &Handler{svc: svc, auth: auth, actor: actor}
}

func (h *Handler) adapters(c fiber.Ctx) error {
	return response.OK(c, h.svc.Adapters())
}

func (h *Handler) list(c fiber.Ctx) error {
	var in ListProvidersInput
	if err := c.Bind().Query(&in); err != nil {
		return err
	}
	out, err := h.svc.List(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) get(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Context(), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) create(c fiber.Ctx) error {
	var in CreateProviderInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Context(), h.actor(c), in)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *Handler) update(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in UpdateProviderInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Context(), h.actor(c), id, in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) activate(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Activate(c.Context(), h.actor(c), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) deactivate(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Deactivate(c.Context(), h.actor(c), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) archive(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Archive(c.Context(), h.actor(c), id); err != nil {
		return err
	}
	return response.Message(c, "provider archived")
}

func (h *Handler) test(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.TestConnection(c.Context(), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

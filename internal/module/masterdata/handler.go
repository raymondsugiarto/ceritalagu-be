package masterdata

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves the public options endpoint and the admin master-data API.
type Handler struct {
	svc   *Service
	auth  fiber.Handler
	actor func(c fiber.Ctx) string
}

// NewHandler builds the handler. auth guards the admin routes; actor returns the admin ID for the audit log.
func NewHandler(svc *Service, auth fiber.Handler, actor func(c fiber.Ctx) string) *Handler {
	return &Handler{svc: svc, auth: auth, actor: actor}
}

func (h *Handler) options(c fiber.Ctx) error {
	out, err := h.svc.QuestionnaireOptions(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) list(c fiber.Ctx) error {
	out, err := h.svc.List(c.Context(), Kind(c.Params("kind")))
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) create(c fiber.Ctx) error {
	var in CreateItemInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Context(), h.actor(c), Kind(c.Params("kind")), in)
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
	var in UpdateItemInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Context(), h.actor(c), Kind(c.Params("kind")), id, in)
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
	if err := h.svc.Archive(c.Context(), h.actor(c), Kind(c.Params("kind")), id); err != nil {
		return err
	}
	return response.Message(c, "archived")
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

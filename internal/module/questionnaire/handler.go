package questionnaire

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves the /questionnaires endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) create(c fiber.Ctx) error {
	var in CreateQuestionnaireInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Context(), guest.FromCtx(c), in)
	if err != nil {
		return err
	}
	return response.Created(c, out)
}

func (h *Handler) get(c fiber.Ctx) error {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return apperror.BadRequest("id must be a valid UUID")
	}
	out, err := h.svc.Get(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

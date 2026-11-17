package user

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves the /users endpoints. Routes are registered in router.go.
type Handler struct {
	svc  *Service
	auth fiber.Handler
}

// NewHandler builds the handler. auth guards every route: users hold customer emails and phones.
func NewHandler(svc *Service, auth fiber.Handler) *Handler {
	return &Handler{svc: svc, auth: auth}
}

func (h *Handler) list(c fiber.Ctx) error {
	var in ListUsersInput
	if err := c.Bind().Query(&in); err != nil {
		return err
	}
	out, err := h.svc.List(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) stats(c fiber.Ctx) error {
	out, err := h.svc.Stats(c.Context())
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
	var in CreateUserInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Context(), in)
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
	var in UpdateUserInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Context(), id, in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) delete(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Context(), id); err != nil {
		return err
	}
	return response.Message(c, "user deleted")
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

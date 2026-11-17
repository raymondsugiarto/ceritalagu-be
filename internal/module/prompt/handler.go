package prompt

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves the admin prompt template API.
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
//	GET    /api/v1/admin/prompts                  versions (newest first), built-in default, placeholders
//	POST   /api/v1/admin/prompts                  save an edit as a new version (optionally activate)
//	POST   /api/v1/admin/prompts/:id/activate     make a version active (also rollback)
//	POST   /api/v1/admin/prompts/use-default      deactivate all; jobs use the built-in default
//	DELETE /api/v1/admin/prompts/:id              archive an inactive version
//	POST   /api/v1/admin/prompts/preview          render a draft with sample answers (no AI call)
//	POST   /api/v1/admin/prompts/playground       render + run once on the active lyrics provider
//	POST   /api/v1/admin/prompts/playground/music lyrics + style once on the active music provider; saved as a playground song
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/prompts", h.auth)
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Post("/use-default", h.useDefault)
	g.Post("/preview", h.preview)
	g.Post("/playground", h.playground)
	g.Post("/playground/music", h.playgroundMusic)
	g.Post("/:id/activate", h.activate)
	g.Delete("/:id", h.archive)
}

func (h *Handler) list(c fiber.Ctx) error {
	out, err := h.svc.List(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) create(c fiber.Ctx) error {
	var in CreateVersionInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Context(), h.actor(c), in)
	if err != nil {
		return err
	}
	return response.Created(c, out)
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

func (h *Handler) useDefault(c fiber.Ctx) error {
	if err := h.svc.UseDefault(c.Context(), h.actor(c)); err != nil {
		return err
	}
	return response.Message(c, "default prompt in use")
}

func (h *Handler) archive(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.svc.Archive(c.Context(), h.actor(c), id); err != nil {
		return err
	}
	return response.Message(c, "archived")
}

func (h *Handler) preview(c fiber.Ctx) error {
	var in PlaygroundInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Preview(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) playground(c fiber.Ctx) error {
	var in PlaygroundInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Playground(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) playgroundMusic(c fiber.Ctx) error {
	var in MusicPlaygroundInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.PlaygroundMusic(c.Context(), h.actor(c), in)
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

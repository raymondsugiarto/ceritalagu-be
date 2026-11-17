package playground

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves the admin playground-song API and the public landing samples.
type Handler struct {
	svc  *Service
	auth fiber.Handler
}

// NewHandler builds the handler. auth guards the admin routes.
func NewHandler(svc *Service, auth fiber.Handler) *Handler { return &Handler{svc: svc, auth: auth} }

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	GET    /api/v1/admin/playground-songs            songs generated in the playground (?landing=true)
//	PATCH  /api/v1/admin/playground-songs/:id        title, caption, show_on_landing, landing_order
//	DELETE /api/v1/admin/playground-songs/:id        archive (soft delete; the file stays in storage)
//	GET    /api/v1/admin/playground-songs/:id/audio  full audio
//	GET    /api/v1/samples                           public: songs picked for the landing page
//	GET    /api/v1/samples/:id/audio                 public: audio of a landing sample
func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/admin/playground-songs", h.auth)
	g.Get("/", h.list)
	g.Patch("/:id", h.update)
	g.Delete("/:id", h.archive)
	g.Get("/:id/audio", h.audio)

	r.Get("/samples", h.samples)
	r.Get("/samples/:id/audio", h.sampleAudio)
}

func (h *Handler) list(c fiber.Ctx) error {
	var in ListSongsInput
	if err := c.Bind().Query(&in); err != nil {
		return err
	}
	out, err := h.svc.List(c.Context(), in)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) update(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	var in UpdateSongInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Context(), id, in)
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
	if err := h.svc.Archive(c.Context(), id); err != nil {
		return err
	}
	return response.Message(c, "archived")
}

func (h *Handler) audio(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	data, ct, err := h.svc.Audio(c.Context(), id)
	if err != nil {
		return err
	}
	return sendAudio(c, data, ct, "private, max-age=3600")
}

func (h *Handler) samples(c fiber.Ctx) error {
	out, err := h.svc.Samples(c.Context())
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) sampleAudio(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	data, ct, err := h.svc.SampleAudio(c.Context(), id)
	if err != nil {
		return err
	}
	return sendAudio(c, data, ct, "public, max-age=3600")
}

func sendAudio(c fiber.Ctx, data []byte, contentType, cache string) error {
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderCacheControl, cache)
	return c.Send(data)
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

package song

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/module/guest"
	"github.com/yourorg/music-api/internal/response"
)

// Handler serves /song-jobs and /songs.
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) createJob(c fiber.Ctx) error {
	var in CreateSongJobInput
	if err := c.Bind().Body(&in); err != nil {
		return err
	}
	out, err := h.svc.CreateJob(c.Context(), guest.FromCtx(c), in)
	if err != nil {
		return err
	}
	return response.Success(c, fiber.StatusAccepted, "song job queued", out)
}

func (h *Handler) getJob(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.GetJob(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

func (h *Handler) regenerate(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Regenerate(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	return response.Success(c, fiber.StatusAccepted, "song job queued", out)
}

func (h *Handler) getSong(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	out, err := h.svc.GetSong(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	return response.OK(c, out)
}

// preview streams audio, so it is the one endpoint that does not return the JSON envelope
// (errors still do, through the global error handler).
func (h *Handler) preview(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	data, err := h.svc.Preview(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "audio/mpeg")
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	return c.Send(data)
}

// fullAudio streams the bought song; ?download=1 asks the browser to save it.
func (h *Handler) fullAudio(c fiber.Ctx) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	data, sg, err := h.svc.FullAudio(c.Context(), guest.FromCtx(c), id)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "audio/mpeg")
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	if c.Query("download") != "" {
		c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+downloadName(sg.Title)+`.mp3"`)
	}
	return c.Send(data)
}

// downloadName keeps letters, digits, spaces and dashes of the title.
func downloadName(title string) string {
	var b []rune
	for _, r := range title {
		if r == ' ' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b = append(b, r)
		}
	}
	if len(b) == 0 {
		return "lagu"
	}
	return string(b)
}

func pathID(c fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", apperror.BadRequest("id must be a valid UUID")
	}
	return id, nil
}

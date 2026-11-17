package song

import "github.com/gofiber/fiber/v3"

// RegisterRoutes mounts the module under /api/v1. It implements app.Module.
//
//	POST /api/v1/song-jobs                  start a song from a questionnaire (202, daily guest quota)
//	GET  /api/v1/song-jobs/:id              job progress (poll every few seconds)
//	POST /api/v1/song-jobs/:id/regenerate   "Buat Ulang" (202, max 2 per song)
//	GET  /api/v1/songs/:id                  result page data (preview lyrics; full lyrics when unlocked)
//	GET  /api/v1/songs/:id/preview          preview audio up to the first chorus (audio/mpeg)
//	GET  /api/v1/songs/:id/audio            full song (audio/mpeg), only once unlocked; ?download=1 saves it
func (h *Handler) RegisterRoutes(r fiber.Router) {
	jobs := r.Group("/song-jobs")
	jobs.Post("/", h.createJob)
	jobs.Get("/:id", h.getJob)
	jobs.Post("/:id/regenerate", h.regenerate)

	songs := r.Group("/songs")
	songs.Get("/:id", h.getSong)
	songs.Get("/:id/preview", h.preview)
	songs.Get("/:id/audio", h.fullAudio)
}

package app

// Liveness and readiness probes for load balancers / Kubernetes.

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// Checker reports whether a dependency (DB, cache, ...) is reachable.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// Health serves /healthz (liveness) and /readyz (readiness).
type Health struct {
	checkers []Checker
}

// NewHealth builds the probes. /readyz runs every checker on each call.
func NewHealth(checkers ...Checker) *Health {
	return &Health{checkers: checkers}
}

// RegisterRoutes mounts the probes at the root, outside /api/v1.
func (h *Health) RegisterRoutes(r fiber.Router) {
	r.Get("/healthz", h.live)
	r.Get("/readyz", h.ready)
}

// live: the process is up. Never checks dependencies.
func (h *Health) live(c fiber.Ctx) error {
	return response.OK(c, fiber.Map{"status": "ok"})
}

// ready: the app can serve traffic. Returns 503 if any dependency check fails.
func (h *Health) ready(c fiber.Ctx) error {
	results := make(map[string]string, len(h.checkers))
	healthy := true
	for _, chk := range h.checkers {
		if err := chk.Check(c.Context()); err != nil {
			results[chk.Name()] = "down"
			healthy = false
			continue
		}
		results[chk.Name()] = "up"
	}
	if !healthy {
		return response.Write(c, response.Response[map[string]string]{
			HTTPCode: fiber.StatusServiceUnavailable,
			Code:     string(apperror.CodeUnavailable),
			Message:  "service unavailable",
			Data:     results,
		})
	}
	return response.OK(c, results)
}

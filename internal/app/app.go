// Package app builds the Fiber application: global middleware, error handling, health probes,
// and mounting of every module under /api/v1.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/yourorg/music-api/config"
	"github.com/yourorg/music-api/internal/i18n"
	"github.com/yourorg/music-api/internal/validator"
)

// Module is implemented by every module's Handler (in its router.go).
type Module interface {
	RegisterRoutes(r fiber.Router)
}

// App is the HTTP server: the Fiber app plus the config and logger it was built with.
type App struct {
	fiber *fiber.App
	cfg   config.Server
	log   *slog.Logger
}

// New wires the app. Health probes mount at the root; modules mount at /api/v1.
func New(cfg config.Config, log *slog.Logger, health *Health, modules ...Module) *App {
	rest := cfg.Server.Rest
	f := fiber.New(fiber.Config{
		AppName:         rest.Name,
		ReadTimeout:     rest.ReadTimeout,
		WriteTimeout:    rest.WriteTimeout,
		IdleTimeout:     rest.IdleTimeout,
		BodyLimit:       rest.BodyLimitBytes,
		ErrorHandler:    errorHandler(log),
		StructValidator: validator.New(),
		// c.IP() reads X-Forwarded-For only when the direct peer is a trusted proxy (anti-spoofing).
		TrustProxy:  len(rest.TrustedProxies) > 0 || rest.TrustLoopbackProxy,
		ProxyHeader: fiber.HeaderXForwardedFor,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies:  rest.TrustedProxies,
			Loopback: rest.TrustLoopbackProxy,
		},
	})

	// Order matters:
	//   requestid -> every later log line and error carries the ID
	//   accessLog -> wraps recover, so panicking requests are still logged with their final status
	//   recover   -> turns panics into errors handled by errorHandler
	//   locale    -> request locale (?locale → NEXT_LOCALE cookie → Accept-Language → id) in c.Context()
	f.Use(requestid.New())
	f.Use(accessLog(log))
	f.Use(recover.New(recover.Config{EnableStackTrace: !rest.IsProduction()}))
	f.Use(helmet.New())
	f.Use(cors.New(cors.Config{
		AllowOrigins:  rest.CorsOrigins,
		AllowMethods:  []string{fiber.MethodGet, fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions},
		AllowHeaders:  []string{fiber.HeaderAuthorization, fiber.HeaderContentType, fiber.HeaderXRequestID},
		ExposeHeaders: []string{fiber.HeaderXRequestID},
	}))
	f.Use(i18n.Middleware())

	health.RegisterRoutes(f)
	v1 := f.Group("/api/v1")
	for _, m := range modules {
		m.RegisterRoutes(v1)
	}

	// Anything unmatched returns the standard 404 envelope.
	f.Use(func(_ fiber.Ctx) error { return fiber.ErrNotFound })

	return &App{fiber: f, cfg: rest, log: log}
}

// Fiber exposes the underlying Fiber app (used by tests via Test).
func (a *App) Fiber() *fiber.App { return a.fiber }

// Listen blocks until the listener stops.
func (a *App) Listen() error {
	addr := fmt.Sprintf("%s:%d", a.cfg.Host, a.cfg.Port)
	a.log.Info("http server starting", "addr", addr, "env", a.cfg.Env)
	return a.fiber.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true})
}

// Shutdown stops accepting connections and waits for in-flight requests until ctx expires.
func (a *App) Shutdown(ctx context.Context) error {
	return a.fiber.ShutdownWithContext(ctx)
}

package app

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/yourorg/music-api/internal/apperror"
	"github.com/yourorg/music-api/internal/response"
)

// errorHandler is the single place errors become HTTP responses.
// Handlers just `return err`; they never write error JSON themselves.
func errorHandler(log *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		var (
			appErr   *apperror.Error
			fiberErr *fiber.Error
			bindErr  *fiber.BindError
		)
		switch {
		case errors.As(err, &appErr):
			// already client-safe
		case errors.As(err, &bindErr):
			appErr = apperror.BadRequest("malformed request " + bindErr.Source).Wrap(err)
		case errors.As(err, &fiberErr):
			appErr = apperror.FromStatus(fiberErr.Code, fiberErr.Message)
		default:
			appErr = apperror.Internal(err)
		}

		rid := requestid.FromContext(c)
		if appErr.Status >= fiber.StatusInternalServerError {
			log.ErrorContext(c.Context(), "request failed",
				"request_id", rid, "method", c.Method(), "path", c.Path(), "error", err)
		}
		return response.Error(c, appErr)
	}
}

// accessLog writes one structured line per request.
func accessLog(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		chainErr := c.Next()

		// Run the error handler now so the logged status matches what the client receives.
		if chainErr != nil {
			if err := c.App().ErrorHandler(c, chainErr); err != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		status := c.Response().StatusCode()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		log.Log(c.Context(), level, "http request",
			"request_id", requestid.FromContext(c),
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.IP(),
		)
		return nil
	}
}

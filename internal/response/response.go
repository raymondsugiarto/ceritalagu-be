// Package response writes the standard JSON envelope used by every endpoint.
//
//	{
//	  "code": "SUCCESS",
//	  "message": "success",
//	  "errors": [],
//	  "data": { ... },              // omitted when there is no data
//	  "timestamp": "2026-10-29T05:00:00Z"
//	}
//
// The request ID is returned in the X-Request-ID response header, not in the body.
package response

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yourorg/music-api/internal/apperror"
)

// CodeSuccess is the code for every successful response. Error codes come from apperror.
const CodeSuccess = "SUCCESS"

// Response is the single envelope shape for success and error responses.
type Response[T any] struct {
	HTTPCode  int                    `json:"-"`
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Errors    []apperror.ErrorDetail `json:"errors"`
	Data      T                      `json:"data,omitzero"` // omitzero, not omitempty: an empty list stays []
	Timestamp time.Time              `json:"timestamp"`
}

// Write sends r with its HTTPCode. Errors is always an array (never null) and Timestamp is set here.
func Write[T any](c fiber.Ctx, r Response[T]) error {
	if r.Errors == nil {
		r.Errors = []apperror.ErrorDetail{}
	}
	r.Timestamp = time.Now().UTC()
	return c.Status(r.HTTPCode).JSON(r)
}

// Success writes a success envelope with a custom status and message.
func Success[T any](c fiber.Ctx, httpCode int, message string, data T) error {
	return Write(c, Response[T]{HTTPCode: httpCode, Code: CodeSuccess, Message: message, Data: data})
}

// OK -> 200 with data.
func OK[T any](c fiber.Ctx, data T) error {
	return Success(c, fiber.StatusOK, "success", data)
}

// Created -> 201 with the created resource.
func Created[T any](c fiber.Ctx, data T) error {
	return Success(c, fiber.StatusCreated, "created", data)
}

// Message -> 200 without data (e.g. after delete). Keeps the envelope, unlike 204 No Content.
func Message(c fiber.Ctx, message string) error {
	return Write(c, Response[any]{HTTPCode: fiber.StatusOK, Code: CodeSuccess, Message: message})
}

// Error writes an error envelope. Only called by the global error handler.
func Error(c fiber.Ctx, e *apperror.Error) error {
	return Write(c, Response[any]{
		HTTPCode: e.Status,
		Code:     string(e.Code),
		Message:  e.Message,
		Errors:   e.Details,
	})
}

// Package apperror defines the single error type that crosses the service -> HTTP boundary.
//
// Rule: services return *apperror.Error for anything the client should see.
// Any other error is treated as an internal error (500) and its message is never exposed.
package apperror

import (
	"fmt"
	"net/http"
)

// Code is the machine-readable error code sent in the response "code" field.
type Code string

// Error codes and the HTTP status each one maps to.
const (
	CodeBadRequest   Code = "BAD_REQUEST"
	CodeValidation   Code = "VALIDATION_ERROR"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"
	CodeNotFound     Code = "NOT_FOUND"
	CodeConflict     Code = "CONFLICT"
	CodeTooMany      Code = "TOO_MANY_REQUESTS"
	CodeUnavailable  Code = "SERVICE_UNAVAILABLE"
	CodeInternal     Code = "INTERNAL_ERROR"
)

// ErrorDetail is one entry of the response "errors" array.
// Field is set for validation errors and empty for general errors.
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// Error is a client-facing error. The global error handler turns it into the response envelope.
type Error struct {
	Code    Code
	Message string        // safe to show to clients
	Status  int           // HTTP status
	Details []ErrorDetail // per-field details (validation errors)
	Err     error         // wrapped cause, logged but never returned to clients
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// Wrap attaches a cause for logging without changing what the client sees.
func (e *Error) Wrap(err error) *Error {
	cp := *e
	cp.Err = err
	return &cp
}

// BadRequest is a malformed request (400).
func BadRequest(msg string) *Error {
	return &Error{Code: CodeBadRequest, Message: msg, Status: http.StatusBadRequest}
}

// Validation is a request that failed field validation (422), with one detail per field.
func Validation(details []ErrorDetail) *Error {
	return &Error{Code: CodeValidation, Message: "request validation failed", Status: http.StatusUnprocessableEntity, Details: details}
}

// Unauthorized is a missing or invalid credential (401).
func Unauthorized(msg string) *Error {
	return &Error{Code: CodeUnauthorized, Message: msg, Status: http.StatusUnauthorized}
}

// Forbidden is an authenticated caller without permission (403).
func Forbidden(msg string) *Error {
	return &Error{Code: CodeForbidden, Message: msg, Status: http.StatusForbidden}
}

// NotFound is a resource that does not exist (404).
func NotFound(msg string) *Error {
	return &Error{Code: CodeNotFound, Message: msg, Status: http.StatusNotFound}
}

// Conflict is a request that clashes with existing state, e.g. a duplicate unique field (409).
func Conflict(msg string) *Error {
	return &Error{Code: CodeConflict, Message: msg, Status: http.StatusConflict}
}

// Internal is an unexpected failure (500). err is logged; clients only see a generic message.
func Internal(err error) *Error {
	return &Error{Code: CodeInternal, Message: "internal server error", Status: http.StatusInternalServerError, Err: err}
}

// FromStatus maps a bare HTTP status (e.g. from the framework) to an Error.
func FromStatus(status int, msg string) *Error {
	code := CodeInternal
	switch status {
	case http.StatusBadRequest:
		code = CodeBadRequest
	case http.StatusUnauthorized:
		code = CodeUnauthorized
	case http.StatusForbidden:
		code = CodeForbidden
	case http.StatusNotFound:
		code = CodeNotFound
	case http.StatusMethodNotAllowed:
		code = CodeNotFound
	case http.StatusConflict:
		code = CodeConflict
	case http.StatusRequestEntityTooLarge:
		code = CodeBadRequest
	case http.StatusTooManyRequests:
		code = CodeTooMany
	}
	if status >= 500 {
		msg = "internal server error"
	}
	return &Error{Code: code, Message: msg, Status: status}
}

// TooMany is a caller over a quota or rate limit (429).
func TooMany(msg string) *Error {
	return &Error{Code: CodeTooMany, Message: msg, Status: http.StatusTooManyRequests}
}

// Unavailable is a dependency that is down (503).
func Unavailable(msg string, err error) *Error {
	return &Error{Code: CodeUnavailable, Message: msg, Status: http.StatusServiceUnavailable, Err: err}
}

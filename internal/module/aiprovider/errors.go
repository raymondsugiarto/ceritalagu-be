package aiprovider

import (
	"errors"
	"fmt"
)

// ErrorKind classifies provider failures so the pipeline can decide to retry and what to tell the user.
type ErrorKind string

// Error kinds shared by every adapter.
const (
	KindRateLimited         ErrorKind = "rate_limited"         // retry with backoff
	KindTimeout             ErrorKind = "timeout"              // retry with backoff
	KindUnavailable         ErrorKind = "unavailable"          // provider 5xx / overloaded; retry
	KindAuth                ErrorKind = "auth"                 // bad API key; admin must fix
	KindInsufficientBalance ErrorKind = "insufficient_balance" // admin must top up
	KindContentFlagged      ErrorKind = "content_flagged"      // input/output rejected by moderation
	KindInvalidRequest      ErrorKind = "invalid_request"      // our request is wrong (limits, params)
	KindInvalidOutput       ErrorKind = "invalid_output"       // response could not be parsed/validated; retry
	KindUnknown             ErrorKind = "unknown"
)

// Error is a provider failure. Message is for logs and the admin job detail, never for customers.
type Error struct {
	Kind         ErrorKind
	Provider     string // e.g. "minimax"
	ProviderCode int    // provider status code (e.g. MiniMax base_resp.status_code), 0 if none
	HTTPStatus   int    // 0 if the request never got a response
	Message      string
	Err          error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s %s", e.Provider, e.Kind)
	if e.ProviderCode != 0 {
		msg += fmt.Sprintf(" (code %d)", e.ProviderCode)
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether trying the same request again may succeed.
func (e *Error) Retryable() bool {
	switch e.Kind {
	case KindRateLimited, KindTimeout, KindUnavailable, KindInvalidOutput:
		return true
	default:
		return false
	}
}

// IsRetryable reports whether err is a retryable *Error. Unclassified errors are not retried.
func IsRetryable(err error) bool {
	var pe *Error
	return errors.As(err, &pe) && pe.Retryable()
}

// KindOf returns the ErrorKind of err, or KindUnknown.
func KindOf(err error) ErrorKind {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Kind
	}
	return KindUnknown
}

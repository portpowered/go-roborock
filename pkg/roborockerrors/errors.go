// Package roborockerrors defines failures shared by the public API and transports.
package roborockerrors

import (
	"errors"
	"fmt"
)

// Kind is a stable failure class.
type Kind string

const (
	// InvalidArgument indicates an invalid request or option.
	InvalidArgument Kind = "invalid_argument"
	// Unauthorized indicates missing, rejected, or expired credentials.
	Unauthorized Kind = "unauthorized"
	// NotFound indicates a missing account or device.
	NotFound Kind = "not_found"
	// RateLimited indicates vendor throttling.
	RateLimited Kind = "rate_limited"
	// Unavailable indicates a failed connection or unavailable service.
	Unavailable Kind = "unavailable"
	// Timeout indicates that a request deadline elapsed.
	Timeout Kind = "timeout"
	// Canceled indicates caller cancellation.
	Canceled Kind = "canceled"
	// Protocol indicates a malformed response or rejected operation.
	Protocol Kind = "protocol"
	// Closed indicates a closed session.
	Closed Kind = "closed"
	// Unsupported indicates an unsupported protocol or capability.
	Unsupported Kind = "unsupported"
	// Backpressure indicates a full session queue.
	Backpressure Kind = "backpressure"
)

// Error preserves a failure class, operation, vendor code, and underlying cause.
// Message must not contain credentials or untrusted response bodies.
type Error struct {
	Kind      Kind
	Operation string
	Code      int
	Message   string
	Cause     error
}

func (e *Error) Error() string {
	return fmt.Sprintf("roborock %s: %s (%s, code %d)", e.Operation, e.Message, e.Kind, e.Code)
}

// Unwrap exposes the cause for errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// Is matches the requested failure kind; empty target kinds do not match.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)

	return ok && t.Kind != "" && e.Kind == t.Kind
}

// New creates a failure with a caller-safe message and retained cause.
func New(kind Kind, operation, message string, cause error) *Error {
	return &Error{Kind: kind, Operation: operation, Code: 0, Message: message, Cause: cause}
}

// Wrap preserves a typed failure's classification and code with a caller-safe operation.
// Untyped failures use fallback without exposing the underlying cause in the message.
func Wrap(fallback Kind, operation, message string, cause error) *Error {
	wrapped := New(fallback, operation, message, cause)

	var typed *Error

	if errors.As(cause, &typed) {
		wrapped.Kind = typed.Kind
		wrapped.Code = typed.Code
	}

	return wrapped
}

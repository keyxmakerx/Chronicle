// Package apperror provides domain-specific error types for Chronicle.
// These errors carry an HTTP status code and a user-safe message. The Echo
// error handler maps them to appropriate HTTP responses automatically.
//
// NEVER return raw database or infrastructure errors to the client. Always
// wrap them in an apperror type or return a generic internal error.
package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

// AppError is the base error type for all domain errors. It carries an
// HTTP status code, a machine-readable error type, and a human-readable
// message safe to show to the client.
type AppError struct {
	// Code is the HTTP status code (e.g., 404, 400, 500).
	Code int `json:"-"`

	// Type is a machine-readable error classifier (e.g., "not_found").
	Type string `json:"type"`

	// Message is a human-readable description safe for the client.
	Message string `json:"message"`

	// Internal holds the underlying error for logging. Never exposed to client.
	Internal error `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Type, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *AppError) Unwrap() error {
	return e.Internal
}

// --- Constructors for common error types ---

// NewNotFound creates a 404 Not Found error.
func NewNotFound(message string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Type:    "not_found",
		Message: message,
	}
}

// NewBadRequest creates a 400 Bad Request error.
func NewBadRequest(message string) *AppError {
	return &AppError{
		Code:    http.StatusBadRequest,
		Type:    "bad_request",
		Message: message,
	}
}

// NewUnauthorized creates a 401 Unauthorized error.
func NewUnauthorized(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Type:    "unauthorized",
		Message: message,
	}
}

// NewForbidden creates a 403 Forbidden error.
func NewForbidden(message string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Type:    "forbidden",
		Message: message,
	}
}

// NewConflict creates a 409 Conflict error.
func NewConflict(message string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Type:    "conflict",
		Message: message,
	}
}

// errMissingContext is the shared internal error for nil precondition checks.
var errMissingContext = errors.New("missing required context")

// NewMissingContext creates a 500 error for handler nil-context guards
// (e.g. campaign context not set, dependency not wired). Provides a
// meaningful Internal error for logging instead of nil.
func NewMissingContext() *AppError {
	return NewInternal(errMissingContext)
}

// NewInternal creates a 500 Internal Server Error. The real error is stored
// in Internal for logging but the client only sees a generic message.
func NewInternal(err error) *AppError {
	return &AppError{
		Code:     http.StatusInternalServerError,
		Type:     "internal_error",
		Message:  "An unexpected error occurred. Please try again.",
		Internal: err,
	}
}

// SafeMessage returns the client-safe error message from an error. If the
// error is an AppError, returns its Message field (which is safe to expose).
// For any other error type, returns a generic message to prevent leaking
// internal details like table names, query structure, or stack traces.
func SafeMessage(err error) string {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Message
	}
	return "an unexpected error occurred"
}

// SafeCode returns the HTTP status code from an AppError, or 500 for
// any other error type.
func SafeCode(err error) int {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code
	}
	return http.StatusInternalServerError
}

// NewValidation creates a 422 Unprocessable Entity error for validation failures.
func NewValidation(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnprocessableEntity,
		Type:    "validation_error",
		Message: message,
	}
}

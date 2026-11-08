// permissions_api.go — wire-contract helpers for the entity
// permissions endpoints.
//
// wrapPermissionError preserves typed *AppError sub-errors (e.g. a
// NotFound from UpdateVisibility) so they reach the wire as a
// structured status instead of a generic 500. respondPermissionsError
// emits the shared { error, message, category } shape also used by
// the foundry_vtt API; Category is derived from AppError.Type so
// callers don't retrofit every apperror constructor.
package entities

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// errorCategoryFromType maps apperror.AppError.Type to the
// five-bucket wire-contract enum. Anything unrecognized defaults
// to "internal" — safer to mis-categorize as internal than to
// silently emit an empty category field.
func errorCategoryFromType(typeName string) string {
	switch typeName {
	case "not_found":
		return "not_found"
	case "validation_error", "bad_request":
		return "validation"
	case "unauthorized", "forbidden":
		return "auth"
	case "conflict":
		return "validation"
	default:
		return "internal"
	}
}

// wrapPermissionError preserves typed *AppError causes and wraps
// untyped errors with a stage-specific Internal so the wire body and
// the operator's logs both carry what actually failed rather than a
// generic message.
//
// stage is a short phrase describing what failed (e.g. "clearing
// existing grants"); it lands in the client-facing Message and the
// slog breadcrumb.
func wrapPermissionError(ctx context.Context, stage string, err error) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae
	}

	slog.ErrorContext(ctx, "entity permissions save: stage failed",
		slog.String("stage", stage),
		slog.Any("error", err),
	)

	return &apperror.AppError{
		Code: http.StatusInternalServerError,
		Type: "internal_error",
		Message: fmt.Sprintf(
			"Could not save permissions: %s failed. "+
				"The Chronicle server logs around this request timestamp "+
				"contain the underlying cause; share the timestamp with the "+
				"operator (or check the logs yourself if you ARE the operator) "+
				"to diagnose further.", stage),
		Internal: err,
	}
}

// respondPermissionsError emits the `{ error, message, category }`
// wire shape for errors from the entity permissions endpoints.
//
// Returns the error untouched if it isn't a typed *AppError so
// Echo's framework handler still gets a chance to render it.
func respondPermissionsError(c *echo.Context, err error) error {
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		return err
	}
	slog.WarnContext(c.Request().Context(), "permissions endpoint error response",
		slog.String("path", c.Request().URL.Path),
		slog.String("method", c.Request().Method),
		slog.Int("http_status", ae.Code),
		slog.String("type", ae.Type),
		slog.String("message", ae.Message),
		slog.Any("internal", ae.Internal),
	)
	return c.JSON(ae.Code, map[string]any{
		"error":    ae.Type,
		"message":  ae.Message,
		"category": errorCategoryFromType(ae.Type),
	})
}

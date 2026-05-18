// permissions_api.go — wire-contract helpers for the entity
// permissions endpoints.
//
// The "An unexpected error occurred. Please try again." text the
// operator reported in cordinator Issue #5 matches
// apperror.NewInternal's Message verbatim. Source review traced
// it to service.SetEntityPermissions, which wraps every
// sub-failure (typed AppErrors AND raw DB errors) as
// NewInternal — so the wire response and operator log both lose
// the actual stage and underlying cause.
//
// This file:
//
//   1. wrapPermissionError preserves typed *AppError sub-errors
//      so a NotFound from UpdateVisibility (e.g., row deleted by
//      a concurrent client) reaches the wire as a structured 404
//      instead of being smothered as 500.
//
//   2. respondPermissionsError emits the cross-repo wire-contract
//      shape { error, message, category } pinned in
//      cordinator/decisions/2026-05-17-error-catalog-wire-contract.md
//      Category is derived from AppError.Type so we don't have to
//      retrofit every constructor in apperror/.
//
// Both are scoped to the permissions endpoint per the C-PERMISSIONS-
// SAVE-FIX dispatch — Chronicle-wide error-shape harmonization is
// a separate effort.
package entities

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

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
// untyped errors with a stage-specific Internal so the wire body
// carries something more useful than the generic
// "An unexpected error occurred" while the underlying cause goes
// to the operator's logs.
//
// stage is a short human-readable phrase describing what failed
// (e.g. "clearing existing grants"); it lands in the client-
// facing Message AND the slog breadcrumb so the operator can
// connect the two.
func wrapPermissionError(ctx context.Context, stage string, err error) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		// Typed cause — preserve it. The handler's wire-shape
		// translator will pick the right HTTP status and
		// category from the existing AppError.
		return ae
	}

	// Untyped (raw DB/IO error). Log the full cause with the
	// stage label so the operator can root-cause in under a
	// minute. Echo's framework error handler also logs at the
	// boundary, but it doesn't know the stage — adding it here
	// is the cheap win.
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

// respondPermissionsError emits the wire-contract response shape
// for errors from the entity permissions endpoints. Same
// `{ error, message, category }` shape the foundry_vtt and
// calendar APIs use, so Foundry-side / future drift guards see a
// uniform contract. Per the C-PERMISSIONS-SAVE-FIX dispatch
// scope, applied to this endpoint only — wider Chronicle error-
// shape migration is a separate effort.
//
// Returns the error untouched if it isn't a typed *AppError so
// Echo's framework handler still gets a chance to render it
// (the framework handler is the last-resort path for code that
// hasn't been updated to the new shape).
func respondPermissionsError(c echo.Context, err error) error {
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

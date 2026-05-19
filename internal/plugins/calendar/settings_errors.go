// settings_errors.go — wire-contract error responder for the
// internal calendar settings PUT handlers (weather, cycles,
// festivals) added in C-CAL-WCF-UI.
//
// The public Foundry-facing calendar API has its own APIError type
// (see api_errors.go) — it could not be reused here because:
//
//   1. The settings PUTs already return apperror.AppError (the
//      Chronicle-wide domain error type) — recasting them as APIError
//      duplicates information.
//   2. The settings handlers are reached through Echo's session +
//      campaign middleware, not the per-campaign signed token path,
//      so the auth/config categories don't map cleanly.
//
// Instead we mirror the entities/permissions_api.go translator pattern
// (shipped in C-PERMISSIONS-SAVE-FIX): a thin helper that converts
// apperror.AppError → the cross-repo wire-contract JSON shape
// `{ error, message, category }` pinned in
// cordinator/decisions/2026-05-17-error-catalog-wire-contract.md.
//
// Untyped errors fall through to Echo's framework error handler so
// existing handlers that don't opt into the wire shape are unaffected.

package calendar

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// settingsErrorCategory maps apperror.AppError.Type into the
// five-bucket wire-contract enum. Mirrors
// entities/permissions_api.go's errorCategoryFromType to keep both
// translators in lockstep — if the wire contract gains a category
// later, the entity-permissions copy needs updating too.
func settingsErrorCategory(typeName string) string {
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

// respondSettingsError emits the wire-contract response shape for
// errors returned by the calendar settings PUT handlers.
//
// Untyped errors pass through untouched so Echo's framework handler
// renders them (it's the safety net for un-migrated handlers).
// Once the entire calendar handler set is migrated, callers can
// stop falling through and we can drop the legacy path.
func respondSettingsError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		return err
	}
	// Status code defaults to AppError's Code (already set per type
	// in apperror constructors); fall back to 500 if a constructor
	// left it zero.
	status := ae.Code
	if status == 0 {
		status = http.StatusInternalServerError
	}
	slog.WarnContext(c.Request().Context(), "calendar settings error response",
		slog.String("path", c.Request().URL.Path),
		slog.String("method", c.Request().Method),
		slog.Int("http_status", status),
		slog.String("type", ae.Type),
		slog.String("message", ae.Message),
		slog.Any("internal", ae.Internal),
	)
	return c.JSON(status, map[string]any{
		"error":    ae.Type,
		"message":  ae.Message,
		"category": settingsErrorCategory(ae.Type),
	})
}

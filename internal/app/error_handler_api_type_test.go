package app

// error_handler_api_type_test.go pins that the JSON error body's `error`
// field carries an AppError's machine-readable Type (e.g.
// "sync_api_disabled"), not http.StatusText(code) — asserted against the
// real errorHandler, not a fixture's own error handler, since a fixture-only
// assertion can pass while the product emits the bare status word.
//
// The field roles match what the Foundry module reads (api-client.mjs:
// `err.code = parsed.error`, `err.serverMessage = parsed.message`) and what
// the calendar blackout response uses (`{"error":"calendar_rebuilding", …}`).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// runAPIErrorHandler drives the real errorHandler over an /api path, which
// isAPIRequest classifies as JSON by prefix alone.
func runAPIErrorHandler(t *testing.T, err error) (int, map[string]string) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/c1/entities", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	(&App{}).errorHandler(c, err)

	var body map[string]string
	if uerr := json.Unmarshal(rec.Body.Bytes(), &body); uerr != nil {
		t.Fatalf("API error body is not JSON (%v): %s", uerr, rec.Body.String())
	}
	return rec.Code, body
}

func TestAPIError_CarriesTheMachineReadableType(t *testing.T) {
	// A hand-built AppError rather than a syncapi import: internal/app must not
	// depend on a plugin to state its own wire contract, and the plugin-isolation
	// guard would object.
	syncAPIDisabled := &apperror.AppError{
		Code:    http.StatusForbidden,
		Type:    "sync_api_disabled",
		Message: "the Sync API integration is switched off for this campaign",
	}

	code, body := runAPIErrorHandler(t, syncAPIDisabled)

	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", code)
	}
	if got := body["error"]; got != "sync_api_disabled" {
		t.Errorf(`body["error"] = %q, want "sync_api_disabled" — a client cannot `+
			`tell this 403 from a permission 403 without it`, got)
	}
	if got := body["message"]; got != syncAPIDisabled.Message {
		t.Errorf(`body["message"] = %q, want the prose %q`, got, syncAPIDisabled.Message)
	}
}

func TestAPIError_TypedErrorsAllCarryTheirType(t *testing.T) {
	// The constructors in internal/apperror each set a Type. None of them
	// reached a client before this contract existed.
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantType string
	}{
		{"not found", apperror.NewNotFound("entity"), http.StatusNotFound, "not_found"},
		{"forbidden", apperror.NewForbidden("nope"), http.StatusForbidden, "forbidden"},
		{"unauthorized", apperror.NewUnauthorized("auth required"), http.StatusUnauthorized, "unauthorized"},
		{"bad request", apperror.NewBadRequest("bad"), http.StatusBadRequest, "bad_request"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := runAPIErrorHandler(t, tc.err)
			if code != tc.wantCode {
				t.Fatalf("status = %d, want %d", code, tc.wantCode)
			}
			if got := body["error"]; got != tc.wantType {
				t.Errorf(`body["error"] = %q, want %q`, got, tc.wantType)
			}
		})
	}
}

func TestAPIError_WrappedTypeStillSurfaces(t *testing.T) {
	// Errors travel wrapped through service and handler layers. errorHandler
	// unwraps with errors.As, so the type must survive the journey — the
	// WebSocket path is the live example of a refusal that gets re-wrapped
	// before anyone reads it.
	inner := &apperror.AppError{
		Code:    http.StatusForbidden,
		Type:    "sync_api_disabled",
		Message: "switched off",
	}
	code, body := runAPIErrorHandler(t, fmt.Errorf("websocket auth: %w", inner))

	if code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 through a wrap", code)
	}
	if got := body["error"]; got != "sync_api_disabled" {
		t.Errorf(`wrapped body["error"] = %q, want "sync_api_disabled"`, got)
	}
}

func TestAPIError_UntypedFallsBackToTheStatusWord(t *testing.T) {
	// Echo's router 404s and panic recovery are not AppErrors and have no type
	// to offer. They must keep their previous shape rather than emit an empty
	// `error` field.
	code, body := runAPIErrorHandler(t, echo.NewHTTPError(http.StatusNotFound, "Not Found"))

	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if got := body["error"]; got != http.StatusText(http.StatusNotFound) {
		t.Errorf(`untyped body["error"] = %q, want %q`, got, http.StatusText(http.StatusNotFound))
	}
}

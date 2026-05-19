// settings_errors_test.go — pins the C-CAL-WCF-UI invariants on the
// internal calendar settings PUT handlers' error responder.
//
// Two load-bearing properties:
//
//   1. Validation errors from the service layer (e.g., SetCycles with
//      empty name) flow through respondSettingsError as
//      `{ error, message, category: "validation" }` — the inline error
//      region in each form renders the `message` and uses `category`
//      to pick severity styling. A regression to a generic 500 here
//      means the operator sees "An unexpected error occurred" instead
//      of "cycle 1: name is required".
//
//   2. Untyped errors pass through untouched so Echo's framework
//      handler still gets a chance to render them. Without this, a
//      mid-handler raw DB error would silently turn into a typed
//      wire-contract 500 with the framework's generic message —
//      indistinguishable from a typed Internal AppError.
//
// Mirrors the permissions_api_test.go pattern shipped in
// C-PERMISSIONS-SAVE-FIX; if the wire contract evolves both translators
// move together.

package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestRespondSettingsError_WireContractShape pins the body shape AND
// the AppError.Type → category mapping. Any drift between the calendar
// translator and entities/permissions_api.go's translator means
// callers see inconsistent shapes across endpoints — exactly what
// C-WIRE-INTEGRITY exists to prevent.
func TestRespondSettingsError_WireContractShape(t *testing.T) {
	cases := []struct {
		name             string
		err              error
		wantStatus       int
		wantError        string
		wantCategory     string
		wantMessageMatch string
	}{
		{
			name:             "BadRequest maps to validation",
			err:              apperror.NewBadRequest("invalid weather payload"),
			wantStatus:       http.StatusBadRequest,
			wantError:        "bad_request",
			wantCategory:     "validation",
			wantMessageMatch: "invalid weather payload",
		},
		{
			name:             "Validation maps to validation",
			err:              apperror.NewValidation("cycle 1: name is required"),
			wantStatus:       http.StatusUnprocessableEntity,
			wantError:        "validation_error",
			wantCategory:     "validation",
			wantMessageMatch: "cycle 1: name is required",
		},
		{
			name:             "NotFound maps to not_found",
			err:              apperror.NewNotFound("calendar not found"),
			wantStatus:       http.StatusNotFound,
			wantError:        "not_found",
			wantCategory:     "not_found",
			wantMessageMatch: "calendar not found",
		},
		{
			name:             "Internal maps to internal",
			err:              apperror.NewInternal(errors.New("db boom")),
			wantStatus:       http.StatusInternalServerError,
			wantError:        "internal_error",
			wantCategory:     "internal",
			wantMessageMatch: "unexpected error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/c1/calendars/cal1/cycles", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := respondSettingsError(c, tc.err); err != nil {
				t.Fatalf("respondSettingsError returned err: %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v (raw: %q)", err, rec.Body.String())
			}
			if body["error"] != tc.wantError {
				t.Errorf("error = %q, want %q", body["error"], tc.wantError)
			}
			if body["category"] != tc.wantCategory {
				t.Errorf("category = %q, want %q", body["category"], tc.wantCategory)
			}
			if body["message"] == "" {
				t.Errorf("message is empty; expected substring %q", tc.wantMessageMatch)
			}
		})
	}
}

// TestRespondSettingsError_PassesUntypedThrough pins the safety-net
// behavior: untyped errors must bubble up unchanged so Echo's framework
// handler renders them. Otherwise the translator would silently turn
// every raw DB / IO error into a typed wire-contract 500 with the
// generic message — losing both the actual cause AND the ability to
// distinguish typed-Internal from untyped errors.
func TestRespondSettingsError_PassesUntypedThrough(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	raw := errors.New("connection reset by peer")
	got := respondSettingsError(c, raw)
	if got != raw {
		t.Errorf("respondSettingsError mutated untyped error; got %v, want %v", got, raw)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("untyped error path should not have written status; got %d", rec.Code)
	}
}

// TestSetCycles_RejectsEmptyName pins the service-level validation
// added in C-CAL-WCF-UI. The acceptance criterion calls out this
// path explicitly: "submit empty name → expect category: validation".
// Service rejects → handler wraps → respondSettingsError emits
// category=validation. This test pins the first link.
func TestSetCycles_RejectsEmptyName(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := NewCalendarService(repo)
	err := svc.SetCycles(context.Background(), "cal-1", []CycleInput{{Name: ""}})
	if err == nil {
		t.Fatal("SetCycles accepted empty cycle name; the C-CAL-WCF-UI validation guard regressed")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error is not AppError: %T", err)
	}
	if ae.Type != "validation_error" {
		t.Errorf("Type = %q, want validation_error (so category maps to 'validation')", ae.Type)
	}
}

// TestSetFestivals_RejectsUnDatedFestival pins the festival-specific
// guard: a festival without month+day AND without after_month renders
// nowhere on the calendar. The service must reject; the inline error
// region in the form is the user-facing surface.
func TestSetFestivals_RejectsUnDatedFestival(t *testing.T) {
	repo := &mockCalendarRepo{}
	svc := NewCalendarService(repo)
	err := svc.SetFestivals(context.Background(), "cal-1", []FestivalInput{{Name: "Floating Day"}})
	if err == nil {
		t.Fatal("SetFestivals accepted a festival with no date anchor; the C-CAL-WCF-UI validation guard regressed")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error is not AppError: %T", err)
	}
	if ae.Type != "validation_error" {
		t.Errorf("Type = %q, want validation_error", ae.Type)
	}
}

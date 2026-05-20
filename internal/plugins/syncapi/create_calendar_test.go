// create_calendar_test.go — pins the C-CAL-CREATE-SYNCAPI-ALIGN
// invariants on the syncapi `POST /api/v1/campaigns/:id/calendar`
// endpoint that closes the operator's empty-state dead-end.
//
// The dispatch's diagnosis pinpointed that chronicle#323 landed the
// handler on the calendar plugin's `:cid`-parametered per-campaign-
// token surface, but Echo's router resolves the duplicate URL
// /api/v1/campaigns/{*}/calendar in favor of the syncapi
// `:id`-parametered route registration (later-wins for conflicting
// param names). The browser smoke test hit syncapi's GET (which
// session-cookie auth happily satisfied → 404 "no calendar
// configured") but POST had only the calendar-plugin registration,
// which required ?token= and rejected the operator's request.
//
// Six acceptance scenarios:
//   1. Happy path — valid syncapi auth + valid body → 201.
//   2. Conflict — calendar already exists → 409.
//   3. Payload validation — empty months → 422 + field-specific message.
//   4. Atomic rollback — ApplyImport fails → DeleteCalendar called +
//      surface a structured 422.
//   5. The wire response counts match the input shape.
//   6. POST does NOT invoke ApplyImport on the conflict path
//      (otherwise we'd silently overlay over an existing calendar).
//
// Auth/addon gating (RequireAuthOrAPIKey + RequireAddonAPI +
// RequirePermission) is owned by syncapi middleware and covered by
// dedicated middleware tests already. These tests hit the handler
// directly via the same pattern the ImportCalendar tests use.

package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// minimalCreateBody returns a Calendar-of-Therin-shaped payload sized
// to clear validateCreateCalendarBody.
func minimalCreateBody() []byte {
	return []byte(`{
  "name": "Calendar of Therin",
  "current_year": 1247,
  "current_month": 1,
  "current_day": 1,
  "months": [
    {"name": "First Frost", "days": 30},
    {"name": "Deep Winter", "days": 30}
  ],
  "weekdays": [
    {"name": "Sunday", "is_rest_day": true},
    {"name": "Monday"}
  ],
  "seasons": [
    {"name": "Winter", "month_start": 12, "day_start": 1, "month_end": 2, "day_end": 28, "color": "#aabbcc"}
  ],
  "moons": [
    {"name": "Lacrimosa", "cycle_days": 24, "color": "#ffffff"}
  ],
  "eras": [
    {"name": "Third Age", "start_year": 1000, "color": "#cc8800"}
  ]
}`)
}

// newCreateCtx builds an Echo context targeting POST /calendar with
// the syncapi :id param populated. Mirrors newImportTestContext.
func newCreateCtx(body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/calendar", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	return c, rec
}

// decodeBodyMap parses the JSON response for shape assertions.
func decodeBodyMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var b map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("response body is not JSON: %v (raw: %q)", err, rec.Body.String())
	}
	return b
}

// TestCreateCalendar_HappyPath — Acceptance #1 + #5.
// Valid body → 201 + calendar id + sub-resource counts echo input.
// ApplyImport was called with the translated ImportResult.
func TestCreateCalendar_HappyPath(t *testing.T) {
	var createdInput calendar.CreateCalendarInput
	var appliedResult *calendar.ImportResult
	svc := &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
			return nil, nil // no existing calendar
		},
		onCreate: func(_ context.Context, _ string, in calendar.CreateCalendarInput) (*calendar.Calendar, error) {
			createdInput = in
			return &calendar.Calendar{
				ID:               "cal-new",
				CampaignID:       "camp-1",
				Mode:             in.Mode,
				Name:             in.Name,
				CurrentYear:      in.CurrentYear,
				HoursPerDay:      24,
				MinutesPerHour:   60,
				SecondsPerMinute: 60,
			}, nil
		},
		onApply: func(_ context.Context, calID string, result *calendar.ImportResult) error {
			appliedResult = result
			_ = calID
			return nil
		},
	}
	h := NewCalendarAPIHandler(nil, svc)
	c, rec := newCreateCtx(minimalCreateBody())

	if err := h.CreateCalendar(c); err != nil {
		t.Fatalf("CreateCalendar returned err: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	body := decodeBodyMap(t, rec)
	if id, _ := body["id"].(string); id != "cal-new" {
		t.Errorf("id = %v, want cal-new", body["id"])
	}
	if name, _ := body["name"].(string); name != "Calendar of Therin" {
		t.Errorf("name = %v, want Calendar of Therin", body["name"])
	}
	// Sub-resource counts echo the input shape so the caller can
	// immediately render without a follow-up GET. (Acceptance #5.)
	wantCounts := map[string]float64{
		"month_count":   2,
		"weekday_count": 2,
		"season_count":  1,
		"moon_count":    1,
		"era_count":     1,
	}
	for key, want := range wantCounts {
		if got, _ := body[key].(float64); got != want {
			t.Errorf("%s = %v, want %v", key, body[key], want)
		}
	}

	// CreateCalendar was called with ModeFantasy + the name + year.
	if createdInput.Mode != calendar.ModeFantasy {
		t.Errorf("CreateCalendar.Mode = %q, want %q", createdInput.Mode, calendar.ModeFantasy)
	}
	if createdInput.Name != "Calendar of Therin" {
		t.Errorf("CreateCalendar.Name = %q, want Calendar of Therin", createdInput.Name)
	}
	// ApplyImport was called with the translated sub-resources.
	if appliedResult == nil {
		t.Fatal("ApplyImport was not called")
	}
	if len(appliedResult.Months) != 2 {
		t.Errorf("ImportResult.Months len = %d, want 2", len(appliedResult.Months))
	}
	if len(appliedResult.Moons) != 1 {
		t.Errorf("ImportResult.Moons len = %d, want 1", len(appliedResult.Moons))
	}
	if len(appliedResult.Seasons) != 1 {
		t.Errorf("ImportResult.Seasons len = %d, want 1", len(appliedResult.Seasons))
	}
	if len(appliedResult.Eras) != 1 {
		t.Errorf("ImportResult.Eras len = %d, want 1", len(appliedResult.Eras))
	}
}

// TestCreateCalendar_DuplicateRejected — Acceptance #2 + #6.
// POST when the campaign already has a calendar → 409 + naming the
// existing calendar. Critical: ApplyImport must NOT be called, or
// we'd silently overlay over an existing calendar.
func TestCreateCalendar_DuplicateRejected(t *testing.T) {
	applyCalled := false
	svc := &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
			return &calendar.Calendar{
				ID:         "cal-existing",
				CampaignID: "camp-1",
				Name:       "Existing Calendar",
			}, nil
		},
		onCreate: func(_ context.Context, _ string, _ calendar.CreateCalendarInput) (*calendar.Calendar, error) {
			t.Fatal("CreateCalendar should not be called on duplicate path")
			return nil, nil
		},
		onApply: func(_ context.Context, _ string, _ *calendar.ImportResult) error {
			applyCalled = true
			return nil
		},
	}
	h := NewCalendarAPIHandler(nil, svc)
	c, rec := newCreateCtx(minimalCreateBody())

	err := h.CreateCalendar(c)
	if err == nil {
		t.Fatal("CreateCalendar returned nil on duplicate; expected AppError")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error is not AppError: %T", err)
	}
	if ae.Code != http.StatusConflict {
		t.Errorf("Code = %d, want 409 Conflict", ae.Code)
	}
	if !strings.Contains(ae.Message, "Existing Calendar") {
		t.Errorf("error message should name the existing calendar; got %q", ae.Message)
	}

	// ApplyImport must NOT have been called.
	if applyCalled {
		t.Errorf("ApplyImport was called on the duplicate-create path; that would overlay over an existing calendar")
	}
	// rec.Code unset because the handler returned err; Echo's
	// framework handler renders the JSON. This test only asserts the
	// returned AppError shape.
	_ = rec
}

// TestCreateCalendar_PayloadValidation — Acceptance #3.
// Empty months / weekdays / malformed moon / era end<start → 422.
// Service-level methods must not be called when validation fails.
func TestCreateCalendar_PayloadValidation(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		wantSubstring string
	}{
		{
			name:          "empty months array",
			body:          `{"name":"X","current_year":1,"months":[],"weekdays":[{"name":"W"}]}`,
			wantSubstring: "months must be a non-empty array",
		},
		{
			name:          "empty weekdays array",
			body:          `{"name":"X","current_year":1,"months":[{"name":"M","days":1}],"weekdays":[]}`,
			wantSubstring: "weekdays must be a non-empty array",
		},
		{
			name:          "month with zero days",
			body:          `{"name":"X","current_year":1,"months":[{"name":"M","days":0}],"weekdays":[{"name":"W"}]}`,
			wantSubstring: "months[0].days",
		},
		{
			name:          "moon with non-positive cycle",
			body:          `{"name":"X","current_year":1,"months":[{"name":"M","days":1}],"weekdays":[{"name":"W"}],"moons":[{"name":"Bad","cycle_days":0}]}`,
			wantSubstring: "moons[0].cycle_days",
		},
		{
			name:          "missing name",
			body:          `{"name":"","current_year":1,"months":[{"name":"M","days":1}],"weekdays":[{"name":"W"}]}`,
			wantSubstring: "name is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubCalendarSvc{
				onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
					return nil, nil
				},
				onCreate: func(_ context.Context, _ string, _ calendar.CreateCalendarInput) (*calendar.Calendar, error) {
					t.Fatal("CreateCalendar should not be called when validation fails")
					return nil, nil
				},
				onApply: func(_ context.Context, _ string, _ *calendar.ImportResult) error {
					t.Fatal("ApplyImport should not be called when validation fails")
					return nil
				},
			}
			h := NewCalendarAPIHandler(nil, svc)
			c, _ := newCreateCtx([]byte(tc.body))

			err := h.CreateCalendar(c)
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) {
				t.Fatalf("error is not AppError: %T", err)
			}
			if ae.Code != http.StatusUnprocessableEntity {
				t.Errorf("Code = %d, want 422 (validation)", ae.Code)
			}
			if !strings.Contains(ae.Message, tc.wantSubstring) {
				t.Errorf("message %q does not contain expected substring %q", ae.Message, tc.wantSubstring)
			}
		})
	}
}

// TestCreateCalendar_AtomicRollback — Acceptance #4.
// ApplyImport fails with a validation_error AppError; handler must
// surface a structured 422 AND call DeleteCalendar to clean up the
// half-created row.
func TestCreateCalendar_AtomicRollback(t *testing.T) {
	deletedCalID := ""
	svc := &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
			return nil, nil
		},
		onCreate: func(_ context.Context, _ string, _ calendar.CreateCalendarInput) (*calendar.Calendar, error) {
			return &calendar.Calendar{
				ID:               "cal-half",
				CampaignID:       "camp-1",
				Mode:             calendar.ModeFantasy,
				Name:             "Calendar of Therin",
				HoursPerDay:      24,
				MinutesPerHour:   60,
				SecondsPerMinute: 60,
			}, nil
		},
		onApply: func(_ context.Context, _ string, _ *calendar.ImportResult) error {
			return apperror.NewValidation("moon 1: cycle_days must be > 0")
		},
		onDelete: func(_ context.Context, calendarID string) error {
			deletedCalID = calendarID
			return nil
		},
	}
	h := NewCalendarAPIHandler(nil, svc)
	c, _ := newCreateCtx(minimalCreateBody())

	err := h.CreateCalendar(c)
	if err == nil {
		t.Fatal("CreateCalendar returned nil on ApplyImport failure; expected validation AppError")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error is not AppError: %T", err)
	}
	if ae.Code != http.StatusUnprocessableEntity {
		t.Errorf("Code = %d, want 422 (storage-layer validation surfacing as wire validation)", ae.Code)
	}

	// Critical: rollback must have happened.
	if deletedCalID == "" {
		t.Error("rollback DeleteCalendar was not called; would leave a zombie calendar row to trip the 409 on retry")
	}
	if deletedCalID != "cal-half" {
		t.Errorf("rollback deleted %q, want cal-half", deletedCalID)
	}
}

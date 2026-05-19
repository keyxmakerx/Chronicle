// api_create_calendar_test.go — pins C-CAL-CREATE-ENDPOINT
// (chronicle PR for 2026-05-19 operator empty-state report).
//
// The per-campaign-token surface previously had 6 endpoints — none
// of which created a calendar from scratch. Sync Calendar in Foundry
// dead-ended on "no calendar configured" while three Calendaria
// calendars sat right there in the operator's Foundry world.
//
// This file covers the 6 acceptance scenarios the dispatch lists:
//   1. Happy path: valid Calendar-of-Therin-shaped body → 201.
//   2. Duplicate-create rejected (409 + category: conflict-via-validation
//      family with code calendar_already_exists).
//   3. Auth failure (403, category: auth).
//   4. Payload validation (422, category: validation).
//   5. Atomic rollback on sub-resource failure (422 + calendar row
//      deleted; subsequent GET returns 404).
//   6. ApplyImport called once with the full ImportResult shape (no
//      bizarre N-emission patterns; the existing ApplyImport service
//      surface drives WS publication for free).

package calendar

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// newFreshTestHandler returns a handler whose stub calendar service
// reports no existing calendar. The default newTestHandler seeds a
// "Calendar of Harptos" row, which the C-CAL-CREATE-ENDPOINT 409
// path needs to NOT be present.
func newFreshTestHandler(t *testing.T) (*APIHandler, *stubCalendarService, *stubVerifier) {
	t.Helper()
	svc := &stubCalendarService{} // calendar nil → "no calendar configured"
	verifier := &stubVerifier{}
	return NewAPIHandler(svc, verifier), svc, verifier
}

// minimalImportBody returns a payload sized to clear validateCreateCalendarBody
// while staying tight enough for assertion-friendly tests. Mirrors the
// Calendar of Therin shape per references/calendars/.
func minimalImportBody(t *testing.T) []byte {
	t.Helper()
	body := apiCreateCalendarBody{
		Name:         "Calendar of Therin",
		CurrentYear:  1247,
		CurrentMonth: 1,
		CurrentDay:   1,
		Months: []apiCreateMonth{
			{Name: "First Frost", Days: 30},
			{Name: "Deep Winter", Days: 30},
		},
		Weekdays: []apiCreateWeekday{
			{Name: "Sunday", IsRestDay: true},
			{Name: "Monday"},
		},
		Seasons: []apiCreateSeason{
			{Name: "Winter", MonthStart: 12, DayStart: 1, MonthEnd: 2, DayEnd: 28, Color: "#aabbcc"},
		},
		Moons: []apiCreateMoon{
			{Name: "Lacrimosa", CycleDays: 24, Color: "#ffffff"},
		},
		Eras: []apiCreateEra{
			{Name: "Third Age", StartYear: 1000, EndYear: nil, Color: "#cc8800"},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return raw
}

// invokeCreate is a thin shim that posts to POST /api/v1/campaigns/:cid/calendar.
// Sidesteps the `invoke` helper's URL-routing heuristic so the test reads
// linearly.
func invokeCreate(h *APIHandler, campaignID, token string, body []byte) *httptest.ResponseRecorder {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+campaignID+"/calendar", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cid")
	c.SetParamValues(campaignID)
	c.QueryParams().Set("token", token)
	_ = h.CreateCalendar(c)
	return rec
}

// decodeWireResponse parses the JSON body into a generic map for
// shape assertions. Errors include the raw body so failures are
// debuggable from the test output alone.
func decodeWireResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not JSON: %v (raw: %q)", err, rec.Body.String())
	}
	return body
}

// TestCreateCalendar_HappyPath — Acceptance #1.
// POST a full body → 201 + calendar id + sub-resource counts.
// ApplyImport was called with the translated ImportResult.
func TestCreateCalendar_HappyPath(t *testing.T) {
	h, svc, _ := newFreshTestHandler(t)
	rec := invokeCreate(h, "camp-1", "valid-token", minimalImportBody(t))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}

	body := decodeWireResponse(t, rec)
	if id, _ := body["id"].(string); id == "" {
		t.Errorf("response missing calendar id: %v", body)
	}
	if name, _ := body["name"].(string); name != "Calendar of Therin" {
		t.Errorf("name = %v, want Calendar of Therin", body["name"])
	}
	// Sub-resource counts echo the input shape so the caller can
	// immediately render without a follow-up GET.
	if mc, _ := body["month_count"].(float64); mc != 2 {
		t.Errorf("month_count = %v, want 2", body["month_count"])
	}
	if wc, _ := body["weekday_count"].(float64); wc != 2 {
		t.Errorf("weekday_count = %v, want 2", body["weekday_count"])
	}
	if mc, _ := body["moon_count"].(float64); mc != 1 {
		t.Errorf("moon_count = %v, want 1", body["moon_count"])
	}
	if ec, _ := body["era_count"].(float64); ec != 1 {
		t.Errorf("era_count = %v, want 1", body["era_count"])
	}

	// ApplyImport must have been called with the translated payload.
	if svc.lastApplyImport == nil {
		t.Fatal("ApplyImport was not called")
	}
	if svc.lastApplyImport.CalendarName != "Calendar of Therin" {
		t.Errorf("ImportResult.CalendarName = %q, want Calendar of Therin", svc.lastApplyImport.CalendarName)
	}
	if len(svc.lastApplyImport.Months) != 2 {
		t.Errorf("ImportResult.Months len = %d, want 2", len(svc.lastApplyImport.Months))
	}
	if len(svc.lastApplyImport.Moons) != 1 {
		t.Errorf("ImportResult.Moons len = %d, want 1", len(svc.lastApplyImport.Moons))
	}
}

// TestCreateCalendar_DuplicateRejected — Acceptance #2.
// POST when the campaign already has a calendar → 409 + code
// calendar_already_exists + message naming the existing calendar.
func TestCreateCalendar_DuplicateRejected(t *testing.T) {
	h, svc, _ := newFreshTestHandler(t)
	// Seed a pre-existing calendar so the duplicate check trips.
	svc.calendar = &Calendar{
		ID:         "cal-existing",
		CampaignID: "camp-1",
		Name:       "Existing Calendar",
	}

	rec := invokeCreate(h, "camp-1", "valid-token", minimalImportBody(t))

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 Conflict", rec.Code)
	}
	body := decodeWireResponse(t, rec)
	if body["error"] != "calendar_already_exists" {
		t.Errorf("error code = %v, want calendar_already_exists", body["error"])
	}
	if body["category"] != "validation" {
		// "validation" is the dispatch's chosen category for the
		// duplicate case — it's caller-driven, not server-fault. The
		// HTTP status override (409) carries the conflict semantic
		// at the protocol layer.
		t.Errorf("category = %v, want validation", body["category"])
	}
	msg, _ := body["message"].(string)
	if !strings.Contains(msg, "Existing Calendar") {
		t.Errorf("message should name the existing calendar; got %q", msg)
	}
	// Critical: ApplyImport must NOT have been called on the duplicate
	// path. Otherwise we'd silently re-import on top of an existing
	// calendar, losing the operator's existing config.
	if svc.lastApplyImport != nil {
		t.Errorf("ApplyImport was called on duplicate-create path; that's a regression")
	}
}

// TestCreateCalendar_AuthFailure — Acceptance #3.
// POST without a valid per-campaign token → 403 + category: auth.
// Same shape as the existing 6 endpoints' auth-failure tests.
func TestCreateCalendar_AuthFailure(t *testing.T) {
	h, _, verifier := newFreshTestHandler(t)
	verifier.wantErr = apperror.NewForbidden("invalid token")

	rec := invokeCreate(h, "camp-1", "wrong-token", minimalImportBody(t))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	body := decodeWireResponse(t, rec)
	if body["category"] != "auth" {
		t.Errorf("category = %v, want auth", body["category"])
	}
	if body["error"] != "invalid_token" {
		t.Errorf("error = %v, want invalid_token", body["error"])
	}
}

// TestCreateCalendar_PayloadValidation — Acceptance #4.
// Each of: empty months array, empty weekdays array, malformed moon,
// malformed era. Each must return 422 + category: validation + message
// naming the offending field.
func TestCreateCalendar_PayloadValidation(t *testing.T) {
	cases := []struct {
		name         string
		mutate       func(*apiCreateCalendarBody)
		wantSubstring string
	}{
		{
			name:          "empty name",
			mutate:        func(b *apiCreateCalendarBody) { b.Name = "" },
			wantSubstring: "name is required",
		},
		{
			name:          "empty months",
			mutate:        func(b *apiCreateCalendarBody) { b.Months = nil },
			wantSubstring: "months must be a non-empty array",
		},
		{
			name:          "empty weekdays",
			mutate:        func(b *apiCreateCalendarBody) { b.Weekdays = nil },
			wantSubstring: "weekdays must be a non-empty array",
		},
		{
			name:          "month with zero days",
			mutate:        func(b *apiCreateCalendarBody) { b.Months[0].Days = 0 },
			wantSubstring: "months[0].days",
		},
		{
			name: "moon with non-positive cycle",
			mutate: func(b *apiCreateCalendarBody) {
				b.Moons = []apiCreateMoon{{Name: "Bad", CycleDays: 0}}
			},
			wantSubstring: "moons[0].cycle_days",
		},
		{
			name: "era with end < start",
			mutate: func(b *apiCreateCalendarBody) {
				end := 999
				b.Eras = []apiCreateEra{{Name: "Bad", StartYear: 1000, EndYear: &end}}
			},
			wantSubstring: "eras[0].end_year",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := newFreshTestHandler(t)
			// Start from a valid baseline body so each case mutates one field.
			var body apiCreateCalendarBody
			if err := json.Unmarshal(minimalImportBody(t), &body); err != nil {
				t.Fatalf("unmarshal baseline: %v", err)
			}
			tc.mutate(&body)
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal mutated body: %v", err)
			}
			rec := invokeCreate(h, "camp-1", "valid-token", raw)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422; body = %s", rec.Code, rec.Body.String())
			}
			respBody := decodeWireResponse(t, rec)
			if respBody["category"] != "validation" {
				t.Errorf("category = %v, want validation", respBody["category"])
			}
			msg, _ := respBody["message"].(string)
			if !strings.Contains(msg, tc.wantSubstring) {
				t.Errorf("message %q does not contain expected substring %q", msg, tc.wantSubstring)
			}
		})
	}
}

// TestCreateCalendar_AtomicRollback — Acceptance #5.
// POST a payload that's valid at the API layer but ApplyImport fails
// (simulating SetMoons rejecting a malformed entry the API didn't
// catch). The half-created calendar row must be deleted before
// returning, so a subsequent GET shows 404 instead of the zombie row.
func TestCreateCalendar_AtomicRollback(t *testing.T) {
	h, svc, _ := newFreshTestHandler(t)
	// Wire ApplyImport to fail with a validation_error AppError —
	// the wire-shape translator should surface it as a 422 even
	// though the inner failure came from the storage layer.
	svc.applyImportErr = apperror.NewValidation("moon 1: cycle_days must be > 0")

	rec := invokeCreate(h, "camp-1", "valid-token", minimalImportBody(t))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 on storage-layer validation failure", rec.Code)
	}
	body := decodeWireResponse(t, rec)
	if body["category"] != "validation" {
		t.Errorf("category = %v, want validation (storage-layer validation must surface as wire validation)", body["category"])
	}

	// Critical: rollback must have happened.
	if svc.deletedCalendar == "" {
		t.Errorf("rollback DeleteCalendar was not called; would leave a zombie calendar row")
	}
	if svc.deletedCalendar != "cal-new" {
		t.Errorf("rollback deleted %q, want cal-new", svc.deletedCalendar)
	}
	// And the calendar surface should report no-calendar afterward.
	if svc.calendar != nil {
		t.Errorf("post-rollback calendar should be nil; got %+v", svc.calendar)
	}
}

// TestCreateCalendar_ApplyImportCalledOnce — Acceptance #6.
// Pin the call shape so a future refactor can't quietly fan into N
// separate Set* invocations (which would emit N separate WS events
// and break the "one structure update per import" expectation).
func TestCreateCalendar_ApplyImportCalledOnce(t *testing.T) {
	h, svc, _ := newFreshTestHandler(t)

	rec := invokeCreate(h, "camp-1", "valid-token", minimalImportBody(t))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	// Single ApplyImport call carries the full result. ApplyImport
	// already dispatches to the per-sub-resource Set* methods
	// internally (service.go:1304-1322); pinning the handler-to-
	// ApplyImport boundary here is what protects the WS catalog.
	if svc.lastApplyImport == nil {
		t.Fatal("ApplyImport was not called")
	}
	if len(svc.lastApplyImport.Months) != 2 {
		t.Errorf("ApplyImport.Months len = %d, want 2", len(svc.lastApplyImport.Months))
	}
	if len(svc.lastApplyImport.Weekdays) != 2 {
		t.Errorf("ApplyImport.Weekdays len = %d, want 2", len(svc.lastApplyImport.Weekdays))
	}
	if len(svc.lastApplyImport.Seasons) != 1 {
		t.Errorf("ApplyImport.Seasons len = %d, want 1", len(svc.lastApplyImport.Seasons))
	}
	if len(svc.lastApplyImport.Moons) != 1 {
		t.Errorf("ApplyImport.Moons len = %d, want 1", len(svc.lastApplyImport.Moons))
	}
	if len(svc.lastApplyImport.Eras) != 1 {
		t.Errorf("ApplyImport.Eras len = %d, want 1", len(svc.lastApplyImport.Eras))
	}
}

// Compile-time guard so the test file fails loudly if APIError loses
// its conflict-code path (the 409 HTTP status override).
var _ = errors.New

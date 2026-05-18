// api_handler_test.go — integration tests for the public
// Foundry-facing calendar API. Each test exercises one HTTP
// endpoint end-to-end through the APIHandler, with a stub service
// and stub token verifier. Pins:
//
//   1. Auth — every endpoint 403s with category=auth on missing
//      or invalid token (verifier returns error).
//   2. Structured 404 on GET /calendar when no calendar exists.
//   3. 1-indexed month/day round-trip: POST month=1/day=1 → GET it
//      back as month=1/day=1, not 0 or 2. The dispatch flags
//      this as the exact off-by-one that would silently break the
//      module.
//   4. `id` stability: POST an event, PUT-update it, GET it back —
//      the id never changes. The module persists this id in user
//      flags as calendarEventMappings; changes break the mapping.
//   5. Visibility enum: wire accepts exactly "everyone" and
//      "gm-only"; emits the same; translates to/from Chronicle's
//      internal "everyone"/"dm_only" without leaking either form
//      onto the other surface.
package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// --- stubs ---

// stubVerifier implements TokenVerifier. By default accepts any
// non-empty token; tests that need rejection set wantErr.
type stubVerifier struct {
	wantErr     error
	gotCampaign string
	gotToken    string
}

func (s *stubVerifier) VerifyManifestToken(_ context.Context, campaignID, token string) error {
	s.gotCampaign = campaignID
	s.gotToken = token
	if s.wantErr != nil {
		return s.wantErr
	}
	if token == "" {
		return errors.New("empty token")
	}
	return nil
}

// stubCalendarService implements just the CalendarService methods
// APIHandler uses. Keeps the calendar plugin's full mockCalendarRepo
// out of the test surface — we're testing the handler/wire layer,
// not the service or repo.
type stubCalendarService struct {
	CalendarService // embedded so we satisfy the full interface with method-not-implemented panics on anything we forget to stub

	calendar    *Calendar
	calendarErr error

	updateErr error
	lastUpdate *UpdateCalendarInput

	allEvents    []Event
	listEventsErr error

	createdEvent *Event
	createErr    error
	lastCreate   *CreateEventInput

	getEventResult map[string]*Event
	getEventErr    error

	updatedEvent *Event
	updateEventErr error
	lastUpdateEvent *UpdateEventInput
	deleteErr    error
	deletedEvent string
}

func (s *stubCalendarService) GetCalendar(_ context.Context, campaignID string) (*Calendar, error) {
	if s.calendarErr != nil {
		return nil, s.calendarErr
	}
	return s.calendar, nil
}

func (s *stubCalendarService) UpdateCalendar(_ context.Context, _ string, input UpdateCalendarInput) error {
	in := input
	s.lastUpdate = &in
	if s.updateErr != nil {
		return s.updateErr
	}
	if s.calendar != nil {
		s.calendar.CurrentYear = input.CurrentYear
		s.calendar.CurrentMonth = input.CurrentMonth
		s.calendar.CurrentDay = input.CurrentDay
		s.calendar.CurrentHour = input.CurrentHour
		s.calendar.CurrentMinute = input.CurrentMinute
	}
	return nil
}

func (s *stubCalendarService) ListAllEventsForCalendar(_ context.Context, _ string) ([]Event, error) {
	return s.allEvents, s.listEventsErr
}

func (s *stubCalendarService) CreateEvent(_ context.Context, calendarID string, input CreateEventInput) (*Event, error) {
	in := input
	s.lastCreate = &in
	if s.createErr != nil {
		return nil, s.createErr
	}
	desc := ""
	if input.Description != nil {
		desc = *input.Description
	}
	descCopy := desc
	evt := &Event{
		ID:          "evt-stable-id-001",
		CalendarID:  calendarID,
		Name:        input.Name,
		Description: &descCopy,
		Year:        input.Year,
		Month:       input.Month,
		Day:         input.Day,
		Visibility:  input.Visibility,
		AllDay:      input.AllDay,
		CreatedAt:   time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
	}
	s.createdEvent = evt
	if s.getEventResult == nil {
		s.getEventResult = map[string]*Event{}
	}
	s.getEventResult[evt.ID] = evt
	return evt, nil
}

func (s *stubCalendarService) GetEvent(_ context.Context, eventID string) (*Event, error) {
	if s.getEventErr != nil {
		return nil, s.getEventErr
	}
	if e, ok := s.getEventResult[eventID]; ok {
		return e, nil
	}
	return nil, nil
}

func (s *stubCalendarService) UpdateEvent(_ context.Context, eventID string, input UpdateEventInput) error {
	in := input
	s.lastUpdateEvent = &in
	if s.updateEventErr != nil {
		return s.updateEventErr
	}
	if e, ok := s.getEventResult[eventID]; ok {
		e.Name = input.Name
		e.Year = input.Year
		e.Month = input.Month
		e.Day = input.Day
		if input.Description != nil {
			d := *input.Description
			e.Description = &d
		}
		e.Visibility = input.Visibility
		e.UpdatedAt = time.Date(2026, 5, 18, 12, 30, 0, 0, time.UTC)
		s.updatedEvent = e
	}
	return nil
}

func (s *stubCalendarService) DeleteEvent(_ context.Context, eventID string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deletedEvent = eventID
	delete(s.getEventResult, eventID)
	return nil
}

// --- harness ---

func newTestHandler(t *testing.T) (*APIHandler, *stubCalendarService, *stubVerifier) {
	t.Helper()
	svc := &stubCalendarService{
		calendar: &Calendar{
			ID:            "cal-1",
			CampaignID:    "camp-1",
			Name:          "Calendar of Harptos",
			CurrentYear:   1492,
			CurrentMonth:  8,
			CurrentDay:    21,
			CurrentHour:   13,
			CurrentMinute: 30,
		},
	}
	verifier := &stubVerifier{}
	return NewAPIHandler(svc, verifier), svc, verifier
}

// invoke is a thin shim that dispatches an incoming request to
// the matching handler method, mirroring what RegisterPublicAPIRoutes
// does at runtime. Tests skip the actual router so they can target
// each handler in isolation.
func invoke(h *APIHandler, method, target, campaignID, eventID, token string, body []byte) *httptest.ResponseRecorder {
	e := echo.New()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("cid", "eventId")
	c.SetParamValues(campaignID, eventID)
	c.QueryParams().Set("token", token)

	switch {
	case method == http.MethodGet && !strings.Contains(target, "/events"):
		_ = h.GetCalendar(c)
	case method == http.MethodPut && strings.HasSuffix(target, "/date"):
		_ = h.PutDate(c)
	case method == http.MethodGet && strings.HasSuffix(target, "/events"):
		_ = h.ListEvents(c)
	case method == http.MethodPost && strings.HasSuffix(target, "/events"):
		_ = h.CreateEvent(c)
	case method == http.MethodPut && strings.Contains(target, "/events/"):
		_ = h.UpdateEvent(c)
	case method == http.MethodDelete && strings.Contains(target, "/events/"):
		_ = h.DeleteEvent(c)
	default:
		panic("invoke: unknown route " + method + " " + target)
	}
	return rec
}

// --- tests ---

// TestGetCalendar_HappyPath confirms the snapshot fields round-trip
// with the exact wire field names pinned in the decision.
func TestGetCalendar_HappyPath(t *testing.T) {
	h, _, _ := newTestHandler(t)
	rec := invoke(h, http.MethodGet, "/api/v1/campaigns/camp-1/calendar", "camp-1", "", "ok-token", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got apiCalendarSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	want := apiCalendarSnapshot{
		Name: "Calendar of Harptos",
		CurrentYear: 1492, CurrentMonth: 8, CurrentDay: 21,
		CurrentHour: 13, CurrentMinute: 30,
	}
	if got != want {
		t.Errorf("snapshot mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// TestGetCalendar_StructuredNotFound asserts that when no calendar
// is configured, the response is 404 with the wire-contract
// { error, message, category: "not_found" } shape — not the
// framework's generic 404. The module's sync-dashboard.mjs catches
// this silently; a generic 404 would break that path.
func TestGetCalendar_StructuredNotFound(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	svc.calendar = nil
	rec := invoke(h, http.MethodGet, "/api/v1/campaigns/camp-1/calendar", "camp-1", "", "ok-token", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body did not unmarshal as map[string]string: %v\nbody: %s", err, rec.Body.String())
	}
	if body["category"] != string(APIErrCategoryNotFound) {
		t.Errorf("category = %q, want %q", body["category"], APIErrCategoryNotFound)
	}
	if body["error"] != "calendar_not_configured" {
		t.Errorf("error = %q, want %q", body["error"], "calendar_not_configured")
	}
	if body["message"] == "" {
		t.Errorf("message should be non-empty four-clause text")
	}
}

// TestAllEndpoints_AuthFailure exercises each of the 6 endpoints
// with an empty token, asserting category=auth and code=invalid_token
// on every one. This is the C-WIRE-INTEGRITY analog for the
// calendar surface.
func TestAllEndpoints_AuthFailure(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"GetCalendar", http.MethodGet, "/api/v1/campaigns/camp-1/calendar", nil},
		{"PutDate", http.MethodPut, "/api/v1/campaigns/camp-1/calendar/date", []byte(`{"year":1,"month":1,"day":1,"hour":0,"minute":0}`)},
		{"ListEvents", http.MethodGet, "/api/v1/campaigns/camp-1/calendar/events", nil},
		{"CreateEvent", http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", []byte(`{"name":"x","year":1,"month":1,"day":1,"description":"","visibility":"everyone"}`)},
		{"UpdateEvent", http.MethodPut, "/api/v1/campaigns/camp-1/calendar/events/evt-1", []byte(`{"name":"x","year":1,"month":1,"day":1,"description":"","visibility":"everyone"}`)},
		{"DeleteEvent", http.MethodDelete, "/api/v1/campaigns/camp-1/calendar/events/evt-1", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := newTestHandler(t)
			eventID := ""
			if strings.Contains(tc.path, "/events/") {
				eventID = "evt-1"
			}
			rec := invoke(h, tc.method, tc.path, "camp-1", eventID, "", tc.body)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}
			if body["category"] != string(APIErrCategoryAuth) {
				t.Errorf("category = %q, want %q", body["category"], APIErrCategoryAuth)
			}
			if body["error"] != "invalid_token" {
				t.Errorf("error = %q, want %q", body["error"], "invalid_token")
			}
		})
	}
}

// TestPutDate_RoundTrip asserts the wire body shape echoes back
// unchanged and the calendar's current date reflects the update.
func TestPutDate_RoundTrip(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	body := []byte(`{"year":1493,"month":3,"day":15,"hour":9,"minute":45}`)
	rec := invoke(h, http.MethodPut, "/api/v1/campaigns/camp-1/calendar/date", "camp-1", "", "ok-token", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var echoed apiDate
	if err := json.Unmarshal(rec.Body.Bytes(), &echoed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := apiDate{Year: 1493, Month: 3, Day: 15, Hour: 9, Minute: 45}
	if echoed != want {
		t.Errorf("echo mismatch: got %+v, want %+v", echoed, want)
	}
	if svc.calendar.CurrentMonth != 3 || svc.calendar.CurrentDay != 15 {
		t.Errorf("service was not updated: cur month/day = %d/%d, want 3/15",
			svc.calendar.CurrentMonth, svc.calendar.CurrentDay)
	}
}

// TestMonthDayOneIndexed pins month=1/day=1 round-tripping
// through POST → GET as 1/1 — the exact off-by-one trap the
// dispatch calls out. Without 1-indexed semantics, the module
// would silently create misdated events.
func TestMonthDayOneIndexed(t *testing.T) {
	h, svc, _ := newTestHandler(t)

	// POST month=1 day=1.
	body := []byte(`{"name":"New Year","year":1493,"month":1,"day":1,"description":"","visibility":"everyone"}`)
	rec := invoke(h, http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var created apiEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created: %v", err)
	}
	if created.Month != 1 || created.Day != 1 {
		t.Errorf("created month/day = %d/%d, want 1/1 — month/day must be 1-indexed",
			created.Month, created.Day)
	}

	// GET it back via ListEvents.
	svc.allEvents = []Event{*svc.createdEvent}
	rec = invoke(h, http.MethodGet, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	var list []apiEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0].Month != 1 || list[0].Day != 1 {
		t.Errorf("listed month/day = %d/%d, want 1/1 — wire 1-indexed semantics broken on read path",
			list[0].Month, list[0].Day)
	}
}

// TestPostRejectsZeroIndexed pins the inverse: a 0 month or 0 day
// is rejected with category=validation so a buggy module never
// silently writes off-by-one data. (The decision says 1-indexed;
// if anything reaches Chronicle as 0, fail loudly.)
func TestPostRejectsZeroIndexed(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
	}{
		{"month 0", `{"name":"x","year":1,"month":0,"day":1,"description":"","visibility":"everyone"}`},
		{"day 0", `{"name":"x","year":1,"month":1,"day":0,"description":"","visibility":"everyone"}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, _, _ := newTestHandler(t)
			rec := invoke(h, http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", []byte(c.body))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
			}
			var body map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body["category"] != string(APIErrCategoryValidation) {
				t.Errorf("category = %q, want validation", body["category"])
			}
		})
	}
}

// TestEventIDStableAcrossUpdate pins the load-bearing contract from
// the decision: "Chronicle MUST return the same id for the lifetime
// of the event" because the Foundry module persists it in user
// flags (calendarEventMappings) for update/delete tracking. A
// changed id breaks the mapping silently.
func TestEventIDStableAcrossUpdate(t *testing.T) {
	h, svc, _ := newTestHandler(t)

	createBody := []byte(`{"name":"Harvest Festival","year":1492,"month":8,"day":21,"description":"","visibility":"everyone"}`)
	rec := invoke(h, http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201", rec.Code)
	}
	var created apiEvent
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("created.ID must not be empty")
	}

	// PUT with the same id — the wire shape requires the id come
	// back unchanged.
	updateBody := []byte(`{"name":"Harvest Festival (renamed)","year":1492,"month":8,"day":21,"description":"updated","visibility":"gm-only"}`)
	rec = invoke(h, http.MethodPut, "/api/v1/campaigns/camp-1/calendar/events/"+created.ID, "camp-1", created.ID, "ok-token", updateBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var updated apiEvent
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.ID != created.ID {
		t.Errorf("id changed across update: created=%q updated=%q — breaks the module's mapping",
			created.ID, updated.ID)
	}
	if updated.Name != "Harvest Festival (renamed)" {
		t.Errorf("Name was not updated: got %q", updated.Name)
	}
	if updated.Description != "updated" {
		t.Errorf("Description was not updated: got %q", updated.Description)
	}
	if svc.lastUpdateEvent == nil {
		t.Fatal("service UpdateEvent was not invoked")
	}
}

// TestVisibilityEnumRoundTrip pins the wire-vs-storage translation
// at both surfaces: wire "gm-only" → stored "dm_only" → wire
// "gm-only" again on read. The dispatch flags this as one of the
// load-bearing invariants because the module emits exact strings.
func TestVisibilityEnumRoundTrip(t *testing.T) {
	cases := []struct {
		wire    string
		storage string
	}{
		{"everyone", "everyone"},
		{"gm-only", "dm_only"},
	}
	for _, tc := range cases {
		t.Run(tc.wire, func(t *testing.T) {
			h, svc, _ := newTestHandler(t)
			body := []byte(`{"name":"x","year":1,"month":1,"day":1,"description":"","visibility":"` + tc.wire + `"}`)
			rec := invoke(h, http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
			}
			if svc.lastCreate == nil {
				t.Fatal("CreateEvent was not invoked")
			}
			if svc.lastCreate.Visibility != tc.storage {
				t.Errorf("storage visibility = %q, want %q (wire %q must translate to storage %q)",
					svc.lastCreate.Visibility, tc.storage, tc.wire, tc.storage)
			}
			var emitted apiEvent
			_ = json.Unmarshal(rec.Body.Bytes(), &emitted)
			if emitted.Visibility != tc.wire {
				t.Errorf("response visibility = %q, want %q (storage %q must translate back to wire %q)",
					emitted.Visibility, tc.wire, tc.storage, tc.wire)
			}
		})
	}
}

// TestVisibilityRejectsUnknown pins the negative case: any value
// the module doesn't emit should be rejected with category=validation.
func TestVisibilityRejectsUnknown(t *testing.T) {
	h, _, _ := newTestHandler(t)
	body := []byte(`{"name":"x","year":1,"month":1,"day":1,"description":"","visibility":"public"}`)
	rec := invoke(h, http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDeleteEvent_HappyAnd404 covers the DELETE path's two cases:
// a known id returns 204; an unknown id returns the structured 404
// so the module can distinguish "already gone" from "auth failed".
func TestDeleteEvent_HappyAnd404(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		h, svc, _ := newTestHandler(t)
		// Seed an event.
		createBody := []byte(`{"name":"x","year":1,"month":1,"day":1,"description":"","visibility":"everyone"}`)
		invoke(h, http.MethodPost, "/api/v1/campaigns/camp-1/calendar/events", "camp-1", "", "ok-token", createBody)
		id := svc.createdEvent.ID

		rec := invoke(h, http.MethodDelete, "/api/v1/campaigns/camp-1/calendar/events/"+id, "camp-1", id, "ok-token", nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
		}
		if svc.deletedEvent != id {
			t.Errorf("deletedEvent = %q, want %q", svc.deletedEvent, id)
		}
	})
	t.Run("unknown id 404", func(t *testing.T) {
		h, _, _ := newTestHandler(t)
		rec := invoke(h, http.MethodDelete, "/api/v1/campaigns/camp-1/calendar/events/does-not-exist", "camp-1", "does-not-exist", "ok-token", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
		}
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["category"] != string(APIErrCategoryNotFound) {
			t.Errorf("category = %q, want not_found", body["category"])
		}
		if body["error"] != "event_not_found" {
			t.Errorf("error = %q, want event_not_found", body["error"])
		}
	})
}

// TestUpdateEventRejectsCrossCalendarID pins the per-campaign
// isolation: a caller with a valid token for campaign A cannot
// mutate an event that belongs to campaign B's calendar via PUT
// (which would otherwise be possible since the wire URL embeds
// only the {cid} we're authenticated against + the global event
// id).
func TestUpdateEventRejectsCrossCalendarID(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	// Plant an event whose CalendarID is NOT the calendar this
	// campaign owns ("cal-1"). Authorised caller is camp-1 → cal-1.
	otherEvt := &Event{
		ID: "evt-other", CalendarID: "cal-OTHER",
		Name: "x", Year: 1, Month: 1, Day: 1,
		Visibility: "everyone",
		UpdatedAt:  time.Now(),
	}
	svc.getEventResult = map[string]*Event{"evt-other": otherEvt}

	updateBody := []byte(`{"name":"hijack","year":1,"month":1,"day":1,"description":"","visibility":"everyone"}`)
	rec := invoke(h, http.MethodPut, "/api/v1/campaigns/camp-1/calendar/events/evt-other", "camp-1", "evt-other", "ok-token", updateBody)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 event_conflict; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "event_conflict" {
		t.Errorf("error = %q, want event_conflict", body["error"])
	}
}

// TestVisibilityTranslation_PureFn unit-tests the translation in
// isolation. Cheap, deterministic, and makes a future rename of
// either enum loud.
func TestVisibilityTranslation_PureFn(t *testing.T) {
	storage, err := wireToStorageVisibility("everyone")
	if err != nil || storage != "everyone" {
		t.Errorf(`wireToStorageVisibility("everyone") = %q, %v`, storage, err)
	}
	storage, err = wireToStorageVisibility("gm-only")
	if err != nil || storage != "dm_only" {
		t.Errorf(`wireToStorageVisibility("gm-only") = %q, %v`, storage, err)
	}
	storage, err = wireToStorageVisibility("")
	if err != nil || storage != "everyone" {
		t.Errorf(`wireToStorageVisibility("") = %q, %v (empty should default to everyone)`, storage, err)
	}
	if _, err := wireToStorageVisibility("public"); err == nil {
		t.Error(`wireToStorageVisibility("public") should error — only "everyone" and "gm-only" are valid wire values`)
	}

	if got := storageToWireVisibility("everyone"); got != "everyone" {
		t.Errorf(`storageToWireVisibility("everyone") = %q, want "everyone"`, got)
	}
	if got := storageToWireVisibility("dm_only"); got != "gm-only" {
		t.Errorf(`storageToWireVisibility("dm_only") = %q, want "gm-only"`, got)
	}
}

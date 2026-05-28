// audit_log_test.go covers V2 Wave 0 PR 4 (C-CAL-V2-AUDIT-LOG-INTEGRATION)
// handler-layer audit emission. Each test exercises a single Handler
// mutator end-to-end through the audit-emit hook with a captured
// mock auditor + a stub CalendarService.

package calendar

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Test infra: capturing audit recorder ---

// mockAuditService captures Log calls so tests can assert action +
// entity fields. Safe for concurrent test execution.
type mockAuditService struct {
	mu      sync.Mutex
	entries []audit.AuditEntry
}

func (m *mockAuditService) Log(_ context.Context, entry *audit.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, *entry)
	return nil
}

func (m *mockAuditService) GetCampaignActivity(_ context.Context, _ string, _ int) ([]audit.AuditEntry, int, error) {
	return nil, 0, nil
}

func (m *mockAuditService) GetEntityHistory(_ context.Context, _ string) ([]audit.AuditEntry, error) {
	return nil, nil
}

func (m *mockAuditService) GetCampaignStats(_ context.Context, _ string) (*audit.CampaignStats, error) {
	return nil, nil
}

// has returns true if any captured entry has the given action and the
// given resource id (matching either EntityID or any Details key).
func (m *mockAuditService) has(action, entityID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Action == action && e.EntityID == entityID {
			return true
		}
	}
	return false
}

// findByAction returns the first captured entry with the given action,
// or zero-value if none. Useful when EntityID is generated.
func (m *mockAuditService) findByAction(action string) audit.AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.Action == action {
			return e
		}
	}
	return audit.AuditEntry{}
}

// --- Test infra: stub CalendarService (embeds the interface to inherit
// any methods we don't override; nil-method calls panic which is what
// we want for unexpected paths). ---

type stubCalSvc struct {
	CalendarService

	cal *Calendar

	createCalErr error
	createdCal   *Calendar

	setMonthsErr error
	setWeekdaysErr error
	setMoonsErr error
	setSeasonsErr error
	setErasErr error
	setEvtCatsErr error

	weatherErr error
	weatherZonesErr error
	cyclesErr error
	festivalsErr error

	createdEvt *Event
	createEvtErr error
	updateEvtErr error
	deleteEvtErr error
	visErr error
	advanceDateErr error
	advanceTimeErr error
	deleteCalErr error
	updateCalErr error

	evt *Event
}

func (s *stubCalSvc) GetCalendarByID(_ context.Context, id string) (*Calendar, error) {
	if s.cal != nil && s.cal.ID == id {
		return s.cal, nil
	}
	return nil, nil
}
func (s *stubCalSvc) CreateCalendar(_ context.Context, campaignID string, _ CreateCalendarInput) (*Calendar, error) {
	if s.createCalErr != nil {
		return nil, s.createCalErr
	}
	if s.createdCal != nil {
		return s.createdCal, nil
	}
	return &Calendar{ID: "cal-1", CampaignID: campaignID, Name: "New"}, nil
}
func (s *stubCalSvc) UpdateCalendar(_ context.Context, _ string, _ UpdateCalendarInput) error {
	return s.updateCalErr
}
func (s *stubCalSvc) DeleteCalendar(_ context.Context, _ string) error { return s.deleteCalErr }
func (s *stubCalSvc) SetMonths(_ context.Context, _ string, _ []MonthInput) error {
	return s.setMonthsErr
}
func (s *stubCalSvc) SetWeekdays(_ context.Context, _ string, _ []WeekdayInput) error {
	return s.setWeekdaysErr
}
func (s *stubCalSvc) SetMoons(_ context.Context, _ string, _ []MoonInput) error {
	return s.setMoonsErr
}
func (s *stubCalSvc) SetSeasons(_ context.Context, _ string, _ []Season) error {
	return s.setSeasonsErr
}
func (s *stubCalSvc) SetEras(_ context.Context, _ string, _ []EraInput) error { return s.setErasErr }
func (s *stubCalSvc) SetEventCategories(_ context.Context, _ string, _ []EventCategoryInput) error {
	return s.setEvtCatsErr
}
func (s *stubCalSvc) SetWeather(_ context.Context, _ string, _ WeatherInput) error {
	return s.weatherErr
}
func (s *stubCalSvc) GetWeatherZones(_ context.Context, _ string) (*WeatherZonesState, error) {
	return &WeatherZonesState{Zones: []WeatherZone{}}, nil
}
func (s *stubCalSvc) SetWeatherZones(_ context.Context, _ string, _ WeatherZonesState) error {
	return s.weatherZonesErr
}
func (s *stubCalSvc) SetCycles(_ context.Context, _ string, _ []CycleInput) error {
	return s.cyclesErr
}
func (s *stubCalSvc) SetFestivals(_ context.Context, _ string, _ []FestivalInput) error {
	return s.festivalsErr
}
func (s *stubCalSvc) CreateEvent(_ context.Context, _ string, _ CreateEventInput) (*Event, error) {
	if s.createEvtErr != nil {
		return nil, s.createEvtErr
	}
	if s.createdEvt != nil {
		return s.createdEvt, nil
	}
	return &Event{ID: "evt-1", Name: "Test Event"}, nil
}
func (s *stubCalSvc) UpdateEvent(_ context.Context, _ string, _ UpdateEventInput) error {
	return s.updateEvtErr
}
func (s *stubCalSvc) DeleteEvent(_ context.Context, _ string) error { return s.deleteEvtErr }
func (s *stubCalSvc) UpdateEventVisibility(_ context.Context, _ string, _ UpdateEventVisibilityInput) error {
	return s.visErr
}
func (s *stubCalSvc) GetEvent(_ context.Context, id string) (*Event, error) {
	if s.evt != nil && s.evt.ID == id {
		return s.evt, nil
	}
	return nil, nil
}
func (s *stubCalSvc) AdvanceDate(_ context.Context, _ string, _ int) error { return s.advanceDateErr }
func (s *stubCalSvc) AdvanceTime(_ context.Context, _ string, _, _ int) error {
	return s.advanceTimeErr
}
func (s *stubCalSvc) GetCalendar(_ context.Context, _ string) (*Calendar, error) {
	return s.cal, nil
}

// --- Test infra: HTTP request helper ---

// newReqWithCC wires an Echo Context with the campaign-context key +
// auth user-id key set so the handler-side audit hook can extract
// both. The dispatch's "actor capture" + "campaign id" sections both
// flow from these two context entries.
func newReqWithCC(method, path string, body []byte, calID, campaignID, userID string) (echo.Context, *httptest.ResponseRecorder, *campaigns.CampaignContext) {
	e := echo.New()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId")
	c.SetParamValues(campaignID, calID)
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: campaignID, Name: "Camp"},
		MemberRole: campaigns.RoleOwner,
	}
	c.Set("campaign_context", cc)
	c.Set("auth_user_id", userID)
	return c, rec, cc
}

// makeHandler returns a Handler wired with the supplied stub service
// + a fresh audit recorder.
func makeHandler(svc CalendarService) (*Handler, *mockAuditService) {
	h := NewHandler(svc)
	rec := &mockAuditService{}
	h.SetAuditService(rec)
	return h, rec
}

// --- Tests ---

// TestUpdateMonthsAPI_EmitsMonthsSet — bulk-set emits the months_set
// action with count payload.
func TestUpdateMonthsAPI_EmitsMonthsSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Test Cal"}}
	h, rec := makeHandler(svc)

	body := []byte(`[{"name":"Jan","days":31,"sort_order":1},{"name":"Feb","days":28,"sort_order":2}]`)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", body, "cal-1", "camp-1", "user-1")
	if err := h.UpdateMonthsAPI(c); err != nil {
		t.Fatalf("UpdateMonthsAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarMonthsSet, "cal-1") {
		t.Errorf("expected %s for cal-1; got %+v", audit.ActionCalendarMonthsSet, rec.entries)
	}
	got := rec.findByAction(audit.ActionCalendarMonthsSet)
	if got.UserID != "user-1" {
		t.Errorf("UserID=%q; want user-1", got.UserID)
	}
	if got.Details["count"] != 2 {
		t.Errorf("Details[count]=%v; want 2", got.Details["count"])
	}
}

// TestUpdateWeekdaysAPI_EmitsWeekdaysSet
func TestUpdateWeekdaysAPI_EmitsWeekdaysSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[{"name":"Mon","sort_order":1}]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateWeekdaysAPI(c); err != nil {
		t.Fatalf("UpdateWeekdaysAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarWeekdaysSet, "cal-1") {
		t.Errorf("expected %s; entries=%+v", audit.ActionCalendarWeekdaysSet, rec.entries)
	}
}

// TestUpdateMoonsAPI_EmitsMoonsSet
func TestUpdateMoonsAPI_EmitsMoonsSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[{"name":"Selune","cycle_days":28}]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateMoonsAPI(c); err != nil {
		t.Fatalf("UpdateMoonsAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarMoonsSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarMoonsSet)
	}
}

// TestUpdateSeasonsAPI_EmitsSeasonsSet
func TestUpdateSeasonsAPI_EmitsSeasonsSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateSeasonsAPI(c); err != nil {
		t.Fatalf("UpdateSeasonsAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarSeasonsSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarSeasonsSet)
	}
}

// TestUpdateErasAPI_EmitsErasSet
func TestUpdateErasAPI_EmitsErasSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateErasAPI(c); err != nil {
		t.Fatalf("UpdateErasAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarErasSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarErasSet)
	}
}

// TestUpdateEventCategoriesAPI_EmitsCategoriesSet
func TestUpdateEventCategoriesAPI_EmitsCategoriesSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateEventCategoriesAPI(c); err != nil {
		t.Fatalf("UpdateEventCategoriesAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarCategoriesSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarCategoriesSet)
	}
}

// TestUpdateWeatherAPI_EmitsWeatherSet
func TestUpdateWeatherAPI_EmitsWeatherSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`{"preset_id":"rain"}`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateWeatherAPI(c); err != nil {
		t.Fatalf("UpdateWeatherAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarWeatherSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarWeatherSet)
	}
}

// TestUpdateWeatherZonesAPI_EmitsWeatherZonesSet
func TestUpdateWeatherZonesAPI_EmitsWeatherZonesSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`{"active_zone":"temperate","zones":[{"zone_id":"temperate","name":"Temperate"}]}`),
		"cal-1", "camp-1", "u-1")
	if err := h.UpdateWeatherZonesAPI(c); err != nil {
		t.Fatalf("UpdateWeatherZonesAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarWeatherZonesSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarWeatherZonesSet)
	}
	got := rec.findByAction(audit.ActionCalendarWeatherZonesSet)
	if got.Details["active_zone"] != "temperate" {
		t.Errorf("Details[active_zone]=%v; want temperate", got.Details["active_zone"])
	}
}

// TestUpdateCyclesAPI_EmitsCyclesSet
func TestUpdateCyclesAPI_EmitsCyclesSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateCyclesAPI(c); err != nil {
		t.Fatalf("UpdateCyclesAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarCyclesSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarCyclesSet)
	}
}

// TestUpdateFestivalsAPI_EmitsFestivalsSet
func TestUpdateFestivalsAPI_EmitsFestivalsSet(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateFestivalsAPI(c); err != nil {
		t.Fatalf("UpdateFestivalsAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarFestivalsSet, "cal-1") {
		t.Errorf("expected %s", audit.ActionCalendarFestivalsSet)
	}
}

// TestCreateEventAPI_EmitsEventCreated
func TestCreateEventAPI_EmitsEventCreated(t *testing.T) {
	svc := &stubCalSvc{
		cal:        &Calendar{ID: "cal-1", CampaignID: "camp-1"},
		createdEvt: &Event{ID: "evt-7", Name: "Festival of Light", CalendarID: "cal-1", Year: 1234, Month: 3, Day: 15},
	}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPost, "/api", []byte(`{"name":"Festival of Light","year":1234,"month":3,"day":15,"visibility":"everyone"}`),
		"cal-1", "camp-1", "u-1")
	if err := h.CreateEventAPI(c); err != nil {
		t.Fatalf("CreateEventAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarEventCreated, "evt-7") {
		t.Errorf("expected %s for evt-7; entries=%+v", audit.ActionCalendarEventCreated, rec.entries)
	}
}

// TestDeleteEventAPI_EmitsEventDeleted
func TestDeleteEventAPI_EmitsEventDeleted(t *testing.T) {
	svc := &stubCalSvc{
		cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"},
		evt: &Event{ID: "evt-5", Name: "Old Event", CalendarID: "cal-1"},
	}
	h, rec := makeHandler(svc)
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api", nil)
	recHTTP := httptest.NewRecorder()
	c := e.NewContext(req, recHTTP)
	c.SetParamNames("id", "eid")
	c.SetParamValues("camp-1", "evt-5")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "u-1")
	if err := h.DeleteEventAPI(c); err != nil {
		t.Fatalf("DeleteEventAPI: %v", err)
	}
	if !rec.has(audit.ActionCalendarEventDeleted, "evt-5") {
		t.Errorf("expected %s for evt-5; entries=%+v", audit.ActionCalendarEventDeleted, rec.entries)
	}
}

// TestUpdateEventVisibilityAPI_EmitsVisibilityChanged — captures
// old + new visibility in payload.
func TestUpdateEventVisibilityAPI_EmitsVisibilityChanged(t *testing.T) {
	svc := &stubCalSvc{
		cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"},
		evt: &Event{ID: "evt-1", Name: "Public Event", CalendarID: "cal-1", Visibility: "everyone"},
	}
	h, rec := makeHandler(svc)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api", strings.NewReader(`{"visibility":"dm_only"}`))
	req.Header.Set("Content-Type", "application/json")
	recHTTP := httptest.NewRecorder()
	c := e.NewContext(req, recHTTP)
	c.SetParamNames("id", "eid")
	c.SetParamValues("camp-1", "evt-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "u-1")
	if err := h.UpdateEventVisibilityAPI(c); err != nil {
		t.Fatalf("UpdateEventVisibilityAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionCalendarEventVisibilityChanged)
	if got.EntityID != "evt-1" {
		t.Errorf("expected evt-1; got %q", got.EntityID)
	}
	if got.Details["old_visibility"] != "everyone" || got.Details["new_visibility"] != "dm_only" {
		t.Errorf("expected old/new visibility in details; got %+v", got.Details)
	}
}

// TestAdvanceDateAPI_EmitsDateAdvanced — captures the days delta.
func TestAdvanceDateAPI_EmitsDateAdvanced(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPost, "/api", []byte(`{"days":7}`), "cal-1", "camp-1", "u-1")
	if err := h.AdvanceDateAPI(c); err != nil {
		t.Fatalf("AdvanceDateAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionCalendarDateAdvanced)
	if got.Details["days"] != 7 {
		t.Errorf("Details[days]=%v; want 7", got.Details["days"])
	}
}

// TestAdvanceTimeAPI_EmitsTimeAdvanced — captures hours+minutes.
func TestAdvanceTimeAPI_EmitsTimeAdvanced(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodPost, "/api", []byte(`{"hours":3,"minutes":15}`), "cal-1", "camp-1", "u-1")
	if err := h.AdvanceTimeAPI(c); err != nil {
		t.Fatalf("AdvanceTimeAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionCalendarTimeAdvanced)
	if got.Details["hours"] != 3 || got.Details["minutes"] != 15 {
		t.Errorf("Details[hours/minutes]=%v/%v; want 3/15", got.Details["hours"], got.Details["minutes"])
	}
}

// TestDeleteCalendarAPI_EmitsCalendarDeleted — pre-state snapshot
// (mode, epoch, year) populated from the pre-delete read.
func TestDeleteCalendarAPI_EmitsCalendarDeleted(t *testing.T) {
	epoch := "DR"
	svc := &stubCalSvc{cal: &Calendar{
		ID: "cal-9", CampaignID: "camp-1", Name: "Doomed",
		Mode: ModeFantasy, EpochName: &epoch, CurrentYear: 1492,
	}}
	h, rec := makeHandler(svc)
	c, _, _ := newReqWithCC(http.MethodDelete, "/api", nil, "cal-9", "camp-1", "u-1")
	if err := h.DeleteCalendarAPI(c); err != nil {
		t.Fatalf("DeleteCalendarAPI: %v", err)
	}
	got := rec.findByAction(audit.ActionCalendarDeleted)
	if got.EntityID != "cal-9" {
		t.Errorf("EntityID=%q; want cal-9", got.EntityID)
	}
	if got.EntityName != "Doomed" {
		t.Errorf("EntityName=%q; want Doomed", got.EntityName)
	}
	if got.Details["epoch_name"] != "DR" {
		t.Errorf("Details[epoch_name]=%v; want DR", got.Details["epoch_name"])
	}
}

// TestAuditEmitNilSafe — when no audit service is wired, calls succeed
// silently (no panic). This is the "audit is optional" guarantee that
// keeps test setups simple in plugins that don't care about audit.
func TestAuditEmitNilSafe(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h := NewHandler(svc) // no SetAuditService
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	if err := h.UpdateMonthsAPI(c); err != nil {
		t.Fatalf("UpdateMonthsAPI: %v", err)
	}
}

// TestAuditEmitFailureDoesNotBlock — audit emit error is logged but
// doesn't bubble up to break the primary operation. Pins dispatch
// §"Failure handling": "Audit-log emission failures should NOT block
// the primary write."
func TestAuditEmitFailureDoesNotBlock(t *testing.T) {
	svc := &stubCalSvc{cal: &Calendar{ID: "cal-1", CampaignID: "camp-1"}}
	h := NewHandler(svc)
	h.SetAuditService(&failingAuditSvc{})
	c, _, _ := newReqWithCC(http.MethodPut, "/api", []byte(`[]`), "cal-1", "camp-1", "u-1")
	// Primary op must succeed even though audit-emit errors.
	if err := h.UpdateMonthsAPI(c); err != nil {
		t.Fatalf("UpdateMonthsAPI: %v (audit error should not block)", err)
	}
}

type failingAuditSvc struct{}

func (failingAuditSvc) Log(_ context.Context, _ *audit.AuditEntry) error {
	return context.Canceled
}
func (failingAuditSvc) GetCampaignActivity(_ context.Context, _ string, _ int) ([]audit.AuditEntry, int, error) {
	return nil, 0, nil
}
func (failingAuditSvc) GetEntityHistory(_ context.Context, _ string) ([]audit.AuditEntry, error) {
	return nil, nil
}
func (failingAuditSvc) GetCampaignStats(_ context.Context, _ string) (*audit.CampaignStats, error) {
	return nil, nil
}

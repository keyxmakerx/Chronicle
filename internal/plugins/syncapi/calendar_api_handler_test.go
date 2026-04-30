package syncapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// stubCalendarSvc satisfies calendar.CalendarService for handler tests.
// Only the methods exercised by ImportCalendar are wired to closures; all
// other methods return zero values so the type still satisfies the full
// interface without dragging in the real service / repo / DB.
type stubCalendarSvc struct {
	onGet    func(context.Context, string) (*calendar.Calendar, error)
	onCreate func(context.Context, string, calendar.CreateCalendarInput) (*calendar.Calendar, error)
	onApply  func(context.Context, string, *calendar.ImportResult) error
}

// --- methods we actually use in tests ---

func (s *stubCalendarSvc) GetCalendar(ctx context.Context, campaignID string) (*calendar.Calendar, error) {
	if s.onGet != nil {
		return s.onGet(ctx, campaignID)
	}
	return nil, nil
}

func (s *stubCalendarSvc) CreateCalendar(ctx context.Context, campaignID string, input calendar.CreateCalendarInput) (*calendar.Calendar, error) {
	if s.onCreate != nil {
		return s.onCreate(ctx, campaignID, input)
	}
	return nil, nil
}

func (s *stubCalendarSvc) ApplyImport(ctx context.Context, calendarID string, result *calendar.ImportResult) error {
	if s.onApply != nil {
		return s.onApply(ctx, calendarID, result)
	}
	return nil
}

// --- interface-fill stubs (zero-value returns) ---

func (s *stubCalendarSvc) GetCalendarByID(context.Context, string) (*calendar.Calendar, error) {
	return nil, nil
}
func (s *stubCalendarSvc) UpdateCalendar(context.Context, string, calendar.UpdateCalendarInput) error {
	return nil
}
func (s *stubCalendarSvc) DeleteCalendar(context.Context, string) error    { return nil }
func (s *stubCalendarSvc) ListCalendars(context.Context, string) ([]calendar.Calendar, error) {
	return nil, nil
}
func (s *stubCalendarSvc) SetDefaultCalendar(context.Context, string, string) error { return nil }
func (s *stubCalendarSvc) SetMonths(context.Context, string, []calendar.MonthInput) error {
	return nil
}
func (s *stubCalendarSvc) SetWeekdays(context.Context, string, []calendar.WeekdayInput) error {
	return nil
}
func (s *stubCalendarSvc) SetMoons(context.Context, string, []calendar.MoonInput) error { return nil }
func (s *stubCalendarSvc) SetSeasons(context.Context, string, []calendar.Season) error  { return nil }
func (s *stubCalendarSvc) SetEras(context.Context, string, []calendar.EraInput) error   { return nil }
func (s *stubCalendarSvc) SetEventCategories(context.Context, string, []calendar.EventCategoryInput) error {
	return nil
}
func (s *stubCalendarSvc) GetEventCategories(context.Context, string) ([]calendar.EventCategory, error) {
	return nil, nil
}
func (s *stubCalendarSvc) GetWeather(context.Context, string) (*calendar.Weather, error) {
	return nil, nil
}
func (s *stubCalendarSvc) SetWeather(context.Context, string, calendar.WeatherInput) error {
	return nil
}
func (s *stubCalendarSvc) GetCycles(context.Context, string) ([]calendar.Cycle, error) {
	return nil, nil
}
func (s *stubCalendarSvc) SetCycles(context.Context, string, []calendar.CycleInput) error {
	return nil
}
func (s *stubCalendarSvc) GetFestivals(context.Context, string) ([]calendar.Festival, error) {
	return nil, nil
}
func (s *stubCalendarSvc) SetFestivals(context.Context, string, []calendar.FestivalInput) error {
	return nil
}
func (s *stubCalendarSvc) CreateEvent(context.Context, string, calendar.CreateEventInput) (*calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) GetEvent(context.Context, string) (*calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) UpdateEvent(context.Context, string, calendar.UpdateEventInput) error {
	return nil
}
func (s *stubCalendarSvc) DeleteEvent(context.Context, string) error { return nil }
func (s *stubCalendarSvc) UpdateEventVisibility(context.Context, string, calendar.UpdateEventVisibilityInput) error {
	return nil
}
func (s *stubCalendarSvc) ListEventsForMonth(context.Context, string, int, int, int, string) ([]calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) ListEventsForEntity(context.Context, string, int, string) ([]calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) ListUpcomingEvents(context.Context, string, int, int, string) ([]calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) ListEventsForYear(context.Context, string, int, int, string) ([]calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) ListEventsForDateRange(context.Context, string, int, int, int, int, int, int, string) ([]calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) SearchCalendarEvents(context.Context, string, string, int) ([]map[string]string, error) {
	return nil, nil
}
func (s *stubCalendarSvc) AdvanceDate(context.Context, string, int) error          { return nil }
func (s *stubCalendarSvc) AdvanceTime(context.Context, string, int, int) error     { return nil }
func (s *stubCalendarSvc) SetDate(context.Context, string, int, int, int, int, int) error {
	return nil
}
func (s *stubCalendarSvc) ListAllEvents(context.Context, string) ([]calendar.Event, error) {
	return nil, nil
}
func (s *stubCalendarSvc) SetEventPublisher(calendar.CalendarEventPublisher) {}

// Compile-time check.
var _ calendar.CalendarService = (*stubCalendarSvc)(nil)

// --- helpers ---

// minimalChroniclePayload is a Chronicle-format calendar import body sized to
// pass DetectAndParse. The detector inspects top-level keys; we only need
// what makes the format unambiguous. See parseChronicle for the full schema.
func minimalChroniclePayload() []byte {
	return []byte(`{
  "format": "chronicle",
  "calendar": {"name": "Bootstrapped"},
  "months": [{"name": "Jan", "days": 30, "sort_order": 1}],
  "weekdays": [{"name": "Day", "sort_order": 1}],
  "settings": {"current_year": 1500, "hours_per_day": 24, "minutes_per_hour": 60, "seconds_per_minute": 60, "leap_year_every": 4}
}`)
}

// newImportTestContext builds an Echo context targeting the import endpoint
// with a Chronicle-format payload and a "camp-1" path param.
func newImportTestContext(body []byte) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/calendar/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	return c, rec
}

// TestImportCalendar_AutoCreatesWhenAbsent pins C-CAL1 Gap 2: a campaign
// with no calendar must end up with one after a successful import. Without
// this, Foundry's Calendaria bootstrap would 404 against a fresh campaign.
func TestImportCalendar_AutoCreatesWhenAbsent(t *testing.T) {
	createdMode := ""
	createdCampaign := ""
	appliedTo := ""
	svc := &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
			return nil, nil // no calendar exists
		},
		onCreate: func(_ context.Context, campaignID string, in calendar.CreateCalendarInput) (*calendar.Calendar, error) {
			createdCampaign = campaignID
			createdMode = in.Mode
			return &calendar.Calendar{ID: "cal-new", CampaignID: campaignID, Mode: in.Mode}, nil
		},
		onApply: func(_ context.Context, calID string, _ *calendar.ImportResult) error {
			appliedTo = calID
			return nil
		},
	}
	h := NewCalendarAPIHandler(nil, svc)

	c, rec := newImportTestContext(minimalChroniclePayload())
	if err := h.ImportCalendar(c); err != nil {
		t.Fatalf("ImportCalendar returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created on auto-create, got %d", rec.Code)
	}
	if createdCampaign != "camp-1" {
		t.Errorf("expected create on campaign 'camp-1', got %q", createdCampaign)
	}
	if createdMode != calendar.ModeFantasy {
		t.Errorf("expected fallback mode %q, got %q", calendar.ModeFantasy, createdMode)
	}
	if appliedTo != "cal-new" {
		t.Errorf("expected ApplyImport on cal-new, got %q", appliedTo)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if got, _ := body["auto_created"].(bool); !got {
		t.Errorf("expected auto_created=true in response, got body=%v", body)
	}
}

// TestImportCalendar_UsesExistingCalendar pins the unchanged path: when a
// calendar already exists, no auto-create happens, status is 200, and the
// auto_created flag is false.
func TestImportCalendar_UsesExistingCalendar(t *testing.T) {
	createCalled := false
	appliedTo := ""
	svc := &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
			return &calendar.Calendar{ID: "cal-existing", CampaignID: "camp-1", Mode: calendar.ModeFantasy}, nil
		},
		onCreate: func(context.Context, string, calendar.CreateCalendarInput) (*calendar.Calendar, error) {
			createCalled = true
			return nil, nil
		},
		onApply: func(_ context.Context, calID string, _ *calendar.ImportResult) error {
			appliedTo = calID
			return nil
		},
	}
	h := NewCalendarAPIHandler(nil, svc)

	c, rec := newImportTestContext(minimalChroniclePayload())
	if err := h.ImportCalendar(c); err != nil {
		t.Fatalf("ImportCalendar returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK on existing calendar, got %d", rec.Code)
	}
	if createCalled {
		t.Error("CreateCalendar should not be called when one already exists")
	}
	if appliedTo != "cal-existing" {
		t.Errorf("expected apply on cal-existing, got %q", appliedTo)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if got, _ := body["auto_created"].(bool); got {
		t.Errorf("expected auto_created=false in response, got body=%v", body)
	}
}

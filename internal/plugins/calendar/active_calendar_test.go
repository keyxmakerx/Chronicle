// active_calendar_test.go covers V2 Wave 1 PR 1 (C-CAL-V2-SHELL-
// FOUNDATION) active-calendar resolution and switcher behavior.
// Service-layer tests pin the resolution chain (pointer → default →
// first-by-sort → nil); handler tests pin the HTTP wiring.

package calendar

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Service-layer tests ---

// TestGetActiveCalendar_PointerWins — when a calendar_active row
// exists and points at a valid calendar in the campaign, resolution
// returns that calendar (not the default).
func TestGetActiveCalendar_PointerWins(t *testing.T) {
	defaultCal := &Calendar{ID: "cal-default", CampaignID: "camp-1", Name: "Default", IsDefault: true}
	switchedCal := &Calendar{ID: "cal-switched", CampaignID: "camp-1", Name: "Switched"}
	repo := &mockCalendarRepo{
		getActiveCalendarIDFn: func(_ context.Context, userID, campaignID string) (string, error) {
			if userID == "user-1" && campaignID == "camp-1" {
				return "cal-switched", nil
			}
			return "", nil
		},
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-switched" {
				return switchedCal, nil
			}
			if id == "cal-default" {
				return defaultCal, nil
			}
			return nil, nil
		},
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return defaultCal, nil
		},
	}
	svc := NewCalendarService(repo)
	got, err := svc.GetActiveCalendar(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetActiveCalendar: %v", err)
	}
	if got == nil || got.ID != "cal-switched" {
		t.Errorf("expected switched calendar; got %+v", got)
	}
}

// TestGetActiveCalendar_StalePointerFallsBack — pointer references a
// calendar in a different campaign (stale post-FK-cascade race);
// resolution falls back to the default rather than IDOR-breaching.
func TestGetActiveCalendar_StalePointerFallsBack(t *testing.T) {
	defaultCal := &Calendar{ID: "cal-default", CampaignID: "camp-1", Name: "Default", IsDefault: true}
	wrongCampaignCal := &Calendar{ID: "cal-elsewhere", CampaignID: "camp-other"}
	repo := &mockCalendarRepo{
		getActiveCalendarIDFn: func(_ context.Context, _, _ string) (string, error) {
			return "cal-elsewhere", nil
		},
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-elsewhere" {
				return wrongCampaignCal, nil
			}
			return nil, nil
		},
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return defaultCal, nil
		},
	}
	svc := NewCalendarService(repo)
	got, err := svc.GetActiveCalendar(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetActiveCalendar: %v", err)
	}
	if got == nil || got.ID != "cal-default" {
		t.Errorf("stale pointer should have fallen back to default; got %+v", got)
	}
}

// TestGetActiveCalendar_NoPointerUsesDefault — fresh user with no
// pointer row returns the campaign's default calendar.
func TestGetActiveCalendar_NoPointerUsesDefault(t *testing.T) {
	defaultCal := &Calendar{ID: "cal-default", CampaignID: "camp-1", Name: "Default", IsDefault: true}
	repo := &mockCalendarRepo{
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return defaultCal, nil
		},
	}
	svc := NewCalendarService(repo)
	got, err := svc.GetActiveCalendar(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetActiveCalendar: %v", err)
	}
	if got == nil || got.ID != "cal-default" {
		t.Errorf("expected default calendar; got %+v", got)
	}
}

// TestGetActiveCalendar_NoDefaultFallsBackToFirstByList — campaign
// with calendars but no is_default flagged returns the first by sort.
func TestGetActiveCalendar_NoDefaultFallsBackToFirstByList(t *testing.T) {
	first := Calendar{ID: "cal-a", CampaignID: "camp-1", Name: "Alpha", SortOrder: 1}
	second := Calendar{ID: "cal-b", CampaignID: "camp-1", Name: "Beta", SortOrder: 2}
	repo := &mockCalendarRepo{
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return nil, nil // no default
		},
		listByCampaignIDFn: func(_ context.Context, _ string) ([]Calendar, error) {
			return []Calendar{first, second}, nil
		},
	}
	svc := NewCalendarService(repo)
	got, err := svc.GetActiveCalendar(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetActiveCalendar: %v", err)
	}
	if got == nil || got.ID != "cal-a" {
		t.Errorf("expected first calendar; got %+v", got)
	}
}

// TestGetActiveCalendar_ZeroCalendars — campaign with zero calendars
// returns nil with no error. Handler renders a "create calendar"
// empty state.
func TestGetActiveCalendar_ZeroCalendars(t *testing.T) {
	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, _ string) ([]Calendar, error) {
			return nil, nil
		},
	}
	svc := NewCalendarService(repo)
	got, err := svc.GetActiveCalendar(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetActiveCalendar: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for zero-calendar campaign; got %+v", got)
	}
}

// TestGetActiveCalendar_EmptyUserID — anonymous/unauth path (public
// campaign visitor with no session) skips the pointer lookup and goes
// straight to the default. Avoids polluting calendar_active with
// rows keyed on empty user_id.
func TestGetActiveCalendar_EmptyUserID(t *testing.T) {
	pointerLooked := false
	defaultCal := &Calendar{ID: "cal-default", CampaignID: "camp-1", Name: "Default"}
	repo := &mockCalendarRepo{
		getActiveCalendarIDFn: func(_ context.Context, _, _ string) (string, error) {
			pointerLooked = true
			return "", nil
		},
		getByCampaignIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return defaultCal, nil
		},
	}
	svc := NewCalendarService(repo)
	got, err := svc.GetActiveCalendar(context.Background(), "", "camp-1")
	if err != nil {
		t.Fatalf("GetActiveCalendar: %v", err)
	}
	if pointerLooked {
		t.Error("empty user_id should skip pointer lookup")
	}
	if got == nil || got.ID != "cal-default" {
		t.Errorf("expected default; got %+v", got)
	}
}

// TestSwitchActiveCalendar_HappyPath — valid calendar in campaign:
// pointer is persisted.
func TestSwitchActiveCalendar_HappyPath(t *testing.T) {
	var capturedUser, capturedCampaign, capturedCal string
	target := &Calendar{ID: "cal-target", CampaignID: "camp-1"}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-target" {
				return target, nil
			}
			return nil, nil
		},
		setActiveCalendarFn: func(_ context.Context, userID, campaignID, calendarID string) error {
			capturedUser, capturedCampaign, capturedCal = userID, campaignID, calendarID
			return nil
		},
	}
	svc := NewCalendarService(repo)
	if err := svc.SwitchActiveCalendar(context.Background(), "user-1", "camp-1", "cal-target"); err != nil {
		t.Fatalf("SwitchActiveCalendar: %v", err)
	}
	if capturedUser != "user-1" || capturedCampaign != "camp-1" || capturedCal != "cal-target" {
		t.Errorf("repo got (%q, %q, %q); want (user-1, camp-1, cal-target)", capturedUser, capturedCampaign, capturedCal)
	}
}

// TestSwitchActiveCalendar_WrongCampaign — IDOR check: switching to
// a calendar in a different campaign is forbidden.
func TestSwitchActiveCalendar_WrongCampaign(t *testing.T) {
	wrong := &Calendar{ID: "cal-other", CampaignID: "camp-other"}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return wrong, nil
		},
	}
	svc := NewCalendarService(repo)
	err := svc.SwitchActiveCalendar(context.Background(), "user-1", "camp-1", "cal-other")
	if err == nil {
		t.Fatal("expected IDOR error switching to wrong-campaign calendar")
	}
	if !strings.Contains(err.Error(), "does not belong") {
		t.Errorf("expected 'does not belong' error; got %v", err)
	}
}

// TestSwitchActiveCalendar_NotFound — switching to a non-existent
// calendar errors NotFound.
func TestSwitchActiveCalendar_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return nil, nil
		},
	}
	svc := NewCalendarService(repo)
	if err := svc.SwitchActiveCalendar(context.Background(), "user-1", "camp-1", "cal-ghost"); err == nil {
		t.Fatal("expected NotFound for nonexistent calendar")
	}
}

// TestSwitchActiveCalendar_EmptyUser — empty user_id rejects (the
// pointer is keyed on user_id; empty would be a corruption vector).
func TestSwitchActiveCalendar_EmptyUser(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{})
	if err := svc.SwitchActiveCalendar(context.Background(), "", "camp-1", "cal-target"); err == nil {
		t.Fatal("expected validation error for empty user_id")
	}
}

// TestSwitchActiveCalendar_RepoError — repo SetActive returns error;
// service wraps it.
func TestSwitchActiveCalendar_RepoError(t *testing.T) {
	target := &Calendar{ID: "cal-target", CampaignID: "camp-1"}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return target, nil
		},
		setActiveCalendarFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("db down")
		},
	}
	svc := NewCalendarService(repo)
	if err := svc.SwitchActiveCalendar(context.Background(), "user-1", "camp-1", "cal-target"); err == nil {
		t.Fatal("expected error to propagate from repo")
	}
}

// --- Handler-layer tests ---

// TestSwitchActiveCalendarAPI_HappyPath — POST switches active cal
// and returns 200 with the new id.
func TestSwitchActiveCalendarAPI_HappyPath(t *testing.T) {
	target := &Calendar{ID: "cal-target", CampaignID: "camp-1"}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return target, nil
		},
	}
	h := NewHandler(NewCalendarService(repo))

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"calendar_id":"cal-target"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer,
	})
	c.Set("auth_user_id", "user-1")

	if err := h.SwitchActiveCalendarAPI(c); err != nil {
		t.Fatalf("SwitchActiveCalendarAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cal-target") {
		t.Errorf("response missing calendar_id; got %q", rec.Body.String())
	}
}

// TestSwitchActiveCalendarAPI_Unauthenticated — missing user_id from
// session returns 401 (not 500).
func TestSwitchActiveCalendarAPI_Unauthenticated(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{"calendar_id":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer,
	})
	// No auth_user_id set.

	err := h.SwitchActiveCalendarAPI(c)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
}

// TestSwitchActiveCalendarAPI_EmptyCalendarID — missing calendar_id
// returns 400.
func TestSwitchActiveCalendarAPI_EmptyCalendarID(t *testing.T) {
	h := NewHandler(NewCalendarService(&mockCalendarRepo{}))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer,
	})
	c.Set("auth_user_id", "user-1")

	if err := h.SwitchActiveCalendarAPI(c); err == nil {
		t.Fatal("expected BadRequest for empty calendar_id")
	}
}

// --- Helper-layer (Month placeholder rendering) tests ---

// TestMonthDays_TodayHighlight — monthDays marks the IsToday day
// when (year, month, day) matches the calendar's current cursor.
func TestMonthDays_TodayHighlight(t *testing.T) {
	cal := &Calendar{
		ID:           "cal-1",
		CurrentYear:  1492,
		CurrentMonth: 3,
		CurrentDay:   15,
		Months: []Month{
			{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}, {Name: "Mar", Days: 31},
		},
		Weekdays: []Weekday{{Name: "Sun"}, {Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"}, {Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"}},
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 3, Day: 15}
	days := monthDays(data)
	if len(days) != 31 {
		t.Fatalf("expected 31 days for March; got %d", len(days))
	}
	if !days[14].IsToday {
		t.Error("day 15 should be marked IsToday")
	}
	if days[14].Day != 15 {
		t.Errorf("expected days[14].Day = 15; got %d", days[14].Day)
	}
	// Different year — same cursor day should NOT be today.
	dataOther := CalendarV2ViewData{ActiveCalendar: cal, Year: 1493, Month: 3, Day: 15}
	daysOther := monthDays(dataOther)
	if daysOther[14].IsToday {
		t.Error("year 1493 day 15 should NOT be marked IsToday")
	}
}

// TestMonthDays_RestDayTint — weekdays with IsRestDay=1 mark cells
// whose day-of-year falls on that weekday.
func TestMonthDays_RestDayTint(t *testing.T) {
	cal := &Calendar{
		ID:           "cal-1",
		CurrentYear:  100,
		CurrentMonth: 1,
		CurrentDay:   1,
		Months:       []Month{{Name: "Jan", Days: 7}},
		// 7-day week with Saturday flagged as rest day.
		Weekdays: []Weekday{
			{Name: "Sun", IsRestDay: false}, {Name: "Mon"}, {Name: "Tue"},
			{Name: "Wed"}, {Name: "Thu"}, {Name: "Fri"},
			{Name: "Sat", IsRestDay: true},
		},
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 1}
	days := monthDays(data)
	if len(days) != 7 {
		t.Fatalf("expected 7 days; got %d", len(days))
	}
	// Year-aware weekday (C-CAL-V2-MONTH-GRID-ALIGN-FIX): absolute day of
	// (100,1,d) = 100*7 + d, so weekday index = d % 7. Day 1 = Mon(1),
	// day 6 = Sat(6, rest), day 7 = Sun(0). The rest tint must line up with the
	// corrected column placement (the Saturday column), i.e. day 6 not day 7.
	if !days[5].IsRestDay {
		t.Error("day 6 (Saturday) should be marked IsRestDay")
	}
	if days[0].IsRestDay {
		t.Error("day 1 (Monday) should NOT be marked rest")
	}
	if days[6].IsRestDay {
		t.Error("day 7 (Sunday) should NOT be marked rest")
	}
}

// TestV2Step_MonthAdvance — month +1 rolls into next year when
// crossing the calendar's last month.
func TestV2Step_MonthAdvance(t *testing.T) {
	cal := &Calendar{Months: []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}, {Name: "Mar", Days: 31}}}
	data := CalendarV2ViewData{ActiveCalendar: cal, View: "month", Year: 100, Month: 3, Day: 15}
	y, m, _ := v2Step(data, 1)
	if y != 101 || m != 1 {
		t.Errorf("month +1 from (100,3) = (%d,%d); want (101,1)", y, m)
	}
	data2 := CalendarV2ViewData{ActiveCalendar: cal, View: "month", Year: 100, Month: 1, Day: 15}
	y2, m2, _ := v2Step(data2, -1)
	if y2 != 99 || m2 != 3 {
		t.Errorf("month -1 from (100,1) = (%d,%d); want (99,3)", y2, m2)
	}
}

// TestV2Step_DayRollover — day +1 on the last day of a month rolls
// into day 1 of the next month.
func TestV2Step_DayRollover(t *testing.T) {
	cal := &Calendar{Months: []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}, {Name: "Mar", Days: 31}}}
	data := CalendarV2ViewData{ActiveCalendar: cal, View: "day", Year: 100, Month: 1, Day: 31}
	y, m, d := v2Step(data, 1)
	if y != 100 || m != 2 || d != 1 {
		t.Errorf("day +1 from (100,1,31) = (%d,%d,%d); want (100,2,1)", y, m, d)
	}
}

// TestMonthHeading_WithEpoch — heading composes "MonthName Year EpochName".
func TestMonthHeading_WithEpoch(t *testing.T) {
	epoch := "DR"
	cal := &Calendar{
		EpochName: &epoch,
		Months:    []Month{{Name: "Mirtul"}, {Name: "Kythorn"}},
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1}
	got := monthHeading(data)
	if got != "Mirtul 1492 DR" {
		t.Errorf("monthHeading = %q; want 'Mirtul 1492 DR'", got)
	}
}

// TestMonthGridStyle_FantasyWeek — grid column count matches the
// calendar's weekday count (e.g. 10-day week → 10 columns).
func TestMonthGridStyle_FantasyWeek(t *testing.T) {
	cal := &Calendar{Weekdays: make([]Weekday, 10)}
	data := CalendarV2ViewData{ActiveCalendar: cal}
	got := monthGridStyle(data)
	if !strings.Contains(got, "repeat(10,") {
		t.Errorf("monthGridStyle = %q; want repeat(10, ...)", got)
	}
}

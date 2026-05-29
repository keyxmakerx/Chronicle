// calendar_v2_sidebar_test.go covers Wave 1.7A §G sidebar mini-month
// helpers + the SetSidebarPinned service method's validation. JS
// pin-toggle UX lives in manual test plan; ribbon-resize JS
// (Phase A) likewise.

package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// --- Mini-month grid ---

func TestMiniMonthDays_BuildsFullMonth(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 15,
		Months:   []Month{{Name: "Jan", Days: 31}},
		Weekdays: make([]Weekday, 7),
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 15}
	days := miniMonthDays(data)
	if len(days) != 31 {
		t.Fatalf("expected 31 days; got %d", len(days))
	}
	// Day 15 should be both today + selected.
	if !days[14].IsToday {
		t.Error("day 15 should be IsToday")
	}
	if !days[14].IsSelected {
		t.Error("day 15 should be IsSelected (cursor day)")
	}
	if days[0].IsToday {
		t.Error("day 1 should not be IsToday")
	}
}

func TestMiniMonthDays_SelectedDifferentFromToday(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 5,
		Months:   []Month{{Name: "Jan", Days: 31}},
		Weekdays: make([]Weekday, 7),
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 20}
	days := miniMonthDays(data)
	if !days[4].IsToday || days[4].IsSelected {
		t.Errorf("day 5: IsToday=true, IsSelected=false; got %+v", days[4])
	}
	if days[19].IsToday || !days[19].IsSelected {
		t.Errorf("day 20: IsToday=false, IsSelected=true; got %+v", days[19])
	}
}

func TestMiniMonthDays_NilCalendarReturnsNil(t *testing.T) {
	if got := miniMonthDays(CalendarV2ViewData{}); got != nil {
		t.Errorf("nil calendar → nil days; got %+v", got)
	}
}

// --- Day cell classes ---

func TestMiniMonthDayClasses_TodayAndSelectedTakePrecedence(t *testing.T) {
	both := miniMonthDayClasses(miniMonthDay{Day: 5, IsToday: true, IsSelected: true})
	if !strings.Contains(both, "bg-accent") || !strings.Contains(both, "text-white") {
		t.Errorf("today+selected = highest emphasis; got %q", both)
	}
	todayOnly := miniMonthDayClasses(miniMonthDay{Day: 5, IsToday: true})
	if !strings.Contains(todayOnly, "ring-accent") {
		t.Errorf("today alone → accent ring; got %q", todayOnly)
	}
	selectedOnly := miniMonthDayClasses(miniMonthDay{Day: 5, IsSelected: true})
	if !strings.Contains(selectedOnly, "bg-accent/20") {
		t.Errorf("selected alone → soft accent tint; got %q", selectedOnly)
	}
	plain := miniMonthDayClasses(miniMonthDay{Day: 5})
	if strings.Contains(plain, "bg-accent") {
		t.Errorf("plain cell shouldn't accent; got %q", plain)
	}
}

// --- Weekday label abbreviation ---

func TestMiniMonthWeekdayLabels_FirstCharOnly(t *testing.T) {
	cal := &Calendar{Weekdays: []Weekday{
		{Name: "Sunday"}, {Name: "Monday"}, {Name: "Tuesday"},
	}}
	data := CalendarV2ViewData{ActiveCalendar: cal}
	got := miniMonthWeekdayLabels(data)
	if len(got) != 3 {
		t.Fatalf("expected 3 labels; got %d", len(got))
	}
	if got[0] != "S" || got[1] != "M" || got[2] != "T" {
		t.Errorf("expected first-char abbreviations; got %+v", got)
	}
}

// --- Data attribute for JS jump ---

func TestMiniMonthDataAttr_FormatsYYYYMMDD(t *testing.T) {
	cal := &Calendar{Months: []Month{{Name: "Jan", Days: 31}}}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 3, Day: 15}
	got := miniMonthDataAttr(data, miniMonthDay{Day: 7})
	if got != "1492-03-07" {
		t.Errorf("expected '1492-03-07' format; got %q", got)
	}
}

// --- Jump href ---

func TestMiniMonthDayHref_PreservesViewAndYearMonth(t *testing.T) {
	cal := &Calendar{ID: "cal-1", Months: []Month{{Name: "Jan", Days: 31}}}
	data := CalendarV2ViewData{
		ActiveCalendar: cal, CampaignID: "camp-1",
		Year: 100, Month: 1, Day: 5, View: "week",
	}
	got := string(miniMonthDayHref(data, miniMonthDay{Day: 12}))
	if !strings.Contains(got, "/campaigns/camp-1/calendar/v2/cal-1/week") {
		t.Errorf("expected V2 route shape; got %q", got)
	}
	if !strings.Contains(got, "day=12") {
		t.Errorf("expected day=12 query; got %q", got)
	}
	if !strings.Contains(got, "year=100") || !strings.Contains(got, "month=1") {
		t.Errorf("expected preserved year+month; got %q", got)
	}
}

func TestMiniMonthDayHref_NilCalendarFallsBackSafely(t *testing.T) {
	data := CalendarV2ViewData{CampaignID: "camp-1"}
	got := string(miniMonthDayHref(data, miniMonthDay{Day: 1}))
	if got != "/campaigns/camp-1/calendar/v2" {
		t.Errorf("nil cal → bare v2 root; got %q", got)
	}
}

// --- Service: SetSidebarPinned ---

func TestSetSidebarPinned_EmptyUserRejects(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{})
	err := svc.SetSidebarPinned(context.Background(), "", "camp-1", true)
	if err == nil {
		t.Fatal("empty user_id should reject")
	}
}

func TestSetSidebarPinned_HappyPath(t *testing.T) {
	var capturedUser, capturedCampaign string
	var capturedPinned bool
	repo := &mockCalendarRepo{
		setSidebarPinnedFn: func(_ context.Context, userID, campaignID string, pinned bool) error {
			capturedUser, capturedCampaign, capturedPinned = userID, campaignID, pinned
			return nil
		},
	}
	svc := NewCalendarService(repo)
	if err := svc.SetSidebarPinned(context.Background(), "user-1", "camp-1", false); err != nil {
		t.Fatalf("SetSidebarPinned: %v", err)
	}
	if capturedUser != "user-1" || capturedCampaign != "camp-1" || capturedPinned {
		t.Errorf("repo called with (%q, %q, %v); want (user-1, camp-1, false)", capturedUser, capturedCampaign, capturedPinned)
	}
}

func TestGetSidebarPinned_EmptyUserReturnsDefault(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{})
	got, err := svc.GetSidebarPinned(context.Background(), "", "camp-1")
	if err != nil {
		t.Fatalf("anonymous get should not error; got %v", err)
	}
	if !got {
		t.Error("anonymous default should be TRUE (platform default)")
	}
}

func TestGetSidebarPinned_PropagatesRepoError(t *testing.T) {
	repo := &mockCalendarRepo{
		getSidebarPinnedFn: func(_ context.Context, _, _ string) (bool, error) {
			return true, errors.New("db down")
		},
	}
	svc := NewCalendarService(repo)
	_, err := svc.GetSidebarPinned(context.Background(), "user-1", "camp-1")
	if err == nil {
		t.Error("repo error should propagate")
	}
}

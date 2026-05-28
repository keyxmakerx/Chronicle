// calendar_v2_events_test.go covers V2 Wave 1 PR 4 event-rendering
// glue: single-day vs multi-day classification, projection from the
// model Event into the calwidget EventCardData shape, and the
// Month-cell overflow / cap logic.

package calendar

import (
	"strings"
	"testing"

	calwidget "github.com/keyxmakerx/chronicle/internal/widgets/calendar_v2"
)

func TestIsMultiDayEvent_NilEndIsSingle(t *testing.T) {
	e := Event{Year: 1492, Month: 3, Day: 15}
	if isMultiDayEvent(e) {
		t.Error("event with no end date should be single-day")
	}
}

func TestIsMultiDayEvent_EqualStartEndIsSingle(t *testing.T) {
	y := 1492
	m := 3
	d := 15
	e := Event{Year: 1492, Month: 3, Day: 15, EndYear: &y, EndMonth: &m, EndDay: &d}
	if isMultiDayEvent(e) {
		t.Error("event with equal start and end should be single-day")
	}
}

func TestIsMultiDayEvent_DifferentDayIsMulti(t *testing.T) {
	d := 16
	e := Event{Year: 1492, Month: 3, Day: 15, EndDay: &d}
	if !isMultiDayEvent(e) {
		t.Error("event with different end day should be multi-day")
	}
}

func TestEventsForDay_FiltersByDateAndMultiDay(t *testing.T) {
	d := 16
	events := []Event{
		{ID: "a", Year: 1492, Month: 3, Day: 15, Visibility: "everyone"},
		{ID: "b", Year: 1492, Month: 3, Day: 15, EndDay: &d, Visibility: "everyone"}, // multi-day
		{ID: "c", Year: 1492, Month: 3, Day: 16, Visibility: "everyone"},
	}
	got := eventsForDay(events, 1492, 3, 15)
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("expected only single-day event 'a' for day 15; got %+v", got)
	}
}

func TestEventToCardData_ResolvesCategoryFromCalendar(t *testing.T) {
	slug := "festival"
	cal := &Calendar{
		Months: []Month{{Name: "Mirtul", Days: 30}, {Name: "Kythorn", Days: 30}, {Name: "Tarsakh", Days: 30}},
		EventCategories: []EventCategory{
			{Slug: "festival", Name: "Festival", Color: "#ff8800"},
		},
	}
	e := Event{
		ID: "evt-1", Name: "Midsummer", Year: 1492, Month: 3, Day: 15,
		Category: &slug, Visibility: "everyone",
	}
	got := eventToCardData(e, cal)
	if got.Name != "Midsummer" {
		t.Errorf("name lost; got %q", got.Name)
	}
	if got.CategoryColor != "#ff8800" {
		t.Errorf("expected category color resolution; got %q", got.CategoryColor)
	}
	if got.CategoryName != "Festival" {
		t.Errorf("expected category name resolution; got %q", got.CategoryName)
	}
	if !got.IsPublic {
		t.Error("everyone-visibility event should be public")
	}
	if !strings.Contains(got.StartLabel, "Tarsakh") {
		t.Errorf("expected start label to use month name; got %q", got.StartLabel)
	}
}

func TestEventToCardData_ColorOverrideWinsOverCategory(t *testing.T) {
	slug := "festival"
	override := "#abcdef"
	cal := &Calendar{
		Months:          []Month{{Name: "Jan", Days: 31}},
		EventCategories: []EventCategory{{Slug: "festival", Color: "#ff8800"}},
	}
	e := Event{ID: "e", Name: "X", Year: 1, Month: 1, Day: 1, Category: &slug, Color: &override, Visibility: "everyone"}
	got := eventToCardData(e, cal)
	if got.CategoryColor != "#abcdef" {
		t.Errorf("explicit Color override should win; got %q", got.CategoryColor)
	}
}

func TestEventToCardData_DmOnlyIsNotPublic(t *testing.T) {
	e := Event{ID: "e", Name: "Secret", Year: 1, Month: 1, Day: 1, Visibility: "dm_only"}
	got := eventToCardData(e, nil)
	if got.IsPublic {
		t.Error("dm_only event should not be public")
	}
}

func TestEventToCardData_TimeLabelComposes(t *testing.T) {
	startH, startM := 14, 30
	endH, endM := 16, 0
	e := Event{
		ID: "e", Name: "X", Year: 1, Month: 1, Day: 1,
		StartHour: &startH, StartMinute: &startM,
		EndHour: &endH, EndMinute: &endM,
		Visibility: "everyone",
	}
	got := eventToCardData(e, nil)
	if !strings.Contains(got.TimeLabel, "14:30") {
		t.Errorf("expected start time; got %q", got.TimeLabel)
	}
	if !strings.Contains(got.TimeLabel, "16:00") {
		t.Errorf("expected end time; got %q", got.TimeLabel)
	}
}

func TestEventToCardData_DefaultsTierStandard(t *testing.T) {
	e := Event{Visibility: "everyone"}
	got := eventToCardData(e, nil)
	if got.Tier != calwidget.TierStandard {
		t.Errorf("default tier should be Standard; got %q", got.Tier)
	}
}

// --- Month cell visibility / overflow ---

func TestMonthCellVisibleAndOverflow_Boundary(t *testing.T) {
	// 5 single-day events on the same day → 3 visible + 2 overflow.
	makeEvent := func(id string) Event {
		return Event{ID: id, Year: 100, Month: 1, Day: 1, Visibility: "everyone"}
	}
	data := CalendarV2ViewData{
		Year: 100, Month: 1,
		Events: []Event{
			makeEvent("a"), makeEvent("b"), makeEvent("c"), makeEvent("d"), makeEvent("e"),
		},
	}
	vis := monthCellVisible(data, 1)
	over := monthCellOverflow(data, 1)
	if len(vis) != monthCellVisibleCap() {
		t.Errorf("visible should be capped at %d; got %d", monthCellVisibleCap(), len(vis))
	}
	if over != 2 {
		t.Errorf("overflow should be 5 - cap; got %d", over)
	}
}

func TestMonthCellVisibleAndOverflow_NoEvents(t *testing.T) {
	data := CalendarV2ViewData{Year: 100, Month: 1}
	if v := monthCellVisible(data, 1); len(v) != 0 {
		t.Errorf("no events → empty visible; got %+v", v)
	}
	if o := monthCellOverflow(data, 1); o != 0 {
		t.Errorf("no events → 0 overflow; got %d", o)
	}
}

// --- addDaysSimple (Week-view date stepping) ---

func TestAddDaysSimple_WithinMonth(t *testing.T) {
	cal := &Calendar{Months: []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}}}
	m, d := addDaysSimple(cal, 1, 10, 5)
	if m != 1 || d != 15 {
		t.Errorf("addDays 10+5 in Jan = (%d,%d); want (1,15)", m, d)
	}
}

func TestAddDaysSimple_CrossesMonthBoundary(t *testing.T) {
	cal := &Calendar{Months: []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}, {Name: "Mar", Days: 31}}}
	m, d := addDaysSimple(cal, 1, 29, 5)
	// 29 → 30 → 31 → Feb 1 → Feb 2 → Feb 3
	if m != 2 || d != 3 {
		t.Errorf("addDays 1/29 +5 = (%d,%d); want (2,3)", m, d)
	}
}

// --- v2EventsJSON serialization ---

func TestV2EventsJSON_EmptyArray(t *testing.T) {
	data := CalendarV2ViewData{}
	if got := v2EventsJSON(data); got != "[]" {
		t.Errorf("empty events → '[]'; got %q", got)
	}
}

func TestV2EventsJSON_RoundTripsName(t *testing.T) {
	data := CalendarV2ViewData{Events: []Event{{ID: "e1", Name: "Test", Visibility: "everyone"}}}
	got := v2EventsJSON(data)
	if !strings.Contains(got, `"name":"Test"`) {
		t.Errorf("expected serialized event; got %q", got)
	}
}

// --- buildEventVisibilityEditor defaults ---

func TestBuildEventVisibilityEditor_DefaultsToPublic(t *testing.T) {
	got := buildEventVisibilityEditor(CalendarV2ViewData{})
	if !got.IsPublic {
		t.Error("default should be public; JS overrides on edit")
	}
	if got.FieldPrefix != "event_visibility" {
		t.Errorf("field prefix = %q; want 'event_visibility'", got.FieldPrefix)
	}
	if len(got.AvailableRoles) < 3 {
		t.Errorf("expected at least 3 roles seeded; got %d", len(got.AvailableRoles))
	}
}

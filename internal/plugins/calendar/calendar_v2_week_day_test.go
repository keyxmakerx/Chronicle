// calendar_v2_week_day_test.go covers Wave 1.5 §C Week + Day
// hour-grid layout helpers: week-window stepping, hour-row math,
// event-hour clipping, all-day vs timed filtering, and the various
// CSS Grid template builders. JS-side drag interactions live in
// manual test plan per the established convention.

package calendar

import (
	"strings"
	"testing"
)

// --- weekDays window ---

func TestWeekDays_Centers7DayWeekOnCursor(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 5,
		Months:   []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}, {Name: "Mar", Days: 31}},
		Weekdays: []Weekday{{Name: "Sun"}, {Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"}, {Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"}},
		HoursPerDay: 24,
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 5}
	days := weekDays(data)
	if len(days) != 7 {
		t.Fatalf("expected 7 days for 7-day-week; got %d", len(days))
	}
	// Cursor at day 5; backstep = 7/2 = 3 → first day should be Jan 2.
	if days[0].Day != 2 || days[0].Month != 1 {
		t.Errorf("first day = %d/%d; want Jan 2", days[0].Month, days[0].Day)
	}
	// Last day should be Jan 8.
	if days[6].Day != 8 || days[6].Month != 1 {
		t.Errorf("last day = %d/%d; want Jan 8", days[6].Month, days[6].Day)
	}
}

func TestWeekDays_CrossesMonthBoundary(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 30,
		Months:   []Month{{Name: "Jan", Days: 31}, {Name: "Feb", Days: 28}},
		Weekdays: make([]Weekday, 7),
		HoursPerDay: 24,
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 30}
	days := weekDays(data)
	// Cursor at Jan 30, backstep 3 → Jan 27, forward → Jan 27..31 + Feb 1..2.
	if days[0].Month != 1 || days[0].Day != 27 {
		t.Errorf("first = %d/%d; want 1/27", days[0].Month, days[0].Day)
	}
	if days[6].Month != 2 || days[6].Day != 2 {
		t.Errorf("last = %d/%d; want 2/2", days[6].Month, days[6].Day)
	}
}

func TestWeekDays_TodayHighlightOnMatchingDay(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 5,
		Months:   []Month{{Name: "Jan", Days: 31}},
		Weekdays: make([]Weekday, 7),
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 5}
	days := weekDays(data)
	var todayCount int
	for _, d := range days {
		if d.IsToday {
			todayCount++
		}
	}
	if todayCount != 1 {
		t.Errorf("expected exactly 1 today-marked day; got %d", todayCount)
	}
}

func TestWeekDays_RestDayCycle(t *testing.T) {
	// 7-day week; Saturday (index 6) is rest day.
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 1,
		Months:   []Month{{Name: "Jan", Days: 31}},
		Weekdays: []Weekday{{Name: "Sun"}, {Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"}, {Name: "Thu"}, {Name: "Fri"}, {Name: "Sat", IsRestDay: true}},
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 4}
	days := weekDays(data)
	if len(days) != 7 {
		t.Fatalf("expected 7 days; got %d", len(days))
	}
	// Cursor day 4 → backstep 3 → start day 1. Day 7 = Saturday (rest).
	var restCount int
	for _, d := range days {
		if d.IsRestDay {
			restCount++
		}
	}
	if restCount != 1 {
		t.Errorf("expected exactly 1 rest day; got %d", restCount)
	}
}

func TestWeekDays_FantasyTenDayWeek(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 6,
		Months:   []Month{{Name: "Trien", Days: 30}, {Name: "Eldar", Days: 30}},
		Weekdays: make([]Weekday, 10),
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 6}
	days := weekDays(data)
	if len(days) != 10 {
		t.Errorf("10-day-week should produce 10 columns; got %d", len(days))
	}
	// Backstep 5 (10/2 = 5) → starts at day 1.
	if days[0].Day != 1 {
		t.Errorf("first column = %d; want 1", days[0].Day)
	}
}

// --- Step helpers ---

func TestStepDaysBackward_WithinMonth(t *testing.T) {
	cal := &Calendar{Months: []Month{{Days: 31}, {Days: 28}}}
	m, d := stepDaysBackward(cal, 1, 15, 5)
	if m != 1 || d != 10 {
		t.Errorf("back 5 from (1,15) = (%d,%d); want (1,10)", m, d)
	}
}

func TestStepDaysBackward_CrossesMonthBoundary(t *testing.T) {
	cal := &Calendar{Months: []Month{{Days: 31}, {Days: 28}}}
	m, d := stepDaysBackward(cal, 2, 5, 10)
	if m != 1 || d != 26 {
		t.Errorf("back 10 from (2,5) = (%d,%d); want (1,26)", m, d)
	}
}

func TestStepDaysForward_CrossesMonthBoundary(t *testing.T) {
	cal := &Calendar{Months: []Month{{Days: 31}, {Days: 28}}}
	m, d := stepDaysForward(cal, 1, 29, 5)
	if m != 2 || d != 3 {
		t.Errorf("forward 5 from (1,29) = (%d,%d); want (2,3)", m, d)
	}
}

// --- Hour rows ---

func TestHourRows_DefaultsTo24WhenZero(t *testing.T) {
	cal := &Calendar{HoursPerDay: 0}
	rows := hourRows(cal)
	if len(rows) != 24 {
		t.Errorf("zero HoursPerDay should default to 24; got %d", len(rows))
	}
}

func TestHourRows_RespectsCustomHoursPerDay(t *testing.T) {
	cal := &Calendar{HoursPerDay: 30} // fantasy 30-hour day
	rows := hourRows(cal)
	if len(rows) != 30 {
		t.Errorf("fantasy 30h day should produce 30 rows; got %d", len(rows))
	}
}

func TestHourRows_NilCalendarSafe(t *testing.T) {
	rows := hourRows(nil)
	if len(rows) != 24 {
		t.Errorf("nil calendar fallback = 24; got %d", len(rows))
	}
}

// --- Hour label formatting ---

func TestFmtHourLabel_TwoDigitPadding(t *testing.T) {
	cases := map[int]string{
		0:  "00:00",
		5:  "05:00",
		12: "12:00",
		23: "23:00",
	}
	for h, want := range cases {
		if got := fmtHourLabel(h); got != want {
			t.Errorf("fmtHourLabel(%d) = %q; want %q", h, got, want)
		}
	}
}

// --- Event filtering ---

func TestEventsForWeekDay_OnlyTimedEvents(t *testing.T) {
	startH := 14
	events := []Event{
		{ID: "timed", Year: 1, Month: 1, Day: 5, StartHour: &startH, Visibility: "everyone"},
		{ID: "allday", Year: 1, Month: 1, Day: 5, Visibility: "everyone"},
		{ID: "otherday", Year: 1, Month: 1, Day: 6, StartHour: &startH, Visibility: "everyone"},
	}
	got := eventsForWeekDay(events, 1, 1, 5)
	if len(got) != 1 || got[0].ID != "timed" {
		t.Errorf("eventsForWeekDay should only return timed events for matching day; got %+v", got)
	}
}

func TestAllDayEventsForDay_OnlyUntimedEvents(t *testing.T) {
	startH := 14
	events := []Event{
		{ID: "timed", Year: 1, Month: 1, Day: 5, StartHour: &startH, Visibility: "everyone"},
		{ID: "allday", Year: 1, Month: 1, Day: 5, Visibility: "everyone"},
	}
	got := allDayEventsForDay(events, 1, 1, 5)
	if len(got) != 1 || got[0].ID != "allday" {
		t.Errorf("allDayEventsForDay should only return untimed events; got %+v", got)
	}
}

// --- Event hour range clipping ---

func TestEventHourRange_ClipsBeyondDayLength(t *testing.T) {
	cal := &Calendar{HoursPerDay: 20}
	startH := 18
	endH := 25 // beyond day length
	e := Event{StartHour: &startH, EndHour: &endH}
	gotStart, gotEnd := eventHourRange(e, cal)
	if gotStart != 18 || gotEnd != 20 {
		t.Errorf("range = (%d, %d); want (18, 20) — clipped", gotStart, gotEnd)
	}
}

func TestEventHourRange_DefaultsToOneHourBlock(t *testing.T) {
	startH := 14
	e := Event{StartHour: &startH} // no end
	gotStart, gotEnd := eventHourRange(e, nil)
	if gotStart != 14 || gotEnd != 15 {
		t.Errorf("missing end should default to start+1; got (%d, %d)", gotStart, gotEnd)
	}
}

func TestEventHourRange_NilStartReturnsSafeDefault(t *testing.T) {
	e := Event{} // no start hour
	gotStart, gotEnd := eventHourRange(e, nil)
	if gotStart != 0 || gotEnd != 1 {
		t.Errorf("nil start should return (0,1); got (%d, %d)", gotStart, gotEnd)
	}
}

// --- CSS Grid style composition ---

func TestWeekColumnsStyle_HourLabelPlusDayColumns(t *testing.T) {
	got := weekColumnsStyle(7)
	if !strings.Contains(got, "60px") {
		t.Errorf("expected 60px hour-label column; got %q", got)
	}
	if !strings.Contains(got, "repeat(7,") {
		t.Errorf("expected repeat(7, ...); got %q", got)
	}
}

func TestWeekHourGridStyle_RowCountMatchesHoursPerDay(t *testing.T) {
	data := CalendarV2ViewData{ActiveCalendar: &Calendar{HoursPerDay: 12}}
	got := weekHourGridStyle(data, 7)
	if !strings.Contains(got, "repeat(12,") {
		t.Errorf("expected 12 hour rows; got %q", got)
	}
}

func TestDayHourGridStyle_TwoColumnLayout(t *testing.T) {
	data := CalendarV2ViewData{ActiveCalendar: &Calendar{HoursPerDay: 24}}
	got := dayHourGridStyle(data)
	if !strings.Contains(got, "60px 1fr") {
		t.Errorf("expected 2-column template; got %q", got)
	}
}

// --- Headings ---

func TestWeekHeading_SameMonthRange(t *testing.T) {
	cal := &Calendar{
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 5,
		Months:   []Month{{Name: "Mirtul", Days: 30}},
		Weekdays: make([]Weekday, 7),
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 100, Month: 1, Day: 5}
	got := weekHeading(data)
	if !strings.Contains(got, "Mirtul") || !strings.Contains(got, "1") || !strings.Contains(got, "100") {
		t.Errorf("week heading = %q; want Mirtul month + day range + year", got)
	}
}

func TestDayHeading_IncludesEpoch(t *testing.T) {
	epoch := "DR"
	cal := &Calendar{
		EpochName: &epoch,
		Months:    []Month{{Name: "Mirtul", Days: 30}},
	}
	data := CalendarV2ViewData{ActiveCalendar: cal, Year: 1492, Month: 1, Day: 15}
	got := dayHeading(data)
	if got != "Mirtul 15, 1492 DR" {
		t.Errorf("day heading = %q; want 'Mirtul 15, 1492 DR'", got)
	}
}

// --- Style composition for timed events ---

func TestWeekTimedEventStyle_OffsetsByAllDayRow(t *testing.T) {
	startH := 9
	endH := 11
	e := Event{StartHour: &startH, EndHour: &endH}
	got := weekTimedEventStyle(e, &Calendar{HoursPerDay: 24})
	// Hour 9 → row 9+2 = 11; hour 11 → row 11+2 = 13.
	if !strings.Contains(got, "grid-row: 11 / 13") {
		t.Errorf("expected grid-row 11/13 (offset by all-day strip); got %q", got)
	}
}

func TestDayTimedEventStyle_NoAllDayOffset(t *testing.T) {
	startH := 9
	endH := 11
	e := Event{StartHour: &startH, EndHour: &endH}
	got := dayTimedEventStyle(e, &Calendar{HoursPerDay: 24})
	// Day view: no all-day-strip row; hour 9 → row 9+1 = 10.
	if !strings.Contains(got, "grid-row: 10 / 12") {
		t.Errorf("expected grid-row 10/12 (no offset); got %q", got)
	}
}

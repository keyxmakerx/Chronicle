// calendar_v2_closer_test.go covers V2 Wave 1 PR 5 closer additions:
// multi-day ribbon stacking + cross-row continuity + era band layout
// + ribbon class + week-row math. The pointer-event / popover JS
// surfaces are covered by manual test plan in the status report.

package calendar

import (
	"strings"
	"testing"
)

// --- Ribbon range / continuity ---

func TestRibbonRangeInMonth_SameMonth(t *testing.T) {
	endDay := 17
	e := Event{Year: 100, Month: 3, Day: 15, EndDay: &endDay, Visibility: "everyone"}
	s, en := ribbonRangeInMonth(e, 31)
	if s != 15 || en != 17 {
		t.Errorf("same-month range = (%d, %d); want (15, 17)", s, en)
	}
}

func TestRibbonRangeInMonth_EndsAfterMonth(t *testing.T) {
	endMonth := 4
	endDay := 5
	e := Event{Year: 100, Month: 3, Day: 28, EndMonth: &endMonth, EndDay: &endDay, Visibility: "everyone"}
	s, en := ribbonRangeInMonth(e, 31)
	if s != 28 || en != 31 {
		t.Errorf("cross-month range should clamp to dim; got (%d, %d); want (28, 31)", s, en)
	}
}

func TestRibbonStartsBeforeMonth(t *testing.T) {
	e := Event{Year: 100, Month: 2, Day: 28}
	if !ribbonStartsBeforeMonth(e, 100, 3) {
		t.Error("event in Feb should start before March")
	}
	if ribbonStartsBeforeMonth(e, 100, 1) {
		t.Error("event in Feb should not start before January")
	}
}

func TestRibbonEndsAfterMonth_CrossesMonth(t *testing.T) {
	endMonth := 4
	e := Event{Year: 100, Month: 3, Day: 30, EndMonth: &endMonth}
	if !ribbonEndsAfterMonth(e, 100, 3) {
		t.Error("event ending in April should be after March")
	}
}

func TestRibbonIntersectsVisibleMonth_TouchesEdge(t *testing.T) {
	endMonth := 3
	e := Event{Year: 100, Month: 2, Day: 28, EndMonth: &endMonth}
	if !ribbonIntersectsVisibleMonth(e, 100, 3) {
		t.Error("event ending in March should intersect March view")
	}
}

// --- Week-row math ---

func TestMonthWeekRowCount_SevenDayWeek(t *testing.T) {
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 31}},
			Weekdays: []Weekday{{}, {}, {}, {}, {}, {}, {}}, // 7
		},
		Month: 1,
	}
	got := monthWeekRowCount(data)
	if got != 5 { // 31 / 7 = 4.4 → ceil to 5
		t.Errorf("31-day month with 7-day week = %d rows; want 5", got)
	}
}

func TestMonthWeekRowCount_FantasyTenDayWeek(t *testing.T) {
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 30}},
			Weekdays: make([]Weekday, 10),
		},
		Month: 1,
	}
	got := monthWeekRowCount(data)
	if got != 3 {
		t.Errorf("30-day month with 10-day week = %d rows; want 3", got)
	}
}

func TestDaysInRow_FillsTrailingZerosOnPartialRow(t *testing.T) {
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 31}},
			Weekdays: make([]Weekday, 7),
		},
		Month: 1,
	}
	// Last row (4) holds days 29-31 + 4 filler zeros.
	row := daysInRow(data, 4)
	if len(row) != 7 {
		t.Fatalf("row should always have len = cols; got %d", len(row))
	}
	if row[0] != 29 || row[1] != 30 || row[2] != 31 || row[3] != 0 {
		t.Errorf("last-row pattern wrong; got %+v", row)
	}
}

// --- Ribbon row stacking ---

func TestMonthRibbonRows_StacksOverlapping(t *testing.T) {
	endDay := 5
	e1 := Event{ID: "a", Year: 100, Month: 1, Day: 1, EndDay: &endDay, Visibility: "everyone"}
	e2 := Event{ID: "b", Year: 100, Month: 1, Day: 3, EndDay: &endDay, Visibility: "everyone"}
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 31}},
			Weekdays: make([]Weekday, 7),
		},
		Year:   100,
		Month:  1,
		Events: []Event{e1, e2},
	}
	rows := monthRibbonRows(data)
	if len(rows) == 0 {
		t.Fatal("expected ribbon rows")
	}
	// Both events overlap in week row 0; should stack on slots 0 and 1.
	row0 := rows[0]
	if len(row0) != 2 {
		t.Fatalf("expected 2 segments in row 0; got %d", len(row0))
	}
	slotSet := map[int]bool{row0[0].StackRow: true, row0[1].StackRow: true}
	if !slotSet[0] || !slotSet[1] {
		t.Errorf("expected stack slots 0 and 1; got %+v", slotSet)
	}
}

func TestMonthRibbonRows_FiltersNonMatchingMonth(t *testing.T) {
	endMonth := 2
	endDay := 5
	e := Event{ID: "z", Year: 100, Month: 1, Day: 15, EndMonth: &endMonth, EndDay: &endDay, Visibility: "everyone"}
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months: []Month{{Days: 31}, {Days: 28}, {Days: 31}},
			Weekdays: make([]Weekday, 7),
		},
		Year:   100,
		Month:  3,
		Events: []Event{e},
	}
	rows := monthRibbonRows(data)
	// March view: event ended Feb 5; should not appear.
	for _, r := range rows {
		if len(r) != 0 {
			t.Errorf("non-intersecting event should not render in March; got %+v", r)
		}
	}
}

func TestMonthRibbonRows_CrossesMonthBoundaryFlatCutsLeft(t *testing.T) {
	endMonth := 2
	endDay := 5
	e := Event{ID: "x", Year: 100, Month: 1, Day: 28, EndMonth: &endMonth, EndDay: &endDay, Visibility: "everyone"}
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months: []Month{{Days: 31}, {Days: 28}, {Days: 31}},
			Weekdays: make([]Weekday, 7),
		},
		Year:   100,
		Month:  2, // viewing Feb
		Events: []Event{e},
	}
	rows := monthRibbonRows(data)
	if len(rows) == 0 || len(rows[0]) == 0 {
		t.Fatalf("expected segment in Feb view for Jan-Feb event")
	}
	seg := rows[0][0]
	if !seg.OpenLeft {
		t.Error("Feb-view segment of Jan-started event should be OpenLeft (flat-cut)")
	}
}

// --- Ribbon classes ---

func TestRibbonClasses_OpenLeftDropsRoundedLeft(t *testing.T) {
	seg := monthRibbonSegment{Tier: "standard", OpenLeft: true, OpenRight: false}
	got := ribbonClasses(seg)
	if !strings.Contains(got, "rounded-l-none") || !strings.Contains(got, "rounded-r-md") {
		t.Errorf("OpenLeft segment classes = %q", got)
	}
}

func TestRibbonClasses_MajorTierAccents(t *testing.T) {
	seg := monthRibbonSegment{Tier: "major"}
	got := ribbonClasses(seg)
	if !strings.Contains(got, "ring-accent") {
		t.Errorf("major-tier ribbon should ring-accent; got %q", got)
	}
}

func TestRibbonClasses_MinorTierFades(t *testing.T) {
	seg := monthRibbonSegment{Tier: "minor"}
	got := ribbonClasses(seg)
	if !strings.Contains(got, "opacity-40") {
		t.Errorf("minor-tier ribbon should fade; got %q", got)
	}
}

func TestRibbonInlineStyle_ComposesGridColumnAndTint(t *testing.T) {
	seg := monthRibbonSegment{StartCol: 3, Span: 2, Color: "#ff0000"}
	got := ribbonInlineStyle(seg)
	if !strings.Contains(got, "grid-column: 3 / span 2") {
		t.Errorf("expected grid-column composition; got %q", got)
	}
	if !strings.Contains(got, "#ff0000") {
		t.Errorf("expected tint with category color; got %q", got)
	}
}

// --- Era bands ---

func TestMonthEraBands_OngoingEra(t *testing.T) {
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 31}},
			Weekdays: make([]Weekday, 7),
			Eras:     []Era{{Name: "Age of Magic", StartYear: 50, Color: "#ff8800"}},
		},
		Year:  100,
		Month: 1,
	}
	bands := monthEraBands(data)
	if len(bands) == 0 {
		t.Fatal("ongoing era covering year 100 should produce bands")
	}
}

func TestMonthEraBands_OutOfRangeFilters(t *testing.T) {
	endYear := 99
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 31}},
			Weekdays: make([]Weekday, 7),
			Eras:     []Era{{Name: "Past Era", StartYear: 50, EndYear: &endYear, Color: "#888"}},
		},
		Year:  100,
		Month: 1,
	}
	bands := monthEraBands(data)
	if len(bands) != 0 {
		t.Errorf("past era should not render in year 100; got %+v", bands)
	}
}

func TestEraBandStyle_ComposesGridAndTint(t *testing.T) {
	band := eraBand{Name: "Era", Color: "#abcdef", StartCol: 1, Span: 7}
	got := eraBandStyle(band)
	if !strings.Contains(got, "grid-column: 1 / span 7") {
		t.Errorf("expected grid-column; got %q", got)
	}
	if !strings.Contains(got, "#abcdef") {
		t.Errorf("expected operator color; got %q", got)
	}
}

// --- monthDayFor mirroring ---

func TestMonthDayFor_DetectsTodayAndRestDay(t *testing.T) {
	data := CalendarV2ViewData{
		Year: 100, Month: 1,
		ActiveCalendar: &Calendar{
			CurrentYear: 100, CurrentMonth: 1, CurrentDay: 7,
			Months:   []Month{{Days: 31}},
			Weekdays: []Weekday{{}, {}, {}, {}, {}, {}, {IsRestDay: true}},
		},
	}
	today := monthDayFor(data, 7)
	if !today.IsToday {
		t.Error("day 7 should be marked today")
	}
	if !today.IsRestDay {
		t.Error("day 7 falls on the rest day in this 7-day week")
	}
	other := monthDayFor(data, 3)
	if other.IsToday {
		t.Error("day 3 should not be today")
	}
}

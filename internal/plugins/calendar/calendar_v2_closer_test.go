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
	// Year 0, day 1 → absolute day 1 → weekday offset 1 (the 1st sits in
	// column 1). 30 days + 1 leading blank = 31 cells over a 10-day week = 4
	// rows (C-CAL-V2-MONTH-GRID-ALIGN-FIX: the offset can spill an extra row).
	got := monthWeekRowCount(data)
	if got != 4 {
		t.Errorf("30-day month (10-day week, lead offset 1) = %d rows; want 4", got)
	}
}

func TestDaysInRow_AppliesLeadingOffsetAndTrailingZeros(t *testing.T) {
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			Months:   []Month{{Days: 31}},
			Weekdays: make([]Weekday, 7),
		},
		Month: 1,
	}
	// Lead offset 1 (day 1 of year 0 → absolute day 1 → column 1). Row 0 must
	// start with ONE leading blank, then day 1.
	first := daysInRow(data, 0)
	if len(first) != 7 {
		t.Fatalf("row len must = cols; got %d", len(first))
	}
	if first[0] != 0 || first[1] != 1 || first[6] != 6 {
		t.Errorf("row 0 should be [0,1,2,3,4,5,6] with the leading blank; got %+v", first)
	}
	// With offset 1, day d sits at grid position d → row 4 holds days 28-31
	// then trailing blanks.
	row := daysInRow(data, 4)
	if row[0] != 28 || row[1] != 29 || row[2] != 30 || row[3] != 31 || row[4] != 0 {
		t.Errorf("last-row pattern wrong; got %+v", row)
	}
}

// gregorian2026 builds the real-life Gregorian fixture from the dispatch:
// June 2026, where the 1st is a Monday and the 8th (today) is a Monday.
func gregorian2026() *Calendar {
	return &Calendar{
		ID:           "greg",
		CurrentYear:  2026,
		CurrentMonth: 6,
		CurrentDay:   8,
		Months: []Month{
			{Name: "January", Days: 31}, {Name: "February", Days: 28},
			{Name: "March", Days: 31}, {Name: "April", Days: 30},
			{Name: "May", Days: 31}, {Name: "June", Days: 30},
			{Name: "July", Days: 31}, {Name: "August", Days: 31},
			{Name: "September", Days: 30}, {Name: "October", Days: 31},
			{Name: "November", Days: 30}, {Name: "December", Days: 31},
		},
		Weekdays: []Weekday{
			{Name: "Sun"}, {Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"},
			{Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"},
		},
	}
}

// TestMonthGrid_WeekdayAlignment_June2026 is the headline regression
// (C-CAL-V2-MONTH-GRID-ALIGN-FIX #1): a month whose 1st is NOT the first
// weekday must place day 1 under its true weekday column, and a multi-day
// ribbon must align to the same corrected columns.
func TestMonthGrid_WeekdayAlignment_June2026(t *testing.T) {
	data := CalendarV2ViewData{ActiveCalendar: gregorian2026(), Year: 2026, Month: 6, Day: 8}

	// June 1 + June 8 2026 are both Mondays → weekday index 1 (Sun=0).
	if off := v2MonthLeadOffset(data); off != 1 {
		t.Fatalf("June 2026 lead offset = %d; want 1 (the 1st is a Monday)", off)
	}
	if idx := v2WeekdayIndex(data, 8); idx != 1 {
		t.Errorf("the 8th should be weekday index 1 (Monday); got %d", idx)
	}

	// Row 0 has ONE leading blank, then day 1 under Monday (column index 1).
	r0 := daysInRow(data, 0)
	if r0[0] != 0 || r0[1] != 1 {
		t.Errorf("row 0 should be [0,1,...] (day 1 under Monday); got %+v", r0)
	}
	// The 8th sits in the next row, again under Monday (column index 1).
	r1 := daysInRow(data, 1)
	if r1[1] != 8 {
		t.Errorf("the 8th should be in column index 1 (Monday) of row 1; got %+v", r1)
	}

	// A multi-day ribbon (June 8–10) must start in the Monday column. StartCol
	// is 1-indexed, so the Monday column (index 1) is StartCol 2 — the same
	// column the day-8 cell occupies.
	endDay := 10
	data.Events = []Event{{ID: "war", Year: 2026, Month: 6, Day: 8, EndDay: &endDay, Visibility: "everyone"}}
	rows := monthRibbonRows(data)
	var seg *monthRibbonSegment
	for ri := range rows {
		for si := range rows[ri] {
			if rows[ri][si].EventID == "war" {
				seg = &rows[ri][si]
			}
		}
	}
	if seg == nil {
		t.Fatal("expected a ribbon segment for the June 8–10 event")
	}
	if seg.StartCol != 2 {
		t.Errorf("ribbon should start in the Monday column (StartCol 2, aligned with day 8); got %d", seg.StartCol)
	}

	// Mini-month: same leading blank so day 1 lands under Monday.
	mini := miniMonthDays(data)
	if len(mini) < 2 || mini[0].Day != 0 || mini[1].Day != 1 {
		t.Errorf("mini-month should lead with one blank then day 1; got %+v", mini[:min(2, len(mini))])
	}
}

// TestMonthDayClasses_TodayNotBlank (C-CAL-V2-MONTH-GRID-ALIGN-FIX #2): the
// today cell must NOT carry `today-pulse` — that keyframe ends at opacity:0 with
// fill-mode:both, which (applied to the cell) made the today cell render blank.
// The static ring/tint marks today while keeping the number legible.
func TestMonthDayClasses_TodayNotBlank(t *testing.T) {
	cls := monthDayClasses(CalendarV2ViewData{}, monthDay{Day: 8, IsToday: true})
	if strings.Contains(cls, "today-pulse") {
		t.Errorf("today cell must not use today-pulse (fades to opacity:0); got %q", cls)
	}
	if !strings.Contains(cls, "ring-accent") {
		t.Errorf("today cell should still carry a visible marker; got %q", cls)
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

// calendar_v2_design_pass1_test.go — C-CAL-DESIGN-PASS-1 (calendar month view:
// command bar · pips · uniform grid · touch). Screenshot checks aren't
// available in this harness, so the operator's alignment rule and the redesign
// contract are encoded as DOM/CSS assertions on the rendered templ output.
package calendar

import (
	"context"
	"strings"
	"testing"
)

// designPass1Data builds an active fantasy calendar for the command bar + month
// grid: two months, a 7-day week, an era (for the fantasy subline), and two
// event categories (colored pips). Cursor = Harvestwane 13, 1492 (today).
func designPass1Data(view string, events []Event) CalendarV2ViewData {
	epoch := "of the Broken Lantern"
	eraEnd := 1500
	cal := &Calendar{
		ID:   "cal-1",
		Name: "The Sunless Reaches",
		Months: []Month{
			{Name: "Harvestwane", Days: 30},
			{Name: "Frostmere", Days: 30},
		},
		Weekdays: []Weekday{
			{Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"}, {Name: "Thu"},
			{Name: "Fri"}, {Name: "Sat"}, {Name: "Sun"},
		},
		EpochName:    &epoch,
		CurrentYear:  1492,
		CurrentMonth: 1,
		CurrentDay:   13,
		Eras: []Era{
			{Name: "Age of the Broken Lantern", StartYear: 1400, EndYear: &eraEnd, Color: "#6366f1"},
		},
		EventCategories: []EventCategory{
			{Slug: "social", Name: "Session", Color: "#2a78d6"},
			{Slug: "festival", Name: "Festival", Color: "#eda100"},
		},
	}
	return CalendarV2ViewData{
		ActiveCalendar: cal,
		AllCalendars:   []Calendar{*cal},
		View:           view,
		Year:           1492,
		Month:          1,
		Day:            13,
		CampaignID:     "camp-1",
		Events:         events,
	}
}

func timedEvent(id, name string, day, hour, min int) Event {
	cat := "social"
	h, m := hour, min
	return Event{ID: id, Name: name, Year: 1492, Month: 1, Day: day, StartHour: &h, StartMinute: &m, Category: &cat, Visibility: "everyone"}
}

func allDayEvent(id, name string, day int) Event {
	cat := "festival"
	return Event{ID: id, Name: name, Year: 1492, Month: 1, Day: day, Category: &cat, Visibility: "everyone"}
}

func renderHeader(t *testing.T, data CalendarV2ViewData) string {
	t.Helper()
	var sb strings.Builder
	if err := calendarV2Header(nil, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render command bar: %v", err)
	}
	return sb.String()
}

func renderCell(t *testing.T, data CalendarV2ViewData, day int) string {
	t.Helper()
	var sb strings.Builder
	if err := monthDayCell(data, monthDayFor(data, day)).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render month cell: %v", err)
	}
	return sb.String()
}

// --- Command bar (§1) -------------------------------------------------------

// TestCommandBar_OwnsNavLabelAndClusters: the sticky command bar carries the
// period nav (prev/Today/next), the period label + fantasy era subline, the
// view pills, the RT-clock slot, and the calendar switcher — the whole header
// surface the dispatch put here.
func TestCommandBar_OwnsNavLabelAndClusters(t *testing.T) {
	html := renderHeader(t, designPass1Data("month", nil))
	for _, want := range []string{
		`aria-label="Previous month"`,     // prev
		`aria-label="Next month"`,         // next
		`aria-label="Today"`,              // Today (the `t` shortcut targets this)
		">Today</a>",                      // the Today control itself
		"Harvestwane 1492 of the Broken Lantern", // period heading
		"Age of the Broken Lantern",       // fantasy era subline
		`aria-label="Calendar view"`,      // the view-pill tablist
		"The Sunless Reaches",             // calendar switcher label
		"sticky",                          // the bar is sticky
		"top-0",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("command bar missing %q", want)
		}
	}
}

// TestCommandBar_NavLabelsTrackView: the nav aria-labels are the keyboard
// shortcut contract (clickByLabel in calendar_v2_shell.js). They must name the
// active view's period unit so j/k step the right amount in every view.
func TestCommandBar_NavLabelsTrackView(t *testing.T) {
	cases := map[string]string{
		"month":  "month",
		"week":   "week",
		"day":    "day",
		"ledger": "year",
	}
	for view, unit := range cases {
		html := renderHeader(t, designPass1Data(view, nil))
		for _, want := range []string{`aria-label="Previous ` + unit + `"`, `aria-label="Next ` + unit + `"`} {
			if !strings.Contains(html, want) {
				t.Errorf("view %q: command bar nav missing %q (breaks the j/k shortcut hook)", view, want)
			}
		}
	}
}

// TestCommandBar_SharedGutterNotMx6: §5 — the bar adopts the one shared
// max-w-7xl gutter (the #539 precedent) instead of the old floating-card mx-6.
func TestCommandBar_SharedGutterNotMx6(t *testing.T) {
	html := renderHeader(t, designPass1Data("month", nil))
	if !strings.Contains(html, "max-w-7xl") || !strings.Contains(html, "mx-auto") {
		t.Errorf("command bar should use the shared max-w-7xl mx-auto gutter; got:\n%s", html)
	}
	if strings.Contains(html, "mx-6") {
		t.Errorf("command bar must not keep the mismatched mx-6 inset")
	}
}

// TestCommandBar_WiresRealTimeClockUnchanged: the RT clock is kept EXACTLY as
// is (dispatch §1) — the header still calls @realTimeClock, so real-time
// calendars keep their live clock and fantasy calendars render nothing.
func TestCommandBar_WiresRealTimeClockUnchanged(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/calendar_v2.templ")
	// The call must live inside the command bar now.
	hdr := src[strings.Index(src, "templ calendarV2Header"):]
	hdr = hdr[:strings.Index(hdr, "\ntempl ")]
	if !strings.Contains(hdr, "@realTimeClock(data.ActiveCalendar)") {
		t.Errorf("command bar must still call @realTimeClock(data.ActiveCalendar) (kept exactly as is)")
	}
}

// --- Month cell pips + titles (§2) ------------------------------------------

// TestMonthCell_TimedEventPipLine: a timed event renders as a pip + a
// time-prefixed one-line title, carrying the SAME hooks the compact card did
// (data-event-card / data-event-id / draggable) so click→edit + drag survive.
func TestMonthCell_TimedEventPipLine(t *testing.T) {
	data := designPass1Data("month", []Event{timedEvent("evt-1", "Session 12", 3, 19, 0)})
	html := renderCell(t, data, 3)
	for _, want := range []string{
		"19:00 Session 12",                    // start-time prefix + name
		"background-color: #2a78d6",           // pip color from the SAME projection
		`data-event-card="compact"`,           // click→quick-edit hook
		`data-event-id="evt-1"`,               // identity hook
		`draggable="true"`,                    // drag hook
	} {
		if !strings.Contains(html, want) {
			t.Errorf("timed pip line missing %q\n%s", want, html)
		}
	}
}

// TestMonthCell_AllDayChip: an all-day (untimed) event renders as a tinted chip
// (left accent bar + color-mix fill), no pip, same interaction hooks.
func TestMonthCell_AllDayChip(t *testing.T) {
	data := designPass1Data("month", []Event{allDayEvent("evt-2", "Founding Day", 1)})
	html := renderCell(t, data, 1)
	for _, want := range []string{
		"Founding Day",
		"border-left: 3px solid #eda100",             // chip left bar in the type color
		"color-mix(in srgb, #eda100 14%, transparent)", // tinted fill
		`data-event-card="compact"`,
		`data-event-id="evt-2"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("all-day chip missing %q\n%s", want, html)
		}
	}
	// An all-day chip has no pip dot.
	if strings.Contains(html, `class="w-1.5 h-1.5 rounded-sm flex-none"`) {
		t.Errorf("all-day chip should not render a pip dot")
	}
}

// TestMonthCell_OverflowPreserved: >cap events keep the "+N more" overflow
// toggle (endpoint/behavior from PR #366) — 3 lines rendered, N hidden.
func TestMonthCell_OverflowPreserved(t *testing.T) {
	evs := []Event{
		timedEvent("a", "One", 16, 9, 0),
		timedEvent("b", "Two", 16, 10, 0),
		timedEvent("c", "Three", 16, 11, 0),
		timedEvent("d", "Four", 16, 12, 0),
		timedEvent("e", "Five", 16, 13, 0),
	}
	data := designPass1Data("month", evs)
	html := renderCell(t, data, 16)
	for _, want := range []string{
		`data-cell-overflow-toggle="true"`,
		`data-cell-overflow-day="16"`,
		"+2 more", // 5 events - cap(3) = 2 hidden
	} {
		if !strings.Contains(html, want) {
			t.Errorf("overflow affordance missing %q", want)
		}
	}
	if n := strings.Count(html, `data-event-card="compact"`); n != monthCellVisibleCap() {
		t.Errorf("expected exactly %d visible event lines (cap); got %d", monthCellVisibleCap(), n)
	}
}

// TestMonthCell_DayCellHooksPreserved: the cell keeps the tap-to-popover +
// drag-create hooks (data-day-cell, cell-drop-target, data-cell-*).
func TestMonthCell_DayCellHooksPreserved(t *testing.T) {
	html := renderCell(t, designPass1Data("month", nil), 5)
	for _, want := range []string{
		`data-day-cell="true"`,
		"cell-drop-target",
		`data-cell-day="5"`,
		`data-cell-month="1"`,
		`data-cell-year="1492"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("day cell missing hook %q", want)
		}
	}
}

// TestMonthCell_ScribeAddAffordanceReservedAndTouch: on an empty cell a Scribe
// gets the add affordance in a RESERVED absolute layer (can't nudge the number)
// that is touch-revealed (cal-touch-reveal, §4). Players never get it.
func TestMonthCell_ScribeAddAffordanceReservedAndTouch(t *testing.T) {
	scribe := designPass1Data("month", nil)
	scribe.IsScribe = true
	html := renderCell(t, scribe, 7)
	for _, want := range []string{
		`data-cell-add-event="true"`,
		"cal-touch-reveal", // touch: visible at rest under pointer:coarse
		"absolute",         // reserved layer — doesn't affect the number box
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scribe add affordance missing %q", want)
		}
	}
	player := renderCell(t, designPass1Data("month", nil), 7)
	if strings.Contains(player, "data-cell-add-event") {
		t.Errorf("non-scribe must not receive the add affordance (server-gated markup)")
	}
}

// --- Grid uniformity / fixed number box (§3) --------------------------------

// TestMonthGrid_UniformRowsOneGrid: the week rows live in ONE grid with 1fr
// auto-rows (equal-height rows) and the cell layer stretches to fill — the
// operator's alignment rule, verified against a live headless render in the
// scratchpad geometry probe (all rows equal height, cells fill).
func TestMonthGrid_UniformRowsOneGrid(t *testing.T) {
	var sb strings.Builder
	if err := monthWeekRows(designPass1Data("month", nil)).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render week rows: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "grid-auto-rows: 1fr") {
		t.Errorf("week rows must share ONE grid with 1fr auto-rows (equal-height rows)")
	}
	if !strings.Contains(html, "content-stretch") || !strings.Contains(html, "flex-1") {
		t.Errorf("the cell layer must stretch (flex-1 + content-stretch) so cells fill the equalized row")
	}
}

// TestMonthCell_NumberBoxFixedAcrossStates: EVERY day number lives in the same
// fixed 22px line box; today only REPAINTS it into a pill (adds the accent
// fill + a fixed 22px width) — the box height is identical for plain, today,
// and event-bearing cells, so state changes never resize or shift the number.
func TestMonthCell_NumberBoxFixedAcrossStates(t *testing.T) {
	plain := renderCell(t, designPass1Data("month", nil), 5)                                   // ordinary
	today := renderCell(t, designPass1Data("month", nil), 13)                                  // today (cursor day)
	withEvent := renderCell(t, designPass1Data("month", []Event{timedEvent("z", "X", 5, 9, 0)}), 5) // has an event

	for name, html := range map[string]string{"plain": plain, "today": today, "withEvent": withEvent} {
		if !strings.Contains(html, "h-[22px]") {
			t.Errorf("%s cell: day number must sit in the fixed 22px line box (h-[22px])", name)
		}
	}
	// Today repaints to an accent pill; plain/event cells do not.
	if !strings.Contains(today, "rounded-full") || !strings.Contains(today, "bg-accent") {
		t.Errorf("today number must repaint into an accent pill")
	}
	if strings.Contains(plain, "rounded-full") {
		t.Errorf("a non-today number must not carry the pill (would imply a different box)")
	}
}

// TestCommandBar_TodayButtonDistinctFromCellClasses: the mockup's `.today`
// button-vs-cell collision must not recur — the command bar's Today control
// and the today NUMBER pill must not share a class that would style both.
func TestCommandBar_TodayButtonDistinctFromCellClasses(t *testing.T) {
	todayNum := monthDayNumberClasses(monthDay{Day: 13, IsToday: true})
	header := renderHeader(t, designPass1Data("month", nil))
	// The Today control is a bordered pill; the today NUMBER is an accent-filled
	// circle. They must not both hinge on one shared class token.
	if strings.Contains(todayNum, "border ") {
		t.Errorf("today number pill should not borrow the Today button's border pill classes")
	}
	// Sanity: the header's Today control exists and is bordered, distinct styling.
	if !strings.Contains(header, ">Today</a>") {
		t.Errorf("command bar Today control missing")
	}
}

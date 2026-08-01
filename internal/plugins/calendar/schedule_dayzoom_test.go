package calendar

// schedule_dayzoom_test.go — DAY ZOOM (C-CALV4-RSVP-P8 Part B, stage 17).
//
// The Day rung shipped as a live control that pressed and changed nothing:
// ?zoom=day round-tripped the query, inked `aria-current` and returned the
// identical week matrix. These tests exist so that cannot happen again quietly
// — every one of them fails on a build where the zoom is decorative.
//
// THE ORACLE IS THE SEALED SHEET, at ?view=schedule&zoom=day: eight hour columns
// for the evening band, the heavier rule on the sixth hour after the band's
// first, per-hour counts rather than the day's peak, and the bracket spanning
// the ranked window's HOURS. Every number below was read off the drawing.

import (
	"strings"
	"testing"
)

// scheduleDayInput is the oracle fixture pointed at DAY zoom on the week's
// Saturday — the day the route's own scheduleResolveDay defaults to and the day
// the drawing draws.
func scheduleDayInput(isGM bool) scheduleBuildInput {
	in := scheduleOracleInput(isGM)
	in.Zoom = "day"
	in.Day = scheduleResolveDay("", scheduleOracleWeek())
	return in
}

// TestScheduleDayZoom_ColumnsAreOneDaysHours is the finding itself, inverted:
// the two zooms must not produce the same column set.
func TestScheduleDayZoom_ColumnsAreOneDaysHours(t *testing.T) {
	week := scheduleBuildMatrix(scheduleOracleInput(true))
	day := scheduleBuildMatrix(scheduleDayInput(true))

	if len(week.Cols) != 7 {
		t.Fatalf("week zoom: want 7 day columns, got %d", len(week.Cols))
	}
	if len(day.Cols) != 8 {
		t.Fatalf("day zoom: want 8 hour columns for the 16–24 band, got %d", len(day.Cols))
	}

	wantHeads := []string{"16", "17", "18", "19", "20", "21", "22", "23"}
	for i, c := range day.Cols {
		if c.Head != wantHeads[i] {
			t.Errorf("column %d head = %q, want %q", i, c.Head, wantHeads[i])
		}
		if c.Sub != "" {
			t.Errorf("column %d carries a day-number sub %q; an hour column has none", i, c.Sub)
		}
		if c.DayKey != "2026-07-25" {
			t.Errorf("column %d data-day = %q, want the selected date on EVERY hour "+
				"column (guard B4: an hour of Saturday is still Saturday)", i, c.DayKey)
		}
		if c.StartMinute != (16+i)*60 || c.EndMinute != (17+i)*60 {
			t.Errorf("column %d spans %d–%d, want one hour", i, c.StartMinute, c.EndMinute)
		}
		// The drawing's `(h - lo) % 6 === 0 && h !== lo`: 22:00 and nothing else
		// in this band.
		wantMajor := c.Head == "22"
		if c.Major != wantMajor {
			t.Errorf("column %d (%s) major = %v, want %v", i, c.Head, c.Major, wantMajor)
		}
	}
}

// TestScheduleDayZoom_TheCountIsTheHoursOwnNotTheDaysPeak. The count lane is the
// one place the two zooms mean different arithmetic, and the drawing drops the
// `@ h` sub with it — a `@ 19` under a column headed `19` is one fact twice.
func TestScheduleDayZoom_TheCountIsTheHoursOwnNotTheDaysPeak(t *testing.T) {
	in := scheduleDayInput(true)
	m := scheduleBuildMatrix(in)
	members := scheduleMembers(in)

	// Read off the sealed sheet at ?zoom=day, hours 16..23.
	want := []int{0, 1, 2, 4, 4, 3, 3, 2}
	if len(m.Counts) != len(want) {
		t.Fatalf("count lane has %d columns, want %d", len(m.Counts), len(want))
	}
	for i, c := range m.Counts {
		if c.Free != want[i] {
			t.Errorf("hour %d count = %d, want %d (the drawing's own numeral)", 16+i, c.Free, want[i])
		}
		if got := scheduleFreeCount(in, members, 5, 16+i); got != c.Free {
			t.Errorf("hour %d: lane prints %d, scheduleFreeCount says %d — the two "+
				"readings of one fact disagree", 16+i, c.Free, got)
		}
		if c.PeakHour != "" {
			t.Errorf("hour %d prints the peak sub %q; the drawing emits the numeral alone "+
				"in day zoom", 16+i, c.PeakHour)
		}
	}
	if len(m.Density) != len(want) {
		t.Fatalf("density lane has %d columns, want %d", len(m.Density), len(want))
	}
	for i, d := range m.Density {
		if d.Free != want[i] {
			t.Errorf("hour %d density = %d, want %d", 16+i, d.Free, want[i])
		}
	}
	// 19:00 and 20:00 both hold the top count, and both wear `peak` — the
	// drawing marks every column that ties for the maximum.
	peaks := 0
	for _, c := range m.Counts {
		if c.Peak {
			peaks++
		}
	}
	if peaks != 2 {
		t.Errorf("%d peak columns, want 2 (19:00 and 20:00 tie at 4)", peaks)
	}

	// WEEK zoom keeps its sub, or the reader cannot tell WHEN the day's peak was.
	for _, c := range scheduleBuildMatrix(scheduleOracleInput(true)).Counts {
		if !strings.HasPrefix(c.PeakHour, "@ ") {
			t.Fatalf("week zoom lost its `@ h` sub: %q", c.PeakHour)
		}
	}
}

// TestScheduleDayZoom_BracketSpansTheWindowsHours. In week zoom the bracket is
// one day wide; in day zoom it is the window's own three hours.
func TestScheduleDayZoom_BracketSpansTheWindowsHours(t *testing.T) {
	m := scheduleBuildMatrix(scheduleDayInput(true))
	if m.Bracket == nil {
		t.Fatal("no bracket in day zoom; the ranked window is on the day on screen")
	}
	// 19:00 is column index 3, so grid lines 5 → 8 for a three-hour window.
	if m.Bracket.Start != 5 || m.Bracket.End != 8 {
		t.Errorf("bracket spans lines %d–%d, want 5–8", m.Bracket.Start, m.Bracket.End)
	}
	if m.Bracket.Label != "19:00–22:00" {
		t.Errorf("bracket label = %q, want %q", m.Bracket.Label, "19:00–22:00")
	}
}

// TestScheduleDayZoom_BracketRefusesADayItIsNotOn. The bracket is the page's
// ANSWER and it wears --accent, which is the tool talking. Drawn over a day the
// numbers never chose it would be the tool saying "play here" about a Wednesday.
func TestScheduleDayZoom_BracketRefusesADayItIsNotOn(t *testing.T) {
	in := scheduleDayInput(true)
	in.Day = "2026-07-22" // Wednesday; rank 1 is Saturday's 19:00
	m := scheduleBuildMatrix(in)
	if m.Bracket != nil {
		t.Fatalf("bracket drawn on %s, where the ranked window is not: %+v", in.Day, m.Bracket)
	}
	// The rest of the day view still renders — refusing the bracket is not
	// refusing the day.
	if len(m.Cols) != 8 || m.Cols[0].DayKey != "2026-07-22" {
		t.Fatalf("the day view collapsed with its bracket: %d cols, first key %q",
			len(m.Cols), m.Cols[0].DayKey)
	}
}

// TestScheduleDayZoom_PopoverIDsAreUniquePerColumn is the defect that keying on
// the date would have shipped: eight hour columns share one date, so eight cells
// in a row would all open the 16:00 popover.
func TestScheduleDayZoom_PopoverIDsAreUniquePerColumn(t *testing.T) {
	m := scheduleBuildMatrix(scheduleDayInput(true))
	seen := map[string]int{}
	for _, p := range m.Pops {
		seen[p.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("popover id %q minted %d times — `popovertarget` would open the "+
				"same detail from every hour in the row", id, n)
		}
	}
	// And the cells point at ids that exist.
	ids := map[string]bool{}
	for _, p := range m.Pops {
		ids[p.ID] = true
	}
	targets := 0
	for _, l := range m.Lanes {
		for _, c := range l.Cells {
			if c.PopID == "" {
				continue
			}
			targets++
			if !ids[c.PopID] {
				t.Errorf("cell targets popover %q, which is not built", c.PopID)
			}
		}
	}
	if targets == 0 {
		t.Fatal("no cell targets a popover at all in day zoom")
	}

	// AND NOTHING IS BUILT THAT NOTHING OPENS. A per-cell popover keyed on the
	// day rather than the column would be minted for all eight hours of a lane
	// that only draws marks in five of them.
	targeted := map[string]bool{}
	for _, l := range m.Lanes {
		for _, c := range l.Cells {
			if c.PopID != "" {
				targeted[c.PopID] = true
			}
		}
	}
	for _, c := range m.Counts {
		if c.PopID != "" {
			targeted[c.PopID] = true
		}
	}
	for _, p := range m.Pops {
		if !targeted[p.ID] {
			t.Errorf("popover %q is built and nothing opens it", p.ID)
		}
	}
}

// TestScheduleDayZoom_HeadNamesOneDay. The frame is the only line that says
// which dates are on screen; a week range over eight hour columns would be the
// head contradicting the grid beneath it.
func TestScheduleDayZoom_HeadNamesOneDay(t *testing.T) {
	got := scheduleBuildMatrix(scheduleDayInput(true)).Frame
	const want = "Sat 25 Jul · 16:00–24:00 · times in Chicago"
	if got != want {
		t.Errorf("day-zoom frame = %q, want %q", got, want)
	}
	weekFrame := scheduleBuildMatrix(scheduleOracleInput(true)).Frame
	if weekFrame == got {
		t.Error("the week frame and the day frame are the same sentence")
	}
}

// TestScheduleDayZoom_CellLabelsCarryTheHour. A screen-reader user gets the
// column's meaning from the accessible name, and in day zoom that meaning
// includes WHICH HOUR — the visual reader gets it from the column head.
func TestScheduleDayZoom_CellLabelsCarryTheHour(t *testing.T) {
	m := scheduleBuildMatrix(scheduleDayInput(true))
	if len(m.Lanes) == 0 || len(m.Lanes[0].Cells) < 8 {
		t.Fatal("no lanes to read")
	}
	first := m.Lanes[0].Cells[0].Label
	if !strings.Contains(first, "Sat 25, 16:00") {
		t.Errorf("cell label %q does not name its hour", first)
	}
	// Week zoom is UNCHANGED: `Sat 25`, no hour.
	w := scheduleBuildMatrix(scheduleOracleInput(true))
	wl := w.Lanes[0].Cells[5].Label
	if !strings.Contains(wl, "Sat 25:") {
		t.Errorf("week cell label changed shape: %q", wl)
	}
}

// TestScheduleDayZoom_PlayerObeysTheSameAbsenceOracle. Permission is ABSENCE,
// and the day view is not a second permission path: a player's payload carries
// no lane and therefore no other member's name, exactly as the week matrix.
func TestScheduleDayZoom_PlayerObeysTheSameAbsenceOracle(t *testing.T) {
	in := scheduleDayInput(false)
	m := scheduleBuildMatrix(in)

	if len(m.Lanes) != 0 {
		t.Fatalf("a player's day matrix carries %d member lanes; it must carry none", len(m.Lanes))
	}
	if len(m.Pops) != 0 {
		t.Fatalf("a player's day matrix carries %d popovers; the names live in those", len(m.Pops))
	}
	for _, c := range m.Counts {
		if c.PopID != "" {
			t.Fatalf("a player's numeral is a button targeting %q — the names are "+
				"ABSENT from their DOM, not greyed", c.PopID)
		}
	}
	// The anonymous aggregates survive at both roles and AGREE with the
	// Director's, because both come from the overlay's own per-hour counts.
	gm := scheduleBuildMatrix(scheduleDayInput(true))
	if len(m.Counts) != len(gm.Counts) {
		t.Fatalf("player has %d count columns, Director %d", len(m.Counts), len(gm.Counts))
	}
	for i := range m.Counts {
		if m.Counts[i].Free != gm.Counts[i].Free {
			t.Errorf("hour %d: player reads %d free, Director reads %d",
				16+i, m.Counts[i].Free, gm.Counts[i].Free)
		}
	}

	// And the rendered page names nobody.
	html := scheduleRenderBodyZoom(t, false, "day")
	for _, name := range []string{"Kael", "Bryn", "Nissa", "Rell"} {
		if strings.Contains(html, name) {
			t.Errorf("a player's rendered day view contains %q", name)
		}
	}
}

// TestScheduleDayZoom_RefusesADayOutsideTheWeek. ?day is display state, and
// display state that names no column this week must not silently become one.
func TestScheduleDayZoom_RefusesADayOutsideTheWeek(t *testing.T) {
	in := scheduleDayInput(true)
	in.Day = "2026-09-14"
	m := scheduleBuildMatrix(in)
	if len(m.Cols) != 7 {
		t.Fatalf("a ?day outside the week produced %d columns; it must fall back to "+
			"the week the overlay actually returned", len(m.Cols))
	}
}

// TestScheduleOracle_FixtureCountsAgreeWithItsOwnLanes. The fixture's per-hour
// counts are derived rather than hand-written, because a harness that can draw
// "nobody is free at 17:00" directly above an outline at 17:00 manufactures
// defects. This pins the derivation AND the week-zoom peaks it must not move.
func TestScheduleOracle_FixtureCountsAgreeWithItsOwnLanes(t *testing.T) {
	av := scheduleOracleAvail()
	for d, day := range av.Days {
		for h := 0; h < 24; h++ {
			want := 0
			for _, lanes := range av.Lanes {
				for _, g := range lanes {
					if g.DayIndex == d && g.StartMinute <= h*60 && h*60 < g.EndMinute {
						want++
						break
					}
				}
			}
			if day.Free[h] != want {
				t.Errorf("day %d hour %02d: overlay says %d free, its own lanes say %d",
					d, h, day.Free[h], want)
			}
		}
	}
	// The week-zoom reading is unmoved: these are the peaks and the hours the
	// hand-written stub produced, and every candidate card is ranked off them.
	wantPeak := []struct{ hour, free int }{
		{19, 1}, {16, 1}, {19, 3}, {17, 1}, {21, 2}, {19, 4}, {20, 3},
	}
	for d, want := range wantPeak {
		bestH, bestN := 16, -1
		for h := 16; h < 24; h++ {
			if n := av.Days[d].Free[h]; n > bestN {
				bestH, bestN = h, n
			}
		}
		if bestH != want.hour || bestN != want.free {
			t.Errorf("day %d peak = %d free at %02d, want %d at %02d",
				d, bestN, bestH, want.free, want.hour)
		}
	}
}

// ── THE 390 REFUSAL ────────────────────────────────────────────────────────

// TestScheduleZoom_NarrowRefusalIsBuiltAndReachable. The comment on
// ScheduleToggle claimed a refusal for four stages while `Disabled` was set
// nowhere, the templ branch that renders it was unreachable, and the `:disabled`
// CSS repair styled a control this surface never rendered. These assertions are
// what makes the claim checkable.
func TestScheduleZoom_NarrowRefusalIsBuiltAndReachable(t *testing.T) {
	opts := scheduleZoomOptions("camp-1", nil, "week")
	if len(opts) != 3 {
		t.Fatalf("Zoom segment has %d rungs, want 3 (Week, the live Day, its "+
			"narrow refusal)", len(opts))
	}

	var refusal *ScheduleToggle
	live := 0
	for i := range opts {
		if opts[i].Disabled {
			refusal = &opts[i]
			continue
		}
		live++
	}
	if refusal == nil {
		t.Fatal("no rung is Disabled — the page refuses nothing, and the doc " +
			"comment, the templ branch and the :disabled rule all describe a " +
			"control that does not exist")
	}
	if live != 2 {
		t.Errorf("%d live rungs, want 2", live)
	}
	if refusal.Key != "day" || refusal.Label != "Day" {
		t.Errorf("the refusal is on %q/%q, want the Day rung", refusal.Key, refusal.Label)
	}
	// The drawing's own sentence, verbatim.
	if refusal.Title != "week zoom is forced at this width" {
		t.Errorf("refusal title = %q, want the drawing's own sentence", refusal.Title)
	}
	if refusal.Href != "" {
		t.Errorf("the refused rung carries href %q — a disabled control that is "+
			"still a link is one a keyboard follows anyway", refusal.Href)
	}
	if refusal.Pressed {
		t.Error("the refused rung is inked as pressed")
	}
	if refusal.Gate != "narrow" {
		t.Errorf("refusal gate = %q, want %q", refusal.Gate, "narrow")
	}

	// And its live twin is gated the other way, or both would show at once.
	var wide *ScheduleToggle
	for i := range opts {
		if opts[i].Key == "day" && !opts[i].Disabled {
			wide = &opts[i]
		}
	}
	if wide == nil || wide.Gate != "wide" {
		t.Fatalf("the live Day rung is not gated wide: %+v", wide)
	}
	if wide.Href == "" {
		t.Error("the live Day rung has no href")
	}
}

// TestScheduleZoom_RefusalRendersAsADisabledButton walks the templ branch that
// was dead code, and checks the classes the media query keys on.
func TestScheduleZoom_RefusalRendersAsADisabledButton(t *testing.T) {
	var sb strings.Builder
	err := scheduleSegView("Zoom", scheduleZoomOptions("camp-1", nil, "week")).
		Render(t.Context(), &sb)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{
		`<button type="button" disabled`,
		`week zoom is forced at this width`,
		`class="sc-rung-narrow"`,
		`class="sc-rung-wide"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the Zoom segment does not contain %q:\n%s", want, html)
		}
	}
	// Exactly one disabled control, and exactly one of each gate.
	if n := strings.Count(html, "disabled"); n != 1 {
		t.Errorf("%d disabled attributes in one segment, want 1", n)
	}
	for _, cls := range []string{"sc-rung-wide", "sc-rung-narrow"} {
		if n := strings.Count(html, cls); n != 1 {
			t.Errorf("%d occurrences of %q, want 1", n, cls)
		}
	}
	// The hour band is NOT gated: it has no refusal and must carry no class.
	var bs strings.Builder
	if err := scheduleSegView("Hour band", scheduleBandOptions("camp-1", nil, "evening")).
		Render(t.Context(), &bs); err != nil {
		t.Fatalf("render band: %v", err)
	}
	if strings.Contains(bs.String(), "sc-rung-") || strings.Contains(bs.String(), "disabled") {
		t.Errorf("the hour band grew a gate or a refusal:\n%s", bs.String())
	}
}

// TestScheduleZoom_TheRefusalHasItsOwnCSSPairAndIsScopedToTheSegment. The bare
// `.sc-wide` / `.sc-narrow` pair is 0,2,0 and `.cal-schedule .seg a` is 0,2,1,
// so reusing it would leave the wide anchor visible at 390 beside its own
// disabled twin — two Day controls at once, which is worse than the defect.
func TestScheduleZoom_TheRefusalHasItsOwnCSSPairAndIsScopedToTheSegment(t *testing.T) {
	css := scheduleCSS(t, "calendar-schedule.css")
	for _, want := range []string{
		".cal-schedule .seg .sc-rung-narrow { display: none; }",
		".cal-schedule .seg .sc-rung-wide { display: inline-flex; }",
		".cal-schedule .seg .sc-rung-wide { display: none; }",
		".cal-schedule .seg .sc-rung-narrow { display: inline-flex; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("calendar-schedule.css is missing %q", want)
		}
	}
	// The phone half must live inside the 640 block, or it applies everywhere.
	narrow := strings.Index(css, "@media (max-width: 640px)")
	if narrow < 0 {
		t.Fatal("no 640 block in the stylesheet")
	}
	hide := strings.Index(css, ".cal-schedule .seg .sc-rung-wide { display: none; }")
	if hide < narrow {
		t.Error("the Day rung is refused OUTSIDE the phone block — it would be " +
			"refused at every width")
	}
	// And the repair the fourth pass landed now styles something real.
	if !strings.Contains(css, ".cal-schedule .seg button:disabled { color: var(--text-muted); cursor: not-allowed; }") {
		t.Error("the :disabled rung repair is gone")
	}
}

// TestScheduleHead_ControlsGrowToTouchTargetsAt390. Four declarations the
// drawing writes in its own 640 block and the sheets did not carry, found while
// measuring the refused Day rung against the still — the rung is one of them.
// Measured before the repair: the Zoom segment 94px against the drawing's 175,
// its rungs 20px tall against 26, the stepper 141px against 358.
func TestScheduleHead_ControlsGrowToTouchTargetsAt390(t *testing.T) {
	css := scheduleCSS(t, "calendar-schedule.css")
	narrow := strings.Index(css, "@media (max-width: 640px)")
	if narrow < 0 {
		t.Fatal("no 640 block in the stylesheet")
	}
	for _, want := range []string{
		".cal-schedule .phead .ctl .seg { flex: 1; }",
		".cal-schedule .phead .ctl .seg a { flex: 1; justify-content: center; height: 26px; }",
		".cal-schedule .phead .ctl .stepper { width: 100%; justify-content: space-between; }",
		".cal-schedule .phead .ctl .btn.sm { height: 34px; }",
	} {
		i := strings.Index(css, want)
		if i < 0 {
			t.Errorf("calendar-schedule.css is missing %q", want)
			continue
		}
		if i < narrow {
			t.Errorf("%q sits OUTSIDE the phone block — it would grow the head's "+
				"controls at every width", want)
		}
	}
}

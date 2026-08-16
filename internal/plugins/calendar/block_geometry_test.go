package calendar

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// --- fixtures ---------------------------------------------------------------

// blockTenDayCal is the signed contract's shape: a 30-day month in TEN-day
// weeks. Proving the producer handles a non-seven-day week natively is half the
// point of calendar-v4 — calv3 hardcoded repeat(7, …).
func blockTenDayCal() *Calendar {
	cal := &Calendar{
		ID: "cal-harptos", CampaignID: "camp-1", Mode: ModeFantasy,
		Name: "Harptos of Imix", CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 14,
		IsDefault: true,
	}
	for i := 0; i < 12; i++ {
		cal.Months = append(cal.Months, Month{ID: i + 1, CalendarID: cal.ID,
			Name: fmt.Sprintf("Month%d", i+1), Days: 30, SortOrder: i})
	}
	cal.Months[0].Name = "Deepwinter"
	for i, n := range []string{"Sar", "Mol", "Zor", "Wir", "Nym", "Lyr", "Tam", "Kes", "Vel", "Odd"} {
		cal.Weekdays = append(cal.Weekdays, Weekday{ID: i + 1, CalendarID: cal.ID, Name: n, SortOrder: i})
	}
	return cal
}

// blockRealTimeCal is a FLAGGED real-time calendar: Gregorian month lengths
// come from the proleptic stdlib, which is what makes February leap-aware.
func blockRealTimeCal() *Calendar {
	zone := "UTC"
	cal := &Calendar{
		ID: "cal-real", CampaignID: "camp-1", Mode: ModeRealLife,
		Name: "Real world", CurrentYear: 2028, CurrentMonth: 2, CurrentDay: 29,
		TracksRealTime: true, RealTimeZone: &zone,
	}
	lens := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	names := []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	for i, n := range names {
		cal.Months = append(cal.Months, Month{ID: i + 1, CalendarID: cal.ID,
			Name: n, Days: lens[i], SortOrder: i})
	}
	for i, n := range []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"} {
		cal.Weekdays = append(cal.Weekdays, Weekday{ID: i + 1, CalendarID: cal.ID, Name: n, SortOrder: i})
	}
	return cal
}

func blockStrPtr(s string) *string { return &s }
func blockIntPtr(i int) *int       { return &i }

// --- THE LEAP GUARD ---------------------------------------------------------

// TestBlockGeometryLeapAwareFebruary2028 is the guard the whole file exists for.
//
// The V2 helpers read raw Months[i].Days, which for February is the stored 28.
// Event.OccursOn routes through Calendar.MonthDays, which for a FLAGGED
// real-time calendar returns the proleptic-Gregorian 29 in 2028. So the old
// grid draws 28 cells while recurrence places an event on the 29th: the event
// exists and has nowhere to render. Nothing on main pins that.
func TestBlockGeometryLeapAwareFebruary2028(t *testing.T) {
	cal := blockRealTimeCal()

	geo := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 1, Year: 2028})
	if geo.Days != 29 {
		t.Fatalf("February 2028 Days = %d, want 29 (leap-aware via Calendar.MonthDays)", geo.Days)
	}
	// The raw column the V2 helpers would have read, for contrast.
	if raw := cal.Months[1].Days; raw != 28 {
		t.Fatalf("fixture drifted: stored February days = %d, want 28", raw)
	}

	var dated int
	seen := map[int]bool{}
	for _, row := range geo.Rows {
		for _, c := range row.Cells {
			if c.Day > 0 {
				dated++
				seen[c.Day] = true
			}
		}
	}
	if dated != 29 {
		t.Fatalf("dated cells = %d, want 29", dated)
	}
	if !seen[29] {
		t.Fatal("no cell carries day 29 — the leap day has nowhere to render")
	}

	// …and OccursOn agrees: a monthly recurrence anchored on Feb 29 lands there.
	rt := RecurrenceMonthly
	e := Event{ID: "e1", CalendarID: cal.ID, Name: "Leap rite",
		Year: 2028, Month: 2, Day: 29, IsRecurring: true, RecurrenceType: &rt,
		Visibility: "everyone"}
	if !e.OccursOn(cal, 2028, 2, 29) {
		t.Fatal("OccursOn(2028-02-29) = false; geometry and recurrence disagree")
	}
	// 2029 is not a leap year: February has 28 days and the instance is skipped,
	// which the geometry must agree with too.
	if e.OccursOn(cal, 2029, 2, 29) {
		t.Fatal("OccursOn(2029-02-29) = true, but 2029 has no 29 February")
	}
	if d := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 1, Year: 2029}).Days; d != 28 {
		t.Fatalf("February 2029 Days = %d, want 28", d)
	}
}

// --- week length ------------------------------------------------------------

func TestBlockWeekLenIsNeverAssumed(t *testing.T) {
	if got := blockWeekLen(blockTenDayCal()); got != 10 {
		t.Fatalf("ten-day week resolved to %d columns, want 10", got)
	}
	bare := &Calendar{ID: "bare", Months: []Month{{Name: "M1", Days: 30}}}
	if got := blockWeekLen(bare); got != blockDefaultWeekLen {
		t.Fatalf("zero-weekday calendar resolved to %d, want the named fallback %d",
			got, blockDefaultWeekLen)
	}
	// The fallback is a NAMED constant, not a literal buried in an expression —
	// that is the whole v4 rule about the number 7.
	if blockDefaultWeekLen != 7 {
		t.Fatalf("blockDefaultWeekLen = %d; changing it is a product decision, not a refactor",
			blockDefaultWeekLen)
	}
	geo := buildMonthGeometry(blockTenDayCal(), blockMonthGeometryInput{MonthIndex: 0, Year: 1523})
	if geo.WeekLen != 10 || len(geo.Weekdays) != 10 {
		t.Fatalf("WeekLen=%d weekdays=%d, want 10/10", geo.WeekLen, len(geo.Weekdays))
	}
	for _, row := range geo.Rows {
		if len(row.Cells) != 10 {
			t.Fatalf("week row has %d cells, want 10", len(row.Cells))
		}
	}
}

func TestBlockHalfRuleIsScopedToTenDayWeeks(t *testing.T) {
	if !blockIsHalfCol(10, 5) {
		t.Fatal("column 5 of a ten-day week must carry the five-column rule")
	}
	for _, col := range []int{1, 4, 6, 10} {
		if blockIsHalfCol(10, col) {
			t.Fatalf("column %d of a ten-day week must not carry the rule", col)
		}
	}
	// A seven-day week has no mid-week rule in any signed still.
	for col := 1; col <= 7; col++ {
		if blockIsHalfCol(7, col) {
			t.Fatalf("column %d of a seven-day week must not carry the rule", col)
		}
	}
	wds := blockWeekdays(blockTenDayCal())
	if !wds[4].Half {
		t.Fatal("the fifth weekday header must carry Half")
	}
	if wds[0].Half || wds[9].Half {
		t.Fatal("only the fifth weekday header carries Half")
	}
}

// TestBlockHalfRuleNeverLandsOnTheLastColumn closes a LATENT edge case that the
// tile construction opened and then removed the safety net for.
//
// WHAT USED TO CATCH IT. `.cal-block-host .cell.lastcol { border-right: 0 }`
// cancelled the five-column rule on a cell that was both `half` and last. That
// rule existed for the RULED treatment's shared hairline, it cancelled nothing
// once the hairline went, and C-CALV4-TILES deleted it — correctly: a CSS rule
// that cancels nothing is a rule the next hand has to reason about.
//
// WHY IT STILL MATTERS. Deleting it also deleted the only thing that would have
// contained the collision if it ever became reachable. The five-column rule is
// drawn in the 3px gutter BETWEEN two tiles; on the last column there is no
// gutter, so a strong rule would hang off the grid's outer edge with nothing to
// sit in — and it would be a producer bug rendered by correct CSS.
//
// The live producer cannot produce it: blockIsHalfCol is true only for weekLen
// == blockHalfRuleWeekLen (10) at col == weekLen/2 (5), and 5 is not 10. But
// NOTHING ASSERTED THAT IT STAYS THAT WAY. Widening the constant to "any even
// week" — the obvious generalisation, and the one blockHalfRuleWeekLen's own
// comment says was refused — leaves weekLen/2 safe, while a rule of the shape
// `col == weekLen` or an off-by-one on a 2-day week does not.
//
// THIS IS A PRODUCER-SIDE TEST AND NOT A CSS BAND-AID, deliberately. Re-adding
// a `.lastcol` cancel would make the wrong flag paint correctly, which is how a
// producer defect becomes invisible instead of impossible.
func TestBlockHalfRuleNeverLandsOnTheLastColumn(t *testing.T) {
	// Every week length a calendar could plausibly declare, plus the degenerate
	// ones: a 1-day week is where `col == weekLen` and `col == weekLen/2` are
	// most likely to collide under a careless rewrite.
	for weekLen := 1; weekLen <= 20; weekLen++ {
		if !blockIsHalfCol(weekLen, weekLen) {
			continue
		}
		t.Errorf("blockIsHalfCol(%d, %d) is true: the LAST column of a %d-day week carries "+
			"the five-column rule. That rule is drawn in the gutter BETWEEN two tiles and "+
			"the last column has no gutter, so it would hang off the grid's outer edge. "+
			"`.cell.lastcol` used to cancel exactly this and was deleted with the ruled "+
			"treatment it belonged to; this test is what replaced it, on the producer side, "+
			"because the fix for a wrong flag is not to paint it correctly",
			weekLen, weekLen, weekLen)
	}
	// And the same claim through the surface that actually stamps the flag, so a
	// header row that derived Half some other way could not slip past.
	//
	// ANCHORED AGAINST VACUITY FIRST. An absence-only sweep passes on a
	// blockWeekdays that stopped setting Half at all, and it passes on a helper
	// whose week length never reaches the sweep's values — both of which would
	// read as "no collision" while proving nothing.
	if ten := blockWeekdays(blockCalWithWeekLen(10)); len(ten) != 10 || !ten[4].Half {
		t.Fatalf("blockWeekdays(10 weekdays) produced %d headers with Half at 5 = %v — the "+
			"sweep below is vacuous unless this positive case holds", len(ten),
			len(ten) == 10 && ten[4].Half)
	}
	for weekLen := 1; weekLen <= 20; weekLen++ {
		wds := blockWeekdays(blockCalWithWeekLen(weekLen))
		if len(wds) == 0 {
			continue
		}
		if wds[len(wds)-1].Half {
			t.Errorf("the last weekday header of a %d-day week carries Half — same collision, "+
				"reached through blockWeekdays rather than through blockIsHalfCol", weekLen)
		}
	}
}

// blockCalWithWeekLen is a minimal calendar declaring exactly n weekdays, for
// the sweep above. Names are irrelevant to Half; only the COUNT is.
func blockCalWithWeekLen(n int) *Calendar {
	cal := &Calendar{}
	for i := 0; i < n; i++ {
		cal.Weekdays = append(cal.Weekdays, Weekday{Name: fmt.Sprintf("d%d", i+1)})
	}
	return cal
}

func TestBlockRowCountFoldsInTheLead(t *testing.T) {
	// 30 days, ten-day weeks, no lead → exactly 3 rows.
	if got := blockRowCount(0, 30, 10); got != 3 {
		t.Fatalf("rows(lead=0, days=30, week=10) = %d, want 3", got)
	}
	// A month whose 1st sits mid-week spills into a fourth row.
	if got := blockRowCount(3, 30, 10); got != 4 {
		t.Fatalf("rows(lead=3, days=30, week=10) = %d, want 4", got)
	}
	if got := blockRowCount(0, 0, 10); got != 0 {
		t.Fatalf("a zero-day month must produce no rows, got %d", got)
	}
}

// --- era bands --------------------------------------------------------------

func TestBlockEraBandsAreEmittedPerWeekRow(t *testing.T) {
	cal := blockTenDayCal()
	cal.Eras = []Era{{ID: 1, CalendarID: cal.ID, Name: "Reckoning of Wards",
		StartYear: 1, EndYear: nil, Color: "#7c5cff", SortOrder: 0}}
	cal.Seasons = []Season{{ID: 1, CalendarID: cal.ID, Name: "Long Night",
		StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 28}}

	geo := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1523})
	// 30 days in ten-day weeks, plus this calendar's lead, is four rows — the
	// era therefore CANNOT be one band, which is the point (§9 deviation 2).
	if geo.Lead != 1 {
		t.Fatalf("fixture drifted: lead = %d, want 1", geo.Lead)
	}
	if len(geo.Rows) != 4 {
		t.Fatalf("want 4 week rows, got %d", len(geo.Rows))
	}
	// Per row: the first and last REAL day the row covers, and the column the
	// first one sits in.
	want := []struct{ startCol, span int }{{2, 9}, {1, 10}, {1, 10}, {1, 1}}
	for i, row := range geo.Rows {
		if len(row.Bands) != 1 {
			t.Fatalf("row %d has %d bands, want 1 — an era spanning four rows must be "+
				"four bands, never one subgrid span", i, len(row.Bands))
		}
		b := row.Bands[0]
		if b.StartCol != want[i].startCol || b.Span != want[i].span {
			t.Fatalf("row %d band = col %d span %d, want col %d span %d",
				i, b.StartCol, b.Span, want[i].startCol, want[i].span)
		}
		if b.BandHue != "#7c5cff" {
			t.Fatalf("row %d --bandhue = %q, want the ERA's colour", i, b.BandHue)
		}
		// The era starts in year 1 and never ends, so it is open on both sides
		// of every row of year 1523.
		if !b.OpenLeft || !b.OpenRight {
			t.Fatalf("row %d openLeft=%v openRight=%v, want both true", i, b.OpenLeft, b.OpenRight)
		}
		wantSuffix := ""
		if i == 0 {
			wantSuffix = "Long Night"
		}
		if b.Suffix != wantSuffix {
			t.Fatalf("row %d suffix = %q, want %q — the season folds in on row 0 ONLY",
				i, b.Suffix, wantSuffix)
		}
	}
}

func TestBlockEraBandsClipToTheMonthsRealDays(t *testing.T) {
	cal := blockTenDayCal()
	cal.Months[0].Days = 24 // a short month: the last row is partial
	cal.Eras = []Era{{Name: "Age", StartYear: 1500, EndYear: blockIntPtr(1600), Color: ""}}

	geo := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1523})
	last := geo.Rows[len(geo.Rows)-1]
	if len(last.Bands) != 1 {
		t.Fatalf("last row bands = %d, want 1", len(last.Bands))
	}
	// Lead 3, ten-day weeks: the last row starts at day 18 and the band stops at
	// day 24 — the trailing blanks are excluded rather than banded.
	if geo.Lead != 3 {
		t.Fatalf("fixture drifted: lead = %d, want 3", geo.Lead)
	}
	if last.Bands[0].StartCol != 1 || last.Bands[0].Span != 7 {
		t.Fatalf("last row band = col %d span %d, want col 1 span 7 (days 18..24)",
			last.Bands[0].StartCol, last.Bands[0].Span)
	}
	if last.Bands[0].BandHue != "" {
		t.Fatalf("a colourless era must emit an empty --bandhue for the renderer's "+
			"structural fallback, got %q", last.Bands[0].BandHue)
	}
}

// TestBlockEraBandEdgeIsUnreachableToday pins a KNOWN CONTRACT GAP, so the day
// it changes, it changes deliberately.
//
// The signed render splits one month between two eras at day 17/18 and draws a
// 1.5px editorial rule (EraBand.Edge) at that boundary. Chronicle's eras are
// YEAR-granular (calendar_eras.start_year / end_year), so a mid-month era
// boundary cannot be expressed and Edge is always false. Flagged to the
// coordinator rather than approximated.
func TestBlockEraBandEdgeIsUnreachableToday(t *testing.T) {
	cal := blockTenDayCal()
	cal.Eras = []Era{
		{Name: "Reckoning of Wards", StartYear: 1500, EndYear: blockIntPtr(1523), SortOrder: 0},
		{Name: "Age of the Emberfall", StartYear: 1523, EndYear: nil, SortOrder: 1},
	}
	geo := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1523})
	for i, row := range geo.Rows {
		if len(row.Bands) != 2 {
			t.Fatalf("row %d: two overlapping eras must both band, got %d", i, len(row.Bands))
		}
		for j, b := range row.Bands {
			if b.Edge {
				t.Fatalf("row %d band %d: Edge=true, but a mid-month era boundary is not "+
					"expressible while eras are year-granular — an era can only begin on "+
					"day 1 of month 1", i, j)
			}
			// A year-granular era covers every real day of the row; only the
			// leading blanks of row 0 are excluded.
			wantSpan := 10
			if i == 0 {
				wantSpan = 10 - geo.Lead
			}
			if i == len(geo.Rows)-1 {
				wantSpan = geo.Days - (i*10 - geo.Lead)
			}
			if b.Span != wantSpan {
				t.Fatalf("row %d band %d span = %d, want %d", i, j, b.Span, wantSpan)
			}
		}
		if row.Bands[1].Suffix != "" {
			t.Fatal("the season suffix must appear once per row, not once per era")
		}
	}
}

func TestBlockEraBandHalfFollowsTheFiveColumnRule(t *testing.T) {
	cal := blockTenDayCal()
	cal.Eras = []Era{{Name: "Age", StartYear: 1, EndYear: nil}}
	geo := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1523})
	if !geo.Rows[0].Bands[0].Half {
		t.Fatal("a band covering column 5 of a ten-day week must carry Half")
	}

	seven := blockRealTimeCal()
	seven.Eras = []Era{{Name: "Age", StartYear: 1, EndYear: nil}}
	sgeo := buildMonthGeometry(seven, blockMonthGeometryInput{MonthIndex: 1, Year: 2028})
	for _, row := range sgeo.Rows {
		for _, b := range row.Bands {
			if b.Half {
				t.Fatal("a seven-day week has no five-column rule, on cells or on bands")
			}
		}
	}
}

// --- intercalary ------------------------------------------------------------

func TestBlockIntercalaryDaysHangOffTheirMonth(t *testing.T) {
	cal := blockTenDayCal()
	// Midwinter sits between month 1 and month 2, as canonical Harptos does.
	cal.Months = append(cal.Months[:1], append([]Month{{
		ID: 99, CalendarID: cal.ID, Name: "Midwinter", Days: 1, SortOrder: 1, IsIntercalary: true,
	}}, cal.Months[1:]...)...)

	got := blockIntercalary(cal, 0, 1523)
	if len(got) != 1 || got[0].Name != "Midwinter" || got[0].Day != 1 {
		t.Fatalf("month 1's intercalary = %+v, want one Midwinter day 1", got)
	}
	if len(blockIntercalary(cal, 2, 1523)) != 0 {
		t.Fatal("an ordinary month that is not followed by an intercalary one has none")
	}
	if idx := blockIntercalaryMonths(cal, 0); len(idx) != 1 || idx[0] != 1 {
		t.Fatalf("blockIntercalaryMonths = %v, want [1]", idx)
	}
	// The grid itself must NOT grow a cell for it — that is the entire reason
	// intercalary days are a separate row.
	geo := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1523})
	if geo.Days != 30 {
		t.Fatalf("the tenday grid must still be 30 days, got %d", geo.Days)
	}
	if len(geo.Intercalary) != 1 {
		t.Fatalf("geometry carries %d intercalary days, want 1", len(geo.Intercalary))
	}
}

// --- moon discs -------------------------------------------------------------

// TestBlockMoonDiscMatchesTheSignedTerminator pins the disc math against the
// signed illumOf()/moonSil() (mockups/calendar-v4.html :1326, :2008).
func TestBlockMoonDiscMatchesTheSignedTerminator(t *testing.T) {
	moons := []Moon{{ID: 1, Name: "Selune", CycleDays: 4, PhaseOffset: 0}}
	// base 0 → day 1 is absolute 0 → phase 0 (new), day 2 → 0.25, day 3 → 0.5.
	cases := []struct {
		day        int
		wantIllum  float64
		wantWaxing bool
		wantMode   string
	}{
		{1, 0, true, "sub"},   // new: empty ring
		{2, 0.5, true, "sub"}, // first quarter: exactly half
		{3, 1, false, "add"},  // full: solid disc
		{4, 0.5, false, "sub"},
	}
	for _, c := range cases {
		d := moonDiscsForDay(moons, 0, c.day, 0)
		if len(d) != 1 {
			t.Fatalf("day %d: got %d discs", c.day, len(d))
		}
		if math.Abs(d[0].Illum-c.wantIllum) > 1e-9 {
			t.Fatalf("day %d illum = %v, want %v", c.day, d[0].Illum, c.wantIllum)
		}
		if d[0].Waxing != c.wantWaxing {
			t.Fatalf("day %d waxing = %v, want %v", c.day, d[0].Waxing, c.wantWaxing)
		}
		if !hasSuffix(d[0].Terminator, c.wantMode) {
			t.Fatalf("day %d terminator = %q, want mode %q", c.day, d[0].Terminator, c.wantMode)
		}
		if d[0].Eclipse {
			t.Fatal("Eclipse must stay false — calendar_celestial_events has no moon_id")
		}
	}
	// The Nameplate declares the ceiling once; the producer only honours it.
	many := []Moon{{Name: "A", CycleDays: 4}, {Name: "B", CycleDays: 6}, {Name: "C", CycleDays: 8}, {Name: "D", CycleDays: 9}}
	if got := len(moonDiscsForDay(many, 0, 1, 3)); got != 3 {
		t.Fatalf("moon cap 3 produced %d discs", got)
	}
	if got := len(moonDiscsForDay(many, 0, 1, 0)); got != 4 {
		t.Fatalf("moon cap 0 means all; produced %d discs", got)
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// TestBlockMoonBaseDayIsSteppedNotRecomputed is the performance guard.
//
// AMENDMENT R4-S21-A (C-SWEEP-R4 stage 21, backend/cal-worldstate-year-dos).
// This guard used to assert a RATIO: that recomputing Calendar.AbsoluteDay per
// cell is at least 5× slower than stepping off one base. That ratio held only
// because AbsoluteDay summed YearLengthForYear from year 0, i.e. it was O(year)
// — and that same linearity was an unauthenticated CPU-exhaustion vector on
// three public world-state routes (53.5 s per request at `?year=2000000000`).
// Stage 21 replaced the year term with a closed form, so the ratio inverted and
// the old assertion began failing on a CORRECT tree. A guard whose premise the
// fix deliberately removes cannot be left as-is, and it must not simply be
// deleted either.
//
// It is AMENDED TO THE STRONGER PROPERTY it was always a proxy for. The old
// form tolerated an O(year) AbsoluteDay as long as this one producer stepped
// around it; the new form forbids an O(year) AbsoluteDay outright, which no
// producer can route around, AND keeps the exactness half untouched. So:
//
//  1. exactness — stepping off one base equals recomputing per day (unchanged);
//  2. agreement — the stepped producer's illumination matches the per-cell
//     computation disc for disc (unchanged);
//  3. year-independence (NEW, replacing the ratio) — the per-cell form costs
//     the same at year 2000000000 as at year 50000. Restore the loop and the
//     50000 leg alone is ~30 AbsoluteDay walks of 50000 iterations, while the
//     2000000000 leg does not finish inside the go test deadline at all.
//
// TestAbsoluteDayClosedFormIsNotLinear in absoluteday_dos_test.go pins the same
// root property directly; this one pins that the BLOCK still gets it.
func TestBlockMoonBaseDayIsSteppedNotRecomputed(t *testing.T) {
	cal := blockTenDayCal()
	cal.LeapYearEvery = 4
	cal.Months[0].LeapYearDays = 1
	cal.Moons = []Moon{{Name: "Selune", CycleDays: 30.4}, {Name: "Shar", CycleDays: 7.2}}

	const year = 50000
	const monthIdx = 3
	days := cal.MonthDays(monthIdx, year)

	// Exactness: stepping off one base must equal recomputing per day.
	base := monthBaseAbsoluteDay(cal, monthIdx, year)
	for d := 1; d <= days; d++ {
		want := cal.AbsoluteDay(year, monthIdx+1, d)
		if got := base + d - 1; got != want {
			t.Fatalf("day %d: stepped absolute day %d != recomputed %d", d, got, want)
		}
	}

	naive := make([][]blockNaivePhase, 0, days)
	for d := 1; d <= days; d++ {
		abs := cal.AbsoluteDay(year, monthIdx+1, d)
		row := make([]blockNaivePhase, 0, len(cal.Moons))
		for _, m := range cal.Moons {
			row = append(row, blockNaivePhase{Phase: m.MoonPhase(abs)})
		}
		naive = append(naive, row)
	}

	fastBase := monthBaseAbsoluteDay(cal, monthIdx, year)
	for d := 1; d <= days; d++ {
		discs := moonDiscsForDay(cal.Moons, fastBase, d, 0)
		for i, disc := range discs {
			wantIllum := (1 - math.Cos(2*math.Pi*naive[d-1][i].Phase)) / 2
			if math.Abs(disc.Illum-wantIllum) > 1e-12 {
				t.Fatalf("day %d moon %d: illum %v != %v", d, i, disc.Illum, wantIllum)
			}
		}
	}

	// Year-independence. Both legs walk a whole month of per-cell AbsoluteDay
	// calls; the only difference is the year, so a year-linear implementation
	// separates them by a factor of 40000. The budget is per-leg wall clock
	// rather than a ratio between the two, because a ratio compares two numbers
	// that a correct implementation makes equal — which is precisely how the
	// pre-amendment form ended up asserting the bug.
	for _, y := range []int{year, 2000000000} {
		n := cal.MonthDays(monthIdx, y)
		sink, elapsed, ok := absDayWithinBudget(500*time.Millisecond, func() int {
			var acc int
			for d := 1; d <= n; d++ {
				acc += cal.AbsoluteDay(y, monthIdx+1, d)
			}
			return acc
		})
		if !ok {
			t.Fatalf("a month of per-cell AbsoluteDay at year %d exceeded %v — AbsoluteDay is "+
				"linear in the year again, which is both a Block render cost and an "+
				"unauthenticated CPU-exhaustion vector on the world-state seed", y, elapsed)
		}
		if sink == 0 {
			t.Fatal("compiler elided the call")
		}
		t.Logf("year %-12d: %d per-cell AbsoluteDay calls in %v", y, n, elapsed)
	}
}

// blockNaivePhase is a local shim so the naive comparison above can hold a raw
// phase without depending on the pinned widget struct's derived fields.
type blockNaivePhase struct{ Phase float64 }

// --- the three-counter divergence pin --------------------------------------

// TestBlockCounterDivergencePin measures and PINS the disagreement between the
// package's day counters. Coordinator ruling COMMON §6.4: this is documented,
// not fixed, in calendar-v4 wave 1 — making constLenDayIndex leap-aware would
// shift the weekday column of every calendar already in the operator's
// production database.
//
// The numbers below are the measurement the PR reports. If they move, a counter
// changed, and that must be a deliberate act with an operator gate.
func TestBlockCounterDivergencePin(t *testing.T) {
	cal := blockTenDayCal()
	cal.LeapYearEvery = 4  // years divisible by 4 are leap
	cal.LeapYearOffset = 0 //
	cal.Months[0].LeapYearDays = 1

	if got := cal.YearLength(); got != 360 {
		t.Fatalf("fixture drifted: YearLength = %d, want 360", got)
	}
	if got := cal.YearLengthForYear(4); got != 361 {
		t.Fatalf("fixture drifted: leap YearLengthForYear = %d, want 361", got)
	}

	// Counter 2 (constLenDayIndex — weekday column + recurrence) never adds a
	// leap day. Counter 1 (AbsoluteDay — moon phase) always does. They diverge
	// by exactly one day per elapsed leap year.
	cases := []struct{ year, wantDivergence int }{
		{0, 0},   // no elapsed years
		{1, 1},   // year 0 was leap
		{4, 1},   // years 0..3 → one leap (year 0)
		{5, 2},   // years 0..4 → two leaps (0, 4)
		{8, 2},   // years 0..7 → two leaps (0, 4)
		{9, 3},   // years 0..8 → three leaps (0, 4, 8)
		{40, 10}, // years 0..39 → ten leaps
	}
	for _, c := range cases {
		abs := cal.AbsoluteDay(c.year, 1, 1)
		fixed := cal.constLenDayIndex(c.year, 1, 1)
		if got := abs - fixed; got != c.wantDivergence {
			t.Fatalf("year %d: AbsoluteDay(%d) - constLenDayIndex(%d) = %d, want %d",
				c.year, abs, fixed, got, c.wantDivergence)
		}
	}

	// The user-visible consequence: the moon disc drawn in a cell is the phase
	// of a date `divergence` days away from the date that cell's WEEKDAY column
	// was computed for. At year 40 that is ten days of phase on a 30.4-day moon
	// — a third of a cycle.
	moon := Moon{Name: "Selune", CycleDays: 30.4}
	discPhase := moon.MoonPhase(cal.AbsoluteDay(40, 1, 1))
	columnPhase := moon.MoonPhase(cal.constLenDayIndex(40, 1, 1))
	if math.Abs(discPhase-columnPhase) < 1e-9 {
		t.Fatal("the two counters agree at year 40; the fixture no longer exercises the divergence")
	}
	t.Logf("year 40: disc phase %.4f vs column-counter phase %.4f (10 days apart)",
		discPhase, columnPhase)

	// And the freeze itself: the weekday column is taken from the FIXED counter,
	// exactly as v2WeekdayIndexFor computes it, so the Block agrees with the V2
	// page and with recurrence rather than with the moon counter.
	for _, y := range []int{1523, 1524, 1600} {
		want := v2WeekdayIndexFor(cal, y, 1, 1)
		if got := blockMonthLead(cal, 0, y); got != want {
			t.Fatalf("year %d: block lead = %d, v2 weekday index = %d — the Block must "+
				"place day 1 in the SAME column the V2 grid does", y, got, want)
		}
	}
}

// --- resolvers --------------------------------------------------------------

func TestFestivalOnDate(t *testing.T) {
	cal := blockTenDayCal()
	cal.Festivals = []Festival{
		{ID: 1, Name: "Greengrass", Month: blockIntPtr(2), Day: blockIntPtr(4)},
		{ID: 2, Name: "Shieldmeet", AfterMonth: blockIntPtr(1)}, // between months: no date
		{ID: 3, Name: "Second on the same day", Month: blockIntPtr(2), Day: blockIntPtr(4)},
	}
	got := FestivalOnDate(cal, 2, 4)
	if len(got) != 2 {
		t.Fatalf("FestivalOnDate(2,4) returned %d, want 2", len(got))
	}
	if len(FestivalOnDate(cal, 1, 1)) != 0 {
		t.Fatal("a between-months festival has no day and must not match a date")
	}
	if FestivalOnDate(nil, 1, 1) != nil {
		t.Fatal("a nil calendar resolves to no festivals, not a panic")
	}
}

func TestCycleEntryForYear(t *testing.T) {
	cyc := &Cycle{ID: 1, Name: "Zodiac", CycleLength: 3, Entries: []CycleEntry{
		{ID: 1, Name: "Wolf", YearOffset: 0, SortOrder: 0},
		{ID: 2, Name: "Stag", YearOffset: 1, SortOrder: 1},
		{ID: 3, Name: "Raven", YearOffset: 2, SortOrder: 2},
	}}
	for year, want := range map[int]string{9: "Wolf", 10: "Stag", 11: "Raven", 12: "Wolf"} {
		e := CycleEntryForYear(cyc, year)
		if e == nil || e.Name != want {
			t.Fatalf("year %d resolved to %v, want %s", year, e, want)
		}
	}
	if CycleEntryForYear(nil, 1) != nil {
		t.Fatal("a nil cycle resolves to nil")
	}
	if CycleEntryForYear(&Cycle{CycleLength: 0}, 1) != nil {
		t.Fatal("an entry-less cycle resolves to nil rather than dividing by zero")
	}
}

func TestBlockGeometryToleratesAHalfBuiltCalendar(t *testing.T) {
	// No months at all: the Block must not panic, and must produce nothing to
	// draw so the Nameplate's fault is the only thing on screen.
	geo := buildMonthGeometry(&Calendar{ID: "empty"}, blockMonthGeometryInput{MonthIndex: 0, Year: 1})
	if geo.Days != 0 || len(geo.Rows) != 0 {
		t.Fatalf("an empty calendar produced days=%d rows=%d", geo.Days, len(geo.Rows))
	}
	if geo.WeekLen != blockDefaultWeekLen {
		t.Fatalf("WeekLen = %d, want the named fallback", geo.WeekLen)
	}
	if g := buildMonthGeometry(nil, blockMonthGeometryInput{}); g.WeekLen != blockDefaultWeekLen {
		t.Fatal("a nil calendar must degrade, not panic")
	}
	// Out-of-range month index degrades the same way.
	if g := buildMonthGeometry(blockTenDayCal(), blockMonthGeometryInput{MonthIndex: 99, Year: 1523}); g.Days != 0 {
		t.Fatalf("month index 99 produced %d days", g.Days)
	}
}

func TestBlockTodayDayOnlyMarksTheCurrentMonth(t *testing.T) {
	cal := blockTenDayCal() // current = 1523-01-14
	if got := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1523}).TodayDay; got != 14 {
		t.Fatalf("TodayDay = %d, want 14", got)
	}
	if got := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 1, Year: 1523}).TodayDay; got != 0 {
		t.Fatalf("TodayDay in another month = %d, want 0", got)
	}
	if got := buildMonthGeometry(cal, blockMonthGeometryInput{MonthIndex: 0, Year: 1524}).TodayDay; got != 0 {
		t.Fatalf("TodayDay in another year = %d, want 0", got)
	}
}

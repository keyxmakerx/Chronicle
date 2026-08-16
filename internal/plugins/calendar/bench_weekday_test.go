// bench_weekday_test.go — C-CALV4-WEEKDAY-VIEWS.
//
// THE FEATURE THESE GUARD WAS A LIVE LOSS. /week and /day 301'd to the Bench's
// MONTH from C-CALV4-V2SUNSET R2-4 until this slice, and the two cutover tests
// carried the loss as an explicit NEGATIVE assertion ("a week segment
// reappeared…"). Those two are inverted in v1_v2_cutover_test.go; everything
// here is the surface they now point at.
//
// WHAT IS ASSERTED, IN THE ORDER IT MATTERS:
//
//  1. the column count and the nav step are ONE number — the calendar's own
//     week length. V2 drew ten columns and stepped seven.
//  2. the window is ALIGNED to the month grid, crosses months and years, and
//     names both months when it does.
//  3. multi-day events appear on every day they span. V2 dropped them entirely.
//  4. the hour axis is the calendar's own HoursPerDay and MinutesPerHour.
//  5. a player and a GM see different sets, from ONE filtered pass.
//  6. the markup carries the ANSWER keys the shipped day card triggers on, and
//     the payload for those keys agrees with the columns ([DC-2]'s law).
//  7. the surface renders at every width and hides no control on a phone.
//  8. the crash V2's stepDaysBackward carries is not ported.
package calendar

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- fixtures ---------------------------------------------------------------

// wdFxTenDay is the product's thesis in a fixture: a TEN-day week, three
// 30-day months, hours and minutes deliberately NOT 24/60 so every "is this a
// hardcoded Earth number" assertion below has something to catch.
func wdFxTenDay() *Calendar {
	epoch := "DR"
	cal := &Calendar{
		ID: "cal-wd", CampaignID: "camp-1", Mode: ModeFantasy, Name: "Harptos of Imix",
		EpochName: &epoch, IsDefault: true,
		CurrentYear: 1523, CurrentMonth: 2, CurrentDay: 14,
		HoursPerDay: 30, MinutesPerHour: 100,
	}
	for i, n := range []string{"Deepwinter", "Thawrun", "Sunfall"} {
		cal.Months = append(cal.Months, Month{ID: i + 1, CalendarID: cal.ID, Name: n, Days: 30, SortOrder: i})
	}
	for i, n := range []string{"Sar", "Mol", "Zor", "Wir", "Nym", "Lyr", "Tam", "Kes", "Vel", "Odd"} {
		w := Weekday{ID: i + 1, CalendarID: cal.ID, Name: n, SortOrder: i}
		// The LAST weekday of the tenday is the rest day, so a rest column is
		// exercised in every week the fixture draws.
		if i == 9 {
			w.IsRestDay = true
		}
		cal.Weekdays = append(cal.Weekdays, w)
	}
	cal.Eras = []Era{{ID: 1, CalendarID: cal.ID, Name: "Reckoning of Wards", StartYear: 1000, Color: "#4477aa"}}
	cal.Seasons = []Season{{ID: 1, CalendarID: cal.ID, Name: "Thaw",
		StartMonth: 1, StartDay: 1, EndMonth: 3, EndDay: 30, Color: "#88aa44"}}
	cal.Moons = []Moon{{ID: 1, CalendarID: cal.ID, Name: "Selune", CycleDays: 30}}
	cal.EventCategories = []EventCategory{
		{ID: 1, CalendarID: cal.ID, Slug: "festival", Name: "Festival", Color: "#aa4477", Icon: "✦"},
	}
	return cal
}

func wdInt(i int) *int { return &i }

// wdView runs the real spine against the shared fake repo.
func wdView(t *testing.T, cal *Calendar, evs []Event, kind string, cursor BlockDate, role int) BenchWeekView {
	t.Helper()
	repo := newBlockFakeRepo(cal)
	repo.events[cal.ID] = evs
	svc := NewBlockService(repo)
	w, err := svc.WeekDayView(context.Background(), BenchWeekRequest{
		CalendarID: cal.ID,
		Viewer:     BlockViewer{UserID: "u-1", Role: role},
		Kind:       kind,
		Cursor:     cursor,
		MoonCap:    benchMoonCap,
	})
	if err != nil {
		t.Fatalf("WeekDayView(%s, %+v): %v", kind, cursor, err)
	}
	return w
}

// --- 1. THE COLUMN COUNT AND THE STEP ARE ONE NUMBER ------------------------

// TestWeekView_ColumnCountIsTheCalendarsOwnWeekLength is the slice's headline
// claim and V2's headline defect.
//
// V2 read len(cal.Weekdays) for the columns (correct) and then stepped
// `day += 7 * dir` for the nav (wrong), so on a ten-day calendar the grid drew
// ten days and Next moved seven — consecutive "weeks" overlapped by three,
// every time, and no test anywhere said so. Here the two come from the SAME
// expression, and this asserts they agree at three different week lengths
// including the one that is not 7.
func TestWeekView_ColumnCountIsTheCalendarsOwnWeekLength(t *testing.T) {
	for _, n := range []int{4, 7, 10} {
		t.Run(fmt.Sprintf("%d-day week", n), func(t *testing.T) {
			cal := wdFxTenDay()
			cal.Weekdays = cal.Weekdays[:0]
			for i := 0; i < n; i++ {
				cal.Weekdays = append(cal.Weekdays, Weekday{ID: i + 1, Name: fmt.Sprintf("D%d", i+1), SortOrder: i})
			}
			w := wdView(t, cal, nil, benchViewWeek, BlockDate{}, permissions.RoleOwner)
			if len(w.Days) != n {
				t.Fatalf("week drew %d columns, want %d — the count is len(cal.Weekdays), never a literal",
					len(w.Days), n)
			}
			if w.WeekLen != n {
				t.Errorf("WeekLen=%d, want %d", w.WeekLen, n)
			}

			// THE STEP. Next must land exactly n of the calendar's own days
			// later, so consecutive weeks TILE — no overlap, no gap.
			cursor := BlockDate{Year: w.Days[0].Year, Month: w.Days[0].Month, Day: w.Days[0].Day}
			nav := benchNavForView(cal, "camp-1", benchViewWeek, cursor)
			next := benchWeekStepDate(cal, cursor, n)
			wantNext := benchViewHref("camp-1", benchViewWeek, next)
			if nav.NextHref != wantNext {
				t.Errorf("Next href = %q, want %q (a %d-day week must step %d days)",
					nav.NextHref, wantNext, n, n)
			}
			// And the week AT that cursor must begin on the day after this
			// week's last column — the tiling claim, stated as data.
			nextWeek := benchWeekWindow(cal, next, n)
			after := benchWeekStepDate(cal,
				BlockDate{Year: w.Days[n-1].Year, Month: w.Days[n-1].Month, Day: w.Days[n-1].Day}, 1)
			if nextWeek[0] != after {
				t.Errorf("the next week begins %+v; the day after this week's last column is %+v — "+
					"consecutive weeks must tile, which is exactly what V2's hardcoded 7 broke", nextWeek[0], after)
			}
		})
	}
}

// TestWeekView_WindowIsAlignedToTheMonthGridsOwnColumns.
//
// V2 CENTRED its window on the cursor (`start = cursor − floor(cols/2)`), so
// the same day appeared under a different weekday name depending on which day
// you navigated from, and the week never agreed with the month grid beside it.
// The v4 window is ALIGNED through the SHARED v2WeekdayIndexFor — the same
// index blockMonthLead computes the grid's lead cell from — so column i is
// weekday i, on every calendar and in every year.
func TestWeekView_WindowIsAlignedToTheMonthGridsOwnColumns(t *testing.T) {
	cal := wdFxTenDay()
	for _, cursorDay := range []int{1, 7, 14, 23, 30} {
		w := wdView(t, cal, nil, benchViewWeek,
			BlockDate{Year: 1523, Month: 2, Day: cursorDay}, permissions.RoleOwner)
		for i, d := range w.Days {
			idx := v2WeekdayIndexFor(cal, d.Year, d.Month, d.Day)
			if idx != i {
				t.Fatalf("cursor day %d: column %d holds %d/%d/%d whose shared weekday index is %d — "+
					"an aligned week must put weekday i in column i, or it disagrees with the month grid",
					cursorDay, i, d.Year, d.Month, d.Day, idx)
			}
			if d.Weekday != cal.Weekdays[i].Name {
				t.Errorf("column %d named %q, want %q", i, d.Weekday, cal.Weekdays[i].Name)
			}
		}
		// The cursor must be INSIDE its own week. A window that excluded the day
		// it was asked about would be the whole feature failing silently.
		found := false
		for _, d := range w.Days {
			if d.Day == cursorDay && d.Month == 2 && d.Year == 1523 {
				found = true
			}
		}
		if !found {
			t.Errorf("the week for day %d does not contain day %d", cursorDay, cursorDay)
		}
	}
}

// TestWeekView_RestDayAndTodayAreMarkedOffTheSharedIndex.
func TestWeekView_RestDayAndTodayAreMarkedOffTheSharedIndex(t *testing.T) {
	cal := wdFxTenDay() // current date is 1523-02-14; weekday 9 ("Odd") rests
	w := wdView(t, cal, nil, benchViewWeek, BlockDate{}, permissions.RoleOwner)

	todays := 0
	rests := 0
	for _, d := range w.Days {
		if d.IsToday {
			todays++
			if d.Day != cal.CurrentDay || d.Month != cal.CurrentMonth || d.Year != cal.CurrentYear {
				t.Errorf("column %d/%d/%d claims to be today; the calendar's date is %d/%d/%d",
					d.Year, d.Month, d.Day, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
			}
		}
		if d.IsRest {
			rests++
			if d.Weekday != "Odd" {
				t.Errorf("rest day landed on %q, want the calendar's own declared rest weekday", d.Weekday)
			}
		}
	}
	if todays != 1 {
		t.Errorf("%d columns marked today, want exactly 1 — the unnavigated week contains the calendar's own date", todays)
	}
	if rests != 1 {
		t.Errorf("%d rest columns, want exactly 1 in a tenday with one declared rest day", rests)
	}
}

// --- 2. CROSSING BOUNDARIES -------------------------------------------------

// TestWeekView_CrossesAMonthBoundaryAndNamesBothMonths.
//
// A week straddling a month boundary is the case the column header's month name
// exists for, and it is the case the spine's month-scoped candidateEvents
// cannot serve — so this asserts BOTH halves: the header names both months, and
// the events from BOTH months arrive.
func TestWeekView_CrossesAMonthBoundaryAndNamesBothMonths(t *testing.T) {
	cal := wdFxTenDay() // 30-day months, 10-day weeks: day 26 is weekday index 5
	evs := []Event{
		{ID: "e-a", CalendarID: cal.ID, Name: "End of Deepwinter", Year: 1523, Month: 1, Day: 25, Visibility: "everyone"},
		{ID: "e-b", CalendarID: cal.ID, Name: "Thawrun opens", Year: 1523, Month: 2, Day: 2, Visibility: "everyone"},
	}
	// Day 30 of month 1 and day 1 of month 2 must share a week for this to be
	// the cross-month case; with 30-day months in tendays they do not, so ask
	// for a window whose cursor sits on the last tenday of month 1 and step
	// forward until the window actually crosses. Simpler and deterministic:
	// shorten month 1 so the boundary falls mid-week.
	cal.Months[0].Days = 26
	w := wdView(t, cal, evs, benchViewWeek,
		BlockDate{Year: 1523, Month: 1, Day: 26}, permissions.RoleOwner)

	months := map[int]bool{}
	names := map[string]bool{}
	for _, d := range w.Days {
		months[d.Month] = true
		names[d.MonthName] = true
		if !strings.Contains(d.Header, d.MonthName) {
			t.Errorf("column header %q does not name its own month %q", d.Header, d.MonthName)
		}
	}
	if len(months) != 2 {
		t.Fatalf("the window covers months %v; this case must straddle exactly two", months)
	}
	if !names["Deepwinter"] || !names["Thawrun"] {
		t.Errorf("headers named %v — a week crossing a boundary must print BOTH real month names", names)
	}
	if got := benchWeekHeading(cal, w.Days, benchViewWeek); !strings.Contains(got, "Deepwinter") ||
		!strings.Contains(got, "Thawrun") || !strings.Contains(got, "DR") {
		t.Errorf("cross-month heading = %q, want both month names and the epoch", got)
	}

	// THE EVENTS FROM BOTH MONTHS ARRIVE. This is the half a month-scoped read
	// silently fails: the far columns simply render empty and nothing says so.
	seen := map[string]bool{}
	for _, d := range w.Days {
		for _, e := range append(append([]BenchWeekEvent{}, d.AllDay...), d.Timed...) {
			seen[e.Mark.Title] = true
		}
	}
	for _, want := range []string{"End of Deepwinter", "Thawrun opens"} {
		if !seen[want] {
			t.Errorf("%q is missing — a week that straddles a boundary must read BOTH months", want)
		}
	}
}

// TestWeekView_CrossesAYearBoundary — the case V2 could not express at all: its
// stepper "stops at year-end without rolling over" by its own comment, and its
// event read was same-year only.
func TestWeekView_CrossesAYearBoundary(t *testing.T) {
	cal := wdFxTenDay()
	// ONE 26-day month, so the year turns over mid-week and the window has to
	// roll a YEAR — the case V2's stepper refused by its own comment ("stops at
	// year-end without rolling over") and V2's same-year event read could not
	// have served anyway.
	cal.Months = cal.Months[:1]
	cal.Months[0].Days = 26
	cal.CurrentMonth, cal.CurrentDay = 1, 1
	evs := []Event{
		{ID: "e-old", CalendarID: cal.ID, Name: "Last night of 1523", Year: 1523, Month: 1, Day: 26, Visibility: "everyone"},
		{ID: "e-new", CalendarID: cal.ID, Name: "First dawn of 1524", Year: 1524, Month: 1, Day: 1, Visibility: "everyone"},
	}
	w := wdView(t, cal, evs, benchViewWeek,
		BlockDate{Year: 1523, Month: 1, Day: 26}, permissions.RoleOwner)

	years := map[int]bool{}
	for _, d := range w.Days {
		years[d.Year] = true
	}
	if len(years) != 2 {
		t.Fatalf("the window covers years %v; this case must straddle exactly two", years)
	}
	if got := benchWeekHeading(cal, w.Days, benchViewWeek); !strings.Contains(got, "1523") ||
		!strings.Contains(got, "1524") {
		t.Errorf("cross-year heading = %q, want BOTH years named", got)
	}
	seen := map[string]bool{}
	for _, d := range w.Days {
		for _, e := range append(append([]BenchWeekEvent{}, d.AllDay...), d.Timed...) {
			seen[e.Mark.Title] = true
		}
	}
	if !seen["First dawn of 1524"] {
		t.Error("the next year's event is missing — a same-year read leaves the far columns silently empty")
	}
}

// --- 3. MULTI-DAY EVENTS ----------------------------------------------------

// TestWeekView_MultiDayEventAppearsOnEveryDayItSpans is a GAP CLOSED, not a
// port.
//
// V2's week AND day views both skipped isMultiDayEvent outright, in both the
// all-day strip and the timed placement, and neither view had a ribbon layer —
// so a three-day festival appeared NOWHERE, on either surface. A GM clicking
// day two of a siege the party was standing in saw an empty day. v4's month
// grid already draws such an event on every spanned day (blockEventSpansDate)
// and this surface uses the same predicate.
func TestWeekView_MultiDayEventAppearsOnEveryDayItSpans(t *testing.T) {
	cal := wdFxTenDay()
	ey, em, ed := 1523, 2, 15
	evs := []Event{{
		ID: "e-siege", CalendarID: cal.ID, Name: "The Long Siege", Visibility: "everyone",
		Year: 1523, Month: 2, Day: 13,
		EndYear: &ey, EndMonth: &em, EndDay: &ed,
		StartHour: wdInt(6), StartMinute: wdInt(0),
		EndHour: wdInt(9), EndMinute: wdInt(0),
	}}
	w := wdView(t, cal, evs, benchViewWeek,
		BlockDate{Year: 1523, Month: 2, Day: 14}, permissions.RoleOwner)

	hits := map[int]BenchWeekEvent{}
	for _, d := range w.Days {
		for _, e := range d.Timed {
			if e.Mark.EventID == "e-siege" {
				hits[d.Day] = e
			}
		}
	}
	for _, day := range []int{13, 14, 15} {
		if _, ok := hits[day]; !ok {
			t.Fatalf("day %d does not carry the three-day siege — this is V2's gap, not V2's feature", day)
		}
		if !hits[day].MultiDay {
			t.Errorf("day %d's siege is not flagged MultiDay", day)
		}
	}
	if len(hits) != 3 {
		t.Errorf("the siege lands on %d days, want exactly its 3", len(hits))
	}

	// THE SPANS. Start day runs from its own hour to the day's end, the middle
	// day is the whole day, the end day runs from the day's start to its stored
	// end hour. V2 had no arithmetic for any of this because it drew none.
	rows := benchWeekHourRows(cal) // 30
	if got := hits[13]; got.RowStart != 7 || got.RowSpan != rows-6 {
		t.Errorf("start day span = row %d + %d, want row 7 (hour 6) running to the day's end (%d rows)",
			got.RowStart, got.RowSpan, rows-6)
	}
	if got := hits[14]; got.RowStart != 1 || got.RowSpan != rows {
		t.Errorf("middle day span = row %d + %d, want the whole day (1 + %d)", got.RowStart, got.RowSpan, rows)
	}
	if got := hits[15]; got.RowStart != 1 || got.RowSpan != 9 {
		t.Errorf("end day span = row %d + %d, want row 1 running to the stored end hour 9", got.RowStart, got.RowSpan)
	}
}

// --- 4. THE HOUR AXIS -------------------------------------------------------

// TestWeekView_HourAxisIsTheCalendarsOwnGeometry.
//
// V2's fmtHourLabel hardcoded a two-digit hour and a literal ":00", so a
// calendar with 1000-minute hours printed a label claiming a 60-minute one, and
// hourRows buried a bare 24. The fixture's calendar is 30 hours of 100 minutes
// precisely so both would be caught.
func TestWeekView_HourAxisIsTheCalendarsOwnGeometry(t *testing.T) {
	cal := wdFxTenDay() // 30 hours per day, 100 minutes per hour
	w := wdView(t, cal, nil, benchViewDay, BlockDate{}, permissions.RoleOwner)
	if w.HourRows != 30 {
		t.Fatalf("HourRows=%d, want the calendar's own 30", w.HourRows)
	}
	if len(w.HourLabels) != 30 {
		t.Fatalf("%d hour labels, want one per row", len(w.HourLabels))
	}
	if w.HourLabels[7] != "07:00" {
		t.Errorf("hour 7 labelled %q, want %q", w.HourLabels[7], "07:00")
	}
	if w.HourLabels[29] != "29:00" {
		t.Errorf("hour 29 labelled %q — a 30-hour day must reach 29", w.HourLabels[29])
	}

	// A 1000-minute hour needs three minute digits; V2 would have printed ":00".
	wide := wdFxTenDay()
	wide.MinutesPerHour = 1000
	if got := benchWeekHourLabel(wide, 7); got != "07:000" {
		t.Errorf("a 1000-minute-hour calendar labelled hour 7 %q, want %q — the label must not "+
			"claim a 60-minute hour", got, "07:000")
	}

	// A calendar declaring nothing falls back to the NAMED constant, not to a
	// literal buried in an expression.
	bare := wdFxTenDay()
	bare.HoursPerDay = 0
	if got := benchWeekHourRows(bare); got != benchWeekDefaultHoursPerDay {
		t.Errorf("fallback hour rows = %d, want the named benchWeekDefaultHoursPerDay", got)
	}
}

// TestWeekView_AllDayAndTimedAreOneWalkSplitTwoWays.
//
// The split predicate is `StartHour == nil`, V2's own. An event with no time
// does not have a time, and parking it at hour 0 would put "some time today" in
// the midnight slot. Count must equal the two lists together — they partition
// ONE walk, so no third state can appear.
func TestWeekView_AllDayAndTimedAreOneWalkSplitTwoWays(t *testing.T) {
	cal := wdFxTenDay()
	evs := []Event{
		{ID: "e-untimed", CalendarID: cal.ID, Name: "Market day", Year: 1523, Month: 2, Day: 14, Visibility: "everyone"},
		{ID: "e-timed", CalendarID: cal.ID, Name: "Council", Year: 1523, Month: 2, Day: 14, Visibility: "everyone",
			StartHour: wdInt(9), StartMinute: wdInt(0)},
	}
	w := wdView(t, cal, evs, benchViewDay, BlockDate{Year: 1523, Month: 2, Day: 14}, permissions.RoleOwner)
	d := w.Days[0]
	if len(d.AllDay) != 1 || d.AllDay[0].Mark.Title != "Market day" {
		t.Errorf("all-day list = %+v, want the untimed event alone", d.AllDay)
	}
	if len(d.Timed) != 1 || d.Timed[0].Mark.Title != "Council" {
		t.Errorf("timed list = %+v, want the timed event alone", d.Timed)
	}
	if d.Count != len(d.AllDay)+len(d.Timed) {
		t.Errorf("Count=%d but the two lists hold %d — they must partition ONE walk",
			d.Count, len(d.AllDay)+len(d.Timed))
	}
	// A single-day event with no stored end hour occupies exactly ONE row —
	// V2's rule, kept, because running it to the day's end would assert a
	// duration the operator never entered.
	if d.Timed[0].RowStart != 10 || d.Timed[0].RowSpan != 1 {
		t.Errorf("an end-less timed event spans row %d + %d, want row 10 (hour 9) + 1",
			d.Timed[0].RowStart, d.Timed[0].RowSpan)
	}
}

// --- 5. VISIBILITY ----------------------------------------------------------

// TestWeekView_PlayerAndGMSeeDifferentSetsFromOneFilteredPass.
//
// The whole surface is derived from a single filterEventsByUser result, so a
// dm_only event is absent from a player's columns, from their counts and from
// their aria labels alike — there is no post-filter anywhere for one of the
// three to be forgotten in.
func TestWeekView_PlayerAndGMSeeDifferentSetsFromOneFilteredPass(t *testing.T) {
	cal := wdFxTenDay()
	evs := []Event{
		{ID: "e-open", CalendarID: cal.ID, Name: "Open court", Year: 1523, Month: 2, Day: 14, Visibility: "everyone"},
		{ID: "e-secret", CalendarID: cal.ID, Name: "Warden's writ", Year: 1523, Month: 2, Day: 14, Visibility: "dm_only"},
	}
	gm := wdView(t, cal, evs, benchViewDay, BlockDate{Year: 1523, Month: 2, Day: 14}, permissions.RoleOwner)
	player := wdView(t, cal, evs, benchViewDay, BlockDate{Year: 1523, Month: 2, Day: 14}, permissions.RolePlayer)

	if gm.Days[0].Count != 2 {
		t.Fatalf("GM day count = %d, want 2", gm.Days[0].Count)
	}
	if player.Days[0].Count != 1 {
		t.Fatalf("player day count = %d, want 1 — the dm_only row must not reach them", player.Days[0].Count)
	}
	pl := benchWeekColumnAria(player.Days[0])
	if strings.Contains(pl, "Warden") {
		t.Error("the hidden event's name reached a player's aria label")
	}
	if !strings.Contains(pl, "1 event") {
		t.Errorf("player aria = %q, want the count to be the VIEWER'S own", pl)
	}
	// The GM's audience mark rides the shared Mark and is nil for a player at
	// the producer — permission is ABSENCE, not a greyed placeholder.
	gmMarked := false
	for _, e := range gm.Days[0].AllDay {
		if e.Mark.Audience != nil {
			gmMarked = true
		}
	}
	if !gmMarked {
		t.Error("the GM's dm_only event carries no audience mark")
	}
	for _, e := range player.Days[0].AllDay {
		if e.Mark.Audience != nil {
			t.Error("a player received an audience mark")
		}
	}
}

// --- 6. THE ANSWER KEYS AND THE DAY CARD ------------------------------------

// TestWeekView_KeysAreUniqueAcrossTheWindow.
//
// The month grid keys a day as "<slug>-<day>" because a month holds each
// ordinal ONCE. A week window does not: it can cross a month boundary, and on a
// calendar with one-day intercalary months it can hold "day 1" twice inside ten
// columns. A colliding key makes two columns open the SAME day card, silently.
func TestWeekView_KeysAreUniqueAcrossTheWindow(t *testing.T) {
	cal := wdFxTenDay()
	// Four one-day intercalary months in a row — canonical Harptos' festival
	// days — is the shape that collides under the month grid's key scheme.
	cal.Months = []Month{
		{ID: 1, Name: "Deepwinter", Days: 3, SortOrder: 0},
		{ID: 2, Name: "Midwinter", Days: 1, SortOrder: 1, IsIntercalary: true},
		{ID: 3, Name: "Greengrass", Days: 1, SortOrder: 2, IsIntercalary: true},
		{ID: 4, Name: "Thawrun", Days: 3, SortOrder: 3},
		{ID: 5, Name: "Shieldmeet", Days: 1, SortOrder: 4, IsIntercalary: true},
		{ID: 6, Name: "Sunfall", Days: 3, SortOrder: 5},
	}
	cal.CurrentMonth, cal.CurrentDay = 1, 1
	w := wdView(t, cal, nil, benchViewWeek, BlockDate{Year: 1523, Month: 1, Day: 1}, permissions.RoleOwner)

	ordinals := map[int]int{}
	keys := map[string]bool{}
	ords := map[string]bool{}
	for _, d := range w.Days {
		ordinals[d.Day]++
		if keys[d.DayKey] {
			t.Fatalf("duplicate data-day key %q in one window — two columns would open one card", d.DayKey)
		}
		if ords[d.DayOrd] {
			t.Fatalf("duplicate data-day-ord %q in one window", d.DayOrd)
		}
		keys[d.DayKey] = true
		ords[d.DayOrd] = true
		if d.Intercalary && !strings.HasPrefix(d.DayOrd, "i") {
			t.Errorf("intercalary column %q lost its `i` ord prefix — calendar_daycard.js reads "+
				"exactly that character to know a day is intercalary", d.DayKey)
		}
	}
	// The fixture must actually reproduce the collision the wider key exists
	// for, or this test proves nothing.
	repeated := false
	for _, n := range ordinals {
		if n > 1 {
			repeated = true
		}
	}
	if !repeated {
		t.Fatal("the fixture did not produce a repeated day ordinal; this test cannot fail as written")
	}
}

// TestWeekView_DayCardPayloadAgreesWithTheColumns — the week/day analogue of
// [DC-2]'s agreement law.
//
// FOR ANY DAY, the set the card lists is EXACTLY the set the column draws. It is
// provable because both are re-serialised from one filtered pass; a payload
// still keyed to the MONTH would additionally make every column a dead click,
// which is the "nothing happens" complaint the day card was built to answer.
func TestWeekView_DayCardPayloadAgreesWithTheColumns(t *testing.T) {
	cal := wdFxTenDay()
	evs := []Event{
		{ID: "e-1", CalendarID: cal.ID, Name: "Open court", Year: 1523, Month: 2, Day: 14,
			Visibility: "everyone", Category: strPtrWD("festival")},
		{ID: "e-2", CalendarID: cal.ID, Name: "Council", Year: 1523, Month: 2, Day: 14,
			Visibility: "everyone", StartHour: wdInt(9), StartMinute: wdInt(0)},
		{ID: "e-3", CalendarID: cal.ID, Name: "Dawn watch", Year: 1523, Month: 2, Day: 16,
			Visibility: "everyone", StartHour: wdInt(4), StartMinute: wdInt(0)},
	}
	w := wdView(t, cal, evs, benchViewWeek, BlockDate{Year: 1523, Month: 2, Day: 14}, permissions.RoleOwner)
	pay := buildDayCardCalendarFromWeek(w, cal, true)

	if len(pay.Days) != len(w.Days) {
		t.Fatalf("payload holds %d days, the surface drew %d", len(pay.Days), len(w.Days))
	}
	if pay.LedgerDocked {
		t.Error("the week surface docks no Ledger; the `Open in the Ledger` door must not be offered")
	}
	for i, d := range w.Days {
		p := pay.Days[i]
		if p.Key != d.DayKey || p.Ord != d.DayOrd {
			t.Fatalf("column %d keyed %q/%q, payload keyed %q/%q — the card would never open",
				i, d.DayKey, d.DayOrd, p.Key, p.Ord)
		}
		if p.Year != d.Year || p.Month != d.Month {
			t.Errorf("column %d is %d/%d but the payload says %d/%d — an editor would write to "+
				"the wrong date", i, d.Year, d.Month, p.Year, p.Month)
		}
		want := map[string]bool{}
		for _, e := range append(append([]BenchWeekEvent{}, d.AllDay...), d.Timed...) {
			want[e.Mark.EventID] = true
		}
		got := map[string]bool{}
		for _, e := range p.Events {
			got[e.ID] = true
		}
		if len(want) != len(got) {
			t.Fatalf("column %d draws %d events, payload lists %d", i, len(want), len(got))
		}
		for id := range want {
			if !got[id] {
				t.Errorf("column %d draws %s but the payload omits it", i, id)
			}
		}
	}

	// [DC-1]'s payload law is not widened by this surface. The week view holds
	// richer Event data than the month grid does (it reads the hours off the
	// row) and none of it may reach the payload.
	raw := dayCardPayloadJSON(dayCardSeed{CanAuthor: true},
		dayCardSource{Block: &BenchBlock{Data: benchNavFxBlock(t, cal, 1, 1523), Week: &w}, Calendar: cal})

	// THE ROUTING ITSELF, asserted on the shipped JSON rather than on the
	// builder in isolation: a Block drawing a week must emit the WEEK's keys.
	// Left keyed to the month, every column on the page is a dead click and the
	// only symptom is "clicking a day does nothing" — the exact complaint the
	// day card was built to answer, reintroduced one surface over.
	if !strings.Contains(raw, `"key":"`+w.Days[0].DayKey+`"`) {
		t.Errorf("the page payload does not carry the week's own key %q — dayCardPayloadJSON "+
			"is still serialising the month for a Block that is drawing a week", w.Days[0].DayKey)
	}
	if strings.Contains(raw, `"ledgerDocked":true`) {
		t.Error("the week payload claims a docked Ledger; this surface docks none")
	}
	for _, forbidden := range []string{"description", "recurrence", "visibility_rules", "startHour", "start_hour"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the week payload carries %q — [DC-1] governs this surface too", forbidden)
		}
	}
}

func strPtrWD(s string) *string { return &s }

// --- 7. THE MARKUP ----------------------------------------------------------

// wdRender renders the week/day surface inside the Bench's own block wrapper,
// which is what calendar_daycard.js resolves a clicked day against.
func wdRender(t *testing.T, w BenchWeekView, isOwner bool) string {
	t.Helper()
	data := BenchData{
		CampaignID: "camp-1", CampaignName: "Imix", IsGM: isOwner, IsOwner: isOwner,
		CalendarCount: 1,
	}
	b := BenchBlock{
		Data:   benchNavFxBlock(t, wdFxTenDay(), 1, 1523),
		Manage: BenchManage{CalendarID: w.CalendarID, Name: "Harptos of Imix"},
		Week:   &w,
		Switch: w.Switch,
	}
	data.Primary = &b
	var sb strings.Builder
	if err := benchBlockView(b, data, "primary").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render week surface: %v", err)
	}
	return sb.String()
}

// TestWeekView_MarkupCarriesTheAnswerKeysTheDayCardTriggersOn.
//
// calendar_daycard.js fires on `[data-day][data-day-ord]` inside a
// `[data-bench-block][data-calendar-id]`. If the column headers and the event
// chips do not carry BOTH attributes, the card never opens and this whole
// surface is read-only — which is exactly the state V2's week view was in (its
// day popover required `[data-day-cell]`, month-only, and did not open at all).
func TestWeekView_MarkupCarriesTheAnswerKeysTheDayCardTriggersOn(t *testing.T) {
	cal := wdFxTenDay()
	evs := []Event{
		{ID: "e-1", CalendarID: cal.ID, Name: "Open court", Year: 1523, Month: 2, Day: 14, Visibility: "everyone"},
		{ID: "e-2", CalendarID: cal.ID, Name: "Council", Year: 1523, Month: 2, Day: 14,
			Visibility: "everyone", StartHour: wdInt(9), StartMinute: wdInt(0)},
	}
	w := wdView(t, cal, evs, benchViewWeek, BlockDate{Year: 1523, Month: 2, Day: 14}, permissions.RoleOwner)
	w.Switch = benchViewSwitch(cal, "camp-1", benchViewWeek, BlockDate{Year: 1523, Month: 2, Day: 14})
	html := wdRender(t, w, true)

	if !strings.Contains(html, `data-bench-block="primary"`) ||
		!strings.Contains(html, `data-calendar-id="cal-wd"`) {
		t.Fatal("the surface is not inside the block wrapper the day card resolves a calendar id from")
	}
	for _, d := range w.Days {
		if !strings.Contains(html, `data-day="`+d.DayKey+`"`) {
			t.Errorf("column %s carries no data-day ANSWER key", d.DayKey)
		}
		if !strings.Contains(html, `data-day-ord="`+d.DayOrd+`"`) {
			t.Errorf("column %s carries no data-day-ord", d.DayKey)
		}
	}
	// Both event shapes are clickable, and both name their event.
	for _, id := range []string{"e-1", "e-2"} {
		if !strings.Contains(html, `data-event-id="`+id+`"`) {
			t.Errorf("event %s is not addressable in the markup", id)
		}
	}

	// GUARD B4, asserted on the RENDER rather than trusted from the lint: every
	// element carrying data-cell also carries data-day.
	for _, tag := range wdOpenTags(html) {
		if strings.Contains(tag, "data-cell=") && !strings.Contains(tag, "data-day=") {
			t.Errorf("a dated node carries data-cell with no data-day ANSWER key: %s", tag)
		}
	}
}

var wdTagRe = regexp.MustCompile(`<[a-zA-Z][^>]*>`)

func wdOpenTags(html string) []string { return wdTagRe.FindAllString(html, -1) }

// TestWeekView_ViewSwitchIsAControlAndUsesTheControlNamespace — guard B3.
//
// `data-view` is one of the six <html> STATE_MARKERS in
// tools/check-calendar-v4-lints.sh. An interactive control reusing a
// state-marker noun is what made every click on the signed mockup re-navigate
// twice; controls end in `-pick`.
func TestWeekView_ViewSwitchIsAControlAndUsesTheControlNamespace(t *testing.T) {
	cal := wdFxTenDay()
	cursor := BlockDate{Year: 1523, Month: 2, Day: 14}
	w := wdView(t, cal, nil, benchViewWeek, cursor, permissions.RoleOwner)
	w.Switch = benchViewSwitch(cal, "camp-1", benchViewWeek, cursor)
	html := wdRender(t, w, true)

	for _, want := range []string{
		`data-view-pick="month"`, `data-view-pick="week"`, `data-view-pick="day"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the view switch is missing %s", want)
		}
	}
	if strings.Contains(html, `data-view=`) {
		t.Error("an interactive control reuses the <html> state marker `data-view` (guard B3)")
	}
	// THE CURSOR SURVIVES THE SWITCH. Clicking Month from a week must land on
	// the month the week's cursor day belongs to, not on today.
	if !strings.Contains(html, `href="/campaigns/camp-1/apps/calendar?y=1523&amp;m=2"`) {
		t.Errorf("the Month pill drops the cursor; hrefs in render:\n%s", wdHrefs(html))
	}
	if !strings.Contains(html, `href="/campaigns/camp-1/apps/calendar?view=day&amp;y=1523&amp;m=2&amp;d=14"`) {
		t.Errorf("the Day pill drops the cursor; hrefs in render:\n%s", wdHrefs(html))
	}
	// The active pill is aria-current, NOT a disabled control.
	if !strings.Contains(html, `aria-current="page"`) {
		t.Error("the active view is not announced with aria-current")
	}
	if strings.Contains(html, `data-view-pick="week" href`) && strings.Contains(html, "disabled") {
		t.Error("the active pill must not be disabled — a control that vanishes when you are " +
			"already there teaches the reader it is missing")
	}
}

var wdHrefRe = regexp.MustCompile(`href="[^"]*"`)

func wdHrefs(html string) string { return strings.Join(wdHrefRe.FindAllString(html, -1), "\n") }

// TestWeekView_NavStepsInTheActiveUnitAndSaysSo.
//
// The aria-labels are not decoration: "Prev" moves a reader by a month, a week
// or a day depending on the view, and a screen reader that only hears "Prev"
// cannot tell which. benchNavUnitLabel reads BenchNav.Unit, so the label and the
// step come from one fact.
func TestWeekView_NavStepsInTheActiveUnitAndSaysSo(t *testing.T) {
	cal := wdFxTenDay()
	cursor := BlockDate{Year: 1523, Month: 2, Day: 14}
	for _, tc := range []struct {
		view, unit string
		stepDays   int
	}{
		{benchViewWeek, "week", 10},
		{benchViewDay, "day", 1},
	} {
		t.Run(tc.view, func(t *testing.T) {
			nav := benchNavForView(cal, "camp-1", tc.view, cursor)
			if !nav.Live || nav.Unit != tc.view {
				t.Fatalf("nav = %+v, want a live trio in unit %q", nav, tc.view)
			}
			if got := benchNavUnitLabel(nav.Unit); got != tc.unit {
				t.Errorf("unit label = %q, want %q", got, tc.unit)
			}
			wantNext := benchViewHref("camp-1", tc.view, benchWeekStepDate(cal, cursor, tc.stepDays))
			if nav.NextHref != wantNext {
				t.Errorf("Next = %q, want %q (%d days)", nav.NextHref, wantNext, tc.stepDays)
			}
			wantPrev := benchViewHref("camp-1", tc.view, benchWeekStepDate(cal, cursor, -tc.stepDays))
			if nav.PrevHref != wantPrev {
				t.Errorf("Prev = %q, want %q", nav.PrevHref, wantPrev)
			}
			// `Today` is the view with NO cursor, which resolves to the
			// calendar's own stored in-world date — not the OS clock.
			if nav.TodayHref != "/campaigns/camp-1/apps/calendar?view="+tc.view {
				t.Errorf("Today = %q, want the bare view", nav.TodayHref)
			}
		})
	}

	// The rendered trio names the unit.
	w := wdView(t, cal, nil, benchViewWeek, cursor, permissions.RoleOwner)
	w.Switch = benchViewSwitch(cal, "camp-1", benchViewWeek, cursor)
	b := BenchBlock{
		Data: benchNavFxBlock(t, cal, 1, 1523), Manage: BenchManage{CalendarID: cal.ID},
		Week: &w, Switch: w.Switch, Nav: benchNavForView(cal, "camp-1", benchViewWeek, cursor),
	}
	var sb strings.Builder
	if err := benchBlockView(b, BenchData{CampaignID: "camp-1"}, "primary").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	for _, want := range []string{`aria-label="Previous week"`, `aria-label="Next week"`, `aria-label="Today"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the week nav is missing %s — the control must name the unit it steps in", want)
		}
	}
}

// --- 8. WIDTHS, AND THE CONTROL V2 HID ON A PHONE ---------------------------

// TestWeekViewCSS_ScrollsRatherThanHidingItselfOnAPhone.
//
// V2 hid the Week and Day pills OUTRIGHT below 768px (`hidden md:contents`)
// because neither view had a narrow layout — so on the device most likely to be
// at the table, the surface did not exist. This asserts the v4 replacement
// instead SCROLLS: the three grids share one horizontal scroller (so the
// headers can never desynchronise from the lanes), the hour axis scrolls
// vertically inside it, and no media query hides the switch.
func TestWeekViewCSS_ScrollsRatherThanHidingItselfOnAPhone(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	flat := strings.Join(strings.Fields(code), " ")

	for _, want := range []string{
		".cal-bench .wkview .wkbody { overflow-x: auto;",
		".cal-bench .wkview .wkscroll { max-block-size: 60vh; overflow-y: auto;",
	} {
		if !strings.Contains(flat, strings.Join(strings.Fields(want), " ")) {
			t.Errorf("calendar-bench.css is missing %q — the week must scroll inside its own box, "+
				"never make the page scroll sideways and never hide itself", want)
		}
	}
	// ONE column track for all three grids: headers, all-day strip and hour
	// lanes. Two definitions is how Tuesday's events end up under Friday's name.
	if n := strings.Count(flat, "grid-template-columns: var(--gut) repeat(var(--cols)"); n != 1 {
		t.Errorf("the shared column track is declared %d times, want exactly 1", n)
	}
	// NOTHING HIDES THE SWITCH. V2's `hidden md:contents` is the shape being
	// refused; a display:none on the control at any width is the same refusal.
	for _, rule := range benchCSSRules(code) {
		if !strings.Contains(rule[0], "wkswitch") {
			continue
		}
		if strings.Contains(strings.ReplaceAll(rule[1], " ", ""), "display:none") {
			t.Errorf("`%s` hides the view switch — V2 hid Week and Day below 768px because they "+
				"had no phone layout; this one scrolls instead", rule[0])
		}
	}
}

// TestWeekViewCSS_NoLiteralSevenAndNoMotion.
//
// TWO OF THE SHEET'S OWN THREE LAWS, asserted on the section this slice added.
// V2's week view carried FOUR live sevens; and the Bench's motion allowlist is
// one wrapper wide, so a transition added here would widen everything.
func TestWeekViewCSS_NoLiteralSevenAndNoMotion(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	flat := strings.Join(strings.Fields(code), " ")

	// The column and row counts are custom properties fed by the producer.
	for _, want := range []string{"repeat(var(--cols)", "repeat(var(--hour-rows)"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the grid does not read %q — the count must come from the calendar", want)
		}
	}
	// SCOPED TO §WEEKDAY, and deliberately so. The sheet DOES carry two
	// `repeat(7, …)` tracks — the RSVP availability overlay — and those are
	// correct: /schedule and the availability matrix are REAL-WORLD surfaces
	// keyed to a Gregorian week, which is the same category distinction that
	// makes "point the week view at /schedule" a category error. A blanket ban
	// would be a false claim about this sheet; the in-world week is what may
	// never assume a number.
	wkFlat := strings.Join(strings.Fields(wdCSSSection(t, code)), " ")
	for _, bad := range []string{"repeat(7,", "repeat(7 ,", "repeat(24,", "repeat(10,"} {
		if strings.Contains(wkFlat, bad) {
			t.Errorf("the §WEEKDAY section contains %q — ten-day weeks and non-24-hour days are "+
				"native and neither number may ever be assumed", bad)
		}
	}
	// The motion budget is unchanged by this section: still exactly one
	// reduced-motion wrapper, and every transition inside it. (This duplicates
	// TestBenchCSS_NoMotionAtAll's structural claim on purpose — that test
	// guards the sheet, this one guards that THIS SECTION did not widen it.)
	const guard = "@media (prefers-reduced-motion: no-preference)"
	if n := strings.Count(code, guard); n != 1 {
		t.Fatalf("%d reduced-motion wrappers, want exactly 1", n)
	}
	wk := wdCSSSection(t, code)
	for _, bad := range []string{"transition", "will-change", "@starting-style", "view-transition"} {
		if strings.Contains(wk, bad) {
			t.Errorf("the §WEEKDAY section contains %q — the week grid never moves", bad)
		}
	}
	// COLOUR IS NEVER LOAD-BEARING ALONE (law 3): every event block carries the
	// locked dash pattern beside its hue.
	if !strings.Contains(wk, "--dash") || !strings.Contains(wk, "--gap") {
		t.Error("the week's event ink carries no greyscale pattern channel — hue alone is not an identity")
	}
}

// wdCSSSection returns the §WEEKDAY block of the sheet, comments already
// stripped. It slices from the section's first selector rather than from a
// comment marker, since the comments are gone by then.
func wdCSSSection(t *testing.T, code string) string {
	t.Helper()
	i := strings.Index(code, ".cal-bench .wkview {")
	if i < 0 {
		t.Fatal("the §WEEKDAY section is not in calendar-bench.css")
	}
	return code[i:]
}

// --- 9. THE CRASH V2 CARRIES ------------------------------------------------

// TestWeekView_MonthlessCalendarDoesNotPanic.
//
// V2's weekDays guards only `cols < 1` and then calls stepDaysBackward, which on
// a calendar with weekdays but ZERO months does month-- → month = len(Months) =
// 0 → cal.Months[-1] → PANIC. Its sibling addDaysSimple carries an explicit
// guard for exactly that case and stepDaysBackward does not. The unguarded shape
// is not ported, and this is the proof.
//
// The honest answer is the Block's own FAULT state — the fault printed where the
// dates would go, and no grid — not a grid of zeroes and not an em dash.
func TestWeekView_MonthlessCalendarDoesNotPanic(t *testing.T) {
	cal := wdFxTenDay()
	cal.Months = nil // weekdays but no months: V2's crash shape exactly

	for _, kind := range []string{benchViewWeek, benchViewDay} {
		w := wdView(t, cal, nil, kind, BlockDate{Year: 1523, Month: 1, Day: 1}, permissions.RoleOwner)
		if w.Fault == "" {
			t.Errorf("%s view on a monthless calendar reported no fault", kind)
		}
		if len(w.Days) != 0 {
			t.Errorf("%s view drew %d columns on a calendar that cannot resolve a date", kind, len(w.Days))
		}
	}
	// The stepper itself is the crash site; it must return its input rather
	// than index a month that is not there.
	if got := benchWeekStepDate(cal, BlockDate{Year: 1, Month: 1, Day: 1}, -1); got != (BlockDate{Year: 1, Month: 1, Day: 1}) {
		t.Errorf("benchWeekStepDate on a monthless calendar returned %+v, want its input", got)
	}
	if got := benchWeekWindow(cal, BlockDate{Year: 1, Month: 1, Day: 1}, 10); got != nil {
		t.Errorf("benchWeekWindow on a monthless calendar returned %+v, want nil", got)
	}
}

// --- 10. THE CURSOR ---------------------------------------------------------

// TestWeekView_CursorDelegatesItsClampToResolveView.
//
// resolveView is the sole clamp authority and this slice does not fork it: a
// junk month clamps into the navigated YEAR (not back to today), an unset view
// is the calendar's own current date, and only the DAY — which resolveView has
// no notion of — is clamped here.
func TestWeekView_CursorDelegatesItsClampToResolveView(t *testing.T) {
	cal := wdFxTenDay() // 3 months of 30 days; current date 1523/2/14
	svc := NewBlockService(newBlockFakeRepo(cal))

	for _, tc := range []struct {
		name string
		in   BlockDate
		want BlockDate
	}{
		{"unset is the calendar's own date", BlockDate{}, BlockDate{Year: 1523, Month: 2, Day: 14}},
		{"?y=&m=&d= round-trips", BlockDate{Year: 1600, Month: 3, Day: 7}, BlockDate{Year: 1600, Month: 3, Day: 7}},
		{"a junk month clamps INTO the navigated year, never back to today",
			BlockDate{Year: 1600, Month: 99}, BlockDate{Year: 1600, Month: 3, Day: 1}},
		{"a day past the month's length clamps to its last day",
			BlockDate{Year: 1600, Month: 1, Day: 99}, BlockDate{Year: 1600, Month: 1, Day: 30}},
		{"a day in a navigated month defaults to its FIRST, not to today's ordinal",
			BlockDate{Year: 1600, Month: 1}, BlockDate{Year: 1600, Month: 1, Day: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := svc.benchWeekCursor(cal, tc.in); got != tc.want {
				t.Errorf("cursor %+v resolved to %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}

	// `?view=` is normalised, never rejected: an unknown view is the MONTH.
	for in, want := range map[string]string{
		"week": benchViewWeek, "WEEK": benchViewWeek, " day ": benchViewDay,
		"weeek": benchViewMonth, "": benchViewMonth, "month": benchViewMonth,
	} {
		if got := normalizeBenchView(in); got != want {
			t.Errorf("normalizeBenchView(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- 11. THE DAY VIEW'S OWN ADDITIONS ---------------------------------------

// TestDayView_CarriesTheContextV2sDayViewDidNotHave.
//
// V2's day view had NO today marker, NO rest-day signal and NO celestial, era or
// season readout of any kind — the only "this is today" cue was the heading
// text. Meanwhile the Bench the reader just left prints every declared moon's
// phase in the Ledger's day panel, so a v4 day view without it would be poorer
// than the surface it replaced. It also prints the WEEKDAY, which V2's
// dayHeading omitted while its own week columns printed it.
func TestDayView_CarriesTheContextV2sDayViewDidNotHave(t *testing.T) {
	cal := wdFxTenDay()
	w := wdView(t, cal, nil, benchViewDay, BlockDate{}, permissions.RoleOwner)
	if len(w.Days) != 1 {
		t.Fatalf("the day view drew %d columns, want 1", len(w.Days))
	}
	d := w.Days[0]
	if !d.IsToday {
		t.Error("the unnavigated day view is not marked as today")
	}
	if d.Weekday == "" || !strings.Contains(d.Header, d.Weekday) {
		t.Errorf("the day header %q does not name the weekday; V2's dayHeading omitted it while "+
			"its own week columns printed it", d.Header)
	}
	if d.Era == "" || d.Season == "" {
		t.Errorf("the day carries era=%q season=%q — both are declared on this calendar", d.Era, d.Season)
	}
	if w.SubLabel == "" || !strings.Contains(w.SubLabel, "Reckoning of Wards") {
		t.Errorf("SubLabel = %q, want the era line", w.SubLabel)
	}
	if len(w.Moons) == 0 {
		t.Fatal("the day view carries no celestial readout; the Ledger's day panel already prints one")
	}
	if w.Moons[0].Name != "Selune" || w.Moons[0].Phase == "" {
		t.Errorf("moon line = %+v, want the declared moon with its phase word", w.Moons[0])
	}
	if !strings.Contains(w.Heading, "DR") {
		t.Errorf("day heading %q drops the epoch", w.Heading)
	}

	// The rendered surface prints it.
	html := wdRender(t, w, true)
	if !strings.Contains(html, "data-bench-week-sky") {
		t.Error("the day view's celestial line is not in the DOM")
	}
	if !strings.Contains(html, `data-bench-week="day"`) {
		t.Error("the surface does not identify itself as the day view")
	}
}

// TestWeekView_SubLabelNamesABoundaryRatherThanPickingOneSide.
//
// A week that crosses an era or a season boundary must SAY it crosses one. The
// alternative — naming only the first day's era — is a quiet lie on exactly the
// week a reader most needs the truth.
func TestWeekView_SubLabelNamesABoundaryRatherThanPickingOneSide(t *testing.T) {
	days := []BenchWeekDay{
		{Era: "Reckoning of Wards", Season: "Thaw"},
		{Era: "Reckoning of Wards", Season: "Thaw"},
		{Era: "Age of the Emberfall", Season: "Bloom"},
	}
	got := benchWeekSubLabel(nil, days)
	for _, want := range []string{"Reckoning of Wards → Age of the Emberfall", "Thaw → Bloom"} {
		if !strings.Contains(got, want) {
			t.Errorf("sub-label = %q, want it to contain %q", got, want)
		}
	}
	same := benchWeekSubLabel(nil, []BenchWeekDay{{Era: "Reckoning of Wards"}, {Era: "Reckoning of Wards"}})
	if same != "Reckoning of Wards" {
		t.Errorf("a week inside one era = %q, want the bare name with no arrow", same)
	}
	if none := benchWeekSubLabel(nil, []BenchWeekDay{{}, {}}); none != "" {
		t.Errorf("a calendar declaring neither = %q, want empty — an absent label says "+
			"'this calendar has no eras', a fabricated one does not", none)
	}
}

// --- 12. THE MONTH IS UNCHANGED ---------------------------------------------

// TestBenchView_MonthRenderIsUnchangedAndTheSwitchIsReachableFromIt.
//
// A navigation feature that changes the unnavigated page is a regression wearing
// a feature's clothes. `?view=` absent must render the Block exactly as before —
// and the switch must still be present there, or it is not reachable at all.
func TestBenchView_MonthRenderIsUnchangedAndTheSwitchIsReachableFromIt(t *testing.T) {
	cal := wdFxTenDay()
	b := BenchBlock{
		Data:   benchNavFxBlock(t, cal, 1, 1523),
		Manage: BenchManage{CalendarID: cal.ID, Name: cal.Name},
		Switch: benchViewSwitch(cal, "camp-1", benchViewMonth, BlockDate{Year: 1523, Month: 2, Day: 14}),
	}
	var sb strings.Builder
	if err := benchBlockView(b, BenchData{CampaignID: "camp-1"}, "primary").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, "data-bench-week") {
		t.Error("the month render contains the week surface")
	}
	if !strings.Contains(html, "cal-block-host") {
		t.Error("the month render lost the Block itself")
	}
	if !strings.Contains(html, `data-view-pick="week"`) {
		t.Error("the view switch is unreachable from the month — a control you can only reach " +
			"once you have already left is not a switch")
	}
	// The Month pill carries no `view=` and no `d=`: a URL that names the
	// surface it is not is noise.
	if !strings.Contains(html, `href="/campaigns/camp-1/apps/calendar?y=1523&amp;m=2"`) {
		t.Errorf("the Month href is not the bare month cursor:\n%s", wdHrefs(html))
	}
}

// TestWeekViewCSS_UsesTheSheetsOwnTokensAndDeclaresTheAxisFallback.
//
// TWO CLAIMS, AND THE SECOND ONE WAS A REAL SHIPPED DEFECT CAUGHT BY WRITING
// THIS TEST.
//
//  1. `calblock.AxisFallback` is the literal string `var(--own-none)`, and
//     every axis this surface prints goes through it whenever the operator's
//     category carries no colour. That token is defined in calendar-block.css
//     under `.cal-block-host`, and THIS SURFACE IS NOT INSIDE ONE — it replaces
//     the Block. Undeclared, `--axis: var(--own-none)` is invalid at
//     computed-value time and the event's rail and tint vanish, on exactly the
//     events with no authored hue.
//
//  2. The section uses the sheet's OWN ramp (`--rule-*`, `--surface-*`,
//     `--text-*`, `--gold`, `--accent`), not invented token names propped up by
//     hardcoded rgb fallbacks. An invented name with an rgb fallback renders —
//     and renders the SAME in light and dark, which is how a surface silently
//     stops being theme-aware.
func TestWeekViewCSS_UsesTheSheetsOwnTokensAndDeclaresTheAxisFallback(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	wk := wdCSSSection(t, code)

	// The fallback token must be declared for BOTH schemes, or the dark render
	// borrows a light grey.
	if !strings.Contains(wk, "--own-none:") {
		t.Fatalf("the §WEEKDAY section never declares --own-none, which is what "+
			"calblock.AxisFallback (%q) resolves to — an untyped event's rail would drop out",
			"var(--own-none)")
	}
	if n := strings.Count(wk, "--own-none:"); n < 2 {
		t.Errorf("--own-none is declared %d time(s); it needs a light AND a dark value, "+
			"like every other hue in this sheet", n)
	}
	if !strings.Contains(wk, ".dark .cal-bench .wkview") {
		t.Error("the week surface declares no dark-scheme token block")
	}

	// No invented token names. Each of these was in the first cut of this
	// section, propped up by a hardcoded rgb() fallback that is identical in
	// both schemes.
	for _, invented := range []string{"--border-subtle", "--surface-2,", "Canvas", "Highlight"} {
		if strings.Contains(wk, invented) {
			t.Errorf("the §WEEKDAY section names %q — this sheet's ramp is --rule-*, --surface-*, "+
				"--text-*, --gold and --accent, and an invented name with an rgb fallback "+
				"renders the same in light and dark", invented)
		}
	}
	// And it actually uses the real ones, so the assertion above is proving a
	// substitution rather than an absence.
	for _, want := range []string{"var(--rule-structural)", "var(--surface-inset)", "var(--accent)", "var(--gold)"} {
		if !strings.Contains(wk, want) {
			t.Errorf("the §WEEKDAY section never uses %s", want)
		}
	}
}

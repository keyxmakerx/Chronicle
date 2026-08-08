// block_multiday_test.go — C-CALV4-GAMEREADY §3, [GR-5].
//
// THE FINDING. A five-day festival marked ONE cell. `blockMarksForDate`'s only
// membership test was `OccursOn`, which matches the stored date and the
// recurrence rule and nothing else, so days 2..N of a span carried no mark at
// all. The day card is built straight off those marks
// (buildDayCardCalendar → dayCardEvents(c.Marks)), which means clicking day
// three of a siege the party was standing in produced `.dc-empty` — "No events
// on this day". THAT IS A POSITIVE FALSE STATEMENT, not an omission, and it is
// the half of the V2-parity regression that stops a session.
//
// THE RIBBON IS REFUSED HERE AND BOOKED (C-CALV4-SPAN-RIBBON). This file
// therefore asserts MARKS — five identical chips on five consecutive days —
// and asserts nothing about how they are drawn. `marks.templ` gains no idiom,
// `calblock.Mark` gains no field, and `internal/widgets/calendar_block/data.go`
// is not opened.
package calendar

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// --- 1. the span marks every one of its days --------------------------------

// TestBlockMarks_MultiDaySpansEveryDay is [GR-5]'s projection guard.
func TestBlockMarks_MultiDaySpansEveryDay(t *testing.T) {
	cal := blockSpanCal()
	in := func(role int) BlockProjectionInput {
		return BlockProjectionInput{
			Calendar:   cal,
			Viewer:     BlockViewer{UserID: "u-gm", Role: role},
			MonthIndex: 0, Year: 1523, MoonCap: 3,
		}
	}

	t.Run("a 5-day event marks all 5 of its days", func(t *testing.T) {
		days := blockSpanMarkDays(t, in(permissions.RoleOwner),
			[]Event{blockSpanEvent("The Long Siege", 1523, 1, 5, 1523, 1, 9, storageVisibilityEveryone)},
			"The Long Siege")
		assertDaysMarked(t, days, []int{5, 6, 7, 8, 9})
	})

	t.Run("a single-day event still marks EXACTLY one", func(t *testing.T) {
		// The regression case. A span match that widened the ordinary event
		// would double-report every quiet day in the month.
		days := blockSpanMarkDays(t, in(permissions.RoleOwner),
			[]Event{{ID: "e-1", Name: "Emberfall Vigil", Year: 1523, Month: 1, Day: 14,
				Visibility: storageVisibilityEveryone}},
			"Emberfall Vigil")
		assertDaysMarked(t, days, []int{14})
	})

	t.Run("an end equal to the start is one day, not a span", func(t *testing.T) {
		days := blockSpanMarkDays(t, in(permissions.RoleOwner),
			[]Event{blockSpanEvent("One Day Fair", 1523, 1, 11, 1523, 1, 11, storageVisibilityEveryone)},
			"One Day Fair")
		assertDaysMarked(t, days, []int{11})
	})

	t.Run("a corrupt end BEFORE its own start degrades to the stored day", func(t *testing.T) {
		// The v4 picker cannot author one; the REST API can. Marking nothing (or
		// the whole month) on bad data is worse than marking the day it says.
		days := blockSpanMarkDays(t, in(permissions.RoleOwner),
			[]Event{blockSpanEvent("Inverted", 1523, 1, 9, 1523, 1, 5, storageVisibilityEveryone)},
			"Inverted")
		assertDaysMarked(t, days, []int{9})
	})

	t.Run("the spanned chip reads EXACTLY like the start chip", func(t *testing.T) {
		// Ruled in [GR-5] sub-decision 1: no "day 3 of 5", no continuation
		// glyph, no truncated variant — all three are ribbon-layer concerns and
		// all three would need a calblock.Mark field, which is pinned.
		d := projectBlock(withEvents(in(permissions.RoleOwner),
			blockSpanEvent("The Long Siege", 1523, 1, 5, 1523, 1, 9, storageVisibilityEveryone)))
		var seen []calblock.Mark
		for _, row := range d.Month.Rows {
			for _, cell := range row.Cells {
				for _, m := range cell.Marks {
					if m.Title == "The Long Siege" {
						seen = append(seen, m)
					}
				}
			}
		}
		if len(seen) != 5 {
			t.Fatalf("the span produced %d marks, want 5", len(seen))
		}
		for i, m := range seen[1:] {
			if m.Title != seen[0].Title || m.Axis != seen[0].Axis ||
				m.Pattern != seen[0].Pattern || m.Glyph != seen[0].Glyph ||
				m.Named != seen[0].Named || m.Time != seen[0].Time {
				t.Errorf("day %d of the span rendered a DIFFERENT chip (%+v) from the start day (%+v)",
					i+2, m, seen[0])
			}
		}
	})

	// [GR-5] sub-decision 2: a spanned day genuinely occupies that day, so it
	// counts. A ceiling that pretended otherwise would under-report a busy week,
	// which is the same class of lie this section removes.
	t.Run("a spanned mark COUNTS toward the cell ceiling like any other", func(t *testing.T) {
		var evs []Event
		for i := 0; i < 4; i++ {
			evs = append(evs, blockSpanEvent(
				"Siege "+string(rune('A'+i)), 1523, 1, 5, 1523, 1, 9, storageVisibilityEveryone))
		}
		d := projectBlock(withEvents(in(permissions.RoleOwner), evs...))
		// blockCapMarks keeps the FULL viewer-visible list (the "+n more" popover
		// reads it) and reports the overflow separately, so the claim is that
		// the middle of the span sees all four AND raises the ceiling — exactly
		// as four single-day events on day 7 would.
		mid := blockSpanCell(t, d, 7)
		if len(mid.Marks) != 4 {
			t.Errorf("day 7 carried %d marks, want 4 — a spanned day occupies the day", len(mid.Marks))
		}
		start := blockSpanCell(t, d, 5)
		if mid.MoreCount != start.MoreCount {
			t.Errorf("day 7's overflow count (%d) differs from the span's start day (%d) — "+
				"a spanned mark must reach the ceiling exactly as any other mark does",
				mid.MoreCount, start.MoreCount)
		}
		if mid.MoreCount == 0 {
			t.Error("four marks on one cell must raise the +N more ceiling")
		}
	})
}

// TestBlockMarks_DmOnlySpanIsAbsentOnEveryDay is the leak guard, and it is the
// reason the span match sits INSIDE the visibility-filtered loop rather than
// beside it.
//
// A MEMBERSHIP TEST AT THE WRONG LOOP DEPTH IS HOW A SPAN BECOMES A LEAK. The
// filter runs once, at the top of projectBlock, and `visible` is what the loop
// walks — so a dm_only five-day event must be invisible on ALL FIVE days to a
// Player, not merely on its first. The Owner control in the same test is what
// makes the assertion mean something: without it, a projection that dropped the
// event for everybody would pass.
func TestBlockMarks_DmOnlySpanIsAbsentOnEveryDay(t *testing.T) {
	cal := blockSpanCal()
	ev := blockSpanEvent("The Hidden Rite", 1523, 1, 5, 1523, 1, 9, "dm_only")

	gm := blockSpanMarkDays(t, BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: 0, Year: 1523, MoonCap: 3,
	}, []Event{ev}, "The Hidden Rite")
	assertDaysMarked(t, gm, []int{5, 6, 7, 8, 9})

	player := blockSpanMarkDays(t, BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-pc", Role: permissions.RolePlayer},
		MonthIndex: 0, Year: 1523, MoonCap: 3,
	}, []Event{ev}, "The Hidden Rite")
	if len(player) != 0 {
		t.Errorf("a dm_only span surfaced to a Player on days %v — the span match must sit "+
			"inside the visibility-filtered loop, not beside it", player)
	}
}

// --- 2. the day card stops lying ---------------------------------------------

// TestDayCard_ListsAnInProgressMultiDayEvent is the finding stated as the GM
// experiences it: the surface a GM actually asks "what is happening today?"
// answered "nothing" on day three of a five-day siege.
//
// It asserts through buildDayCardCalendar rather than through the projection so
// that the fix is proven where the lie was TOLD, not only where it originated.
func TestDayCard_ListsAnInProgressMultiDayEvent(t *testing.T) {
	cal := blockSpanCal()
	d := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: 0, Year: 1523, MoonCap: 3,
		Events: []Event{blockSpanEvent("The Long Siege", 1523, 1, 5, 1523, 1, 9, storageVisibilityEveryone)},
	})
	card := buildDayCardCalendar(d, cal, true)
	for _, day := range []int{5, 6, 7, 8, 9} {
		names := dayCardEventNames(card, day)
		if !names["The Long Siege"] {
			t.Errorf(`day %d of the siege renders as an EMPTY day card — `+
				`"No events on this day" is a positive false statement about a day `+
				`the party is standing in`, day)
		}
	}
	if names := dayCardEventNames(card, 10); names["The Long Siege"] {
		t.Error("the day AFTER the span carried the event")
	}
	if names := dayCardEventNames(card, 4); names["The Long Siege"] {
		t.Error("the day BEFORE the span carried the event")
	}
}

// --- 3. the STOP-AND-FLAG measurement: recurrence × span ---------------------

// TestBlockMarks_RecurringSpanIsMEASUREDNotDesigned records what the two
// features do when composed, because [GR-5] names this as a STOP-AND-FLAG:
// "measure what happens and REPORT it; do not design the answer here."
//
// WHAT IT MEASURES, so the next hand does not have to re-derive it: the stored
// [start, end] window is matched ONCE, at the base dates, while `OccursOn`
// expands only the base DAY forward. A weekly three-day event therefore marks
// all three of its first days and then ONE day per later week. That is neither
// obviously right nor obviously wrong — a recurring festival plausibly wants its
// whole length each time — and this test asserts today's behaviour so a future
// slice that changes it has to say so out loud.
//
// THE PLAYABLE FLOOR IS UNAFFECTED: a NON-recurring span works, which is what
// §3 was chartered to buy. Booked as C-CALV4-SPAN-RECURRENCE.
func TestBlockMarks_RecurringSpanIsMEASUREDNotDesigned(t *testing.T) {
	cal := blockSpanCal() // 7-day week
	weekly := RecurrenceWeekly
	ev := blockSpanEvent("The Thrice-Held Rite", 1523, 1, 5, 1523, 1, 7, storageVisibilityEveryone)
	ev.IsRecurring, ev.RecurrenceType = true, &weekly

	days := blockSpanMarkDays(t, BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: 0, Year: 1523, MoonCap: 3,
	}, []Event{ev}, "The Thrice-Held Rite")

	// MEASURED, 2026-08-08: the first occurrence spans (5,6,7); every later
	// weekly occurrence marks its base day only (12, 19, 26).
	assertDaysMarked(t, days, []int{5, 6, 7, 12, 19, 26})
}

// --- 4. the same claim, against a real MariaDB -------------------------------

// TestBlockMultiDay_Integration proves the span end to end.
//
// WHY A FAKE IS NOT ENOUGH, and it is not a theoretical worry: §6 of this same
// slice found that `yearly` expanded perfectly in memory while the ROW was
// never loaded, because a repository query carried a hand-typed list. A span is
// exactly as exposed — `ListEventsForMonth` selects on `e.year = ? AND
// e.month = ?`, so whether a span reaches the month it runs into is a QUERY
// question that no projection test can ask.
//
// THE CROSS-MONTH ARM IS THE ONE THAT MATTERS. A festival from day 28 of one
// month to day 3 of the next has its stored month in the FIRST month only.
func TestBlockMultiDay_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-day integration test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	campaignID, cal := calTestSeedSpanCalendar(t, db)

	spine := NewBlockService(NewBlockRepository(db))
	gm := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}
	player := BlockViewer{UserID: "u-pc", Role: permissions.RolePlayer}

	marks := func(v BlockViewer, year, month int) map[int]map[string]bool {
		t.Helper()
		d, err := spine.Block(ctx, BlockRequest{
			CalendarID: cal.ID, CampaignID: campaignID, Viewer: v,
			View: BlockDate{Year: year, Month: month},
		})
		if err != nil {
			t.Fatalf("Block(%d/%d): %v", year, month, err)
		}
		out := map[int]map[string]bool{}
		for _, row := range d.Month.Rows {
			for _, cell := range row.Cells {
				if cell.Day == 0 {
					continue
				}
				names := map[string]bool{}
				for _, m := range cell.Marks {
					names[m.Title] = true
				}
				out[cell.Day] = names
			}
		}
		return out
	}

	t.Run("a 5-day siege marks all 5 days, on the database", func(t *testing.T) {
		got := marks(gm, 1523, 1)
		for _, day := range []int{5, 6, 7, 8, 9} {
			if !got[day]["The Long Siege"] {
				t.Errorf("day %d of the siege carried no mark", day)
			}
		}
		for _, day := range []int{4, 10} {
			if got[day]["The Long Siege"] {
				t.Errorf("day %d is outside the span and must not carry it", day)
			}
		}
	})

	t.Run("a dm_only span is absent on ALL of its days for a Player", func(t *testing.T) {
		gmSeen := marks(gm, 1523, 1)
		pcSeen := marks(player, 1523, 1)
		for _, day := range []int{5, 6, 7, 8, 9} {
			if !gmSeen[day]["The Hidden Rite"] {
				t.Fatalf("the GM control failed on day %d — the assertion below would be vacuous", day)
			}
			if pcSeen[day]["The Hidden Rite"] {
				t.Errorf("day %d leaked a dm_only span to a Player", day)
			}
		}
	})

	t.Run("a span that CROSSES a month boundary marks its days in BOTH months", func(t *testing.T) {
		first := marks(gm, 1523, 1)
		second := marks(gm, 1523, 2)
		for _, day := range []int{28, 29, 30} {
			if !first[day]["Turn of the Year"] {
				t.Errorf("month 1 day %d carried no mark for the crossing festival", day)
			}
		}
		for _, day := range []int{1, 2, 3} {
			if !second[day]["Turn of the Year"] {
				t.Errorf("month 2 day %d carried no mark — the row never reached the month "+
					"it runs into, which no projection-level test can see", day)
			}
		}
	})
}

// --- fixtures ----------------------------------------------------------------

// blockSpanCal is a three-month, 7-day-week fantasy calendar with 30-day months
// — deliberately plain, so a span assertion is never entangled with leap or
// intercalary geometry.
func blockSpanCal() *Calendar {
	months := make([]Month, 3)
	for i := range months {
		months[i] = Month{Name: []string{"Deepwinter", "Thawrun", "Sunfall"}[i], Days: 30}
	}
	return &Calendar{
		ID: "cal-span", Name: "Harptos of Imix", Mode: ModeFantasy,
		Months: months, Weekdays: make([]Weekday, 7),
		CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 1,
	}
}

func blockSpanEvent(name string, y, m, d, ey, em, ed int, vis string) Event {
	return Event{
		ID: "e-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")), Name: name,
		Year: y, Month: m, Day: d,
		EndYear: &ey, EndMonth: &em, EndDay: &ed,
		Visibility: vis,
	}
}

func withEvents(in BlockProjectionInput, evs ...Event) BlockProjectionInput {
	in.Events = evs
	return in
}

// blockSpanMarkDays returns every day of the rendered month carrying a mark
// titled `title`.
func blockSpanMarkDays(t *testing.T, in BlockProjectionInput, evs []Event, title string) []int {
	t.Helper()
	d := projectBlock(withEvents(in, evs...))
	var out []int
	for _, row := range d.Month.Rows {
		for _, cell := range row.Cells {
			for _, m := range cell.Marks {
				if m.Title == title {
					out = append(out, cell.Day)
				}
			}
		}
	}
	return out
}

func assertDaysMarked(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("marked days %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("marked days %v, want %v", got, want)
		}
	}
}

func blockSpanCell(t *testing.T, d calblock.BlockData, day int) calblock.DayCell {
	t.Helper()
	for _, row := range d.Month.Rows {
		for _, cell := range row.Cells {
			if cell.Day == day {
				return cell
			}
		}
	}
	t.Fatalf("day %d is not in the rendered month", day)
	return calblock.DayCell{}
}

func dayCardEventNames(card dayCardCalendar, day int) map[string]bool {
	out := map[string]bool{}
	for _, d := range card.Days {
		if d.Day != day {
			continue
		}
		for _, e := range d.Events {
			out[e.Title] = true
		}
	}
	return out
}

// calTestSeedSpanCalendar seeds the §3 database fixture: an ordinary five-day
// siege, a dm_only five-day rite (the leak control), and a festival that
// deliberately CROSSES a month boundary.
func calTestSeedSpanCalendar(t *testing.T, db *sql.DB) (string, *Calendar) {
	t.Helper()
	ctx := context.Background()
	campaignID := calTestSeedCampaign(t, db)
	repo := NewCalendarRepository(db)

	cal := &Calendar{
		ID: calTestID(t), CampaignID: campaignID, Name: "Harptos of Imix", Mode: ModeFantasy,
		IsDefault: true, CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if err := repo.SetMonths(ctx, cal.ID, []MonthInput{
		{Name: "Deepwinter", Days: 30}, {Name: "Thawrun", Days: 30}, {Name: "Sunfall", Days: 30},
	}); err != nil {
		t.Fatalf("set months: %v", err)
	}
	if err := repo.SetWeekdays(ctx, cal.ID, []WeekdayInput{
		{Name: "Sar"}, {Name: "Mol"}, {Name: "Zor"}, {Name: "Wir"},
		{Name: "Nym"}, {Name: "Lyr"}, {Name: "Tam"},
	}); err != nil {
		t.Fatalf("set weekdays: %v", err)
	}

	for _, e := range []Event{
		blockSpanEvent("The Long Siege", 1523, 1, 5, 1523, 1, 9, storageVisibilityEveryone),
		blockSpanEvent("The Hidden Rite", 1523, 1, 5, 1523, 1, 9, "dm_only"),
		blockSpanEvent("Turn of the Year", 1523, 1, 28, 1523, 2, 3, storageVisibilityEveryone),
	} {
		row := e
		row.ID = calTestID(t)
		row.CalendarID = cal.ID
		if err := repo.CreateEvent(ctx, &row); err != nil {
			t.Fatalf("create event %q: %v", row.Name, err)
		}
	}
	return campaignID, cal
}

// real_date_anchor_test.go — the anchor arithmetic, pinned.
//
// This is the file that has to be right. Everything downstream of the anchor —
// the availability strip, "when is our next session in-world", any future
// mapping of a real deadline onto a fantasy day — reads a date out of it and
// SHOWS THAT DATE TO A HUMAN. Arithmetic that is off by one does not fail; it
// answers, confidently, with the wrong day, and the surface has no way to know.
//
// So the tests here are round-trips and boundaries rather than spot values:
// a spot value can be copied wrong in both the code and the test.
package calendar

import (
	"fmt"
	"testing"
	"time"
)

// anchorCal is a real, irregular fantasy calendar — deliberately NOT twelve
// 30-day months.
//
// A uniform calendar hides exactly the bug this arithmetic can have: with every
// month the same length, walking months and dividing both give the same answer,
// so a month-walk that is off by one still round-trips. Harptos's real month
// lengths (30 each, but a 31-day Hammer here) plus a ten-day week make the
// month boundaries land somewhere a division would not.
func anchorCal(mut func(*Calendar)) *Calendar {
	c := &Calendar{
		ID: "cal-anchor", CampaignID: "camp-1", Name: "Harptos", Mode: "fantasy",
		Months: []Month{
			{Name: "Hammer", Days: 31, SortOrder: 0},
			{Name: "Alturiak", Days: 28, SortOrder: 1, LeapYearDays: 1},
			{Name: "Ches", Days: 30, SortOrder: 2},
			{Name: "Tarsakh", Days: 30, SortOrder: 3},
			{Name: "Mirtul", Days: 31, SortOrder: 4},
		},
		Weekdays: []Weekday{
			{Name: "1st"}, {Name: "2nd"}, {Name: "3rd"}, {Name: "4th"}, {Name: "5th"},
			{Name: "6th"}, {Name: "7th"}, {Name: "8th"}, {Name: "9th"}, {Name: "10th"},
		},
		LeapYearEvery: 4, LeapYearOffset: 0,
		CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 14,
	}
	if mut != nil {
		mut(c)
	}
	return c
}

func anchored(t *testing.T, c *Calendar, y, m, d int, real string) *Calendar {
	t.Helper()
	rd, err := time.Parse("2006-01-02", real)
	if err != nil {
		t.Fatalf("bad fixture date %q: %v", real, err)
	}
	c.AnchorYear, c.AnchorMonth, c.AnchorDay = &y, &m, &d
	c.AnchorRealDate = &rd
	return c
}

// ── the honest absence ──────────────────────────────────────────────────────

// TestAnchor_UnanchoredAnswersNothing is the first test on purpose.
//
// Every other behaviour in this file is only safe because this one holds: a
// calendar with no anchor must REFUSE, not fall back. A zero time.Time returned
// with ok=true renders as "1 Jan 0001" or, worse, as today — and the entire
// point of the strip staying dormant through the last slice was that a
// fabricated date on screen is worse than an empty one.
func TestAnchor_UnanchoredAnswersNothing(t *testing.T) {
	cases := []struct {
		name string
		cal  *Calendar
	}{
		{"nothing set", anchorCal(nil)},
		{"no months at all", anchored(t, anchorCal(func(c *Calendar) { c.Months = nil }), 1492, 1, 1, "2026-01-01")},
		{"every month zero days", anchored(t, anchorCal(func(c *Calendar) {
			for i := range c.Months {
				c.Months[i].Days, c.Months[i].LeapYearDays = 0, 0
			}
		}), 1492, 1, 1, "2026-01-01")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cal.HasRealAnchor() {
				t.Fatal("HasRealAnchor() is true — the rest of this file's guarantees do not hold")
			}
			if _, ok := tc.cal.RealDateFor(1492, 1, 1); ok {
				t.Error("RealDateFor answered ok on an unanchored calendar; a plausible wrong " +
					"date is the failure mode the whole anchor exists to avoid")
			}
			if _, _, _, ok := tc.cal.InWorldDateFor(time.Now()); ok {
				t.Error("InWorldDateFor answered ok on an unanchored calendar")
			}
		})
	}
}

// TestAnchor_APartialAnchorIsNoAnchor. Three of four fields is not a weaker
// anchor, it is an unanswerable one — there is nothing to count from or count
// to. A row hand-edited in the database must degrade to "not anchored" rather
// than to a wrong answer, so the READ side enforces this and not only the
// writer.
func TestAnchor_APartialAnchorIsNoAnchor(t *testing.T) {
	full := func() *Calendar { return anchored(t, anchorCal(nil), 1492, 4, 14, "2026-10-03") }
	if !full().HasRealAnchor() {
		t.Fatal("the fixture itself is not anchored — every case below would pass vacuously")
	}
	drops := map[string]func(*Calendar){
		"no year":      func(c *Calendar) { c.AnchorYear = nil },
		"no month":     func(c *Calendar) { c.AnchorMonth = nil },
		"no day":       func(c *Calendar) { c.AnchorDay = nil },
		"no real date": func(c *Calendar) { c.AnchorRealDate = nil },
	}
	for name, drop := range drops {
		t.Run(name, func(t *testing.T) {
			c := full()
			drop(c)
			if c.HasRealAnchor() {
				t.Error("a partial anchor reported as anchored — it maps nothing, and " +
					"reporting it as usable turns a missing field into a wrong date")
			}
		})
	}
}

// ── the arithmetic ──────────────────────────────────────────────────────────

// TestAnchor_TheAnchorDayMapsToItself is the one fact the whole scheme rests
// on. If the anchor's own day does not come back as its own real date, every
// other day is off by the same amount and nothing in the product would notice.
func TestAnchor_TheAnchorDayMapsToItself(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 4, 14, "2026-10-03")
	got, ok := c.RealDateFor(1492, 4, 14)
	if !ok {
		t.Fatal("the anchored calendar refused its own anchor date")
	}
	if want := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("the anchor day maps to %s, not to its own real date %s",
			got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
	y, m, d, ok := c.InWorldDateFor(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if !ok || y != 1492 || m != 4 || d != 14 {
		t.Errorf("the anchor's real date maps back to %d/%d/%d (ok=%v), not to the anchor", y, m, d, ok)
	}
}

// TestAnchor_ConsecutiveDaysAreConsecutiveDates walks a whole year one in-world
// day at a time and requires the real date to advance by exactly one, every
// time, across every month boundary and the year boundary.
//
// THIS IS THE TEST THAT CATCHES AN OFF-BY-ONE AT A MONTH EDGE, which is the
// only place this arithmetic can plausibly be wrong: AbsoluteDay sums FULL
// months then adds the day, so a `<=` where a `<` belongs skips or repeats a
// date exactly once per month and nothing else in the product would show it.
func TestAnchor_ConsecutiveDaysAreConsecutiveDates(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 1, 1, "2026-01-01")
	prev, ok := c.RealDateFor(1492, 1, 1)
	if !ok {
		t.Fatal("unanchored fixture")
	}
	steps := 0
	for _, year := range []int{1492, 1493} { // 1492 is a leap year here (÷4)
		for mi := range c.Months {
			for day := 1; day <= c.MonthDays(mi, year); day++ {
				if year == 1492 && mi == 0 && day == 1 {
					continue // the seed
				}
				got, ok := c.RealDateFor(year, mi+1, day)
				if !ok {
					t.Fatalf("refused %d/%d/%d", year, mi+1, day)
				}
				if want := prev.AddDate(0, 0, 1); !got.Equal(want) {
					t.Fatalf("%s %d, %d maps to %s — the previous in-world day was %s, so this "+
						"is a %.0f-day jump. A gap or repeat at a month edge is what an "+
						"off-by-one in the month walk looks like",
						c.Months[mi].Name, day, year, got.Format("2006-01-02"),
						prev.Format("2006-01-02"), got.Sub(prev).Hours()/24)
				}
				prev = got
				steps++
			}
		}
	}
	if steps < 200 {
		t.Fatalf("only %d days walked — the fixture is too small to cross the boundaries "+
			"this test exists to check", steps)
	}
	t.Logf("walked %d consecutive in-world days across two years with no gap or repeat", steps)
}

// TestAnchor_RoundTripsBothWays. Every in-world day in the walk must survive
// fantasy → real → fantasy unchanged.
//
// The two directions are separate implementations — one sums forward with
// AbsoluteDay, the other estimates a year and walks months back — so a
// round-trip is a genuine cross-check rather than a function agreeing with
// itself.
func TestAnchor_RoundTripsBothWays(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 4, 14, "2026-10-03")
	checked := 0
	for _, year := range []int{1490, 1491, 1492, 1493, 1496} {
		for mi := range c.Months {
			for _, day := range []int{1, 2, 14, c.MonthDays(mi, year)} {
				if day < 1 || day > c.MonthDays(mi, year) {
					continue
				}
				real, ok := c.RealDateFor(year, mi+1, day)
				if !ok {
					t.Fatalf("refused %d/%d/%d", year, mi+1, day)
				}
				gy, gm, gd, ok := c.InWorldDateFor(real)
				if !ok {
					t.Fatalf("%s did not map back to any in-world date", real.Format("2006-01-02"))
				}
				if gy != year || gm != mi+1 || gd != day {
					t.Fatalf("%d/%d/%d → %s → %d/%d/%d: the round trip lost the day",
						year, mi+1, day, real.Format("2006-01-02"), gy, gm, gd)
				}
				checked++
			}
		}
	}
	t.Logf("%d in-world dates survived fantasy → real → fantasy across five years", checked)
}

// TestAnchor_LeapYearsAreCounted. The fixture's Alturiak gains a day every 4th
// year. A year that is one day longer than the arithmetic thinks puts every
// subsequent date one further out, forever — the error does not stay local.
func TestAnchor_LeapYearsAreCounted(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 1, 1, "2026-01-01")
	if !c.IsLeapYear(1492) {
		t.Fatal("the fixture's anchor year is not a leap year — this test would prove nothing")
	}
	a, _ := c.RealDateFor(1492, 1, 1)
	b, _ := c.RealDateFor(1493, 1, 1)
	gap := int(b.Sub(a).Hours() / 24)
	if want := c.YearLengthForYear(1492); gap != want {
		t.Errorf("1492 spans %d real days but YearLengthForYear says %d. A leap day the "+
			"anchor does not count shifts every date after it, permanently", gap, want)
	}
	// …and the common year that follows must be one day shorter.
	d, _ := c.RealDateFor(1494, 1, 1)
	if common := int(d.Sub(b).Hours() / 24); common != gap-1 {
		t.Errorf("the common year 1493 spans %d days against the leap year's %d; expected "+
			"exactly one fewer", common, gap)
	}
}

// TestAnchor_TheNamedDayHoldsWhenTheStructureChanges is migration 018's stored
// shape, asserted as behaviour.
//
// The alternative design stored a day COUNT. Both work until the owner edits
// the calendar — and then storing the count holds the count fixed and slides
// the in-world date the anchor NAMES, which is not what "our campaign began on
// Mirtul 3" means. Because the y/m/d is stored and AbsoluteDay re-derives, the
// named day keeps its real date and the days around it re-flow.
func TestAnchor_TheNamedDayHoldsWhenTheStructureChanges(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 5, 3, "2026-10-03")
	before, _ := c.RealDateFor(1492, 5, 3)

	// The owner discovers Ches should have had 29 days, not 30.
	c.Months[2].Days = 29

	after, ok := c.RealDateFor(1492, 5, 3)
	if !ok {
		t.Fatal("the anchor stopped resolving after a structure edit")
	}
	if !before.Equal(after) {
		t.Errorf("the anchored day moved from %s to %s when an unrelated month was "+
			"corrected. Migration 018 stores the NAMED date precisely so this cannot "+
			"happen", before.Format("2006-01-02"), after.Format("2006-01-02"))
	}
}

// TestAnchor_ALateEveningInstantIsStillToday.
//
// `t.UTC().Date()` would roll a 19:00 instant in America/Chicago onto the next
// day before truncating, so a GM asking "what is today, in-world" after dinner
// would be told tomorrow. The date a human means is the date in THEIR zone.
func TestAnchor_ALateEveningInstantIsStillToday(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 4, 14, "2026-10-03")
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("no tzdata for America/Chicago: %v", err)
	}
	evening := time.Date(2026, 10, 3, 21, 30, 0, 0, chicago) // 02:30 UTC on the 4th
	y, m, d, ok := c.InWorldDateFor(evening)
	if !ok {
		t.Fatal("refused a zoned instant")
	}
	if y != 1492 || m != 4 || d != 14 {
		t.Errorf("21:30 on 3 Oct in Chicago resolved to %d/%d/%d — that is the NEXT in-world "+
			"day. The instant was truncated in UTC instead of in its own zone", y, m, d)
	}
}

// ── the boundary ────────────────────────────────────────────────────────────

// TestAnchor_ValidateRefusesADayTheCalendarDoesNotHave.
//
// This is the one class of bad input that cannot be caught downstream: the
// arithmetic NEVER FAILS on a phantom date. AbsoluteDay(1492, 2, 40) returns a
// number quite happily, so an anchor on a day that does not exist maps every
// other day off by however far the phantom overshoots — silently, forever.
func TestAnchor_ValidateRefusesADayTheCalendarDoesNotHave(t *testing.T) {
	c := anchorCal(nil)
	ok := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		a       RealDateAnchor
		wantErr bool
	}{
		{"a real day", RealDateAnchor{Year: 1492, Month: 4, Day: 14, RealDate: ok}, false},
		{"the last day of a 28-day month", RealDateAnchor{Year: 1493, Month: 2, Day: 28, RealDate: ok}, false},
		{"the leap day, in a leap year", RealDateAnchor{Year: 1492, Month: 2, Day: 29, RealDate: ok}, false},
		{"the leap day, in a common year", RealDateAnchor{Year: 1493, Month: 2, Day: 29, RealDate: ok}, true},
		{"day 0", RealDateAnchor{Year: 1492, Month: 4, Day: 0, RealDate: ok}, true},
		{"day 31 of a 30-day month", RealDateAnchor{Year: 1492, Month: 4, Day: 31, RealDate: ok}, true},
		{"month 0", RealDateAnchor{Year: 1492, Month: 0, Day: 1, RealDate: ok}, true},
		{"a month past the last", RealDateAnchor{Year: 1492, Month: 6, Day: 1, RealDate: ok}, true},
		{"a negative year", RealDateAnchor{Year: -1, Month: 1, Day: 1, RealDate: ok}, true},
		{"no real date", RealDateAnchor{Year: 1492, Month: 4, Day: 14}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.a.Validate(c)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate = %v, want error: %v", err, tc.wantErr)
			}
			if err != nil {
				t.Logf("refused with: %v", err)
				// The message reaches an owner in a settings form, so it has to
				// name the problem rather than restate that there is one.
				if s := err.Error(); s == "" || s == "invalid" || s == "error" {
					t.Errorf("the refusal reads %q — an owner cannot act on that", s)
				}
			}
		})
	}
}

// TestAnchor_FromAbsoluteDayRefusesRatherThanGuessing. The inverse's failure
// modes must be reported, never approximated: a fallback date here would be
// indistinguishable from a real answer at every call site.
func TestAnchor_FromAbsoluteDayRefusesRatherThanGuessing(t *testing.T) {
	cases := []struct {
		name string
		cal  *Calendar
		abs  int
	}{
		{"no months", anchorCal(func(c *Calendar) { c.Months = nil }), 100},
		{"zero-length year", anchorCal(func(c *Calendar) {
			for i := range c.Months {
				c.Months[i].Days, c.Months[i].LeapYearDays = 0, 0
			}
		}), 100},
		{"an index before the reckoning", anchorCal(nil), -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if y, m, d, ok := tc.cal.FromAbsoluteDay(tc.abs); ok {
				t.Errorf("resolved to %d/%d/%d instead of refusing", y, m, d)
			}
		})
	}
}

// TestAnchor_FromAbsoluteDayIsTheInverseOfAbsoluteDay, directly, with no anchor
// involved — so a failure here is located in the arithmetic rather than in the
// anchor that uses it.
func TestAnchor_FromAbsoluteDayIsTheInverseOfAbsoluteDay(t *testing.T) {
	c := anchorCal(nil)
	for _, year := range []int{0, 1, 4, 1492, 1493} {
		for mi := range c.Months {
			for day := 1; day <= c.MonthDays(mi, year); day++ {
				abs := c.AbsoluteDay(year, mi+1, day)
				gy, gm, gd, ok := c.FromAbsoluteDay(abs)
				if !ok {
					t.Fatalf("AbsoluteDay(%d,%d,%d)=%d did not invert", year, mi+1, day, abs)
				}
				if gy != year || gm != mi+1 || gd != day {
					t.Fatalf("AbsoluteDay(%d,%d,%d)=%d inverted to %d/%d/%d",
						year, mi+1, day, abs, gy, gm, gd)
				}
			}
		}
	}
}

// TestAnchor_TheSearchIsBounded. FromAbsoluteDay estimates a year by dividing,
// then corrects. A structure edit can move the estimate a long way from the
// answer, and this runs once per rendered day — so it must terminate and report
// rather than spin.
func TestAnchor_TheSearchIsBounded(t *testing.T) {
	c := anchorCal(nil)
	done := make(chan string, 1)
	go func() {
		// An index far beyond anything the calendar models.
		y, m, d, ok := c.FromAbsoluteDay(1 << 40)
		done <- fmt.Sprintf("%d/%d/%d ok=%v", y, m, d, ok)
	}()
	select {
	case got := <-done:
		t.Logf("a 2^40 index returned %s rather than spinning", got)
	case <-time.After(5 * time.Second):
		t.Fatal("FromAbsoluteDay did not terminate on an out-of-range index — the correction " +
			"loop is unbounded, and it runs once per rendered day")
	}
}

// TestAnchor_RoundTripsThroughTheStorageShape. The Calendar row carries four
// nullable pointers; every writer takes one struct. A mismatch between those
// two shapes is exactly how three-of-four gets written.
func TestAnchor_RoundTripsThroughTheStorageShape(t *testing.T) {
	c := anchored(t, anchorCal(nil), 1492, 4, 14, "2026-10-03")
	a := realDateAnchorOf(c)
	if a == nil {
		t.Fatal("an anchored calendar read back as unanchored")
	}
	if a.Year != 1492 || a.Month != 4 || a.Day != 14 {
		t.Errorf("read back %d/%d/%d", a.Year, a.Month, a.Day)
	}
	if want := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC); !a.RealDate.Equal(want) {
		t.Errorf("read back real date %s, want %s", a.RealDate, want)
	}
	if realDateAnchorOf(anchorCal(nil)) != nil {
		t.Error("an unanchored calendar read back as anchored")
	}
}

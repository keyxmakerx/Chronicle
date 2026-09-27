// calendar_recurrence_test.go tests Event.OccursOn, the single recurrence
// expansion predicate every grid/list projection routes through, across
// month + year boundaries and the monthly leap rule (cal.MonthDays).
package calendar

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// recurrenceCal builds a fantasy calendar with a 7-day week and 12 months, most
// 30 days, with month index 1 carrying 28 base days + 1 leap day so the monthly
// leap rule is exercised. Leap years every 4.
func recurrenceCal() *Calendar {
	months := make([]Month, 12)
	for i := range months {
		months[i] = Month{Days: 30}
	}
	months[1] = Month{Days: 28, LeapYearDays: 1}
	return &Calendar{
		Months:        months,
		Weekdays:      make([]Weekday, 7),
		LeapYearEvery: 4,
	}
}

func ptr[T any](v T) *T { return &v }

func recurEvent(rtype string, y, m, d int) Event {
	return Event{Year: y, Month: m, Day: d, IsRecurring: true, RecurrenceType: &rtype}
}

func TestEventOccursOn(t *testing.T) {
	cal := recurrenceCal()

	tests := []struct {
		name    string
		ev      Event
		y, m, d int
		want    bool
	}{
		// Non-recurring: only the stored date.
		{"non-recurring base", Event{Year: 1, Month: 1, Day: 5}, 1, 1, 5, true},
		{"non-recurring other day", Event{Year: 1, Month: 1, Day: 5}, 1, 1, 12, false},

		// Weekly (every 7 days), base (1,1,1).
		{"weekly base", recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 1, true},
		{"weekly +7", recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 8, true},
		{"weekly +14", recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 15, true},
		{"weekly +1 (no)", recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 2, false},
		{"weekly before base (no)", recurEvent(RecurrenceWeekly, 1, 2, 1), 1, 1, 25, false},
		// Across a month boundary: (1,1,29) + 7 = (1,2,6) (month 1 has 30 days).
		{"weekly across month", recurEvent(RecurrenceWeekly, 1, 1, 29), 1, 2, 6, true},
		{"weekly across month (no)", recurEvent(RecurrenceWeekly, 1, 1, 29), 1, 2, 5, false},
		// Across a year boundary: YearLength = 358, (1,12,29)+7 = (2,1,6).
		{"weekly across year", recurEvent(RecurrenceWeekly, 1, 12, 29), 2, 1, 6, true},

		// Biweekly (every 14 days).
		{"biweekly +14", recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 1, 15, true},
		{"biweekly +7 (no)", recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 1, 8, false},

		// Monthly: same day-of-month each month.
		{"monthly +1mo", recurEvent(RecurrenceMonthly, 1, 1, 15), 1, 2, 15, true},
		{"monthly +2mo", recurEvent(RecurrenceMonthly, 1, 1, 15), 1, 3, 15, true},
		{"monthly other day (no)", recurEvent(RecurrenceMonthly, 1, 1, 15), 1, 2, 16, false},
		// Leap rule: a day-30 monthly event skips the 28-day month (index 1) in a
		// non-leap year, but a day-29 monthly lands in a leap year (29-day month 2).
		{"monthly day30 skips short month", recurEvent(RecurrenceMonthly, 1, 1, 30), 1, 2, 30, false},
		{"monthly day29 in leap month", recurEvent(RecurrenceMonthly, 4, 1, 29), 4, 2, 29, true},
		{"monthly day29 in non-leap month (no)", recurEvent(RecurrenceMonthly, 1, 1, 29), 1, 2, 29, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ev.OccursOn(cal, tt.y, tt.m, tt.d); got != tt.want {
				t.Errorf("OccursOn(%d-%d-%d) = %v, want %v", tt.y, tt.m, tt.d, got, tt.want)
			}
		})
	}
}

func TestEventOccursOn_Custom(t *testing.T) {
	cal := recurrenceCal()
	ev := recurEvent(RecurrenceCustom, 1, 1, 1)
	ev.RecurrenceInterval = ptr(3) // every 3 weeks = 21 days
	if !ev.OccursOn(cal, 1, 1, 22) {
		t.Errorf("custom interval 3 should occur at +21 days")
	}
	if ev.OccursOn(cal, 1, 1, 8) {
		t.Errorf("custom interval 3 must NOT occur at +7 days")
	}
}

func TestEventOccursOn_EndAndMax(t *testing.T) {
	cal := recurrenceCal()

	// Recurrence end date (inclusive): weekly base (1,1,1) ending (1,1,15).
	ev := recurEvent(RecurrenceWeekly, 1, 1, 1)
	ev.RecurrenceEndYear, ev.RecurrenceEndMonth, ev.RecurrenceEndDay = ptr(1), ptr(1), ptr(15)
	if !ev.OccursOn(cal, 1, 1, 15) {
		t.Errorf("recurrence should occur on the end date (inclusive)")
	}
	if ev.OccursOn(cal, 1, 1, 22) {
		t.Errorf("recurrence must NOT occur past the end date")
	}

	// Max occurrences (0-based index): base + 1 more, then stop.
	ev2 := recurEvent(RecurrenceWeekly, 1, 1, 1)
	ev2.RecurrenceMaxOccurrences = ptr(2)
	if !ev2.OccursOn(cal, 1, 1, 1) || !ev2.OccursOn(cal, 1, 1, 8) {
		t.Errorf("first two occurrences should land")
	}
	if ev2.OccursOn(cal, 1, 1, 15) {
		t.Errorf("the third occurrence must be capped by max occurrences")
	}
}

// TestRecurrenceMigration_AddsAndDropsColumn pins the default-safe migration:
// up ADDs recurrence_day_of_week (NULL default → existing rows untouched), down
// DROPs it.
func TestRecurrenceMigration_AddsAndDropsColumn(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "migrations")
	up, err := os.ReadFile(filepath.Join(dir, "011_event_recurrence_dow.up.sql"))
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	down, err := os.ReadFile(filepath.Join(dir, "011_event_recurrence_dow.down.sql"))
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if !strings.Contains(string(up), "ADD COLUMN recurrence_day_of_week") || !strings.Contains(string(up), "DEFAULT NULL") {
		t.Errorf("up migration must ADD recurrence_day_of_week with a NULL default (existing rows untouched)")
	}
	if !strings.Contains(string(down), "DROP COLUMN recurrence_day_of_week") {
		t.Errorf("down migration must DROP recurrence_day_of_week")
	}
}

// TestEventOccursOn_MonthlyHonoursInterval pins that a monthly event honours
// recurrence_interval, counted in months, the way the week-based branch
// applies its interval in weeks.
//
// The `interval 1 / 0 / absent / negative` rows are as load-bearing as the
// positive ones: they pin that those four intervals still expand every month.
func TestEventOccursOn_MonthlyHonoursInterval(t *testing.T) {
	cal := recurrenceCal()

	monthly := func(interval *int) Event {
		ev := recurEvent(RecurrenceMonthly, 1, 1, 15)
		ev.RecurrenceInterval = interval
		return ev
	}

	tests := []struct {
		name     string
		interval *int
		y, m, d  int
		want     bool
	}{
		// Every 3 months from (1,1,15): months 1, 4, 7, 10, then (2,1,15).
		{"every-3 base", ptr(3), 1, 1, 15, true},
		{"every-3 +1mo is NOT an occurrence", ptr(3), 1, 2, 15, false},
		{"every-3 +2mo is NOT an occurrence", ptr(3), 1, 3, 15, false},
		{"every-3 +3mo", ptr(3), 1, 4, 15, true},
		{"every-3 +6mo", ptr(3), 1, 7, 15, true},
		{"every-3 +9mo", ptr(3), 1, 10, 15, true},
		// Across the year boundary: 12 months on from the base is +12, and
		// 12 % 3 == 0, so it lands.
		{"every-3 +12mo crosses the year", ptr(3), 2, 1, 15, true},
		{"every-3 +13mo (no)", ptr(3), 2, 2, 15, false},
		{"every-3 +15mo", ptr(3), 2, 4, 15, true},

		// Every 2 months — the smallest interval that changes anything.
		{"every-2 +2mo", ptr(2), 1, 3, 15, true},
		{"every-2 +1mo (no)", ptr(2), 1, 2, 15, false},

		// NOTHING BELOW 2 MOVES. These four are the whole "no stored row
		// changes" claim: each must expand exactly as it did before the fix.
		{"interval 1 still every month", ptr(1), 1, 2, 15, true},
		{"interval 0 still every month", ptr(0), 1, 2, 15, true},
		{"interval absent still every month", nil, 1, 2, 15, true},
		{"interval negative still every month", ptr(-4), 1, 2, 15, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := monthly(tt.interval)
			if got := ev.OccursOn(cal, tt.y, tt.m, tt.d); got != tt.want {
				t.Errorf("OccursOn(%d-%d-%d) with interval %v = %v, want %v",
					tt.y, tt.m, tt.d, tt.interval, got, tt.want)
			}
		})
	}
}

// TestEventOccursOn_MonthlyIntervalRespectsMax pins that
// RecurrenceMaxOccurrences on a monthly event counts occurrences, not months:
// "every 3 months, 4 times" means occurrences at +0, +3, +6 and +9 months.
func TestEventOccursOn_MonthlyIntervalRespectsMax(t *testing.T) {
	cal := recurrenceCal()
	ev := recurEvent(RecurrenceMonthly, 1, 1, 15)
	ev.RecurrenceInterval = ptr(3)
	ev.RecurrenceMaxOccurrences = ptr(4)

	// Occurrence indices 0..3 → months 1, 4, 7, 10 of year 1.
	for _, m := range []int{1, 4, 7, 10} {
		if !ev.OccursOn(cal, 1, m, 15) {
			t.Errorf("every-3-months × 4: month %d should be one of the four occurrences", m)
		}
	}
	// Occurrence index 4 → 12 months on, past the cap.
	if ev.OccursOn(cal, 2, 1, 15) {
		t.Error("every-3-months × 4: the 5th occurrence must be past the cap")
	}

	// With no interval the cap keeps counting months, which for step 1 is the
	// same number — so capped monthly events with no interval do not move.
	plain := recurEvent(RecurrenceMonthly, 1, 1, 15)
	plain.RecurrenceMaxOccurrences = ptr(4)
	for _, m := range []int{1, 2, 3, 4} {
		if !plain.OccursOn(cal, 1, m, 15) {
			t.Errorf("plain monthly × 4: month %d should occur", m)
		}
	}
	if plain.OccursOn(cal, 1, 5, 15) {
		t.Error("plain monthly × 4: the 5th month must be past the cap")
	}
}

// TestAbsDayIndexReconciledWithAbsoluteDay pins the V5 fix: absDayIndex's
// non-real-time branch used to be a leap-naive constant-length sum
// (year*YearLength()) that diverged from the leap-aware AbsoluteDay by
// exactly one day per elapsed leap year. It now delegates to AbsoluteDay
// directly, so the two can never drift apart again.
func TestAbsDayIndexReconciledWithAbsoluteDay(t *testing.T) {
	cal := recurrenceCal() // LeapYearEvery: 4, LeapYearOffset: 0 -> year 4, 8, ... are leap.

	// Consecutive days must always advance the index by exactly 1, including
	// across the year-3 -> year-4 (leap year) boundary.
	prev := cal.absDayIndex(3, 12, 1)
	for day := 2; day <= 30; day++ {
		cur := cal.absDayIndex(3, 12, day)
		if cur-prev != 1 {
			t.Fatalf("absDayIndex(3,12,%d)-prev = %d, want 1", day, cur-prev)
		}
		prev = cur
	}
	if first := cal.absDayIndex(4, 1, 1); first-prev != 1 {
		t.Fatalf("crossing into year 4 must still advance by exactly 1: got delta %d", first-prev)
	}

	// The reconciled counter must equal AbsoluteDay exactly, for both
	// non-leap and leap years -- this equality IS the fix.
	for _, yr := range []int{1, 4, 5, 8} {
		for m := 1; m <= 12; m++ {
			days := cal.MonthDays(m-1, yr)
			for d := 1; d <= days; d++ {
				if got, want := cal.absDayIndex(yr, m, d), cal.AbsoluteDay(yr, m, d); got != want {
					t.Fatalf("absDayIndex(%d,%d,%d) = %d, want %d (AbsoluteDay)", yr, m, d, got, want)
				}
			}
		}
	}

	// A full leap year (year 4) must span YearLength()+1 days (the +1 leap
	// day on month index 1) -- the old counter, using the constant
	// YearLength() everywhere, would have reported YearLength() here too,
	// silently losing the leap day from the recurrence/weekday counter while
	// AbsoluteDay's moon-phase counter kept it.
	if total, want := cal.absDayIndex(5, 1, 1)-cal.absDayIndex(4, 1, 1), cal.YearLength()+1; total != want {
		t.Errorf("year 4 (a leap year) spans %d days, want %d", total, want)
	}
	// A non-leap year (year 1) still spans exactly YearLength() days.
	if total, want := cal.absDayIndex(2, 1, 1)-cal.absDayIndex(1, 1, 1), cal.YearLength(); total != want {
		t.Errorf("year 1 (not a leap year) spans %d days, want %d", total, want)
	}
}

// TestAbsDayIndexStaysMonotonicForNonPositiveYears pins that delegating to
// AbsoluteDay for year > 0 did not also break year <= 0: AbsoluteDay's own
// doc comment says "a negative or zero year contributes nothing" to its
// leap term, so calling it unconditionally would have collapsed every
// non-positive year onto the same index (a campaign event dated year -5
// would have reported the same day as one dated year 0), making a
// weekly-recurring event starting in a negative year fire before its own
// start date. absDayIndex's year<=0 branch (linearDayIndex) must stay
// strictly increasing across the zero boundary instead.
func TestAbsDayIndexStaysMonotonicForNonPositiveYears(t *testing.T) {
	cal := recurrenceCal()

	got := map[int]bool{}
	prev := cal.absDayIndex(-5, 1, 1)
	for _, yr := range []int{-4, -3, -2, -1, 0, 1} {
		cur := cal.absDayIndex(yr, 1, 1)
		if cur <= prev {
			t.Fatalf("absDayIndex(%d,1,1) = %d, want strictly greater than absDayIndex for the previous year (%d)", yr, cur, prev)
		}
		if got[cur] {
			t.Fatalf("absDayIndex(%d,1,1) = %d collides with an earlier year's index", yr, cur)
		}
		got[cur] = true
		prev = cur
	}

	// A weekly event starting at year -1 must never be reported as occurring
	// before its own start date.
	rt := RecurrenceWeekly
	e := Event{Year: -1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &rt}
	if e.OccursOn(cal, -5, 1, 1) {
		t.Error("a weekly event starting in year -1 must not occur in year -5, before its own start")
	}
	if !e.OccursOn(cal, -1, 1, 1) {
		t.Error("a weekly event must occur on its own start date")
	}
}

// TestWeekdayIndex_MonthStartsNewWeek_Harptos pins the Harptos-shaped
// behavior: with MonthStartsNewWeek, day 1 of every month is always the
// first weekday, and a day inside an intercalary month belongs to no week at
// all (-1).
func TestWeekdayIndex_MonthStartsNewWeek_Harptos(t *testing.T) {
	// Simplified Harptos shape: two 30-day regular months separated by a
	// 1-day intercalary festival month, ten-day weeks (a real Harptos has
	// twelve 30-day months and five intercalary festivals; three months are
	// enough to exercise the reset-at-every-month-including-after-a-festival
	// rule without the full calendar).
	cal := &Calendar{
		Months: []Month{
			{Days: 30},
			{Days: 1, IsIntercalary: true},
			{Days: 30},
		},
		Weekdays:           make([]Weekday, 10),
		MonthStartsNewWeek: true,
	}

	tests := []struct {
		name             string
		year, month, day int
		want             int
	}{
		{"day 1 of month 1 is always weekday 0", 1, 1, 1, 0},
		{"day 10 of month 1 is the tenday's last weekday", 1, 1, 10, 9},
		{"day 11 resets within the same month (the next tenday's day 1)", 1, 1, 11, 0},
		{"day 30 of month 1 (the third tenday's last day)", 1, 1, 30, 9},
		{"an intercalary festival day belongs to no week", 1, 2, 1, -1},
		{"day 1 of month 3 resets to weekday 0, regardless of the festival before it", 1, 3, 1, 0},
		{"day 10 of month 3", 1, 3, 10, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cal.WeekdayIndex(tt.year, tt.month, tt.day); got != tt.want {
				t.Errorf("WeekdayIndex(%d,%d,%d) = %d, want %d", tt.year, tt.month, tt.day, got, tt.want)
			}
		})
	}
}

// TestWeekdayIndex_ContinuousAcrossMonthBoundary pins the default
// (MonthStartsNewWeek=false) behavior: weeks run continuously across a month
// boundary, unlike the Harptos-shaped reset case above.
func TestWeekdayIndex_ContinuousAcrossMonthBoundary(t *testing.T) {
	cal := &Calendar{
		Months: []Month{
			{Days: 30},
			{Days: 30},
		},
		Weekdays: make([]Weekday, 10), // MonthStartsNewWeek defaults to false.
	}

	last := cal.WeekdayIndex(1, 1, 30)
	first := cal.WeekdayIndex(1, 2, 1)
	if want := (last + 1) % cal.WeekLength(); first != want {
		t.Errorf("weeks must run continuously across a month boundary: day30=%d, next-month day1=%d, want %d",
			last, first, want)
	}

	// A date before the calendar's epoch must not panic or return a
	// negative index -- WeekdayIndex guards the modulo for exactly this case.
	if idx := cal.WeekdayIndex(-1, 1, 1); idx < 0 || idx >= cal.WeekLength() {
		t.Errorf("WeekdayIndex(-1,1,1) = %d, want a value in [0,%d)", idx, cal.WeekLength())
	}
}

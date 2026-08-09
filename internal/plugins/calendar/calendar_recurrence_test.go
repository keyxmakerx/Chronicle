// calendar_recurrence_test.go — the recurrence expansion predicate
// (C-CAL-EDITOR-EXPANSION PR2). Event.OccursOn is the single source of truth
// every grid/list projection routes through, so it is table-tested here across
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
		name       string
		ev         Event
		y, m, d    int
		want       bool
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

// --- C-SWEEP-R4 stage 22: monthly honours recurrence_interval ----------------

// TestEventOccursOn_MonthlyHonoursInterval is the regression for
// backlog/occurson-monthly-ignores-interval.
//
// The monthly branch used to check the day-of-month and the occurrence cap and
// return, so a stored "every N months" fired EVERY month — the operator's rule
// was accepted, persisted, and then not applied. The week-based branch has
// always applied its interval; this asserts monthly now does the same, counted
// in months.
//
// The `interval 1 / 0 / absent / negative` rows are as load-bearing as the
// positive ones: they are the proof that no stored row moves except the ones
// that were being mis-expanded. Every existing monthly event in an operator's
// database has one of those four intervals, because the shipped editor sends 0
// for the month unit (calendar_daycard.js recurrenceBody).
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

// TestEventOccursOn_MonthlyIntervalRespectsMax pins the half that was booked as
// un-fixable until the interval fork was settled: RecurrenceMaxOccurrences on a
// monthly event counted MONTHS, not occurrences.
//
// "Every 3 months, 4 times" means occurrences at +0, +3, +6 and +9 months. The
// old code compared the raw month offset against the cap, so it stopped after
// month +3 — i.e. after the SECOND occurrence, delivering half the series the
// operator asked for. n/step is the 0-based occurrence index, the same quantity
// `diff/stride` is on the week-based branch.
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

// recurrence_grid_week_test.go pins week-based recurrence on a calendar whose
// MonthStartsNewWeek switch is on: repeats follow the month grid's weekday
// columns and week rows, not a plain day stride.
package calendar

import "testing"

// harptosShaped: ten-day weeks, twelve 30-day months each followed by a 1-day
// festival, the last festival gaining a leap day every four years.
func harptosShaped(switchOn bool) *Calendar {
	var months []Month
	for i := 0; i < 12; i++ {
		months = append(months, Month{Days: 30})
		f := Month{Days: 1, IsIntercalary: true}
		if i == 11 {
			f.LeapYearDays = 1
		}
		months = append(months, f)
	}
	return &Calendar{
		Months:             months,
		Weekdays:           make([]Weekday, 10),
		LeapYearEvery:      4,
		MonthStartsNewWeek: switchOn,
	}
}

// sevenDay: 7-day week, 31- and 28-day months (short last rows), the 28-day
// month gaining a leap day (a fifth row) every four years.
func sevenDay(switchOn bool) *Calendar {
	return &Calendar{
		Months: []Month{
			{Days: 31}, {Days: 28, LeapYearDays: 1}, {Days: 31}, {Days: 30},
		},
		Weekdays:           make([]Weekday, 7),
		LeapYearEvery:      4,
		MonthStartsNewWeek: switchOn,
	}
}

type gridDate struct{ y, m, d int }

// gridOracle expands an event by walking every day with an explicit row
// counter, the slow independent statement of the rule.
func gridOracle(cal *Calendar, ev Event, weeks int, years int) map[gridDate]bool {
	out := map[gridDate]bool{}
	wl := cal.WeekLength()
	baseRow, baseCol, row := -1, -1, 0
	n := 0
	for y := ev.Year; y < ev.Year+years; y++ {
		for mi := range cal.Months {
			md := cal.MonthDays(mi, y)
			if cal.Months[mi].IsIntercalary {
				if y == ev.Year && mi+1 == ev.Month {
					for d := 1; d <= md; d++ {
						if d == ev.Day {
							out[gridDate{y, mi + 1, d}] = true
						}
					}
				}
				continue
			}
			for d := 1; d <= md; d++ {
				r := row + (d-1)/wl
				c := (d - 1) % wl
				if y == ev.Year && mi+1 == ev.Month && d == ev.Day {
					baseRow, baseCol = r, c
				}
				if baseRow < 0 || c != baseCol || r < baseRow || (r-baseRow)%weeks != 0 {
					continue
				}
				if ev.RecurrenceMaxOccurrences != nil && (r-baseRow)/weeks >= *ev.RecurrenceMaxOccurrences {
					continue
				}
				out[gridDate{y, mi + 1, d}] = true
				n++
			}
			row += (md + wl - 1) / wl
		}
	}
	return out
}

func TestGridWeekRecurrenceMatchesOracle(t *testing.T) {
	cals := map[string]*Calendar{"harptos": harptosShaped(true), "seven-day": sevenDay(true)}
	cases := []struct {
		name string
		ev   Event
	}{
		{"weekly", recurEvent(RecurrenceWeekly, 1, 1, 1)},
		{"weekly mid-row", recurEvent(RecurrenceWeekly, 1, 1, 4)},
		{"biweekly", recurEvent(RecurrenceBiWeekly, 1, 1, 3)},
		{"custom every 3", withInterval(recurEvent(RecurrenceCustom, 2, 1, 2), 3)},
		{"custom every 5 across leap year", withInterval(recurEvent(RecurrenceCustom, 3, 1, 5), 5)},
		{"weekly late in year", recurEvent(RecurrenceWeekly, 3, 3, 1)},
		{"weekly capped", func() Event {
			e := recurEvent(RecurrenceWeekly, 1, 1, 1)
			e.RecurrenceMaxOccurrences = ptr(7)
			return e
		}()},
		{"biweekly capped", func() Event {
			e := recurEvent(RecurrenceBiWeekly, 1, 1, 1)
			e.RecurrenceMaxOccurrences = ptr(5)
			return e
		}()},
	}
	for cn, cal := range cals {
		for _, tc := range cases {
			t.Run(cn+"/"+tc.name, func(t *testing.T) {
				weeks := recurrenceWeeks(*tc.ev.RecurrenceType, tc.ev.RecurrenceInterval)
				want := gridOracle(cal, tc.ev, weeks, 7)
				if len(want) < 3 {
					t.Fatalf("oracle produced %d dates; the case proves nothing", len(want))
				}
				for y := tc.ev.Year; y < tc.ev.Year+7; y++ {
					for mi := range cal.Months {
						for d := 1; d <= cal.MonthDays(mi, y); d++ {
							got := tc.ev.OccursOn(cal, y, mi+1, d)
							if got != want[gridDate{y, mi + 1, d}] {
								t.Fatalf("OccursOn(%d-%d-%d) = %v, want %v", y, mi+1, d, got, !got)
							}
						}
					}
				}
			})
		}
	}
}

func withInterval(e Event, n int) Event {
	e.RecurrenceInterval = &n
	return e
}

func TestGridWeekRecurrenceExamples(t *testing.T) {
	h := harptosShaped(true)
	// Months are 1-based: Hammer = 1, its festival = 2, Alturiak = 3, ...
	tests := []struct {
		name    string
		cal     *Calendar
		ev      Event
		y, m, d int
		want    bool
	}{
		{"weekly day 1", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 1, true},
		{"weekly day 11", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 11, true},
		{"weekly day 21", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 1, 21, true},
		{"weekly next month day 1", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 3, 1, true},
		{"weekly next month day 11", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 3, 11, true},
		{"weekly next month day 10 (no)", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 3, 10, false},
		{"weekly next month day 30 (no)", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 3, 30, false},
		{"weekly never on a festival", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 2, 1, false},
		{"biweekly day 11 (no)", h, recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 1, 11, false},
		{"biweekly day 21", h, recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 1, 21, true},
		// Three rows per month, so every-2 rows lands on row 0, 2, 4 ...: next
		// month's day 11 is row 4.
		{"biweekly next month day 11", h, recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 3, 11, true},
		{"biweekly next month day 1 (no)", h, recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 3, 1, false},
		{"custom 3 next month day 1", h, withInterval(recurEvent(RecurrenceCustom, 1, 1, 1), 3), 1, 3, 1, true},
		{"custom 3 same month day 11 (no)", h, withInterval(recurEvent(RecurrenceCustom, 1, 1, 1), 3), 1, 1, 11, false},
		// 12 months x 3 rows = 36 rows a common year, so every-3 stays in step
		// across the year boundary; the leap day adds no row.
		{"custom 3 across year", h, withInterval(recurEvent(RecurrenceCustom, 1, 1, 1), 3), 2, 1, 1, true},
		{"custom 5 across year (no)", h, withInterval(recurEvent(RecurrenceCustom, 1, 1, 1), 5), 2, 1, 1, false},
		{"leap year festival day 2 is no weekday", h, recurEvent(RecurrenceWeekly, 1, 1, 1), 4, 24, 2, false},
		{"before base", h, recurEvent(RecurrenceWeekly, 1, 3, 1), 1, 1, 1, false},
		{"base on festival, own date", h, recurEvent(RecurrenceWeekly, 1, 2, 1), 1, 2, 1, true},
		{"base on festival, next month day 1 (no)", h, recurEvent(RecurrenceWeekly, 1, 2, 1), 1, 3, 1, false},
		{"base on festival, next year (no)", h, recurEvent(RecurrenceWeekly, 1, 2, 1), 2, 2, 1, false},
		{"max occurrences kept", h, func() Event {
			e := recurEvent(RecurrenceWeekly, 1, 1, 1)
			e.RecurrenceMaxOccurrences = ptr(3)
			return e
		}(), 1, 1, 21, true},
		{"max occurrences exceeded", h, func() Event {
			e := recurEvent(RecurrenceWeekly, 1, 1, 1)
			e.RecurrenceMaxOccurrences = ptr(3)
			return e
		}(), 1, 3, 1, false},
		{"end date inclusive", h, func() Event {
			e := recurEvent(RecurrenceWeekly, 1, 1, 1)
			e.RecurrenceEndYear, e.RecurrenceEndMonth, e.RecurrenceEndDay = ptr(1), ptr(1), ptr(11)
			return e
		}(), 1, 1, 11, true},
		{"after end date", h, func() Event {
			e := recurEvent(RecurrenceWeekly, 1, 1, 1)
			e.RecurrenceEndYear, e.RecurrenceEndMonth, e.RecurrenceEndDay = ptr(1), ptr(1), ptr(11)
			return e
		}(), 1, 1, 21, false},
		// A switch-off calendar keeps the plain stride (30 days is 3 weeks of
		// 10, but the festival shifts it by one day).
		{"switch off keeps day stride", harptosShaped(false), recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 3, 10, true},
		{"switch off, no stride hit", harptosShaped(false), recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 3, 1, false},

		// Seven-day week, 31-day month: row 4 holds days 29-31 only.
		{"short row: 31 lands in col 2", sevenDay(true), recurEvent(RecurrenceWeekly, 1, 1, 3), 1, 1, 31, true},
		{"short row: col 3 has no day 32", sevenDay(true), recurEvent(RecurrenceWeekly, 1, 1, 4), 1, 1, 31, false},
		{"short row: next month col 3 first", sevenDay(true), recurEvent(RecurrenceWeekly, 1, 1, 4), 1, 2, 4, true},
		{"short row: next month day 1 for col 0", sevenDay(true), recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 2, 1, true},
		{"short row: biweekly skips month boundary rows", sevenDay(true), recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 2, 1, false},
		// Month 1 has 5 rows (0-4), so month 2 row 0 is row 5 (odd).
		{"short row: biweekly next month row 1", sevenDay(true), recurEvent(RecurrenceBiWeekly, 1, 1, 1), 1, 2, 8, true},
		{"switch off seven-day", sevenDay(false), recurEvent(RecurrenceWeekly, 1, 1, 1), 1, 2, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ev.OccursOn(tt.cal, tt.y, tt.m, tt.d); got != tt.want {
				t.Errorf("OccursOn(%d-%d-%d) = %v, want %v", tt.y, tt.m, tt.d, got, tt.want)
			}
		})
	}
}

// A far-apart date must not cost a walk over the years between.
func TestGridWeekRecurrenceFarDatesAreCheap(t *testing.T) {
	cal := sevenDay(true)
	ev := recurEvent(RecurrenceWeekly, 1, 1, 1)
	if got := cal.weekRowIndex(5_000_000_000, 1, 1) - cal.weekRowIndex(1, 1, 1); got <= 0 {
		t.Fatalf("rows must grow with years, got %d", got)
	}
	_ = ev.OccursOn(cal, 5_000_000_000, 3, 1)
}

func TestGridWeekRecurrenceYearsBeforeOne(t *testing.T) {
	cal := harptosShaped(true)
	ev := recurEvent(RecurrenceWeekly, -1, 1, 1)
	if !ev.OccursOn(cal, 0, 1, 1) || !ev.OccursOn(cal, 1, 1, 11) || ev.OccursOn(cal, 0, 1, 2) {
		t.Fatal("weekly from year -1 must stay in column 0 across year 0")
	}
}

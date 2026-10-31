package calendar

import (
	"testing"
	"time"
)

// This file pins the fix for a CPU-exhaustion DoS: `?year` on the world-state
// seed is unauthenticated and unbounded, and reaches Calendar.AbsoluteDay,
// whose year term must stay closed-form rather than a loop from 0. The tests
// below prove the closed form matches the loop's exact number for every shape
// of calendar the model can express, and that it is no longer linear.

// absDayLoopReference is the original O(year) implementation, kept as the
// oracle: the closed form must match it exactly, so tests assert equality
// against this rather than against baked-in numbers. A negative year counts
// backwards, subtracting each year from it up to year 0.
func absDayLoopReference(c *Calendar, year, month, day int) int {
	total := 0
	for y := 0; y < year; y++ {
		total += c.YearLengthForYear(y)
	}
	for y := year; y < 0; y++ {
		total -= c.YearLengthForYear(y)
	}
	for i := 0; i < month-1 && i < len(c.Months); i++ {
		total += c.MonthDays(i, year)
	}
	total += day
	return total
}

func dosFixtureCalendar(leapEvery, leapOffset int) *Calendar {
	return &Calendar{
		Months: []Month{
			{Days: 30, LeapYearDays: 1}, {Days: 31}, {Days: 30}, {Days: 31},
			{Days: 30}, {Days: 31}, {Days: 30}, {Days: 31},
			{Days: 30}, {Days: 31}, {Days: 30}, {Days: 31},
		},
		LeapYearEvery:  leapEvery,
		LeapYearOffset: leapOffset,
	}
}

// TestAbsoluteDayClosedFormMatchesLoop pins the closed form against the loop it
// replaced. The year range stays small enough that the O(year) oracle is cheap.
func TestAbsoluteDayClosedFormMatchesLoop(t *testing.T) {
	cals := map[string]*Calendar{
		// No modulus at all — IsLeapYear is always false.
		"no-leap": dosFixtureCalendar(0, 0),
		// The ordinary case.
		"every-4-offset-0": dosFixtureCalendar(4, 0),
		// A non-zero offset shifts which residue class is leap; this is the
		// case a naive year/every count gets wrong.
		"every-4-offset-1": dosFixtureCalendar(4, 1),
		"every-4-offset-3": dosFixtureCalendar(4, 3),
		// An offset larger than the modulus must reduce, not run off.
		"every-7-offset-100": dosFixtureCalendar(7, 100),
		// A NEGATIVE offset: Go's % yields a negative remainder here, which is
		// exactly the case the least-non-negative-residue reduction exists for.
		"every-4-offset-neg9": dosFixtureCalendar(4, -9),
		// Every year is a leap year.
		"every-1": dosFixtureCalendar(1, 0),
		// A calendar whose months declare no leap days at all: the leap count
		// is irrelevant and must not perturb the total.
		"no-leap-days": {
			Months:        []Month{{Days: 45}, {Days: 45}, {Days: 45}},
			LeapYearEvery: 4,
		},
		// A calendar with no months at all — the year term is zero and the
		// month loop must not index anything.
		"no-months": {LeapYearEvery: 4},
	}

	// Years deliberately include 0 and negatives, where leap years before
	// year 0 must be counted backwards.
	years := []int{-1000, -401, -9, -8, -7, -5, -4, -3, -1, 0, 1, 2, 3, 4, 5, 99, 100, 101, 400, 1523, 4001}
	months := []int{0, 1, 2, 6, 12, 13, 99}
	days := []int{0, 1, 15, 45}

	for name, cal := range cals {
		for _, y := range years {
			for _, m := range months {
				for _, d := range days {
					want := absDayLoopReference(cal, y, m, d)
					got := cal.AbsoluteDay(y, m, d)
					if got != want {
						t.Fatalf("%s: AbsoluteDay(%d,%d,%d) = %d, loop reference = %d",
							name, y, m, d, got, want)
					}
				}
			}
		}
	}
}

// absDayWithinBudget runs work and reports whether it finished inside budget.
// It races work against a timer, rather than measuring elapsed time after the
// fact, so a regression to linear time fails fast instead of hanging until
// `go test`'s own deadline. The abandoned goroutine, if any, is collected
// when the process exits.
func absDayWithinBudget(budget time.Duration, work func() int) (int, time.Duration, bool) {
	type result struct {
		sink    int
		elapsed time.Duration
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() { done <- result{work(), time.Since(start)} }()
	select {
	case r := <-done:
		return r.sink, r.elapsed, r.elapsed <= budget
	case <-time.After(budget):
		return 0, budget, false
	}
}

// TestAbsoluteDayClosedFormIsNotLinear is the DoS regression: it asserts a
// wall-clock budget, since the defect was wall-clock (an unauthenticated
// `?year` could pin a core for the better part of a minute). The budget is
// loose so a loaded CI box cannot flake it.
func TestAbsoluteDayClosedFormIsNotLinear(t *testing.T) {
	cal := dosFixtureCalendar(4, 0)

	// The three years measured on the unfixed code, worst first:
	//   2000000000 → 53.5 s   100000000 → 2.71 s   250000 → 7.9 ms
	for _, year := range []int{2000000000, 100000000, 250000} {
		const iterations = 10000
		sink, elapsed, ok := absDayWithinBudget(time.Second, func() int {
			var acc int
			for i := 0; i < iterations; i++ {
				acc += cal.AbsoluteDay(year, 6, 15)
			}
			return acc
		})
		if !ok {
			t.Fatalf("AbsoluteDay(year=%d) × %d exceeded %v — the year term is linear in the "+
				"year again; an unauthenticated ?year on the world-state seed is a "+
				"CPU-exhaustion vector", year, iterations, elapsed)
		}
		if sink == 0 {
			t.Fatal("compiler elided the call")
		}
		t.Logf("year=%-12d %d calls in %v", year, iterations, elapsed)
	}
}

// TestAbsoluteDayHugeYearIsExact pins the closed form's arithmetic at large
// years. The expected totals are derived by hand from the fixture, not
// captured from the implementation, so a regression can't rewrite its own
// expectation.
func TestAbsoluteDayHugeYearIsExact(t *testing.T) {
	cal := dosFixtureCalendar(4, 0)
	cases := []struct {
		year int
		want int
	}{
		// 366·250000 + 1·62500 leap days + (31+31+30+31+30) + 15
		{250000, 91500000 + 62500 + 153 + 15},
		// 366·100000000 + 1·25000000 + 153 + 15
		{100000000, 36600000000 + 25000000 + 153 + 15},
		// 366·2000000000 + 1·500000000 + 153 + 15
		{2000000000, 732000000000 + 500000000 + 153 + 15},
	}
	for _, tc := range cases {
		if got := cal.AbsoluteDay(tc.year, 6, 15); got != tc.want {
			t.Errorf("AbsoluteDay(%d,6,15) = %d, want %d", tc.year, got, tc.want)
		}
	}
}

// TestAbsoluteDayOrdersAcrossYearZero walks every day from year -9 to year 3
// and requires each to be exactly one more than the day before, so dates in
// year 0 and before it sort in calendar order.
func TestAbsoluteDayOrdersAcrossYearZero(t *testing.T) {
	cals := map[string]*Calendar{
		"no-leap":             dosFixtureCalendar(0, 0),
		"every-4-offset-0":    dosFixtureCalendar(4, 0),
		"every-4-offset-1":    dosFixtureCalendar(4, 1),
		"every-4-offset-neg9": dosFixtureCalendar(4, -9),
		"every-1":             dosFixtureCalendar(1, 0),
	}
	for name, cal := range cals {
		prev, first := 0, true
		for y := -9; y <= 3; y++ {
			for m := 1; m <= len(cal.Months); m++ {
				for d := 1; d <= cal.MonthDays(m-1, y); d++ {
					got := cal.AbsoluteDay(y, m, d)
					if !first && got != prev+1 {
						t.Fatalf("%s: AbsoluteDay(%d,%d,%d) = %d, want %d", name, y, m, d, got, prev+1)
					}
					prev, first = got, false
				}
			}
		}
	}

	cal := dosFixtureCalendar(4, 0)
	tests := []struct {
		name       string
		y1, m1, d1 int
		y2, m2, d2 int
	}{
		{"year -5 before year -3", -5, 6, 1, -3, 1, 1},
		{"late in year -1 before year 0", -1, 12, 31, 0, 1, 1},
		{"year 0 before year 1", 0, 12, 31, 1, 1, 1},
		{"day in year -5 before a later day", -5, 1, 1, -5, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if a, b := cal.AbsoluteDay(tt.y1, tt.m1, tt.d1), cal.AbsoluteDay(tt.y2, tt.m2, tt.d2); a >= b {
				t.Fatalf("AbsoluteDay(%d,%d,%d) = %d is not before AbsoluteDay(%d,%d,%d) = %d", tt.y1, tt.m1, tt.d1, a, tt.y2, tt.m2, tt.d2, b)
			}
		})
	}
}

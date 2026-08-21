package calendar

import (
	"testing"
	"time"
)

// C-SWEEP-R4 stage 21 — backend/cal-worldstate-year-dos.
//
// `?year` on the world-state seed is unauthenticated and unbounded, and it
// reached Calendar.AbsoluteDay, whose year term used to be a loop from 0. The
// two tests below are the pair that keeps the fix honest: the first proves the
// closed form still computes the loop's exact number for every shape of
// calendar the model can express, the second proves it is no longer linear.

// absDayLoopReference is the ORIGINAL O(year) implementation, kept here as the
// oracle. The closed form is only allowed to be faster — never different — so
// the test asserts equality against this rather than against baked-in numbers,
// which would not have caught a leap-count off-by-one at an offset the author
// happened not to tabulate.
func absDayLoopReference(c *Calendar, year, month, day int) int {
	total := 0
	for y := 0; y < year; y++ {
		total += c.YearLengthForYear(y)
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

	// Years deliberately include 0 and negatives: the loop contributed nothing
	// for those and the closed form must agree rather than "improve" on it.
	years := []int{-1000, -1, 0, 1, 2, 3, 4, 5, 99, 100, 101, 400, 1523, 4001}
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
//
// THE GOROUTINE IS THE POINT, not ceremony. A linear AbsoluteDay at
// year=2000000000 takes 53 s per call, so a plain `start := time.Now(); work();
// time.Since(start) > budget` check does not FAIL when the regression returns —
// it HANGS until `go test`'s own deadline shoots it, half an hour of CI later,
// with a stack dump instead of the sentence that says what broke. Racing the
// work against a timer turns the regression into a fast, legible failure. The
// abandoned goroutine finishes into a buffered channel nobody reads and is
// collected when the process exits; that is acceptable in a test whose only
// other option is to wait for it.
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

// TestAbsoluteDayClosedFormIsNotLinear is the DoS regression proper.
//
// It asserts a WALL-CLOCK BUDGET rather than an instruction count because the
// defect was wall-clock: one unauthenticated GET pinned a core for the better
// part of a minute. The budget is loose (a full second for ten thousand calls
// at the largest year) so a loaded CI box cannot flake it — the loop is six
// orders of magnitude away from passing, not a factor of two.
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

// TestAbsoluteDayHugeYearIsExact guards the half of the fix a timing budget
// cannot see: that the fast path is still ARITHMETICALLY right at the sizes
// that motivated it. These totals are derived by hand from the fixture
// (12 months summing to 366 common days, one leap day every 4 years from
// year 0, month 6 → five whole months of 31+30+31+30+31 in a leap year, day 15)
// rather than captured from the implementation, so a regression in the closed
// form cannot quietly rewrite its own expectation.
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

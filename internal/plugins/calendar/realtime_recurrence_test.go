// realtime_recurrence_test.go — C-CAL-RT-RECUR-FIX. Pins that week-based
// recurrence (weekly/biweekly/custom) on a real-time calendar expands through
// the proleptic-Gregorian JDN day counter (Calendar.absDayIndex's real-time
// branch), so it stops drifting +1 column and double-rendering across the
// 2028-02-29 leap boundary — landing on the SAME column the P3 0a weekday fix
// already renders. Non-real-time (fantasy / reallife-manual) recurrence stays
// byte-identical to the constant-length stride, proven against an independent
// reproduction of the old formula.
package calendar

import "testing"

// clDayIndex reproduces the pre-fix constant-YearLength absolute-day counter
// (year*YearLength() + prior configured month days + day) so the fantasy
// zero-change control can assert the non-real-time path is byte-identical to it.
func clDayIndex(cal *Calendar, year, month, day int) int {
	abs := year * cal.YearLength()
	for i := 0; i < month-1 && i < len(cal.Months); i++ {
		abs += cal.Months[i].Days
	}
	return abs + day
}

// TestRealTimeRecurrence_WeeklyNoDoubleRender pins the double-render half of the
// bug. Base 2028-01-04 is a Tuesday. Under the old constant-length count Feb 29
// and Mar 1 collapsed onto the same absolute index (configured Feb=28 while a
// real-time MonthDays reports the Gregorian 29), so a Tuesday weekly event
// matched BOTH — rendering onto Wed Mar 1 as well. The JDN counter gives Feb 29
// and Mar 1 distinct indices, so only the real Tuesday (Feb 29) matches.
func TestRealTimeRecurrence_WeeklyNoDoubleRender(t *testing.T) {
	cal := rtWeekdayCal() // real-time Gregorian, Sun..Sat week
	ev := recurEvent(RecurrenceWeekly, 2028, 1, 4)

	cases := []struct {
		m, d int
		want bool
		note string
	}{
		{1, 4, true, "base Tuesday"},
		{2, 29, true, "Tue — the correct occurrence"},
		{3, 1, false, "Wed — old code double-rendered here (Feb29/Mar1 shared an index)"},
		{2, 28, false, "Mon"},
		{3, 2, false, "Thu"},
	}
	for _, tc := range cases {
		if got := ev.OccursOn(cal, 2028, tc.m, tc.d); got != tc.want {
			t.Errorf("weekly@2028-01-04 OccursOn(2028-%02d-%02d) = %v; want %v (%s)",
				tc.m, tc.d, got, tc.want, tc.note)
		}
	}
}

// TestRealTimeRecurrence_WeeklyNoLeapDrift pins the +1-column-drift half. Base
// 2028-02-23 is a Wednesday. The old count missed the leap day, so the next
// occurrence landed on Thu Mar 2 (drifted +1) and the true Wednesday Mar 1 was
// skipped. JDN keeps every Wednesday a Wednesday: Mar 1 matches, Mar 2 does not.
func TestRealTimeRecurrence_WeeklyNoLeapDrift(t *testing.T) {
	cal := rtWeekdayCal()
	ev := recurEvent(RecurrenceWeekly, 2028, 2, 23)

	cases := []struct {
		m, d int
		want bool
		note string
	}{
		{2, 23, true, "base Wednesday"},
		{3, 1, true, "Wed — JDN-correct column (old code skipped it)"},
		{3, 2, false, "Thu — old code drifted the occurrence here"},
		{2, 29, false, "Tue"},
		{3, 8, true, "Wed +14"},
	}
	for _, tc := range cases {
		if got := ev.OccursOn(cal, 2028, tc.m, tc.d); got != tc.want {
			t.Errorf("weekly@2028-02-23 OccursOn(2028-%02d-%02d) = %v; want %v (%s)",
				tc.m, tc.d, got, tc.want, tc.note)
		}
	}
}

// TestRealTimeRecurrence_MatchesDisplayColumn ties recurrence to the display
// grid directly: for a real-time weekly event, an occurrence lands on exactly
// the dates whose display weekday column (v2WeekdayIndexFor — the P3 0a JDN
// path) equals the base's. This is the invariant the fix restores — a weekly
// event and the column it renders in can never disagree, including across the
// 2028 leap boundary.
func TestRealTimeRecurrence_MatchesDisplayColumn(t *testing.T) {
	cal := rtWeekdayCal()
	baseY, baseM, baseD := 2028, 2, 23
	ev := recurEvent(RecurrenceWeekly, baseY, baseM, baseD)
	baseCol := v2WeekdayIndexFor(cal, baseY, baseM, baseD)

	// Walk every day across the leap boundary; the recurrence predicate must
	// agree with "same display column as the base" on each one.
	for _, dt := range [][2]int{
		{2, 23}, {2, 24}, {2, 25}, {2, 26}, {2, 27}, {2, 28}, {2, 29},
		{3, 1}, {3, 2}, {3, 3}, {3, 4}, {3, 5}, {3, 6}, {3, 7}, {3, 8},
	} {
		m, d := dt[0], dt[1]
		wantCol := v2WeekdayIndexFor(cal, 2028, m, d) == baseCol
		if got := ev.OccursOn(cal, 2028, m, d); got != wantCol {
			t.Errorf("2028-%02d-%02d: OccursOn=%v but sameDisplayColumn=%v (col=%d, base=%d) — recurrence must match the JDN grid",
				m, d, got, wantCol, v2WeekdayIndexFor(cal, 2028, m, d), baseCol)
		}
	}
}

// TestRealTimeRecurrence_PreDriftUnchanged pins that real-time recurrence is
// unchanged before the first missed leap day (2028-02-29): a 2026 weekly event
// matches exactly the constant-length stride (JDN and the old formula agree when
// no Gregorian leap day lies in the span), so the fix is a no-op pre-2028.
func TestRealTimeRecurrence_PreDriftUnchanged(t *testing.T) {
	cal := rtWeekdayCal()
	ev := recurEvent(RecurrenceWeekly, 2026, 6, 8) // a Monday

	cases := []struct {
		m, d int
		want bool
	}{
		{6, 8, true},   // base
		{6, 9, false},  // +1
		{6, 15, true},  // +7
		{6, 22, true},  // +14
		{6, 16, false}, // +8
	}
	for _, tc := range cases {
		if got := ev.OccursOn(cal, 2026, tc.m, tc.d); got != tc.want {
			t.Errorf("weekly@2026-06-08 OccursOn(2026-%02d-%02d) = %v; want %v (pre-drift, must be unchanged)",
				tc.m, tc.d, got, tc.want)
		}
	}
}

// TestRealTimeRecurrence_FantasyByteIdentical is the zero-change control: a
// non-real-time (fantasy) calendar's week-based recurrence must follow the
// constant-length stride byte-for-byte — the JDN branch is never entered for it.
// Asserted against clDayIndex, an independent reproduction of the pre-fix
// formula, across dates that include a would-be leap boundary.
func TestRealTimeRecurrence_FantasyByteIdentical(t *testing.T) {
	cal := recurrenceCal() // fantasy: UsesRealTime()=false, 7-day week, YearLength 358
	if cal.UsesRealTime() {
		t.Fatal("fantasy control calendar must not be real-time")
	}
	wl := cal.WeekLength()
	base := recurEvent(RecurrenceWeekly, 1, 1, 1)
	baseIdx := clDayIndex(cal, 1, 1, 1)

	for _, dt := range [][3]int{
		{1, 1, 1}, {1, 1, 8}, {1, 1, 2}, {1, 2, 6}, {2, 1, 6},
		{4, 2, 29}, {4, 3, 1}, {2028, 2, 28}, {2028, 3, 1},
	} {
		y, m, d := dt[0], dt[1], dt[2]
		idx := clDayIndex(cal, y, m, d)
		want := idx >= baseIdx && (idx-baseIdx)%wl == 0
		if got := base.OccursOn(cal, y, m, d); got != want {
			t.Errorf("fantasy weekly OccursOn(%d-%02d-%02d) = %v; want %v (constant-length stride, byte-identical)",
				y, m, d, got, want)
		}
	}
}

// TestRealTimeRecurrence_MonthlyUnaffected pins the STOP-AND-FLAG check result:
// monthly recurrence matches on day-of-month equality plus monthsBetween (pure
// month arithmetic), never on a day-count-mod-stride, so it never shared the
// weekday-drift bug. Switching absDayIndex to JDN affects monthly only through
// forward-ordering and the recurrence-end bound, both monotonic under JDN — so
// a real-time monthly event still lands on its day-of-month across the 2028 leap
// boundary and nowhere else.
func TestRealTimeRecurrence_MonthlyUnaffected(t *testing.T) {
	cal := rtWeekdayCal()
	ev := recurEvent(RecurrenceMonthly, 2028, 1, 15)

	cases := []struct {
		m, d int
		want bool
		note string
	}{
		{1, 15, true, "base day-of-month"},
		{2, 15, true, "same day, next month — across the leap month"},
		{3, 15, true, "same day, after the leap day"},
		{2, 16, false, "different day-of-month"},
		{2, 29, false, "leap day is not the 15th"},
	}
	for _, tc := range cases {
		if got := ev.OccursOn(cal, 2028, tc.m, tc.d); got != tc.want {
			t.Errorf("monthly@2028-01-15 OccursOn(2028-%02d-%02d) = %v; want %v (%s)",
				tc.m, tc.d, got, tc.want, tc.note)
		}
	}
}

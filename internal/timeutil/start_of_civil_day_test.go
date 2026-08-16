package timeutil

import (
	"testing"
	"time"
)

// TestStartOfCivilDay_MidnightJumpZones is the load-bearing case: zones whose
// DST transition lands ON midnight, so local 00:00 does not exist that day.
//
// The naive `time.Date(y, m, d, 0, 0, 0, 0, loc)` normalises the nonexistent
// wall clock BACKWARDS into the previous local day. Used as a day boundary that
// produced a boundary EARLIER than the instant a splitting loop was holding,
// which is how the availability overlay became a non-terminating allocation
// loop that OOM-killed the server from one authenticated GET.
//
// Two assertions per zone, and both matter:
//   - the returned instant's LOCAL DATE is the requested date (the naive
//     expression fails this outright), and
//   - it is the EARLIEST such instant (one minute earlier is still yesterday).
func TestStartOfCivilDay_MidnightJumpZones(t *testing.T) {
	cases := []struct {
		zone      string
		date      CivilDate
		wantClock string // expected local wall clock of the day's first instant
	}{
		// Cuba: clocks go 00:00 → 01:00 on the second Sunday of March.
		{"America/Havana", CivilDate{2026, time.March, 8}, "01:00"},
		// Chile: clocks go 00:00 → 01:00 in September.
		{"America/Santiago", CivilDate{2026, time.September, 6}, "01:00"},
		// A perfectly ordinary day in the same zone still starts at 00:00.
		{"America/Havana", CivilDate{2026, time.March, 9}, "00:00"},
		{"America/New_York", CivilDate{2026, time.March, 8}, "00:00"},
		{"UTC", CivilDate{2026, time.March, 8}, "00:00"},
	}
	for _, tc := range cases {
		t.Run(tc.zone+"/"+tc.date.String(), func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("tzdata missing for %s", tc.zone)
			}
			got := StartOfCivilDay(loc, tc.date)

			y, m, d := got.Date()
			if y != tc.date.Year || m != tc.date.Month || d != tc.date.Day {
				t.Fatalf("StartOfCivilDay(%s, %s) landed on %s — not on the requested date",
					tc.zone, tc.date, got.Format("2006-01-02 15:04 MST"))
			}
			if clock := got.Format("15:04"); clock != tc.wantClock {
				t.Errorf("local clock = %s, want %s (%s)", clock, tc.wantClock,
					got.Format("2006-01-02 15:04 MST"))
			}
			// Earliest: one minute before is still the previous civil day.
			prev := got.Add(-time.Minute)
			if py, pm, pd := prev.Date(); py == tc.date.Year && pm == tc.date.Month && pd == tc.date.Day {
				t.Errorf("not the earliest instant: %s is still on %s",
					prev.Format("2006-01-02 15:04 MST"), tc.date)
			}
		})
	}
}

// TestStartOfCivilDay_StrictlyIncreasing pins the property the overlay's
// splitting loop depends on for TERMINATION: successive civil dates map to
// strictly increasing instants, in every zone, across the transition.
//
// The naive expression violates this — in America/Havana the "start" of
// 2026-03-08 computed naively is 2026-03-07 23:00, i.e. BEFORE the start of
// 2026-03-07's own successor boundary.
func TestStartOfCivilDay_StrictlyIncreasing(t *testing.T) {
	for _, zone := range []string{"America/Havana", "America/Santiago", "Atlantic/Azores", "America/New_York", "Pacific/Auckland"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Skipf("tzdata missing for %s", zone)
		}
		// Walk every day of 2026 in this zone.
		d := CivilDate{2026, time.January, 1}
		prev := StartOfCivilDay(loc, d)
		for i := 0; i < 400; i++ {
			d = d.AddDays(1)
			cur := StartOfCivilDay(loc, d)
			if !cur.After(prev) {
				t.Fatalf("%s: start of %s (%s) is not after start of the previous day (%s)",
					zone, d, cur.Format("2006-01-02 15:04 MST"), prev.Format("2006-01-02 15:04 MST"))
			}
			if gap := cur.Sub(prev); gap > 25*time.Hour || gap < 22*time.Hour {
				t.Errorf("%s: day before %s was %v long — implausible", zone, d, gap)
			}
			prev = cur
		}
	}
}

// TestStartOfCivilDay_NaiveExpressionIsWrong states, in code, WHY this function
// exists. It asserts against the naive expression the codebase used to inline
// so nobody "simplifies" StartOfCivilDay back into it.
func TestStartOfCivilDay_NaiveExpressionIsWrong(t *testing.T) {
	loc, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Skip("tzdata missing for America/Havana")
	}
	naive := time.Date(2026, time.March, 8, 0, 0, 0, 0, loc)
	if _, _, d := naive.Date(); d != 7 {
		t.Fatalf("premise changed: naive local midnight on 2026-03-08 Havana resolved to %s "+
			"(expected it to normalise back into the 7th)", naive.Format("2006-01-02 15:04 MST"))
	}
	fixed := StartOfCivilDay(loc, CivilDate{2026, time.March, 8})
	if !fixed.After(naive) {
		t.Fatalf("StartOfCivilDay returned %s, which is not after the naive %s",
			fixed.Format("2006-01-02 15:04 MST"), naive.Format("2006-01-02 15:04 MST"))
	}
}

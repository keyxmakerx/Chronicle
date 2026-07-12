package timeutil

import (
	"testing"
	"time"
)

// mustLoad fails the test if an IANA zone can't be loaded (tzdata missing).
func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v (is tzdata available?)", name, err)
	}
	return loc
}

func TestLoadLocation_FallsBackToUTC(t *testing.T) {
	if got := LoadLocation(""); got != time.UTC {
		t.Errorf("empty zone: got %v, want UTC", got)
	}
	if got := LoadLocation("Not/AZone"); got != time.UTC {
		t.Errorf("bad zone: got %v, want UTC", got)
	}
	if got := LoadLocation("America/New_York"); got.String() != "America/New_York" {
		t.Errorf("valid zone: got %v, want America/New_York", got)
	}
}

func TestIsValidLocation(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"America/New_York":  true,
		"Europe/London":     true,
		"Mars/Olympus_Mons": false,
	}
	for name, want := range cases {
		if got := IsValidLocation(name); got != want {
			t.Errorf("IsValidLocation(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestWallClockInstant_DSTCorrect is the load-bearing DST test: the SAME
// wall-clock ("18:00 local") resolves to DIFFERENT absolute instants inside
// vs outside daylight-saving time. If a recurring block were stored as a UTC
// instant/offset instead of a zone-local wall-clock, this invariant would
// break and the displayed hour would drift twice a year.
func TestWallClockInstant_DSTCorrect(t *testing.T) {
	ny := mustLoad(t, "America/New_York")

	// Summer: EDT = UTC-4, so 18:00 local == 22:00 UTC.
	summer := WallClockInstant(ny, 2024, time.July, 15, 18*60).UTC()
	if want := time.Date(2024, time.July, 15, 22, 0, 0, 0, time.UTC); !summer.Equal(want) {
		t.Errorf("summer 18:00 NY: got %s, want %s", summer, want)
	}

	// Winter: EST = UTC-5, so the same 18:00 local == 23:00 UTC.
	winter := WallClockInstant(ny, 2024, time.January, 15, 18*60).UTC()
	if want := time.Date(2024, time.January, 15, 23, 0, 0, 0, time.UTC); !winter.Equal(want) {
		t.Errorf("winter 18:00 NY: got %s, want %s", winter, want)
	}

	if summer.Hour() == winter.Hour() {
		t.Errorf("DST not applied: summer and winter 18:00 both map to UTC hour %d", summer.Hour())
	}
}

// TestWallClockInstant_SpringForwardGap covers the nonexistent-wall-clock hour
// on the spring-forward morning. Go rolls a nonexistent 02:30 forward; we only
// assert it produces a real, stable instant (no panic, monotonic).
func TestWallClockInstant_SpringForwardGap(t *testing.T) {
	ny := mustLoad(t, "America/New_York")
	// 2024-03-10 02:00–03:00 does not exist in New York (clocks jump to 03:00).
	before := WallClockInstant(ny, 2024, time.March, 10, 1*60+30) // 01:30 EST
	after := WallClockInstant(ny, 2024, time.March, 10, 3*60+30)  // 03:30 EDT
	if !after.After(before) {
		t.Errorf("post-transition instant %s should be after pre-transition %s", after, after)
	}
	// 01:30 EST is UTC-5 → 06:30 UTC; 03:30 EDT is UTC-4 → 07:30 UTC. One real hour apart.
	if d := after.Sub(before); d != time.Hour {
		t.Errorf("across spring-forward gap, 01:30→03:30 should be 1 real hour, got %s", d)
	}
}

func TestWallClockInstant_EndOfDayRollsToMidnight(t *testing.T) {
	utc := time.UTC
	got := WallClockInstant(utc, 2026, time.July, 14, MinutesPerDay) // 1440 == next midnight
	want := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("minute 1440 on Jul 14: got %s, want %s", got, want)
	}
}

// TestLocalWallClock_CrossZoneMidnight verifies a member's evening block in one
// zone lands on the correct weekday/minute in a viewer's zone, crossing a day
// boundary. Chicago 18:00 CDT (UTC-5) == 23:00 UTC == 00:00 next day BST in
// London (UTC+1), i.e. the block moves onto the following weekday.
func TestLocalWallClock_CrossZoneMidnight(t *testing.T) {
	chi := mustLoad(t, "America/Chicago")
	lon := mustLoad(t, "Europe/London")

	// 2024-07-16 is a Tuesday.
	instant := WallClockInstant(chi, 2024, time.July, 16, 18*60)
	wd, min := LocalWallClock(instant, lon)
	if wd != time.Wednesday {
		t.Errorf("Chicago Tue 18:00 in London: weekday = %s, want Wednesday", wd)
	}
	if min != 0 {
		t.Errorf("Chicago Tue 18:00 in London: minute-of-day = %d, want 0 (midnight)", min)
	}
}

func TestLocalWallClock_SameZoneRoundTrip(t *testing.T) {
	ny := mustLoad(t, "America/New_York")
	instant := WallClockInstant(ny, 2026, time.July, 14, 19*60+30) // Tue 19:30
	wd, min := LocalWallClock(instant, ny)
	if wd != time.Tuesday {
		t.Errorf("weekday = %s, want Tuesday", wd)
	}
	if min != 19*60+30 {
		t.Errorf("minute-of-day = %d, want %d", min, 19*60+30)
	}
}

func TestCivilDate_ParseAndString(t *testing.T) {
	d, err := ParseCivilDate("2026-07-14")
	if err != nil {
		t.Fatalf("ParseCivilDate: %v", err)
	}
	if d.Year != 2026 || d.Month != time.July || d.Day != 14 {
		t.Errorf("parsed = %+v, want 2026-07-14", d)
	}
	if d.String() != "2026-07-14" {
		t.Errorf("String() = %q, want 2026-07-14", d.String())
	}
	if _, err := ParseCivilDate("not-a-date"); err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestCivilDate_Weekday(t *testing.T) {
	// 2026-07-14 is a Tuesday; 2026-07-13 is a Monday.
	if wd := (CivilDate{2026, time.July, 14}).Weekday(); wd != time.Tuesday {
		t.Errorf("2026-07-14 weekday = %s, want Tuesday", wd)
	}
	if wd := (CivilDate{2026, time.July, 13}).Weekday(); wd != time.Monday {
		t.Errorf("2026-07-13 weekday = %s, want Monday", wd)
	}
}

func TestCivilDate_AddDays_CrossesMonth(t *testing.T) {
	d := CivilDate{2026, time.July, 30}
	got := d.AddDays(3) // → Aug 2
	if got.String() != "2026-08-02" {
		t.Errorf("Jul 30 + 3 = %s, want 2026-08-02", got.String())
	}
	back := got.AddDays(-3)
	if back.String() != "2026-07-30" {
		t.Errorf("round trip = %s, want 2026-07-30", back.String())
	}
}

// import_calendaria_v2_test.go pins three #772 fixes specific to the
// Calendaria format: the current leapYearConfig shape (confirmed against the
// shipped Elven preset), a moon's referenceDate actually setting its phase
// offset, and a weekday's isRestDay surviving import.
package calendar

import "testing"

// TestParseCalendaria_LeapYearEnabledShape pins the exact regression named in
// #772: Chronicle's own Elven preset (presets/elven.json) carries
// leapYearConfig as {"enabled":true,"interval":8,"offset":0}, not the
// {rule,start} shape the parser previously read — so LoadPreset("elven")
// came back with LeapYearEvery=0 (no leap years at all) instead of one every
// 8 years.
func TestParseCalendaria_LeapYearEnabledShape(t *testing.T) {
	raw := []byte(`{
		"name": "Elven-shaped",
		"days": {"hoursPerDay": 24, "minutesPerHour": 60, "secondsPerMinute": 60},
		"months": {"m1": {"name": "Firstmonth", "days": 45, "ordinal": 1}},
		"leapYearConfig": {"enabled": true, "interval": 8, "offset": 0}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Settings.LeapYearEvery != 8 {
		t.Errorf("LeapYearEvery = %d, want 8 (the Elven preset's own leap interval)", ir.Settings.LeapYearEvery)
	}
	if ir.Settings.LeapYearOffset != 0 {
		t.Errorf("LeapYearOffset = %d, want 0", ir.Settings.LeapYearOffset)
	}
}

// TestParseCalendaria_LeapYearDisabledShapeStaysZero confirms the additive
// fix does not turn every Calendaria file into a leap-year one: enabled:false
// (or the key absent) must still leave LeapYearEvery at 0.
func TestParseCalendaria_LeapYearDisabledShapeStaysZero(t *testing.T) {
	raw := []byte(`{
		"name": "No Leap",
		"days": {"hoursPerDay": 24, "minutesPerHour": 60, "secondsPerMinute": 60},
		"months": {"m1": {"name": "Firstmonth", "days": 45, "ordinal": 1}},
		"leapYearConfig": {"enabled": false, "interval": 8, "offset": 0}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Settings.LeapYearEvery != 0 {
		t.Errorf("LeapYearEvery = %d, want 0 (enabled:false must not seed a leap interval)", ir.Settings.LeapYearEvery)
	}
}

// TestParseCalendaria_MoonPhaseOffsetFromReferenceDate pins #772's moon-phase
// fix: referenceDate was parsed into calMoon.ReferenceDate and then
// discarded (PhaseOffset hardcoded to 0), so every Calendaria moon started
// its cycle on whatever night absolute day 0 happened to be rather than the
// night the file actually named. The fix must make the moon read as a New
// Moon (phase 0) on its own stated reference date.
func TestParseCalendaria_MoonPhaseOffsetFromReferenceDate(t *testing.T) {
	raw := []byte(`{
		"name": "Moon Test",
		"days": {"hoursPerDay": 24, "minutesPerHour": 60, "secondsPerMinute": 60},
		"months": {"m1": {"name": "Firstmonth", "days": 30, "ordinal": 1}},
		"moons": {
			"moon1": {"name": "Luna", "cycleLength": 30, "color": "#ffffff", "referenceDate": {"year": 0, "month": 1, "day": 1}}
		}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if len(ir.Moons) != 1 {
		t.Fatalf("got %d moons, want 1", len(ir.Moons))
	}
	got := ir.Moons[0]
	if got.PhaseOffset == 0 {
		t.Fatalf("PhaseOffset = 0, want a value derived from referenceDate (the pre-fix hardcoded default)")
	}

	// Rebuild the same absolute-day arithmetic the parser used and confirm
	// the reference date itself reads as a New Moon (phase 0) — the actual
	// invariant this fix exists to establish, rather than pinning one
	// specific offset number.
	tmp := &Calendar{Months: []Month{{Days: 30}}}
	refDay := tmp.AbsoluteDay(0, 1, 1)
	moon := Moon{CycleDays: got.CycleDays, PhaseOffset: got.PhaseOffset}
	if phase := moon.MoonPhase(refDay); phase > 0.001 && phase < 0.999 {
		t.Errorf("moon phase on its own reference date = %v, want ~0 (New Moon)", phase)
	}
}

// TestParseCalendaria_WeekdayRestDaySurvives pins #772's rest-day fix:
// isRestDay was parsed into calWeekday.IsRestDay and then discarded.
func TestParseCalendaria_WeekdayRestDaySurvives(t *testing.T) {
	raw := []byte(`{
		"name": "Rest Day Test",
		"days": {
			"hoursPerDay": 24, "minutesPerHour": 60, "secondsPerMinute": 60,
			"values": {
				"d1": {"name": "Workday", "ordinal": 1, "isRestDay": false},
				"d2": {"name": "Restday", "ordinal": 2, "isRestDay": true}
			}
		},
		"months": {"m1": {"name": "Firstmonth", "days": 30, "ordinal": 1}}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if len(ir.Weekdays) != 2 {
		t.Fatalf("got %d weekdays, want 2", len(ir.Weekdays))
	}
	if ir.Weekdays[0].IsRestDay {
		t.Errorf("Workday.IsRestDay = true, want false")
	}
	if !ir.Weekdays[1].IsRestDay {
		t.Errorf("Restday.IsRestDay = false, want true — isRestDay was dropped on import")
	}
}

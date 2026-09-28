// import_simplecalendar_v2_test.go pins parsing specific to the Simple
// Calendar format: a moon's firstNewMoon contributing to its phase offset
// alongside cycleDayAdjust, and a weekday's restday surviving import.
package calendar

import "testing"

// TestParseSimpleCalendar_MoonPhaseOffsetFromFirstNewMoon pins that
// firstNewMoon (scFirstNewMoon) and cycleDayAdjust combine into PhaseOffset:
// the moon reads as a New Moon on (firstNewMoon + cycleDayAdjust days).
func TestParseSimpleCalendar_MoonPhaseOffsetFromFirstNewMoon(t *testing.T) {
	raw := []byte(`{
		"calendar": {
			"name": "Moon Test",
			"year": {"numericRepresentation": 500},
			"months": [{"name": "Firstmonth", "numberOfDays": 30}],
			"weekdays": [{"name": "Oneday"}],
			"moons": [{
				"name": "Luna", "cycleLength": 30, "cycleDayAdjust": 5,
				"firstNewMoon": {"year": 0, "month": 0, "day": 0}
			}]
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

	// firstNewMoon {0,0,0} is 0-indexed -> calendar date (year 0, month 1,
	// day 1); the true new moon then lands cycleDayAdjust (5) days later.
	tmp := &Calendar{Months: []Month{{Days: 30}}}
	trueNewMoonDay := tmp.AbsoluteDay(0, 1, 1) + 5
	moon := Moon{CycleDays: got.CycleDays, PhaseOffset: got.PhaseOffset}
	if phase := moon.MoonPhase(trueNewMoonDay); phase > 0.001 && phase < 0.999 {
		t.Errorf("moon phase on (firstNewMoon + cycleDayAdjust) = %v, want ~0 (New Moon)", phase)
	}
}

// TestParseSimpleCalendar_WeekdayRestDaySurvives pins that a weekday's
// restday survives from scWeekday.Restday into the imported result.
func TestParseSimpleCalendar_WeekdayRestDaySurvives(t *testing.T) {
	raw := []byte(`{
		"calendar": {
			"name": "Rest Day Test",
			"year": {"numericRepresentation": 1},
			"months": [{"name": "Firstmonth", "numberOfDays": 30}],
			"weekdays": [
				{"name": "Workday", "restday": false},
				{"name": "Restday", "restday": true}
			]
		}
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
		t.Errorf("Restday.IsRestDay = false, want true — restday was dropped on import")
	}
}

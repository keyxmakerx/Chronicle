// import_fantasycalendar_v2_test.go pins parsing specific to the
// Fantasy-Calendar.com format: a moon marked hidden imports as Director-only
// rather than being dropped, and an era's start month/day survive import
// instead of collapsing to "month 1, day 1" of its start year.
package calendar

import "testing"

// TestParseFantasyCalendar_HiddenMoonImportsAsDirectorOnly pins that a moon
// with "hidden": true imports as Director-only rather than being skipped,
// so a GM's own secret moon comes over instead of vanishing.
func TestParseFantasyCalendar_HiddenMoonImportsAsDirectorOnly(t *testing.T) {
	raw := []byte(`{
		"name": "Moon Test",
		"static_data": {
			"year_data": {
				"global_week": ["Sunday"],
				"timespans": [{"name": "First Month", "type": "month", "length": 30}]
			},
			"moons": [
				{"name": "Visible Moon", "cycle": 30, "hidden": false},
				{"name": "Secret Moon", "cycle": 27, "hidden": true}
			],
			"clock": {"hours": 24, "minutes": 60}
		},
		"dynamic_data": {"year": 1, "timespan": 0, "day": 0}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if len(ir.Moons) != 2 {
		t.Fatalf("got %d moons, want 2 — a hidden moon must import, not vanish", len(ir.Moons))
	}
	var visible, secret *MoonInput
	for i := range ir.Moons {
		m := &ir.Moons[i]
		switch m.Name {
		case "Visible Moon":
			visible = m
		case "Secret Moon":
			secret = m
		}
	}
	if visible == nil {
		t.Fatal("Visible Moon missing from import")
	}
	if visible.HiddenFromPlayers {
		t.Error("Visible Moon imported as hidden, want visible")
	}
	if secret == nil {
		t.Fatal("Secret Moon missing from import — hidden moons must not be dropped")
	}
	if !secret.HiddenFromPlayers {
		t.Error("Secret Moon imported as visible, want Director-only (HiddenFromPlayers)")
	}
}

// TestParseFantasyCalendar_EraStartMonthDaySurvives pins that an era's
// date.timespan/date.day (0-indexed, like every other Fantasy-Calendar
// month-index/day field) survive import, rather than collapsing every era
// to "month 1, day 1" of its year regardless of when it actually began.
func TestParseFantasyCalendar_EraStartMonthDaySurvives(t *testing.T) {
	raw := []byte(`{
		"name": "Era Test",
		"static_data": {
			"year_data": {
				"global_week": ["Sunday"],
				"timespans": [
					{"name": "First Month", "type": "month", "length": 30},
					{"name": "Second Month", "type": "month", "length": 30},
					{"name": "Third Month", "type": "month", "length": 30}
				]
			},
			"eras": [
				{"name": "Second Age", "description": "It began mid-year", "date": {"year": 100, "timespan": 1, "day": 14}}
			],
			"clock": {"hours": 24, "minutes": 60}
		},
		"dynamic_data": {"year": 1, "timespan": 0, "day": 0}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if len(ir.Eras) != 1 {
		t.Fatalf("got %d eras, want 1", len(ir.Eras))
	}
	got := ir.Eras[0]
	if got.StartYear != 100 {
		t.Errorf("StartYear = %d, want 100", got.StartYear)
	}
	// timespan 1 (0-indexed) -> month 2; day 14 (0-indexed) -> day 15.
	if got.StartMonth != 2 {
		t.Errorf("StartMonth = %d, want 2 (0-indexed timespan 1)", got.StartMonth)
	}
	if got.StartDay != 15 {
		t.Errorf("StartDay = %d, want 15 (0-indexed day 14)", got.StartDay)
	}
}

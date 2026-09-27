package calendar

import "testing"

// TestParseFantasyCalendar_PopulatesToday pins the fix for a real bug: the
// parser used to leave result.Today at its zero value entirely, even though
// dynamic_data (the export's own "current state" section) always carries a
// year, a current timespan (month) and a day — so a Fantasy-Calendar import
// used to force the wizard's owner to confirm a "missing" current date the
// source file actually specified. Year is the load-bearing assertion (the
// #741 rule this review's item explicitly asks for); month/day are checked
// too since the fix populates all three the same way.
func TestParseFantasyCalendar_PopulatesToday(t *testing.T) {
	raw := []byte(`{
		"name": "Test Fantasy Calendar",
		"static_data": {
			"year_data": {
				"global_week": ["Sunday", "Monday"],
				"timespans": [
					{"name": "First Month", "type": "month", "length": 30},
					{"name": "Second Month", "type": "month", "length": 30}
				]
			},
			"clock": {"hours": 24, "minutes": 60}
		},
		"dynamic_data": {"year": 1024, "timespan": 1, "day": 5}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Format != FormatFantasyCal {
		t.Fatalf("detected format = %q, want %q", ir.Format, FormatFantasyCal)
	}
	if ir.Today.Year != 1024 {
		t.Errorf("Today.Year = %d, want 1024 (was previously left at the zero value entirely)", ir.Today.Year)
	}
	if ir.Today.Month == nil || *ir.Today.Month != 2 {
		t.Errorf("Today.Month = %v, want a pointer to 2 (0-indexed timespan 1 -> month 2)", ir.Today.Month)
	}
	if ir.Today.Day == nil || *ir.Today.Day != 6 {
		t.Errorf("Today.Day = %v, want a pointer to 6 (0-indexed day 5 -> day 6)", ir.Today.Day)
	}
}

// TestParseSimpleCalendar_NoCurrentDateLeavesMonthDayNil pins the other half
// of the #741 audit: a Simple Calendar file that omits "currentDate"
// entirely (a template/definitions-only export) must leave Today.Month/Day
// nil, never a pointer to a disguised "day 1" — scCalendar.CurrentDate is a
// pointer specifically so this is detectable.
func TestParseSimpleCalendar_NoCurrentDateLeavesMonthDayNil(t *testing.T) {
	raw := []byte(`{
		"calendar": {
			"name": "No Current Date",
			"year": {"numericRepresentation": 500},
			"months": [{"name": "Firstmonth", "numberOfDays": 30}],
			"weekdays": [{"name": "Oneday"}]
		}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Today.Year != 500 {
		t.Errorf("Today.Year = %d, want 500", ir.Today.Year)
	}
	if ir.Today.Month != nil {
		t.Errorf("Today.Month = %v, want nil (the source never specified a currentDate at all)", *ir.Today.Month)
	}
	if ir.Today.Day != nil {
		t.Errorf("Today.Day = %v, want nil (the source never specified a currentDate at all)", *ir.Today.Day)
	}
}

// TestParseSimpleCalendar_ZeroCurrentDateIsRealNotMissing is the control for
// the test above: Simple Calendar's currentDate is 0-indexed, so an actually
// PRESENT {"month":0,"day":0} genuinely means the first month/day, not
// "unspecified" — the pointer-based fix must not collapse this into nil too.
func TestParseSimpleCalendar_ZeroCurrentDateIsRealNotMissing(t *testing.T) {
	raw := []byte(`{
		"calendar": {
			"name": "Day Zero",
			"year": {"numericRepresentation": 500},
			"months": [{"name": "Firstmonth", "numberOfDays": 30}],
			"weekdays": [{"name": "Oneday"}],
			"currentDate": {"year": 500, "month": 0, "day": 0}
		}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Today.Month == nil || *ir.Today.Month != 1 {
		t.Errorf("Today.Month = %v, want a pointer to 1 (0-indexed month 0 -> month 1)", ir.Today.Month)
	}
	if ir.Today.Day == nil || *ir.Today.Day != 1 {
		t.Errorf("Today.Day = %v, want a pointer to 1 (0-indexed day 0 -> day 1)", ir.Today.Day)
	}
}

// import_clamp_test.go covers #741's "warn, never refuse, when a structural
// oddity is found": a Simple Calendar or Calendaria file carrying a bad
// value (an out-of-range season reference, a non-positive month length, a
// blank name) must still import successfully, with the bad value clamped
// to something the schema can store and reported on ImportResult.Warnings
// — never a hard failure that aborts the whole import.
package calendar

import (
	"strings"
	"testing"
)

// TestClampCalendarStructure_ClampsAndWarns is a direct, table-driven test
// of the shared normalization pass parseCalendaria and parseSimpleCalendar
// both run their output through. It is deliberately independent of either
// parser's own internal clamping (calendariaSeasonRange/dayOfYearToMonthDay
// already bound Calendaria's OWN computed season ranges before this pass
// ever runs — see clampCalendarStructure's doc comment) — this proves the
// shared pass itself, regardless of which parser feeds it.
func TestClampCalendarStructure_ClampsAndWarns(t *testing.T) {
	result := &ImportResult{
		Months: []MonthInput{
			{Name: "", Days: 30, SortOrder: 0},
			{Name: "Second", Days: -5, SortOrder: 1},
		},
		Weekdays: []WeekdayInput{
			{Name: "", SortOrder: 0},
		},
		Eras: []EraInput{
			{Name: "", StartYear: 1, Color: "#000000"},
		},
		Seasons: []Season{
			// References month 5 in a 2-month calendar, and a day beyond
			// month 1's 30 days.
			{Name: "Endless Summer", StartMonth: 5, StartDay: 99, EndMonth: 5, EndDay: 99, Color: "#808080"},
		},
	}

	if err := clampCalendarStructure(result); err != nil {
		t.Fatalf("clampCalendarStructure: %v", err)
	}

	if result.Months[0].Name != "Month 1" {
		t.Errorf("month 0 name = %q, want a fallback name", result.Months[0].Name)
	}
	if result.Months[1].Days != 1 {
		t.Errorf("month 1 days = %d, want clamped to 1", result.Months[1].Days)
	}
	if result.Weekdays[0].Name != "Day 1" {
		t.Errorf("weekday 0 name = %q, want a fallback name", result.Weekdays[0].Name)
	}
	if result.Eras[0].Name != "Era 1" {
		t.Errorf("era 0 name = %q, want a fallback name", result.Eras[0].Name)
	}
	s := result.Seasons[0]
	if s.StartMonth < 1 || s.StartMonth > 2 || s.EndMonth < 1 || s.EndMonth > 2 {
		t.Errorf("season month = [%d,%d], want clamped into [1,2]", s.StartMonth, s.EndMonth)
	}
	if s.StartDay < 1 || s.StartDay > result.Months[s.StartMonth-1].Days {
		t.Errorf("season start day = %d, want clamped into month %d's range (1..%d)", s.StartDay, s.StartMonth, result.Months[s.StartMonth-1].Days)
	}

	if len(result.Warnings) == 0 {
		t.Fatal("expected at least one warning, got none")
	}
	wantSubstrings := []string{"month 1 had no name", "non-positive length", "weekday 1 had no name", "era 1 had no name", "outside this calendar's 2 months"}
	for _, want := range wantSubstrings {
		found := false
		for _, w := range result.Warnings {
			if strings.Contains(w, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a warning containing %q, got %v", want, result.Warnings)
		}
	}
}

// TestParseSimpleCalendar_OutOfRangeSeasonIsClampedNotFailed is the
// end-to-end case: parseSimpleCalendarInner computes a season's start/end
// month+day directly from the file's own (unchecked) startingMonth/
// startingDay with no bound against the calendar's actual month count —
// before this fix a season naming a month past the end of the calendar
// silently produced a StartMonth/EndMonth no month grid could render,
// never an error and never a warning. It must now still succeed, but with
// the value clamped and reported.
func TestParseSimpleCalendar_OutOfRangeSeasonIsClampedNotFailed(t *testing.T) {
	raw := []byte(`{
		"calendar": {
			"name": "Broken Calendar",
			"year": {"numericRepresentation": 1},
			"months": [
				{"name": "First", "numericRepresentation": 1, "numberOfDays": 30},
				{"name": "Second", "numericRepresentation": 2, "numberOfDays": 30}
			],
			"seasons": [
				{"name": "Endless Summer", "startingMonth": 50, "startingDay": 0}
			]
		}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse must succeed (warn, never refuse), got: %v", err)
	}
	if ir.Format != FormatSimpleCal {
		t.Fatalf("format = %q, want %q", ir.Format, FormatSimpleCal)
	}
	if len(ir.Seasons) != 1 {
		t.Fatalf("got %d seasons, want 1", len(ir.Seasons))
	}
	s := ir.Seasons[0]
	if s.StartMonth < 1 || s.StartMonth > len(ir.Months) {
		t.Errorf("season StartMonth = %d, want clamped into [1,%d]", s.StartMonth, len(ir.Months))
	}
	if s.EndMonth < 1 || s.EndMonth > len(ir.Months) {
		t.Errorf("season EndMonth = %d, want clamped into [1,%d]", s.EndMonth, len(ir.Months))
	}
	if len(ir.Warnings) == 0 {
		t.Error("expected a warning about the clamped season, got none")
	}
}

// TestParseCalendaria_NonPositiveMonthLengthIsClampedNotFailed covers the
// Calendaria side of the same contract: a month with a non-positive length
// (bad source data — a real export should never produce this, but nothing
// stops a hand-edited or third-party-tool file from doing so) must not
// propagate into the calendar's stored geometry unclamped.
func TestParseCalendaria_NonPositiveMonthLengthIsClampedNotFailed(t *testing.T) {
	raw := calendariaSeasonFixture([]int{0, 30}, []fixtureSeason{
		{Name: "Only Season", DayStart: 1, DayEnd: 10},
	})

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse must succeed (warn, never refuse), got: %v", err)
	}
	if ir.Format != FormatCalendaria {
		t.Fatalf("format = %q, want %q", ir.Format, FormatCalendaria)
	}
	if len(ir.Months) != 2 {
		t.Fatalf("got %d months, want 2", len(ir.Months))
	}
	if ir.Months[0].Days != 1 {
		t.Errorf("month 0 days = %d, want clamped to 1 (was 0 in the source file)", ir.Months[0].Days)
	}
	found := false
	for _, w := range ir.Warnings {
		if strings.Contains(w, "non-positive length") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning about the non-positive month length, got %v", ir.Warnings)
	}
}

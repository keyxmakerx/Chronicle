package calendar

import "testing"

// TestParseCalendaria_WeeksWithoutDaysValues (W1 / R4 crash-guard): some
// Calendaria files define weekdays under the top-level "weeks" object instead of
// "days.values". In that case cal.Days.Values is a nil map; the old fallback
// wrote into it ("assignment to entry in nil map") and panicked the whole import.
// The fix allocates the map first. This pins that a weeks-only file imports
// cleanly and the weekdays come through in ordinal order.
func TestParseCalendaria_WeeksWithoutDaysValues(t *testing.T) {
	// No "days":{"values":…} key → cal.Days.Values is nil. "weeks" carries the
	// weekday list (the documented Calendaria variant).
	raw := []byte(`{
		"name": "Weeks-only Calendar",
		"months": { "0": { "name": "Firstmonth", "days": 30, "ordinal": 0 } },
		"weeks": {
			"0": { "name": "Restday", "ordinal": 1, "isRestDay": true },
			"1": { "name": "Workday", "ordinal": 0 }
		}
	}`)

	res, err := parseCalendaria(raw) // must not panic
	if err != nil {
		t.Fatalf("parseCalendaria(weeks-only) errored: %v", err)
	}
	if len(res.Weekdays) != 2 {
		t.Fatalf("expected 2 weekdays from the weeks map; got %d", len(res.Weekdays))
	}
	// Sorted by ordinal: Workday (0) then Restday (1).
	if res.Weekdays[0].Name != "Workday" || res.Weekdays[1].Name != "Restday" {
		t.Errorf("weekdays not ordinal-sorted: got %q, %q", res.Weekdays[0].Name, res.Weekdays[1].Name)
	}
}

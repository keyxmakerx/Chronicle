package calendarai

import (
	"reflect"
	"testing"
)

// TestListKeyWarnings pins that a key a list entry is not read for is named
// in a warning, with the entry, and that every key the readers use is quiet.
func TestListKeyWarnings(t *testing.T) {
	r := Record{Fields: map[string]any{
		"months": []any{
			map[string]any{"name": "Frostfall", "days": 30, "Leap_Days": 1, "intercalary": false, "season": "Winter"},
			map[string]any{"name": "Thaw", "dayz": 28},
		},
		"weekdays": []any{"Moonday", map[string]any{"name": "Restday", "rest_day": true, "holiday": true}},
		"seasons":  []any{map[string]any{"name": "Winter", "start_month": 1, "end_month": 1, "start_day": 1, "end_day": 30, "color": "#fff"}},
		"moons": []any{map[string]any{"name": "Selune", "cycle": 30, "phase_offset": 2, "color": "#eee", "strength": 0.5,
			"hidden": false, "page": "none", "moves": "cycle", "phase": "full"}},
		"eras": []any{map[string]any{"name": "Dawn", "start_year": 1, "start_month": 1, "start_day": 1, "end_year": 9,
			"end_month": 2, "end_day": 3, "color": "#000", "description": "x", "page": "none"}},
	}}
	want := []string{
		"`dayz` is not a key of a month (\"Thaw\"), so it was ignored",
		"`phase` is not a key of a moon (\"Selune\"), so it was ignored",
		"`holiday` is not a key of a weekday (\"Restday\"), so it was ignored",
	}
	got := listKeyWarnings(r)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("listKeyWarnings =\n%q\nwant\n%q", got, want)
	}
}

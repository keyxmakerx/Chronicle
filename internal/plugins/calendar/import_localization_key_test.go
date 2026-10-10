package calendar

import "testing"

// TestStripLocalizationKey pins that only key-shaped strings are stripped;
// ordinary names containing dots must survive intact and never become empty.
func TestStripLocalizationKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"calendaria key", "CALENDARIA.Month.Hammer", "Hammer"},
		{"deep calendaria key", "CALENDARIA.Calendar.Gregorian.Month.January", "January"},
		{"simple calendar key", "SIMPLE_CALENDAR.x", "x"},
		{"FSC key", "FSC.Calendar.Gregorian", "Gregorian"},
		{"digits in namespace", "ABC2.Name", "Name"},
		{"plain name", "Hammer", "Hammer"},
		{"trimmed", "  Hammer  ", "Hammer"},
		{"abbreviation with trailing dot", "D.R.", "D.R."},
		{"sentence-like", "A. B", "A. B"},
		{"st ives", "St. Ives", "St. Ives"},
		{"lowercase namespace", "foo.bar", "foo.bar"},
		{"single-char namespace", "A.B", "A.B"},
		{"empty segment", "CAL..Name", "CAL..Name"},
		{"trailing dot key", "CAL.Name.", "CAL.Name."},
		{"whitespace inside key", "CAL.Some Name", "CAL.Some Name"},
		{"empty", "", ""},
		{"blank", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripLocalizationKey(tt.in); got != tt.want {
				t.Errorf("stripLocalizationKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

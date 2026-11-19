package timeutil

import (
	"testing"
	"time"
)

func TestFixedDate(t *testing.T) {
	at := time.Date(2026, time.November, 18, 15, 4, 0, 0, time.UTC)
	tests := []struct {
		form DateForm
		want string
	}{
		{DateShort, "18 Nov 2026"},
		{DateDay, "18 Nov"},
		{DateLong, "18 November 2026"},
		{DateWeekday, "Wednesday 18 November"},
		{DateWithTime, "18 Nov 2026, 15:04"},
		{"unknown", "18 Nov 2026"},
	}
	for _, tc := range tests {
		if got := FixedDate(at, tc.form); got != tc.want {
			t.Errorf("FixedDate(%q) = %q, want %q", tc.form, got, tc.want)
		}
	}
}

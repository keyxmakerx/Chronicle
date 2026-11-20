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

func TestFixedDate_Recent(t *testing.T) {
	now := time.Date(2026, time.November, 20, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, time.November, 20, 1, 0, 0, 0, time.UTC), "Friday"},
		{time.Date(2026, time.November, 17, 23, 0, 0, 0, time.UTC), "Tuesday"},
		{time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC), "Saturday"},
		{time.Date(2026, time.November, 13, 12, 0, 0, 0, time.UTC), "13 Nov"},
		{time.Date(2025, time.December, 30, 12, 0, 0, 0, time.UTC), "30 Dec 2025"},
		{time.Date(2026, time.November, 22, 12, 0, 0, 0, time.UTC), "22 Nov"},
	}
	for _, tc := range tests {
		if got := fixedDate(tc.at, DateRecent, now); got != tc.want {
			t.Errorf("recent %s = %q, want %q", tc.at, got, tc.want)
		}
	}
}

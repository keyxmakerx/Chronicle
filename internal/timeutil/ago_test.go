package timeutil

import (
	"testing"
	"time"
)

func TestAgo(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"future clock skew", -5 * time.Minute, "just now"},
		{"seconds", 30 * time.Second, "just now"},
		{"one minute", time.Minute, "1 minute ago"},
		{"minutes", 59 * time.Minute, "59 minutes ago"},
		{"one hour", time.Hour, "1 hour ago"},
		{"hours", 23 * time.Hour, "23 hours ago"},
		{"one day", 24 * time.Hour, "1 day ago"},
		{"days", 2*24*time.Hour + time.Hour, "2 days ago"},
		{"one month", 30 * 24 * time.Hour, "1 month ago"},
		{"months", 200 * 24 * time.Hour, "6 months ago"},
		{"one year", 365 * 24 * time.Hour, "1 year ago"},
		{"years", 800 * 24 * time.Hour, "2 years ago"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Ago(now.Add(-tc.age), now); got != tc.want {
				t.Errorf("Ago(-%v) = %q, want %q", tc.age, got, tc.want)
			}
		})
	}
	if got := EditedAgo(now.Add(-48*time.Hour), now); got != "Edited 2 days ago" {
		t.Errorf("EditedAgo = %q", got)
	}
}

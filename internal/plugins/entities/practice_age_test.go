package entities

import (
	"testing"
	"time"
)

func TestPracticeAge(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Minute, "just now"},
		{5 * time.Hour, "today"},
		{30 * time.Hour, "yesterday"},
		{5 * 24 * time.Hour, "5 days ago"},
	}
	for _, tc := range tests {
		if got := practiceAge(now, now.Add(-tc.ago)); got != tc.want {
			t.Errorf("practiceAge(%v ago) = %q, want %q", tc.ago, got, tc.want)
		}
	}
}

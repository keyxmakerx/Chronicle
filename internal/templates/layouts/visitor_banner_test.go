package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// A pass names its last day the way a person would say it.
func TestVisitEndDay(t *testing.T) {
	now := time.Date(2026, 11, 12, 21, 0, 0, 0, time.UTC) // a Thursday evening
	tests := []struct {
		name  string
		until time.Time
		want  string
	}{
		{"same evening", now.Add(time.Hour), "today"},
		{"past midnight", now.Add(5 * time.Hour), "tomorrow"},
		{"three days", now.Add(72 * time.Hour), "Sunday"},
		{"six days", now.Add(6 * 24 * time.Hour), "Wednesday"},
		{"a week out", now.Add(7 * 24 * time.Hour), "Nov 19"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visitEndDay(tt.until, now); got != tt.want {
				t.Errorf("visitEndDay = %q, want %q", got, tt.want)
			}
		})
	}
}

// A visiting guest sees the visitor strip, not the keep-your-account one.
func TestVisitorBanner(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	if err := VisitorBanner(now.Add(49*time.Hour), now).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "You're looking around until Saturday. You can read, but not write or roll.") || !strings.Contains(html, `data-cta="visitor"`) || strings.Contains(html, "Keep my account") {
		t.Fatalf("banner:\n%s", html)
	}
}

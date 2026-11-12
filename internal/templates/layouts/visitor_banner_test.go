package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// The countdown rounds up, so it never says a day is left when the pass is
// about to close, and switches to hours on the last day.
func TestVisitLeft(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		left time.Duration
		want string
	}{
		{30 * time.Minute, "Less than an hour left"},
		{90 * time.Minute, "2 hours left"},
		{24 * time.Hour, "1 day left"},
		{25 * time.Hour, "2 days left"},
		{72 * time.Hour, "3 days left"},
	}
	for _, tt := range tests {
		if got := visitLeft(now.Add(tt.left), now); got != tt.want {
			t.Errorf("visitLeft(%v) = %q, want %q", tt.left, got, tt.want)
		}
	}
}

// A visiting guest sees the visitor strip, not the keep-your-account one.
func TestVisitorBanner(t *testing.T) {
	now := time.Now()
	var buf bytes.Buffer
	if err := VisitorBanner(now.Add(49*time.Hour), now).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "You're looking around. 3 days left.") || !strings.Contains(html, `data-cta="visitor"`) {
		t.Fatalf("banner:\n%s", html)
	}
}

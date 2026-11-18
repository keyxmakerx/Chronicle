package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

func TestDateRendersTimeWithFallback(t *testing.T) {
	at := time.Date(2026, time.November, 18, 15, 4, 0, 0, time.UTC)
	var b bytes.Buffer
	if err := Date(at, timeutil.DateLong).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, frag := range []string{`datetime="2026-11-18T15:04:00Z"`, `data-date="long"`, `>18 November 2026</time>`} {
		if !strings.Contains(got, frag) {
			t.Errorf("output %q missing %q", got, frag)
		}
	}
}

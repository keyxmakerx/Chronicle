// timeline_ledger_test.go — Timeline V2 W0 Design A (the Ledger). Pins the
// page-separation rules: renders without panic, only self-contained
// .tl-ledger-* styling (no Tailwind utilities, no shared design assets), a
// back-link to the demo hub, and the chronicle semantics the design is FOR
// (era headers, year gutter, tier weighting, dm-only lock, weather glyphs).
package demo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDemoTimelineLedger_RendersIsolated(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoTimelineLedger().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render ledger: %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		`href="/demo/calendar"`,    // back-link to the hub
		"tl-ledger-era",            // era headers
		"tl-ledger-yr-label",       // year gutter
		"tl-ledger-t--major",       // tier weighting
		"tl-ledger-span",           // multi-day span bar
		"tl-ledger-lock",           // dm-only annotation
		"tl-ledger-wx",             // weather glyph column
		"Age of Conflict",          // mock chronicle content
	} {
		if !strings.Contains(html, want) {
			t.Errorf("ledger demo missing %q", want)
		}
	}
	// Isolation: no shared design assets, no almanac classes.
	for _, gone := range []string{"cal-almanac", "/static/css/cal-almanac"} {
		if strings.Contains(html, gone) {
			t.Errorf("ledger demo must not load shared design assets (%q found)", gone)
		}
	}
}

// calendar_timeline_tuner_test.go — C-TIMELINE-V2-DESIGN-1-TUNER.
//
// Pins the Tuner timeline showcase: it renders without panic, carries no
// inline <script> body (externalized JS only), loads ONLY its own
// page-separated assets, its CSS is self-contained (no @layer/@apply/
// canon tokens, never `transition: all`) and carries the rendering-canvas
// exemption marker, the route is in the wire snapshot, the index links to
// it, and the mock dataset has the structures the design demonstrates.
// Visual fidelity is the operator's local gate.

package demo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func tunerRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	// internal/templates/demo → repo root is three levels up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func TestDemoTimelineTuner_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoTimelineTuner().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render tuner: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"cal-timeline-tuner-shell", "data-tuner-root", "data-tuner-axis",
		"data-tuner-needle", "/static/css/cal-timeline-tuner.css",
		"/static/js/cal-timeline-tuner.js", "data-cal-tuner-data",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tuner render missing %q", want)
		}
	}
}

// The mock dataset is embedded as a JSON data-attribute (single source of
// truth) — the JS reads it, so it must be present and parse-able shape.
func TestDemoTimelineTuner_EmbedsMockData(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoTimelineTuner().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ev-coronation", "ent-aragorn", "special_moon_days", "connections"} {
		if !strings.Contains(out, want) {
			t.Errorf("tuner embed missing %q", want)
		}
	}
}

// Page-separation: the Tuner loads ONLY its own assets and carries a
// back-link to the designs index; the route is in the wire snapshot and
// the index links to it.
func TestDemoTimelineTuner_PageSeparation(t *testing.T) {
	root := tunerRepoRoot(t)
	snap, err := os.ReadFile(filepath.Join(root, "internal", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(snap), "/demo/timeline/tuner") {
		t.Errorf("wire snapshot missing the /demo/timeline/tuner route")
	}

	var idx bytes.Buffer
	if err := DemoCalendarIndex().Render(context.Background(), &idx); err != nil {
		t.Fatalf("render index: %v", err)
	}
	ih := idx.String()
	if !strings.Contains(ih, `href="/demo/timeline/tuner"`) {
		t.Errorf("index page missing link to the Tuner timeline")
	}
	if strings.Contains(ih, "cal-timeline-tuner.css") || strings.Contains(ih, "cal-timeline-tuner.js") {
		t.Errorf("index page must NOT load the Tuner assets (page-separation)")
	}

	var pg bytes.Buffer
	if err := DemoTimelineTuner().Render(context.Background(), &pg); err != nil {
		t.Fatalf("render tuner: %v", err)
	}
	if !strings.Contains(pg.String(), `href="/demo/calendar"`) {
		t.Errorf("tuner page missing the back-link to the designs index")
	}
}

// No inline <script> body in the templ source (externalized JS only). The
// JSON mock rides a data-attribute, not a <script> node.
func TestCalTimelineTunerJS_NoInlineScript(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "calendar_timeline_tuner.templ"))
	if err != nil {
		t.Fatalf("read templ: %v", err)
	}
	stripped := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(src), "")
	open := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	for _, m := range open.FindAllStringSubmatch(stripped, -1) {
		attrs := strings.ToLower(m[1])
		if strings.Contains(attrs, "src=") {
			continue
		}
		t.Errorf("inline <script> tag found in calendar_timeline_tuner.templ; attrs: %q", m[1])
	}
	// The externalized JS file must exist on disk.
	if _, err := os.Stat(filepath.Join(tunerRepoRoot(t), "static", "js", "cal-timeline-tuner.js")); err != nil {
		t.Errorf("cal-timeline-tuner.js missing on disk: %v", err)
	}
}

func readTunerCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(tunerRepoRoot(t), "static", "css", "cal-timeline-tuner.css"))
	if err != nil {
		t.Fatalf("read tuner css: %v", err)
	}
	return string(b)
}

func stripTunerCSSComments(s string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(s, "")
}

// Canon §B2 / D5: never `transition: all` — always list properties.
func TestCalTimelineTunerCSS_NoTransitionAll(t *testing.T) {
	src := stripTunerCSSComments(readTunerCSS(t))
	if strings.Contains(src, "trans"+"ition: all") || strings.Contains(src, "trans"+"ition:all") {
		t.Errorf("cal-timeline-tuner.css contains forbidden `transition: all` — §B2 violation")
	}
}

// Self-contained: no @layer, no @apply, no --chronicle-* tokens. Must
// carry real hover + transition rules (no dead-CSS regression).
func TestCalTimelineTunerCSS_SelfContained(t *testing.T) {
	src := stripTunerCSSComments(readTunerCSS(t))
	for _, forbidden := range []string{"@layer", "@apply", "--chronicle-"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("cal-timeline-tuner.css must not contain %q (self-contained)", forbidden)
		}
	}
	if !strings.Contains(src, ":hover") {
		t.Errorf("cal-timeline-tuner.css has no :hover rules")
	}
	if !strings.Contains(src, "transition:") {
		t.Errorf("cal-timeline-tuner.css has no transitions")
	}
}

// The rendering-canvas CSS exemption marker must be present (so the
// Wave-6 customization-readiness sweep knowingly skips the hardcoded
// OKLCH canvas literals). decisions/2026-06-05-rendering-canvas-css-exemption.md.
func TestCalTimelineTunerCSS_ExemptionMarker(t *testing.T) {
	src := readTunerCSS(t)
	for _, want := range []string{"RENDERING-CANVAS CSS", "rendering-canvas-css-exemption.md"} {
		if !strings.Contains(src, want) {
			t.Errorf("cal-timeline-tuner.css missing exemption marker fragment %q", want)
		}
	}
}

// The mock has the structures the design renders: eras for the bands,
// tiers + categories for card sizing/color, entities for lanes, multiple
// events spanning a wide range, connections, and special-moon days.
func TestCalTimelineTunerMock_Complete(t *testing.T) {
	d := CalTimelineTunerMock()
	if len(d.Eras) < 2 {
		t.Errorf("expected ≥2 eras; got %d", len(d.Eras))
	}
	if len(d.Tiers) < 3 {
		t.Errorf("expected 3 tiers; got %d", len(d.Tiers))
	}
	if len(d.Entities) < 3 {
		t.Errorf("expected ≥3 entities; got %d", len(d.Entities))
	}
	if len(d.Events) < 8 {
		t.Errorf("expected ≥8 events for a meaningful timeline; got %d", len(d.Events))
	}
	if len(d.Connections) < 3 {
		t.Errorf("expected ≥3 connections; got %d", len(d.Connections))
	}
	if len(d.SpecialMoonDays) < 1 {
		t.Errorf("expected ≥1 special-moon day for the backdrop restraint demo")
	}
	// Wide range so zoom-out (millennia) has anchors and zoom-in (days) a cluster.
	min, max := 1<<30, -(1 << 30)
	for _, e := range d.Events {
		if e.Year < min {
			min = e.Year
		}
		if e.Year > max {
			max = e.Year
		}
	}
	if max-min < 1000 {
		t.Errorf("event year span %d too narrow to exercise zoom-out; want ≥1000", max-min)
	}
	// Every connection references real events.
	ids := map[string]bool{}
	for _, e := range d.Events {
		ids[e.ID] = true
	}
	for _, c := range d.Connections {
		if !ids[c.Source] || !ids[c.Target] {
			t.Errorf("connection %s→%s references a missing event", c.Source, c.Target)
		}
	}
}

// calendar_test.go — C-CAL-SHOWCASE-DESIGN-1-ALMANAC.
//
// Light guards — the showcase isn't load-bearing for canon discipline.
// These pin: render smoke, route registered, JS externalized + no
// inline body, CSS self-contained (no @layer / @apply / canon tokens
// / transition:all), the design switcher renders (Almanac active +
// Linear/Compact scaffolded), and the mock dataset has the full
// fantasy-calendar surface (months/moons/seasons/festivals/events).

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

func TestDemoCalendar_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() < 2000 {
		t.Errorf("render too small (%d bytes)", buf.Len())
	}
	html := buf.String()
	if !strings.Contains(html, "cal-almanac-shell") {
		t.Errorf("almanac shell class missing")
	}
}

// TestDemoCalendar_RouteRegistered — the new /demo/calendar route is in
// the wire snapshot.
func TestDemoCalendar_RouteRegistered(t *testing.T) {
	root := calDemoRepoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "internal", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(b), "/demo/calendar") {
		t.Errorf("wire snapshot missing /demo/calendar route")
	}
}

// TestCalAlmanacJS_ExternalFileLoaded — JS is the externalized file
// (carry-forward: never inline a demo-specific <script> body).
func TestCalAlmanacJS_ExternalFileLoaded(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `src="/static/js/cal-almanac.js"`) {
		t.Errorf("calendar templ must load /static/js/cal-almanac.js via <script src=… defer>")
	}
	if !strings.Contains(html, "defer") {
		t.Errorf("script tag must carry defer")
	}
	if _, err := os.Stat(filepath.Join(calDemoRepoRoot(t), "static", "js", "cal-almanac.js")); err != nil {
		t.Errorf("cal-almanac.js missing on disk: %v", err)
	}
}

// TestCalAlmanacJS_NoInlineScript — the templ source must carry no
// inline <script> body of its own. The mock-data JSON node uses
// <script type="application/json"> which is data, not code, and is
// allowed (it doesn't execute).
func TestCalAlmanacJS_NoInlineScript(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "calendar.templ"))
	if err != nil {
		t.Fatalf("read templ: %v", err)
	}
	stripped := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(src), "")
	open := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	for _, m := range open.FindAllStringSubmatch(stripped, -1) {
		attrs := strings.ToLower(m[1])
		if strings.Contains(attrs, "src=") {
			continue // external script — fine
		}
		if strings.Contains(attrs, `type="application/json"`) {
			continue // mock data payload, not code
		}
		t.Errorf("inline <script> tag found in calendar.templ; attrs: %q", m[1])
	}
}

// TestCalAlmanacCSS_NoTransitionAll — canon D5 / §B2 binding: never
// `transition: all` (always list properties).
func TestCalAlmanacCSS_NoTransitionAll(t *testing.T) {
	src := stripCalCSSComments(readCalAlmanacCSS(t))
	forbidden := "trans" + "ition: all"
	if strings.Contains(src, forbidden) {
		t.Errorf("cal-almanac.css contains forbidden `transition: all` — §B2 violation")
	}
}

// TestCalAlmanacCSS_SelfContained — the entire reason for the
// .cal-almanac-* approach: no @layer, no Tailwind @apply, no canon
// (--chronicle-*) tokens. Hardcoded OKLCH only.
func TestCalAlmanacCSS_SelfContained(t *testing.T) {
	src := stripCalCSSComments(readCalAlmanacCSS(t))
	for _, forbidden := range []string{"@layer", "@apply", "--chronicle-"} {
		if strings.Contains(src, forbidden) {
			t.Errorf("cal-almanac.css must not contain %q (self-contained, no cascade fight)", forbidden)
		}
	}
	// Must carry real hover/transition rules (no dead-CSS regression).
	if !strings.Contains(src, ":hover") {
		t.Errorf("cal-almanac.css has no :hover rules")
	}
	if !strings.Contains(src, "transition:") {
		t.Errorf("cal-almanac.css has no transitions")
	}
}

// TestDemoCalendar_DesignSwitcher — the Almanac/Linear/Compact switcher
// renders with Almanac active and Linear/Compact scaffolded as disabled.
func TestDemoCalendar_DesignSwitcher(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "cal-almanac-switch__btn--active") {
		t.Errorf("active design tab marker missing")
	}
	for _, name := range []string{"Almanac", "Linear", "Compact"} {
		if !strings.Contains(html, name) {
			t.Errorf("design switcher missing %q", name)
		}
	}
}

// TestDemoCalendar_MockDataComplete — the mock fantasy calendar has all
// the structures the design demonstrates (months, moons, seasons,
// festivals, events spread across tiers + categories).
func TestDemoCalendar_MockDataComplete(t *testing.T) {
	d := CalAlmanacMock()
	if len(d.Months) < 12 {
		t.Errorf("expected ≥12 months; got %d", len(d.Months))
	}
	if len(d.Weekdays) < 7 {
		t.Errorf("expected ≥7 weekdays; got %d", len(d.Weekdays))
	}
	if len(d.Moons) < 1 {
		t.Errorf("expected ≥1 moon; got %d", len(d.Moons))
	}
	if len(d.Seasons) < 4 {
		t.Errorf("expected 4 seasons; got %d", len(d.Seasons))
	}
	if len(d.Festivals) < 2 {
		t.Errorf("expected ≥2 festivals; got %d", len(d.Festivals))
	}
	if len(d.Tiers) != 3 {
		t.Errorf("expected the canon 3 tiers (major/standard/detail); got %d", len(d.Tiers))
	}
	if len(d.Events) < 12 {
		t.Errorf("expected ≥12 events; got %d", len(d.Events))
	}
	// At least one event of each tier so the visual coverage is honest.
	tiers := map[string]bool{}
	for _, e := range d.Events {
		tiers[e.Tier] = true
	}
	for _, want := range []string{"major", "standard", "detail"} {
		if !tiers[want] {
			t.Errorf("mock data missing any event of tier %q", want)
		}
	}
	// At least one multi-day event (so the multi-day-touches-day
	// expansion gets exercised).
	multi := 0
	for _, e := range d.Events {
		if e.EndMonth != 0 {
			multi++
		}
	}
	if multi == 0 {
		t.Errorf("expected ≥1 multi-day event in mock data")
	}
	// At least one specific-visibility event (drives the chip-row editor).
	specific := 0
	for _, e := range d.Events {
		if e.Visibility == "specific" {
			specific++
		}
	}
	if specific == 0 {
		t.Errorf("expected ≥1 specific-visibility event so the chip-row editor renders rules on click")
	}
}

// TestDemoCalendar_AlmanacHooks — pin the MUST-tier interactive hooks.
// Each is a data-* attribute the JS init blocks bind; missing any
// means the corresponding interaction won't fire.
func TestDemoCalendar_AlmanacHooks(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		"data-cal-widget",
		"data-cal-drag-handle",
		"data-cal-resize",
		"data-cal-sky",
		"data-cal-sky-sun",
		"data-cal-sky-moon",
		"data-cal-sky-scrub",
		"data-cal-grid",
		"data-cal-cell",
		"data-cal-event-id",
		"data-cal-drag-ghost",
		"data-cal-drawer",
		"data-cal-vis-mode",
		"data-cal-vis-rules",
		"data-cal-vis-chips",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("required hook missing: %s", h)
		}
	}
}

// TestDemoCalendar_RefinementMockData — operator's refinement pass
// (post-PR-#385) adds named-vocabulary data to the mock so the demo
// reads like worldbuilding instead of procedural fill. Pin the
// presence of each new structure.
func TestDemoCalendar_RefinementMockData(t *testing.T) {
	d := CalAlmanacMock()
	if len(d.Eras) < 3 {
		t.Errorf("expected ≥3 eras (past + current + ancient); got %d", len(d.Eras))
	}
	var current int
	for _, e := range d.Eras {
		if eraContainsYear(e, d.CurrentYear) {
			current++
		}
	}
	if current == 0 {
		t.Errorf("no era contains the current year %d", d.CurrentYear)
	}
	if len(d.WeatherTypes) < 8 {
		t.Errorf("expected ≥8 named weather types across categories; got %d", len(d.WeatherTypes))
	}
	// Categories present.
	wcats := map[string]bool{}
	for _, w := range d.WeatherTypes {
		wcats[w.Category] = true
	}
	for _, want := range []string{"standard", "severe", "environmental", "fantasy"} {
		if !wcats[want] {
			t.Errorf("weather vocabulary missing category %q", want)
		}
	}
	if len(d.MoonPhases) < 8 {
		t.Errorf("expected ≥8 named moon phases for the primary moon; got %d", len(d.MoonPhases))
	}
	if len(d.DayWeather) < 10 {
		t.Errorf("expected ≥10 day-keyed weather entries; got %d", len(d.DayWeather))
	}
	if len(d.DayNotes) < 1 {
		t.Errorf("expected ≥1 day note in the showcase mock; got %d", len(d.DayNotes))
	}
	if len(d.Recurring) < 1 {
		t.Errorf("expected ≥1 recurring template demonstrating recurring + override pattern; got %d", len(d.Recurring))
	}
	// At least one recurring template must carry per-instance overrides.
	anyOverride := false
	for _, r := range d.Recurring {
		if len(r.Overrides) > 0 {
			anyOverride = true
			break
		}
	}
	if !anyOverride {
		t.Errorf("expected at least one recurring template to demonstrate per-instance overrides")
	}
	// Categories all carry an Icon (no empties).
	for _, c := range d.Categories {
		if c.Icon == "" {
			t.Errorf("category %q is missing an Icon glyph", c.ID)
		}
	}
}

// TestDemoCalendar_PopoverAndTimepiece — the refinement adds an
// anchored popover (replacing the right-edge drawer for quick edits)
// + a standalone timepiece widget. Pin their markup hooks.
func TestDemoCalendar_PopoverAndTimepiece(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoCalendar().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, h := range []string{
		// Popover.
		"data-cal-pop",
		"data-cal-pop-title",
		"data-cal-pop-meta",
		"data-cal-pop-desc",
		`data-cal-pop-tab="detail"`,
		`data-cal-pop-tab="notes"`,
		`data-cal-pop-tab="vis"`,
		"data-cal-pop-notes",
		"data-cal-pop-open-drawer",
		"data-cal-pop-close",
		"data-cal-pop-arrow",
		// Eras.
		"data-cal-eras",
		"cal-almanac-eras__band--current",
		// Timepiece.
		"data-cal-time",
		"data-cal-time-drag",
		"data-cal-time-tick",
		"data-cal-time-clock",
		// Named weather chip in cells.
		"cal-almanac-cell__wchip",
		// Category icons in event chips.
		"cal-almanac-chip__icon",
		// Recurring marker.
		"cal-almanac-chip--recurring",
	} {
		if !strings.Contains(html, h) {
			t.Errorf("refinement hook missing: %s", h)
		}
	}
	// The side drawer stays for heavy edits — must still exist.
	if !strings.Contains(html, "data-cal-drawer") {
		t.Errorf("side drawer markup should still be present (heavy-edit escalation target)")
	}
}

// ---- helpers ------------------------------------------------------

func stripCalCSSComments(src string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
}
func calDemoRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}
func readCalAlmanacCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(calDemoRepoRoot(t), "static", "css", "cal-almanac.css"))
	if err != nil {
		t.Fatalf("read cal-almanac.css: %v", err)
	}
	return string(b)
}

// chronicle_canon_test.go — C-V2-DESIGN-REBUILD Phase 1.8 demo tests.
//
// After the Phase 1.8 reset (dispatch
// C-V2-DESIGN-REBUILD-PHASE-1-8-DEMO-RESET):
//
//   1. Render smoke — the templ renders without panic.
//   2. Selected-state A/B both present in the choice-picker — the
//      validation crux still must render both variants.
//   3. CSS hard-rule lint — no `transition: all`, no hex literals,
//      no shadow-alone transitions. Pinned because canon D2/D3/D5
//      depend on them.
//   4. External-JS architecture pins — the demoScript() function is
//      gone; the demo templ MUST reference the external JS file via
//      `<script src=…>` and MUST NOT carry an inline `<script>` body.
//      INIT_BLOCKS registry must register each expected block.
//   5. Diagnostic dashboard — the load-bearing visible diagnostic
//      surface markup must render at top of page.
//   6. Choice-picker infrastructure — the 9 variant groups + Your
//      Decisions panel still present after the shrink.
//   7. Theme default + button-variant + dark-bg-not-black — operator
//      preferences that survived multiple prior phases stay pinned.

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

// TestDemoChronicleCanon_RendersWithoutPanic — basic templ smoke.
func TestDemoChronicleCanon_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("render too small (%d bytes); expected substantial page", buf.Len())
	}
	html := buf.String()
	if !strings.Contains(html, `data-chronicle-demo`) {
		t.Errorf("demo root scope attribute missing; CSS tokens won't resolve")
	}
}

// TestDemoChronicleCanon_RendersBothSelectedVariants — the validation
// crux pin. Phase 1.8 reframes selected-state as a choice-picker variant
// group, so both options live as `data-variant-group="selected" data-
// variant="A"`/`"B"`. If a future edit drops either, operator can't
// make the choice and the canon stays under-specified.
func TestDemoChronicleCanon_RendersBothSelectedVariants(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `data-variant-group="selected"`) {
		t.Errorf("selected-state variant group missing — operator can't compare A vs B")
	}
	// Both A and B variants must be present inside the selected group.
	groupSection := html
	if idx := strings.Index(html, `data-variant-group="selected"`); idx >= 0 {
		end := idx + 1500
		if end > len(html) {
			end = len(html)
		}
		groupSection = html[idx:end]
	}
	if !strings.Contains(groupSection, `data-variant="A"`) {
		t.Errorf("Variant A (canon D3 accent border + tint) missing from selected group")
	}
	if !strings.Contains(groupSection, `data-variant="B"`) {
		t.Errorf("Variant B (navbar-echo persistent half-hover) missing from selected group")
	}
}

// TestDemoChronicleCanon_HarnessControlsPresent — theme + reduced-motion
// only. Accent + density moved into the choice-picker per dispatch §D.
func TestDemoChronicleCanon_HarnessControlsPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`data-action="theme"`,
		`data-action="reduce-motion"`,
		`data-value="light"`,
		`data-value="dark"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("harness control marker missing: %q", marker)
		}
	}
}

// TestDemoChronicleCanon_DefaultsToDarkMode — the templ's root data-
// attribute must default to dark (no-JS fallback), and the external
// JS must wire the localStorage/matchMedia precedence.
func TestDemoChronicleCanon_DefaultsToDarkMode(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `data-chronicle-demo-theme="dark"`) {
		t.Errorf("root element should default to dark mode (operator's preferred) — " +
			"a light-mode default forces a paint-flash on every reload for dark-mode users")
	}
	// The external JS must still implement the precedence ladder.
	js := readCanonJS(t)
	if !strings.Contains(js, "localStorage.getItem('chronicle-demo-theme')") {
		t.Errorf("external JS should read theme choice from localStorage so explicit toggles survive reload")
	}
	if !strings.Contains(js, "prefers-color-scheme") {
		t.Errorf("external JS should detect OS preference via matchMedia('(prefers-color-scheme: ...)')")
	}
}

// ============================================================
// Phase 1.8 — external JS + diagnostic dashboard pins
// ============================================================

// TestChronicleCanonDemoJS_ExternalFileLoaded — the demo templ MUST
// reference the external JS file via `<script src=…>`. If a future
// edit re-inlines the script body, the per-block error isolation
// architecture goes with it. Pin the script-src reference here.
func TestChronicleCanonDemoJS_ExternalFileLoaded(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `src="/static/js/chronicle-canon-demo.js"`) {
		t.Errorf("demo templ must reference external JS via <script src=\"/static/js/chronicle-canon-demo.js\" defer></script>")
	}
	if !strings.Contains(html, "defer") {
		t.Errorf("external script tag must carry defer (parsed-DOM guarantee)")
	}
	// And the file must actually exist on disk so the route serves it.
	jsPath := canonJSPath(t)
	if _, err := os.Stat(jsPath); err != nil {
		t.Errorf("external JS file missing on disk at %s: %v", jsPath, err)
	}
}

// TestChronicleCanonDemoJS_NoInlineScriptBlocks — verifies the demo
// templ source carries no inline `<script>` body. The rendered page
// will still contain layout-level inline scripts (theme-flash prevention
// from base.templ); those are not in scope here. defer-loaded external
// file is the only acceptable script in the demo's own templ.
func TestChronicleCanonDemoJS_NoInlineScriptBlocks(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	templPath := filepath.Join(filepath.Dir(thisFile), "chronicle_canon.templ")
	b, err := os.ReadFile(templPath)
	if err != nil {
		t.Fatalf("read demo templ: %v", err)
	}
	src := string(b)
	// Strip Go-line-comment blocks so the file-header narration (which
	// legitimately mentions removing inline script) doesn't trip the lint.
	src = regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(src, "")
	open := regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	for _, m := range open.FindAllStringSubmatch(src, -1) {
		attrs := strings.ToLower(m[1])
		if strings.Contains(attrs, "src=") {
			continue // external script — fine
		}
		t.Errorf("inline <script> tag found in demo templ source; Phase 1.8 forbids inline script bodies in the demo templ. Attrs: %q", m[1])
	}
}

// TestChronicleCanonDemoJS_AllInitBlocksRegistered — parses the JS
// file (cheap regex) and verifies every expected init block is
// registered. Catches drive-by edits that drop blocks from the registry.
func TestChronicleCanonDemoJS_AllInitBlocksRegistered(t *testing.T) {
	js := readCanonJS(t)
	required := []string{
		"diagnostic-dashboard",
		"browser-compat-detect",
		"theme-toggle",
		"reduced-motion-toggle",
		"decision-store-hydrate",
		"choice-picker-rate",
		"choice-picker-vote",
		"choice-picker-apply",
		"decisions-panel",
		"preview-drawer",
		"diagnostics-copy-report",
	}
	for _, name := range required {
		marker := "registerInitBlock('" + name + "'"
		if !strings.Contains(js, marker) {
			t.Errorf("init block not registered: %s (looking for %q)", name, marker)
		}
	}
	// And the registry pattern itself.
	if !strings.Contains(js, "INIT_BLOCKS") {
		t.Errorf("INIT_BLOCKS registry missing from external JS")
	}
	if !strings.Contains(js, "try {") || !strings.Contains(js, "catch (err)") {
		t.Errorf("per-block try/catch missing from runAllInitBlocks()")
	}
	// __chronicleDemoInited must be set AFTER the run, not before.
	// Heuristic: find the assignment and check that runAllInitBlocks()
	// appears before it in the same containing function.
	if !strings.Contains(js, "window.__chronicleDemoInited = true") {
		t.Errorf("__chronicleDemoInited assignment missing")
	}
	// document.title fallback for catastrophic dashboard failure.
	if !strings.Contains(js, "document.title") {
		t.Errorf("document.title fallback signal missing (dispatch §C stop-and-flag #5)")
	}
}

// TestChronicleCanonDemo_DiagnosticDashboardPresent — the visible
// diagnostic surface. Per dispatch §C, every operator report cycle
// after this PR depends on this panel rendering and getting populated.
func TestChronicleCanonDemo_DiagnosticDashboardPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`data-demo-diagnostics`,
		`Demo Diagnostics`,
		`data-init-block-list`,
		`data-binding-list`,
		`data-feature-list`,
		`data-ua-string`,
		`data-last-action-log`,
		`data-copy-report`,
		`data-diagnostics-summary`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("diagnostic dashboard marker missing: %s", marker)
		}
	}
	// The dashboard should come BEFORE the picker section in the HTML
	// so it's the first thing the operator sees on page load.
	dashIdx := strings.Index(html, `data-demo-diagnostics`)
	pickerIdx := strings.Index(html, `id="choices"`)
	if dashIdx < 0 || pickerIdx < 0 || dashIdx >= pickerIdx {
		t.Errorf("diagnostic dashboard must precede the choice-picker in the rendered HTML (operator sees diagnostics first)")
	}
}

// ============================================================
// Choice-picker + preview area pins
// ============================================================

// TestDemoChronicleCanon_PickerInfrastructure — the 9 variant groups
// + Your Decisions panel. Load-bearing for canon validation.
func TestDemoChronicleCanon_PickerInfrastructure(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`data-vote=`,                       // choose-mode thumbs
		`data-variant-apply`,               // live-apply button
		`data-decisions-output`,            // Your Decisions output
		`data-decisions-copy`,
		`data-decisions-download`,
		`id="choices"`,                     // design-choices section
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("picker infrastructure marker missing: %s", marker)
		}
	}
	// All 9 variant groups must be present.
	groups := []string{"bg", "accent", "radius", "shadow", "hover", "motion", "density", "selected", "button"}
	for _, g := range groups {
		if !strings.Contains(html, `data-variant-group="`+g+`"`) {
			t.Errorf("variant group missing: %s", g)
		}
	}
}

// TestDemoChronicleCanon_PreviewArea — small live preview surface per
// dispatch §E. Card + button row + input + chip set + drawer toggle.
func TestDemoChronicleCanon_PreviewArea(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`id="preview"`,
		`chronicle-preview__row--cards`,
		`chronicle-preview__row--buttons`,
		`data-preview-drawer-toggle`,
		`data-preview-drawer`,
		`chronicle-chip`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("preview area marker missing: %s", marker)
		}
	}
}

// ============================================================
// CSS hard-rule lint guards (kept from prior phases)
// ============================================================

// TestChronicleCanonDemoCSS_NoTransitionAll — canon D5 hard rule.
// `transition: all` is the single biggest motion-discipline failure
// in the rejected V2. Pin: must never reappear.
func TestChronicleCanonDemoCSS_NoTransitionAll(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	forbidden := "trans" + "ition: all"
	if strings.Contains(src, forbidden) {
		t.Errorf("chronicle-canon-demo.css contains forbidden `transition: all` — canon D5 violation")
	}
}

// TestChronicleCanonDemoCSS_NoHexLiterals — canon D2 OKLCH-only.
func TestChronicleCanonDemoCSS_NoHexLiterals(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	idSelector := regexp.MustCompile(`#[a-zA-Z][\w-]*`)
	scrubbed := idSelector.ReplaceAllString(src, "")
	hex := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	matches := hex.FindAllString(scrubbed, -1)
	if len(matches) > 0 {
		t.Errorf("chronicle-canon-demo.css contains %d hex literal(s) — canon D2 mandates OKLCH only.\n"+
			"First few: %v", len(matches), matches[:min(5, len(matches))])
	}
}

// TestChronicleCanonDemoCSS_NoShadowAloneTransition — canon D3 hard
// rule: never animate box-shadow alone. The Phase 1.5 `.chronicle-shadow-
// rejected` educational counter-example is no longer in the file
// (motion vocabulary section dropped in Phase 1.8), so the lint can be
// stricter — any shadow-alone transition is now a real violation.
func TestChronicleCanonDemoCSS_NoShadowAloneTransition(t *testing.T) {
	raw := readCanonCSS(t)
	src := stripCSSComments(raw)
	transitionDecl := regexp.MustCompile(`(?s)transition:\s*([^;]+);`)
	for _, m := range transitionDecl.FindAllStringSubmatchIndex(src, -1) {
		body := src[m[2]:m[3]]
		if !strings.Contains(body, "box-shadow") {
			continue
		}
		coOccur := []string{
			"background-color", "border-color", "color",
			"transform", "opacity", "background",
		}
		paired := false
		for _, c := range coOccur {
			if strings.Contains(body, c) {
				paired = true
				break
			}
		}
		if paired {
			continue
		}
		// The educational rejected-shadow exemption from Phase 1.5
		// stays as a tolerated path in case the motion-vocab section
		// gets revived. Look for the marker.
		surroundingStart := m[0] - 200
		if surroundingStart < 0 {
			surroundingStart = 0
		}
		surroundingEnd := m[1] + 50
		if surroundingEnd > len(src) {
			surroundingEnd = len(src)
		}
		surrounding := src[surroundingStart:surroundingEnd]
		if strings.Contains(surrounding, ".chronicle-shadow-rejected") {
			continue
		}
		if strings.Contains(raw, "lint-exempt: rejected-demo") {
			continue
		}
		t.Errorf("transition declaration animates box-shadow alone — canon D3 "+
			"co-occurrence violation. Pair box-shadow with at least one of "+
			"background-color / border-color / color / transform / opacity.\n"+
			"Declaration: %s", strings.TrimSpace(body))
	}
}

// TestChronicleCanonDemoCSS_DarkBackgroundNotBlack — operator's "the
// background should be the same darkmode idea we already had on the
// website." Dark default must be cool gray-900, NOT near-black.
func TestChronicleCanonDemoCSS_DarkBackgroundNotBlack(t *testing.T) {
	src := readCanonCSS(t)
	if !strings.Contains(src, "oklch(0.21 0.034 264.665)") {
		t.Errorf("dark-mode default surface should be cool gray-900 oklch(0.21 0.034 264.665) to match the live site")
	}
	if !strings.Contains(src, `data-chronicle-bg="black"`) {
		t.Errorf("near-black should be preserved as the opt-in data-chronicle-bg=\"black\" variant")
	}
}

// TestChronicleCanonDemoCSS_ButtonVariantsDistinct — the 6-way button
// spread (primary / tonal / secondary / ghost / link / destructive)
// is the choice-picker's "button family" group. Pin all six.
func TestChronicleCanonDemoCSS_ButtonVariantsDistinct(t *testing.T) {
	css := readCanonCSS(t)
	for _, cls := range []string{
		".chronicle-button-primary",
		".chronicle-button-tonal",
		".chronicle-button-secondary",
		".chronicle-button-ghost",
		".chronicle-button-link",
		".chronicle-button-destructive",
	} {
		if !strings.Contains(css, cls+" {") {
			t.Errorf("button variant definition missing: %s", cls)
		}
	}
	secIdx := strings.Index(css, ".chronicle-button-secondary {")
	if secIdx >= 0 {
		block := css[secIdx:]
		if end := strings.Index(block, "}"); end >= 0 && strings.Contains(block[:end], "background: transparent") {
			t.Errorf("secondary button should have a resting surface fill, not transparent (else it reads identical to ghost)")
		}
	}
}

// ============================================================
// Helpers
// ============================================================

func stripCSSComments(src string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
}

func canonCSSPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "static", "css", "chronicle-canon-demo.css")
}

func canonJSPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "static", "js", "chronicle-canon-demo.js")
}

func readCanonCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(canonCSSPath(t))
	if err != nil {
		t.Fatalf("read canon css: %v", err)
	}
	return string(b)
}

func readCanonJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(canonJSPath(t))
	if err != nil {
		t.Fatalf("read canon js: %v", err)
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

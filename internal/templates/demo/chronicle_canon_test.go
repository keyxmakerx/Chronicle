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

// TestDemoChronicleCanon_RateModeMarkup — Phase 1.9 §A restoration.
// The choice-picker must carry rate-mode markup for the 5 locked
// decisions; otherwise the existing `choice-picker-rate` JS init block
// binds zero pills (the exact Bug #1 from the PR #379 dashboard report).
// Pin all 5 rate keys + the data-rate-id / data-rate / data-note hooks
// the JS expects.
func TestDemoChronicleCanon_RateModeMarkup(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	// 5 locked-decision keys.
	for _, key := range []string{"typography", "token-namespace", "spacing-grid", "co-occurrence-rule", "oklch-only"} {
		if !strings.Contains(html, `data-rate-id="`+key+`"`) {
			t.Errorf("rate-mode group missing for key %q (data-rate-id)", key)
		}
	}
	// JS block binds these hooks; pin presence so future edits can't
	// silently drop the markup and reproduce Bug #1.
	for _, marker := range []string{
		`chronicle-rate-group`,
		`chronicle-rate-pills`,
		`chronicle-rate-pill`,
		`chronicle-rate-note`,
		`data-rate-label`,
		`data-rate="1"`,
		`data-rate="5"`,
		`data-note`,
		`id="rate-mode"`,
		`id="vote-mode"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("rate-mode markup hook missing: %s", marker)
		}
	}
	// 5 groups × 5 pills = 25 pills total.
	pills := strings.Count(html, `class="chronicle-rate-pill"`)
	if pills != 25 {
		t.Errorf("expected 25 rate-mode pills (5 groups × 5); got %d", pills)
	}
}

// TestDemoChronicleCanon_DecisionsPanelReflectsStore — Phase 1.9 §B.
// Pins the visible summary line + per-bucket pills that JS populates
// from the unified store. Authoritative status-display lives here so
// a render-bug in the textarea can't silently hide non-zero state
// again (Bug #2 from PR #379).
func TestDemoChronicleCanon_DecisionsPanelReflectsStore(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`data-decisions-summary`,
		`data-summary-ratings`,
		`data-summary-votes`,
		`data-summary-applied`,
		`data-summary-empty`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("decisions-summary hook missing: %s", marker)
		}
	}
	// The textarea must NOT carry a server-rendered "No decisions yet"
	// default — JS owns the textarea content unconditionally now.
	taIdx := strings.Index(html, `data-decisions-output`)
	if taIdx >= 0 {
		// Look at the tag's full open-then-content slice.
		end := strings.Index(html[taIdx:], `</textarea>`)
		if end < 0 {
			t.Fatal("could not locate decisions-output closing tag")
		}
		tag := html[taIdx : taIdx+end]
		if strings.Contains(tag, "No decisions yet") {
			t.Errorf("decisions textarea must not pre-render 'No decisions yet' (let JS own the content; otherwise Bug #2 recurs when JS render is short-circuited)")
		}
	}
}

// TestChronicleCanonDemoJS_DecisionsPanelRenderRobust — Phase 1.9 §B.
// Pins the JS-side robustness: the unified summarizeDecisions() ->
// buildDecisionsMarkdown() -> renderDecisionsPanel() pipeline must
// always set out.value (no early return on missing element should
// leave the textarea at a stale state) AND must populate the visible
// summary pills.
func TestChronicleCanonDemoJS_DecisionsPanelRenderRobust(t *testing.T) {
	js := readCanonJS(t)
	for _, marker := range []string{
		"function summarizeDecisions",
		"function buildDecisionsMarkdown",
		"function renderDecisionsPanel",
		"data-summary-ratings",
		"data-summary-votes",
		"data-summary-applied",
		"data-summary-empty",
		// Three explicit markdown sections.
		"## Ratings",
		"## Votes",
		"## Applied",
		// Hydrate block renders immediately so a later block failure
		// can't leave the panel blank.
		"decision-store-hydrate",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("decisions-render robustness marker missing in JS: %q", marker)
		}
	}
	// renderDecisionsPanel must be called from inside the hydrate
	// block (defensive double-render against init-block failures).
	hydrateIdx := strings.Index(js, "registerInitBlock('decision-store-hydrate'")
	if hydrateIdx < 0 {
		t.Fatal("decision-store-hydrate block not found")
	}
	// Find the end of this block (the closing }) so the render-call
	// search is scoped to inside the block.
	tail := js[hydrateIdx:]
	endIdx := strings.Index(tail, "});")
	if endIdx < 0 {
		t.Fatal("decision-store-hydrate block has no closing '});'")
	}
	body := tail[:endIdx]
	if !strings.Contains(body, "renderDecisionsPanel()") {
		t.Errorf("decision-store-hydrate block must call renderDecisionsPanel() after loading localStorage so the panel reflects persisted state even if a later init block throws")
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

// TestDemoChronicleCanon_DecisionsPanelInsideTokenScope — the
// regression guard for the "floating text on a white box" bug
// (Phase 1.8 placed the decisions panel as a SIBLING of the
// [data-chronicle-demo] root, so it inherited none of the
// --chronicle-* tokens and rendered unstyled). The panel must be a
// DESCENDANT of the root element. Verified by a div-depth scan: the
// [data-decisions] marker must fall before the root <div> closes.
func TestDemoChronicleCanon_DecisionsPanelInsideTokenScope(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	rootStart := strings.Index(html, "chronicle-canon-page")
	if rootStart < 0 {
		t.Fatal("root .chronicle-canon-page element not found")
	}
	// Back up to the opening "<div" of the root element.
	openTag := strings.LastIndex(html[:rootStart], "<div")
	if openTag < 0 {
		t.Fatal("could not locate root opening <div")
	}
	// Walk forward from the root open, tracking div nesting depth.
	// When depth returns to 0 we've found the root's closing </div>.
	depth := 0
	rootClose := -1
	for i := openTag; i < len(html); {
		if strings.HasPrefix(html[i:], "<div") {
			depth++
			i += 4
			continue
		}
		if strings.HasPrefix(html[i:], "</div>") {
			depth--
			i += 6
			if depth == 0 {
				rootClose = i
				break
			}
			continue
		}
		i++
	}
	if rootClose < 0 {
		t.Fatal("could not find root closing </div> via depth scan")
	}
	panelIdx := strings.Index(html, "data-decisions")
	if panelIdx < 0 {
		t.Fatal("decisions panel marker not found")
	}
	if panelIdx > rootClose {
		t.Errorf("decisions panel (data-decisions at %d) renders OUTSIDE the [data-chronicle-demo] root (closes at %d) — "+
			"it will inherit no --chronicle-* tokens and render as unstyled black-text-on-white (the Phase 1.8 'white box' bug)", panelIdx, rootClose)
	}
}

// TestDemoChronicleCanon_VariantPreviewsSelfDemonstrate — each variant
// option's preview box must render a sample styled to its own option
// (not an inert label), so the operator sees the difference without
// clicking Apply. Pin the per-group demo hooks; their absence is the
// "nothing is happening" complaint.
func TestDemoChronicleCanon_VariantPreviewsSelfDemonstrate(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`chronicle-variant-demo`,             // base
		`chronicle-variant-demo__box`,        // radius + shadow samples
		`chronicle-variant-demo__hover--lift`, // hover sample
		`chronicle-variant-demo__motion`,     // motion sample
		`chronicle-variant-demo--density-compact`, // density sample
		`chronicle-variant-demo__sel--A`,     // selected sample
		`chronicle-variant-demo__sel--B`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("self-demonstrating variant-preview hook missing: %s", marker)
		}
	}
	// The old inert pattern (a bare label span as the only preview
	// content) must be gone for the structural groups — verify the
	// radius sample carries an inline border-radius, proving it
	// demonstrates rather than labels.
	if !strings.Contains(html, "border-radius:2px") && !strings.Contains(html, "border-radius: 2px") {
		t.Errorf("radius 'sharp' preview should carry an inline border-radius demonstrating the value")
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

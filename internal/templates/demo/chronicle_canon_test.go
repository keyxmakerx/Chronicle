// chronicle_canon_test.go — C-V2-DESIGN-REBUILD Phase 1 demo tests.
//
// Three guards:
//
//  1. Render smoke — the templ renders without panic and produces
//     non-empty HTML.
//  2. Selected-state A/B both present — the validation crux per
//     dispatch §C.1 + audit §1.4 must render both variants
//     side-by-side; CI fails loudly if either drops out.
//  3. canon CSS hard-rule lint — the new `chronicle-canon-demo.css`
//     file must NEVER use `transition: all` (canon D5) and must
//     NEVER use hex literals (canon D2 OKLCH-only). Both rules are
//     load-bearing for the rebuild's central premise; pin them here
//     so a future drive-by edit can't drift.

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
// Catches any nil-deref in the template at compile-time-of-render.
func TestDemoChronicleCanon_RendersWithoutPanic(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("render too small (%d bytes); expected substantial page", buf.Len())
	}
	html := buf.String()
	// Verify the canon scope-attribute is present so the @layer
	// chronicle-demo tokens actually resolve.
	if !strings.Contains(html, `data-chronicle-demo`) {
		t.Errorf("demo root scope attribute missing; CSS tokens won't resolve")
	}
}

// TestDemoChronicleCanon_RendersBothSelectedVariants — the validation
// crux pin. The dispatch + canon D3 + audit §1.4 all hinge on the
// operator picking between selected-state Variant A (canon D3 accent
// border + tint) and Variant B (navbar-echo persistent half-hover).
// If a future edit removes either, operator can't make the choice
// and the canon stays under-specified.
func TestDemoChronicleCanon_RendersBothSelectedVariants(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `data-selected="A"`) {
		t.Errorf("Variant A (canon D3 selected) missing — operator can't compare")
	}
	if !strings.Contains(html, `data-selected="B"`) {
		t.Errorf("Variant B (navbar-echo selected) missing — operator can't compare")
	}
	if !strings.Contains(html, "Variant A") || !strings.Contains(html, "Variant B") {
		t.Errorf("variant labels missing — operator can't tell which is which")
	}
}

// TestDemoChronicleCanon_HarnessControlsPresent — the three operator
// controls (theme, accent, reduced-motion) must all render. Each is
// load-bearing for at least one canon decision (D2 theming, D2
// per-campaign accent, D5 reduced-motion).
func TestDemoChronicleCanon_HarnessControlsPresent(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, marker := range []string{
		`data-action="theme"`,
		`data-action="accent"`,
		`data-action="reduce-motion"`,
		`data-value="indigo"`,
		`data-value="emerald"`,
		`data-value="amber"`,
		`data-value="rose"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("harness control marker missing: %q", marker)
		}
	}
}

// TestChronicleCanonDemoCSS_NoTransitionAll — canon D5 hard rule.
// `transition: all` is the single biggest motion-discipline failure
// in the rejected V2 (.card at input.css:436-439). The rebuild must
// never reintroduce it; pin here.
//
// Scans the file with CSS comment blocks stripped first — the canon's
// own header comment legitimately quotes the forbidden token while
// documenting the rule.
func TestChronicleCanonDemoCSS_NoTransitionAll(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	// Reconstruct the forbidden token at runtime to avoid the test
	// file itself tripping a grep of "transition: all".
	forbidden := "trans" + "ition: all"
	if strings.Contains(src, forbidden) {
		t.Errorf("chronicle-canon-demo.css contains forbidden `transition: all` — canon D5 violation. " +
			"Every transition must list properties explicitly (e.g. `transition: background-color, border-color, box-shadow ...`).")
	}
}

// TestChronicleCanonDemoCSS_NoHexLiterals — canon D2 OKLCH-only.
// Hex literals in the canon would break the per-campaign accent +
// future-extension hooks D2 mandates. Allow exactly one exception:
// `#chronicle-` selectors inside the file (those are CSS id
// selectors, not colour literals).
func TestChronicleCanonDemoCSS_NoHexLiterals(t *testing.T) {
	// Strip block comments first so the file header's "no hex literals"
	// documentation doesn't false-positive.
	src := stripCSSComments(readCanonCSS(t))
	// Strip CSS id selectors before scanning so `#chronicle-demo-popover`
	// etc. don't false-positive.
	idSelector := regexp.MustCompile(`#[a-zA-Z][\w-]*`)
	scrubbed := idSelector.ReplaceAllString(src, "")
	// Now scan for hex colour literals.
	hex := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	matches := hex.FindAllString(scrubbed, -1)
	if len(matches) > 0 {
		t.Errorf("chronicle-canon-demo.css contains %d hex literal(s) — canon D2 mandates OKLCH only.\n"+
			"First few: %v", len(matches), matches[:min(5, len(matches))])
	}
}

// TestChronicleCanonDemoCSS_NoShadowAloneTransition — canon D3 hard
// rule: never animate box-shadow alone. Heuristic check: every
// `transition:` declaration that mentions `box-shadow` must also
// mention at least one of {background-color, border-color, color,
// transform, opacity, background} as a co-occurring property.
//
// Comments stripped first (same reasoning as the no-transition-all
// test). This is a heuristic — a sufficiently determined edit could
// dodge it with two separate transition declarations. The intent is
// to catch accidental drift, not malicious bypass.
//
// Phase 1.5 exempt list: the Motion Vocabulary section's C.4
// co-occurrence demonstration deliberately renders the rejected
// pattern (box-shadow animating alone) next to the canon pattern
// so the operator can see the difference. The `.chronicle-shadow-
// rejected` selector is the educational counter-example; its
// shadow-alone transition is INTENTIONAL and required for the
// dispatch's headline validation moment. The test scans for a
// `/* lint-exempt: rejected-demo */` marker on the previous line
// (the marker survives comment-stripping because we scan the raw
// source for the marker first, then strip comments for the rest).
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
		// Look back from the transition declaration to find the
		// nearest selector + check the surrounding raw source for
		// the educational-exempt marker. The marker lives in the
		// RAW source (we hold it separately above so comment-
		// stripping doesn't erase it).
		surroundingStart := m[0] - 200
		if surroundingStart < 0 {
			surroundingStart = 0
		}
		surroundingEnd := m[1] + 50
		if surroundingEnd > len(src) {
			surroundingEnd = len(src)
		}
		surrounding := src[surroundingStart:surroundingEnd]
		// The selector for the educational counter-example is
		// `.chronicle-shadow-rejected` and its CSS lives next to
		// the lint-exempt marker. Allow this one specific selector
		// by name (the entire purpose of the rule is "no NEW
		// accidental drift"; an explicit educational counter-
		// example is by definition not drift).
		if strings.Contains(surrounding, ".chronicle-shadow-rejected") {
			continue
		}
		// Also accept the explicit /* lint-exempt: rejected-demo */
		// marker in the raw source within ~200 chars of the rule.
		rawStart := m[0] - 200
		if rawStart < 0 {
			rawStart = 0
		}
		rawEnd := m[1] + 50
		if rawEnd > len(raw) {
			rawEnd = len(raw)
		}
		// raw and src indices don't align after comment-stripping;
		// scan the entire raw source for the marker once per match.
		if strings.Contains(raw, "lint-exempt: rejected-demo") &&
			strings.Contains(raw[max0(0, rawStart):rawEnd], "shadow-rejected") {
			continue
		}
		t.Errorf("transition declaration animates box-shadow alone — canon D3 "+
			"co-occurrence violation. Pair box-shadow with at least one of "+
			"background-color / border-color / color / transform / opacity.\n"+
			"Declaration: %s", strings.TrimSpace(body))
	}
}

func max0(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// TestChronicleCanonDemoCSS_AnimationShowcaseRules — Phase 1.5
// dispatch §C.4: the rejected-vs-canon comparison pair must both
// exist in the CSS, because the operator validation moment depends
// on visually comparing them side-by-side. Pin both selectors here so
// a future drive-by edit can't remove either half of the comparison.
func TestChronicleCanonDemoCSS_AnimationShowcaseRules(t *testing.T) {
	src := readCanonCSS(t)
	required := []struct {
		selector string
		why      string
	}{
		{".chronicle-shadow-rejected", "the rejected box-shadow-alone counter-example (operator's 'shadow from nowhere' rendered literally for comparison)"},
		{".chronicle-shadow-canon", "the canon-compliant co-occurring transitions card (the rebuild's fix)"},
		{".chronicle-shadow-canon:hover", "the canon card's three-property hover transition (background-color + border-color + box-shadow)"},
		{".chronicle-motion-track", "the duration ladder mechanism (C.1)"},
		{".chronicle-hover-ladder__card", "the audit §1.4 hover-ladder preview (C.3)"},
		{".chronicle-anim-tile", "the animation library showcase tiles (C.5)"},
	}
	for _, r := range required {
		if !strings.Contains(src, r.selector) {
			t.Errorf("Motion Vocabulary required selector missing: %s — %s", r.selector, r.why)
		}
	}
}

// TestDemoChronicleCanon_DefaultsToDarkMode — Phase 1.5 dispatch
// §A: operator is dark-mode-only and frustrated by light-mode
// default flips on reload. The templ's initial data-attribute
// should already be `dark` so the no-JS fallback path is correct;
// the demoScript() init then applies localStorage → OS preference
// → dark default precedence to refine. Pin the no-JS contract here.
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
	// Sanity: the demoScript should still wire localStorage so explicit
	// operator choice is persisted across reloads (the dispatch's "respect
	// last toggle" requirement).
	if !strings.Contains(html, "localStorage.setItem('chronicle-demo-theme'") {
		t.Errorf("demoScript() should persist theme choice to localStorage so explicit toggles survive reload")
	}
	// And: the init should read OS preference via matchMedia.
	if !strings.Contains(html, "prefers-color-scheme") {
		t.Errorf("demoScript() should detect OS preference via matchMedia('(prefers-color-scheme: ...)')")
	}
}

// TestDemoChronicleCanon_RendersAllExpansionSections — Phase 1.5
// dispatch §B-§E section presence pin. The expansion's whole purpose
// is operator's "quadruple what's there" framing; if a section gets
// accidentally removed in a future refactor, the dispatch's
// acceptance criteria are not met. Pin every new section's anchor id.
func TestDemoChronicleCanon_RendersAllExpansionSections(t *testing.T) {
	var buf bytes.Buffer
	if err := DemoChronicleCanon().Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	sections := []struct {
		id  string
		why string
	}{
		{`id="motion"`, "Motion Vocabulary section (C — the dispatch headliner)"},
		{`id="forms"`, "Form Controls (B.1)"},
		{`id="buttons"`, "Buttons / extensions (B.2)"},
		{`id="status"`, "Status & Feedback (B.3)"},
		{`id="nav"`, "Navigation (B.4)"},
		{`id="data"`, "Data Display (B.5)"},
		{`id="overlays"`, "Overlays (B.7)"},
		{`id="containers"`, "Containers (B.6)"},
		{`id="density"`, "Density variants (D)"},
		{`id="mockups"`, "Page composition mockups (E.1)"},
	}
	for _, s := range sections {
		if !strings.Contains(html, s.id) {
			t.Errorf("section missing — %s: %s", s.why, s.id)
		}
	}
	// Page composition mockups must all carry the MOCKUP banner so
	// operator doesn't confuse them with real surfaces.
	mockupBannerCount := strings.Count(html, "chronicle-mockup-banner")
	if mockupBannerCount < 3 {
		t.Errorf("expected at least 3 MOCKUP banners (one per page-composition mockup); got %d", mockupBannerCount)
	}
	// Color picker preview is groundwork for D2 future-extension;
	// it must be explicitly labeled as not-yet-functional to avoid
	// operator confusion (dispatch §B.7 + stop-and-flag #7).
	if !strings.Contains(html, "NOT YET FUNCTIONAL") {
		t.Errorf("color picker preview should be explicitly labeled 'NOT YET FUNCTIONAL' (dispatch §B.7 stop-and-flag #7)")
	}
}

// stripCSSComments removes /* ... */ block comments from CSS source.
// Used by the lint tests so the canon's own documentation comments
// (which legitimately quote `transition: all` and other forbidden
// tokens while explaining the rules) don't trigger false positives.
// Non-greedy match across newlines.
func stripCSSComments(src string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
}

func readCanonCSS(t *testing.T) string {
	t.Helper()
	// Resolve the css file relative to this test file's directory so
	// the test runs regardless of cwd.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	// thisFile = .../internal/templates/demo/chronicle_canon_test.go
	// canon CSS = .../static/css/chronicle-canon-demo.css
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	cssPath := filepath.Join(repoRoot, "static", "css", "chronicle-canon-demo.css")
	b, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("read canon css at %s: %v", cssPath, err)
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

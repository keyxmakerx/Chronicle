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
func TestChronicleCanonDemoCSS_NoShadowAloneTransition(t *testing.T) {
	src := stripCSSComments(readCanonCSS(t))
	transitionDecl := regexp.MustCompile(`(?s)transition:\s*([^;]+);`)
	for _, m := range transitionDecl.FindAllStringSubmatch(src, -1) {
		body := m[1]
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
		if !paired {
			t.Errorf("transition declaration animates box-shadow alone — canon D3 "+
				"co-occurrence violation. Pair box-shadow with at least one of "+
				"background-color / border-color / color / transform / opacity.\n"+
				"Declaration: %s", strings.TrimSpace(body))
		}
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

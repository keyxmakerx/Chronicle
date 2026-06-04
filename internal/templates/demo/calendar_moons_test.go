// calendar_moons_test.go — C-CAL-WORLDSTATE-W2-MOON-LIBRARY (CATALOG §12.1).
//
// Static guards on the moon library: the MOON_DESIGNS registry, the vendored
// assets (Noto + Twemoji lunar sets + 12 procedural SVGs), the templ seed +
// demo controls, and the reduced-motion freeze on the one animated design.
// Phase-index / named-phase runtime behaviour is in test/js/moons.test.mjs;
// visual fidelity is the operator's local gate.

package demo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalAlmanac_MoonDesignsRegistry(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, id := range []string{
		"moon-watercolor", "moon-holographic", "moon-etched", "moon-constellation",
		"moon-realistic-selene", "moon-realistic-silver", "moon-realistic-warm",
		"moon-realistic-full", "moon-realistic-eclipse", "moon-realistic-ancient",
		"moon-realistic-icy", "moon-realistic-volcanic",
	} {
		if !strings.Contains(js, "'"+id+"'") {
			t.Errorf("MOON_DESIGNS missing procedural design: %q", id)
		}
	}
	for _, m := range []string{
		"var MOON_DESIGNS", "function applyMoonDesigns(", "function moonPhaseIndex(",
		"function moonNamedPhase(", "var EMOJI_PHASE_CODES", "window.__calMoonDesigns",
		"'noto'", "'twemoji'",
	} {
		if !strings.Contains(js, m) {
			t.Errorf("moon-library marker missing: %q", m)
		}
	}
}

// TestCalAlmanac_MoonAssetsVendored — all 26 emoji glyphs + 12 procedural SVGs
// are vendored locally (no runtime CDN).
func TestCalAlmanac_MoonAssetsVendored(t *testing.T) {
	root := calDemoRepoRoot(t)
	codes := []string{"1f311", "1f312", "1f313", "1f314", "1f315", "1f316", "1f317", "1f318", "1f319", "1f31a", "1f31b", "1f31c", "1f31d"}
	for _, c := range codes {
		for _, fam := range []string{"noto-emoji", "twemoji"} {
			p := filepath.Join(root, "static", "vendor", fam, "moons", c+".svg")
			if _, err := os.Stat(p); err != nil {
				t.Errorf("vendored emoji missing: %s", p)
			}
		}
	}
	for _, id := range []string{
		"moon-watercolor", "moon-holographic", "moon-etched", "moon-constellation",
		"moon-realistic-selene", "moon-realistic-silver", "moon-realistic-warm",
		"moon-realistic-full", "moon-realistic-eclipse", "moon-realistic-ancient",
		"moon-realistic-icy", "moon-realistic-volcanic",
	} {
		p := filepath.Join(root, "static", "vendor", "cal-moons", id+".svg")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("vendored procedural SVG missing: %s", p)
		}
	}
	// No runtime CDN reference for moon assets in the rendered page.
	html := renderAlmanac(t)
	if strings.Contains(html, "jsdelivr") || strings.Contains(html, "githubusercontent") {
		t.Errorf("moon assets must be vendored, not hot-loaded from a CDN")
	}
}

// TestCalAlmanac_MoonTemplAndControls — the moon element seeds the design mode,
// and the demo panel exposes the design picker + Randomize + Add.
func TestCalAlmanac_MoonTemplAndControls(t *testing.T) {
	html := renderAlmanac(t)
	for _, m := range []string{
		"data-cal-moon-mode", "data-cal-moon-design",
		"data-cal-democtl-moon-design", "data-cal-democtl-moon-randomize", "data-cal-democtl-moon-add",
		"data-cal-democtl-moon-tint",
	} {
		if !strings.Contains(html, m) {
			t.Errorf("moon templ/control hook missing: %q", m)
		}
	}
}

// TestCalAlmanac_MoonReducedMotion — the holographic design (the only animated
// moon) must freeze under prefers-reduced-motion.
func TestCalAlmanac_MoonReducedMotion(t *testing.T) {
	css := stripCalCSSComments(readCalAlmanacCSS(t))
	if !strings.Contains(css, "--moon-img") {
		t.Errorf("moon design image var (--moon-img) missing from CSS")
	}
	if !strings.Contains(css, `[data-cal-moon-design="moon-holographic"] { animation: none !important`) {
		t.Errorf("holographic moon must freeze (animation: none) under reduced-motion")
	}
	// Attribution present for the vendored families.
	credits, err := os.ReadFile(filepath.Join(calDemoRepoRoot(t), "CREDITS.md"))
	if err != nil {
		t.Fatalf("CREDITS.md missing: %v", err)
	}
	for _, must := range []string{"Noto Emoji", "OFL", "Twemoji", "CC-BY"} {
		if !strings.Contains(string(credits), must) {
			t.Errorf("CREDITS.md missing moon attribution marker: %q", must)
		}
	}
}

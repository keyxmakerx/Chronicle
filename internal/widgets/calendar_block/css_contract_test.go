// css_contract_test.go — the stylesheet IS the contract, so it is pinned.
//
// The render tests assert on templ output and cannot see whether the sheet
// actually defines the container queries the markup depends on. That is the gap
// #568 fell into: every DOM assertion stayed green while the stylesheet dropped
// the utilities the markup named. These read the sheet itself.
//
// PIN DISCIPLINE (COMMON §3): every assertion below flattens whitespace first
// and uses strings.Count / strings.Contains. Nothing here uses a bare
// strings.Index result as a slice bound — that PANICS on a rename instead of
// failing cleanly, which is how calendar_v2_design_pass1_test.go:147 behaves and
// why it must never be copied.
package calendar_block

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"runtime"
	"strings"
	"testing"
)

func blockCSS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "static", "css", "calendar-block.css"))
	if err != nil {
		t.Fatalf("read calendar-block.css: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("calendar-block.css is empty")
	}
	return string(body)
}

var cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// stripComments removes comment bodies so prose about "transitions" cannot trip
// a rule that forbids them.
func stripComments(css string) string { return cssCommentRe.ReplaceAllString(css, " ") }

// ── no motion ───────────────────────────────────────────────────────────────

// TestCSS_NoMotionAtAll. COMMON §6.5: no transitions inside the Block, at all,
// in wave 1. This stylesheet is the most likely place a reviewer will look for a
// violation, so it is also the place a test should look.
func TestCSS_NoMotionAtAll(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, bad := range []string{
		"transition", "animation", "@keyframes", "will-change",
		"@starting-style", "view-transition",
	} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-block.css contains %q — the Block does not move in wave 1. "+
				"The ANSWER primitive and its --m-swell rail scale ship with W-B.", bad)
		}
	}
}

// TestCSS_GuardB1_DashAndGapNeverAnimate. Morphing a dash pattern destroys the
// greyscale identity channel the signed greyscale render promises, and it is the
// obvious "improvement" the next hand reaches for. Kept as its own test so it
// survives W-B reintroducing motion for everything else.
func TestCSS_GuardB1_DashAndGapNeverAnimate(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, decl := range []string{"transition", "animation"} {
		for _, line := range strings.Split(code, "\n") {
			l := strings.ToLower(line)
			if strings.Contains(l, decl) && (strings.Contains(l, "--dash") || strings.Contains(l, "--gap")) {
				t.Errorf("canon guard B1: %q must never appear in a %s — %s", "--dash/--gap", decl, strings.TrimSpace(line))
			}
		}
	}
}

// TestCSS_GuardB2_NoFontShorthand. The `font:` shorthand with `inherit` as the
// family is invalid CSS. It silently dropped EIGHTY-ONE declarations in the v4
// mockup and nearly all type fell back to inherited 14px, with nothing failing
// loudly. This file uses longhands only.
func TestCSS_GuardB2_NoFontShorthand(t *testing.T) {
	code := stripComments(blockCSS(t))
	shorthand := regexp.MustCompile(`(^|[;{\s])font\s*:`)
	if loc := shorthand.FindStringIndex(code); loc != nil {
		t.Errorf("canon guard B2: the `font:` shorthand appears near %q — use longhands",
			strings.TrimSpace(code[loc[0]:min(loc[0]+80, len(code))]))
	}
}

// ── the cascade regime ──────────────────────────────────────────────────────

// TestCSS_UnlayeredAndSelfContained. input.css's content is layered; an
// unlayered standalone sheet beats every layer, which is exactly why the two
// cascade regimes never fight. Adding @layer here would put this sheet BACK into
// the fight it was written to avoid.
func TestCSS_UnlayeredAndSelfContained(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, bad := range []string{"@layer", "@apply", "@tailwind", "theme(", "@import"} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-block.css contains %q — the sheet is unlayered and self-contained "+
				"per the rendering-canvas CSS exemption", bad)
		}
	}
	// The exemption marker must stay, so the Wave-6 customization-readiness
	// sweep skips this file knowingly rather than treating it as debt.
	raw := blockCSS(t)
	for _, want := range []string{"EXEMPTION MARKER", "rendering-canvas-css-exemption"} {
		if !strings.Contains(raw, want) {
			t.Errorf("the exemption marker fragment %q is missing from the header comment", want)
		}
	}
}

var (
	// A rule prelude is everything between the previous }/;/{ and the next {.
	preludeRe = regexp.MustCompile(`(?s)(^|[;{}])([^;{}]*)\{`)
)

// TestCSS_EverySelectorIsScoped. An unlayered sheet outranks the app's layered
// CSS, so a bare `.badge` rule in here would silently restyle the whole product.
// Nothing may match outside a .cal-block-host subtree.
func TestCSS_EverySelectorIsScoped(t *testing.T) {
	code := stripComments(blockCSS(t))
	var offenders []string
	for _, m := range preludeRe.FindAllStringSubmatch(code, -1) {
		prelude := strings.TrimSpace(m[2])
		if prelude == "" || strings.HasPrefix(prelude, "@") {
			continue // at-rules carry their own nested preludes, matched separately
		}
		for _, sel := range strings.Split(prelude, ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			if !strings.Contains(sel, ".cal-block-host") && !strings.Contains(sel, ".dark .cal-block-host") {
				offenders = append(offenders, sel)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("unscoped selector(s) would leak out of the Block and outrank the app's "+
			"layered CSS:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// ── ten-day weeks are native ────────────────────────────────────────────────

// TestCSS_NoLiteralWeekLength. --week-len drives every column count. calv3
// hardcoded repeat(7,1fr) and that is the very week length this brief exists to
// escape.
func TestCSS_NoLiteralWeekLength(t *testing.T) {
	code := stripComments(blockCSS(t))
	if m := regexp.MustCompile(`repeat\(\s*\d`).FindString(code); m != "" {
		t.Errorf("a literal column count reached the grid: %q — every repeat() must take var(--week-len)", m)
	}
	gtc := regexp.MustCompile(`grid-template-columns\s*:\s*([^;}]+)`)
	found := 0
	for _, m := range gtc.FindAllStringSubmatch(code, -1) {
		val := strings.TrimSpace(m[1])
		found++
		if strings.Contains(val, "subgrid") {
			continue // rows inherit the month's tracks; that is the point of subgrid
		}
		if strings.Contains(val, "var(--week-len)") {
			continue
		}
		if strings.Contains(val, "minmax(0, 1fr) auto") {
			continue // the full-tier body: instrument + docked Ledger, not a week
		}
		t.Errorf("grid-template-columns:%s is neither a subgrid, a --week-len repeat, nor the "+
			"body split", val)
	}
	if found < 3 {
		t.Errorf("only %d grid-template-columns declarations found; expected at least the grid, "+
			"the row subgrid and the full-tier body", found)
	}
	if !strings.Contains(code, "calc(var(--week-len) * 30px)") {
		t.Error("the grid's min-width must scale with --week-len, not with a literal")
	}
}

// TestCSS_BandHalfDrawsNoRightEdge. EraBand.Half means the half column lands
// INSIDE this band (r51, data.go) — but a band is one grid item spanning many
// columns, so its own border-right lands at the band's edge, wherever that
// happens to be, never reliably at the half column. Only the dedicated
// .halfrule ruler may draw the five-column rule across the band row (P5 §3.5);
// the half class on a band is a semantic marker and draws nothing.
func TestCSS_BandHalfDrawsNoRightEdge(t *testing.T) {
	code := stripComments(blockCSS(t))
	blockRe := regexp.MustCompile(`(?s)([^;{}]*)\{([^}]*)\}`)
	// The border shorthand sets all four sides, so it would smuggle the right
	// edge back in without the literal "border-right" ever appearing.
	borderShorthand := regexp.MustCompile(`(^|[;\s])border\s*:`)
	sawRuler := false
	for _, m := range blockRe.FindAllStringSubmatch(code, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		if strings.Contains(sel, ".halfrule") {
			// The ruler element is the ONE place the rule may be drawn — and
			// must keep being drawn, or deleting it becomes the next "fix".
			if strings.Contains(body, "border-right: 1px solid var(--rule-structural-strong)") {
				sawRuler = true
			}
			continue
		}
		if strings.Contains(sel, ".band") && strings.Contains(sel, ".half") &&
			(strings.Contains(body, "border-right") || borderShorthand.MatchString(body)) {
			t.Errorf("%q draws a border on the band's own edge — the half class marks the band, "+
				"it draws nothing; the .halfrule ruler owns the rule at the half column", sel)
		}
	}
	if !sawRuler {
		t.Error("the .halfrule ruler lost its structural border-right — it is the only element " +
			"that draws the five-column rule across the band row")
	}
}

// ── sizing is CSS, and it is these numbers ──────────────────────────────────

// TestCSS_SizingIsContainerQueries pins the four size-class thresholds and the
// density threshold to the same numbers sizing.go tests. A JS implementation is
// broken by construction on this platform (htmx.config.allowScriptTags = false,
// boot.js:163), so if these ever move the whole sizing story moves with them.
func TestCSS_SizingIsContainerQueries(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, want := range []string{
		"container-name: cal-block",
		"container-name: cal-cell",
		"@container cal-block (min-width: 900px)",                          // full
		"@container cal-block (min-width: 240px) and (max-width: 299.98px)", // mini
		"@container cal-block (max-width: 239.98px)",                        // sub-mini
		"@container cal-cell (min-width: 84px)",                             // density
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the stylesheet is missing %q", want)
		}
	}
	// std is the BASELINE rather than a query, so a Block rendered before its
	// container is measured degrades to the middle class, not the widest.
	if strings.Contains(code, "@container cal-block (min-width: 300px)") {
		t.Error("std must be the unqueried baseline; a min-width:300px query would make " +
			"the pre-measurement state the mini one")
	}
	// The declared heights are invariant, so dropping a Block into any host page
	// can never shove that page around.
	for _, want := range []string{
		"min-height: 200px", "max-height: 520px", // std baseline
		"min-height: 220px", "max-height: 780px", // full
		"height: 180px", // mini
	} {
		if !strings.Contains(code, want) {
			t.Errorf("a declared height is missing: %q", want)
		}
	}
	// The docked Ledger's width IS the 300px the full-tier arithmetic subtracts.
	if !strings.Contains(code, "width: 300px") {
		t.Errorf("the docked Ledger must be %dpx wide — the same %dpx ColWidth subtracts",
			ledgerDock, ledgerDock)
	}
}

// ── canon D3 ────────────────────────────────────────────────────────────────

// TestCSS_D3_RingIsCarriedInsideBoxShadow. The border-thickening is a 0.5px ring
// carried INSIDE box-shadow, so a shadow animating alone is structurally
// impossible. That is the "shadows from nowhere" rejection, fixed at the
// primitive rather than per component.
func TestCSS_D3_RingIsCarriedInsideBoxShadow(t *testing.T) {
	code := stripComments(blockCSS(t))
	shadowRe := regexp.MustCompile(`box-shadow\s*:\s*([^;}]+)`)
	sawRing := false
	for _, m := range shadowRe.FindAllStringSubmatch(code, -1) {
		val := strings.TrimSpace(m[1])
		if strings.Contains(val, "var(--elev-1)") {
			if !strings.Contains(val, "0 0 0 .5px") {
				t.Errorf("box-shadow:%s raises elevation without carrying the 0.5px ring — "+
					"that is a shadow from nowhere", val)
			}
			sawRing = true
		}
	}
	if !sawRing {
		t.Error("the D3 hover primitive is missing: no box-shadow pairs --elev-1 with the ring")
	}
	// Rest is ALWAYS none, and selection is identity rather than elevation.
	flat := regexp.MustCompile(`\s+`).ReplaceAllString(code, " ")
	if !strings.Contains(flat, ".surf { background: var(--surface-card); border: 1px solid var(--rule-structural); box-shadow: none;") {
		t.Error(".surf must rest at box-shadow:none")
	}
	if !strings.Contains(flat, ".surf.act:active { background: color-mix(in oklch, var(--surface-card) 88%, var(--text-primary)); box-shadow: none;") {
		t.Error("press must RETURN the shadow to none — elevation decreases under the finger")
	}
	if !strings.Contains(flat, ".surf.sel:hover { border-color: var(--accent); background: color-mix(in oklch, var(--surface-card) 96%, var(--accent)); box-shadow: inset 0 0 0 1.5px var(--accent);") {
		t.Error("selection must be an inset ring with NO outer shadow — identity, not elevation")
	}
}

// ── colour is never load-bearing ────────────────────────────────────────────

// TestCSS_EightLockedPatterns. Every one of the eight must resolve with the hue
// removed; p7 and p8 are unassigned headroom and must still be defined, or the
// next axis to need one silently renders as p1.
func TestCSS_EightLockedPatterns(t *testing.T) {
	code := stripComments(blockCSS(t))
	// p1 is the bare `background-color: var(--axis)` case on .rail/.ulseg.
	if !strings.Contains(code, ".ulseg {\n  --dash: 100%;\n  --gap: 0px;\n  background-color: var(--axis);") &&
		!strings.Contains(regexp.MustCompile(`\s+`).ReplaceAllString(code, " "),
			".ulseg { --dash: 100%; --gap: 0px; background-color: var(--axis);") {
		t.Error("p1 (solid) is the unclassed baseline on .rail/.ulseg and must set --dash/--gap")
	}
	for _, p := range []string{"p2", "p3", "p4", "p5", "p6", "p7", "p8"} {
		if !strings.Contains(code, "."+p) {
			t.Errorf("locked pattern %s is not defined — colour would become the only channel for it", p)
		}
	}
	// Each dashed pattern owns a --dash and a --gap.
	for _, p := range []string{"p2", "p3", "p4", "p6", "p8"} {
		re := regexp.MustCompile(`\.` + p + `\s*\{[^}]*--dash[^}]*--gap[^}]*\}`)
		if !re.MatchString(code) {
			t.Errorf("%s must declare BOTH --dash and --gap", p)
		}
	}
	// p5 and p7 split the cross axis / notch the main axis rather than dashing.
	for _, want := range []string{".rail.p5", ".ulseg.p5", ".rail.p7", ".ulseg.p7"} {
		if !strings.Contains(code, want) {
			t.Errorf("%s must be defined on both mark orientations", want)
		}
	}
}

// TestCSS_ChannelDiscipline. --axis is the ONLY channel the marks layer may
// reference and it is FORBIDDEN from referencing --accent (canon A7). The moment
// accent means "related to what you're pointing at" it stops meaning "selected".
func TestCSS_ChannelDiscipline(t *testing.T) {
	code := stripComments(blockCSS(t))
	blockRe := regexp.MustCompile(`(?s)([^;{}]*)\{([^}]*)\}`)
	markSelectors := []string{".rail", ".ulseg", ".chip", ".tick i", ".ph"}
	for _, m := range blockRe.FindAllStringSubmatch(code, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		isMark := false
		for _, ms := range markSelectors {
			if strings.Contains(sel, ms) {
				isMark = true
				break
			}
		}
		if isMark && strings.Contains(body, "var(--accent)") {
			t.Errorf("the marks layer references --accent in %q — forbidden by canon A7", sel)
		}
	}
	// …and none of the three inherited colour channels is @property-registered
	// as animatable. That is a recorded REFUSAL, not an omission.
	if strings.Contains(code, "@property") {
		t.Error("canon A7 is a REFUSAL: --axis / --cal / --bandhue are never @property-registered " +
			"as animatable colours. Interpolating one is a per-frame style recalculation across " +
			"up to 900 gradient-backed nodes.")
	}
}

// ── the tie toggle's flip (C-CALV4-HOST-P3 §3) ──────────────────────────────

// TestCSS_TieToggleFlipsInkWithoutJS. The toggle's whole legality rests on the
// stylesheet: the markup tests can see that two radios exist, but only this can
// see whether anything reads them. Without the `:has()` rules the control is
// present, focusable, and does nothing — which is worse than absent.
//
// It also pins the TWO ink levels. The signed setTie() inks an untied mark at
// 0.28 in tied mode and at 0.7 in whole mode (calendar-v4.html:2823-2824);
// before this slice the sheet carried one branch at one level, so tied mode
// under-dimmed and whole mode did not dim at all.
func TestCSS_TieToggleFlipsInkWithoutJS(t *testing.T) {
	code := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(code), " ")

	for _, want := range []string{
		`:has(.tiepick[data-tie-pick="tied"]:checked)`,
		`:has(.tiepick[data-tie-pick="whole"]:checked)`,
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("no rule reads the tie radios (%s missing) — the control would be inert, "+
				"and it cannot be rescued with JS: a <script> in an HTMX-swapped fragment never runs", want)
		}
	}
	// The mark classes the flip re-inks must both be covered — a day carrying one
	// named chip and one underline segment must dim consistently.
	for _, want := range []string{".chip.untied", ".ulseg.untied"} {
		if strings.Count(flat, want) < 2 {
			t.Errorf("%s must be re-inked by BOTH the baseline and the :has() flip", want)
		}
	}
	// The server-rendered baseline survives for engines without :has().
	if !strings.Contains(flat, `[data-tie-mode="whole"]`) || !strings.Contains(flat, `[data-tie-mode="tied"]`) {
		t.Error("both data-tie-mode baselines must exist — they are the answer on an engine without :has()")
	}

	// Two levels, ordered. Tied mode must dim HARDER than whole mode, and whole
	// mode must still dim: an untied mark at full ink makes "which of these are
	// actually this entity's" unanswerable in whole mode.
	tied, whole := cssVarNumber(t, code, "--tied-ink"), cssVarNumber(t, code, "--untied-ink")
	if !(tied < whole) {
		t.Errorf("--tied-ink (%v) must be lower than --untied-ink (%v)", tied, whole)
	}
	if !(whole < 1) {
		t.Errorf("--untied-ink (%v) must dim: whole mode still distinguishes the entity's own days", whole)
	}
	if !(tied > 0) {
		t.Errorf("--tied-ink (%v) must stay visible: the toggle changes ink, never membership", tied)
	}
}

// cssVarNumber reads a numeric custom-property value out of the sheet. Fails
// cleanly when the property is gone, rather than panicking on a slice bound —
// see the PIN DISCIPLINE note at the top of this file.
func cssVarNumber(t *testing.T, code, name string) float64 {
	t.Helper()
	re := regexp.MustCompile(regexp.QuoteMeta(name) + `\s*:\s*([0-9.]+)\s*;`)
	m := re.FindStringSubmatch(code)
	if m == nil {
		t.Fatalf("calendar-block.css does not define %s", name)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%s is not a number: %q", name, m[1])
	}
	return v
}

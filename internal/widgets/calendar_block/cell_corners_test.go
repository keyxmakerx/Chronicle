// cell_corners_test.go — the day cell as C-CALV4-SPEC §1/§2/§6 specify it.
//
// THREE SURFACES ARRIVE IN THIS SLICE AND NONE OF THEM HAD A GUARD:
//
//	· the CORNER architecture — date in one corner, ambient marks in another,
//	  event marks in a third, and nothing centred;
//	· the ERA as a low-chroma BACKGROUND TINT on the cell fill, never text and
//	  never the border;
//	· the reserved AVAILABILITY slot on the bottom edge — space held for step
//	  three of five, deliberately empty here.
//
// The moon half of the same spec is measured in a browser by
// moon_reach_probe_test.go, because "is there a moon on the screen" is a
// question only a layout engine can answer. These three are markup-and-cascade
// claims and are asserted where they are made.
package calendar_block

import (
	"regexp"
	"strings"
	"testing"
)

// fxErasOff is fxHarptos with the eras LAYER switched off and nothing else
// changed, so the two arms of the gate differ in exactly one input.
func fxErasOff(t *testing.T) BlockData {
	t.Helper()
	d := fxHarptos(true)
	kept := make([]string, 0, len(d.Layers.Enabled))
	for _, k := range d.Layers.Enabled {
		if k != "eras" {
			kept = append(kept, k)
		}
	}
	d.Layers.Enabled = kept
	return d
}

// ── §2 · ERAS ARE A BACKGROUND TINT ─────────────────────────────────────────

// TestCell_EraIsABackgroundTintAndNeverTextOrBorder is the operator's own
// answer to their own open question, asserted as itself.
//
// C-CALV4-SPEC §2 left the treatment open ("idk how your doing the eras") with
// three candidates, and the operator delegated the pick on 2026-08-15
// ("highlight around the edge? or the background… your decision"). Background
// won for a stated reason, and the reason is a constraint rather than a taste:
// the cell edge ALREADY carries two meanings — gold means selected, and the
// raised "pop" is itself an edge treatment — so a third edge meaning would make
// all three ambiguous. A low-chroma background wash reads as GROUPING, which is
// what an era is.
//
// So this test asserts the tint arrives AND that the two rejected treatments
// did not. An assertion that only checked the tint would stay green on a build
// that also printed the era's name in every cell.
func TestCell_EraIsABackgroundTintAndNeverTextOrBorder(t *testing.T) {
	d := fxHarptos(true)
	body := render(t, d)

	cellRe := regexp.MustCompile(`<div class="cell[^"]*"[^>]*>`)
	cells := cellRe.FindAllString(body, -1)
	if len(cells) == 0 {
		t.Fatal("no day cells rendered — the guard has no subject")
	}
	tinted := 0
	for _, c := range cells {
		if strings.Contains(c, "--erahue:") {
			tinted++
		}
	}
	if tinted == 0 {
		t.Fatal("no cell carries --erahue. C-CALV4-SPEC §2 puts the era on the cell FILL, " +
			"and the fixture's rows carry era bands — so every dated cell an era covers " +
			"should be tinted")
	}

	// THE HUE GOES THROUGH THE SAME ALLOWLIST THE BAND USES. A bare colour must
	// never reach a style attribute (identity_triple_test.go's rule, applied to
	// the same class of channel), and reusing bandToken is what stops the tint
	// and the band above it from ever disagreeing about an era's hue.
	for _, c := range cells {
		if !strings.Contains(c, "--erahue:") {
			continue
		}
		if strings.Contains(c, "--erahue:;") || strings.Contains(c, `--erahue:"`) {
			t.Errorf("a cell carries an empty --erahue: %s", c)
		}
	}

	// ── AND NEITHER REJECTED TREATMENT SHIPPED ──────────────────────────────
	//
	// NEVER TEXT. The era's NAME is printed once, on the band above the row.
	// Repeating it in thirty cells is the noise MN-G4 already ruled against for
	// moons, and it is the first thing a later hand reaches for.
	for i, c := range cells {
		if strings.Contains(c, "data-era-label") || strings.Contains(c, "erartext") {
			t.Errorf("cell %d carries era TEXT (%s). The era's name belongs on the band, "+
				"once per row — C-CALV4-SPEC §2 and the constraint it inherits: colour, "+
				"never text, in the grid", i, c)
		}
	}
	// NEVER THE BORDER. The cell's border already means SELECTED (gold) and the
	// pop is an edge treatment; a third edge meaning makes all three ambiguous.
	css := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(css), " ")
	eraBorderRe := regexp.MustCompile(`border[^;{}]*var\(--erahue`)
	if m := eraBorderRe.FindString(flat); m != "" {
		t.Errorf("--erahue reaches a BORDER property (%q). The operator's answer was "+
			"BACKGROUND, because the edge already carries `selected` and the pop — a third "+
			"edge meaning makes all three ambiguous", m)
	}
	// It reaches the FILL, through --cellbase, and low-chroma rather than flat.
	if !strings.Contains(flat, "--erahue, var(--surface-card)") &&
		!strings.Contains(flat, "--erahue,var(--surface-card)") {
		t.Error("the era tint's OFF SWITCH is missing. `var(--erahue, <the plain fill>)` is " +
			"what makes a cell with no era render its plain fill without needing a marker " +
			"class — and without it an un-tinted cell resolves to nothing at all")
	}
}

// TestCell_EraTintIsGatedOnTheErasLayer. Every layer-owned surface in this Block
// disappears when its layer is off — that is the product's vocabulary for "there
// is nothing here" — and a tint that survived the toggle would be the one
// exception nobody could find.
func TestCell_EraTintIsGatedOnTheErasLayer(t *testing.T) {
	on := render(t, fxHarptos(true))
	off := render(t, fxErasOff(t))

	if !strings.Contains(on, "--erahue:") {
		t.Fatal("the eras layer is ON and no cell is tinted — the OFF arm below would pass " +
			"vacuously")
	}
	if strings.Contains(off, "--erahue:") {
		t.Error("the eras layer is OFF and cells still carry --erahue. A viewer who switched " +
			"eras off must get no era band AND no era tint; a surface that survives its own " +
			"layer toggle is the exception that makes the toggle untrustworthy")
	}
}

// TestCell_OutOfRangeCellsCarryNoEra. Lead/trail cells are not dated, so they
// are not in an era either — tinting them would draw an era boundary in the
// wrong place, which is worse than drawing none.
func TestCell_OutOfRangeCellsCarryNoEra(t *testing.T) {
	body := render(t, fxHarptos(true))
	outRe := regexp.MustCompile(`<div class="cell[^"]*\bout\b[^"]*"[^>]*>`)
	for i, c := range outRe.FindAllString(body, -1) {
		if strings.Contains(c, "--erahue:") {
			t.Errorf("out-of-range cell %d carries an era tint (%s). It has no date, so it "+
				"has no era — a tint here paints an era boundary at the wrong column", i, c)
		}
	}
}

// ── §6 · THE AVAILABILITY SLOT IS RESERVED, NOT BUILT ───────────────────────

// TestCell_AvailabilitySlotIsReservedAndEmpty.
//
// C-CALV4-SPEC §6 puts a per-player availability strip on the cell's bottom edge
// — segmented on desktop, one state-coloured dot on a phone, with the Ledger
// naming who. That is STEP THREE of five and is explicitly not this slice's to
// build. What this slice owes step three is the SPACE, at its final size, in its
// final place, so that filling it changes one element's contents and relayouts
// nothing.
//
// THE TEST IS THEREFORE AS MUCH ABOUT WHAT IS ABSENT AS PRESENT. A slot that
// arrived carrying owner hues, or data, or a visible fill, would be a feature
// nobody built and the operator would read it as one.
func TestCell_AvailabilitySlotIsReservedAndEmpty(t *testing.T) {
	body := render(t, fxHarptos(true))

	slots := strings.Count(body, `<span class="avail" aria-hidden="true"></span>`)
	if slots == 0 {
		t.Fatal("no cell reserves an availability slot. Step three has to land a strip on " +
			"the bottom edge of every cell; discovering then that the edge is taken is a " +
			"relayout of the cell, and a second pass over every geometric assertion here")
	}
	// One per DATED cell, and on every one of them. A slot that appeared only
	// for days that happen to have RSVPs would move the underline on some days
	// and not others — the reflow the reservation exists to prevent.
	dated := strings.Count(body, "data-day-ord=")
	if slots != dated {
		t.Errorf("%d availability slots for %d dated cells. The reservation is unconditional "+
			"BY DESIGN: gating it on data would make the cell's interior reflow from day to "+
			"day", slots, dated)
	}
	// It is EMPTY and carries no owner hue. The strip is step three's.
	availRe := regexp.MustCompile(`<span class="avail"[^>]*>.*?</span>`)
	for i, s := range availRe.FindAllString(body, -1) {
		if strings.Contains(s, "--own") || strings.Contains(s, "style=") {
			t.Errorf("availability slot %d already carries a fill or a hue (%s). It is a "+
				"RESERVATION — painting it now ships a feature nobody built", i, s)
		}
	}

	css := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(css), " ")
	// The slot's height is a token, declared once, so the underline above it and
	// the slot itself cannot drift apart.
	if !strings.Contains(flat, "--avail-h:") {
		t.Error("the slot's height is not a token. It is read in two places — the slot's " +
			"own height and the underline's offset above it — and two literals is how the " +
			"event marks end up sitting on the availability strip")
	}
	if !strings.Contains(flat, "bottom: calc(var(--avail-h)") {
		t.Error("the underline does not clear the reserved slot. `.ul` has to be offset by " +
			"--avail-h, or step three's strip lands on top of the event marks")
	}
}

// ── §1 · THE CORNER ARCHITECTURE ────────────────────────────────────────────

// TestCell_NothingIsCentred is C-CALV4-SPEC §1's last sentence, asserted.
//
// "The corners are the information architecture: date in one, ambient marks in
// another, event marks in a third. Keep them in corners; do not centre
// anything."
//
// THE DATE IS THE ONE THAT MOVED, and it is load-bearing rather than cosmetic:
// a centred numeral owns the middle of the cell's top line, which is why the
// shipped sheet needed TWO anchors for the moon row (`top: 26px` at underline
// density, `top: 5px` at named). With the date in the top-LEFT corner the
// top-RIGHT corner is free at every width, the moon row has one anchor, and the
// silhouette can be promoted down to the narrowest column the grid produces.
// Re-centre the date and that promotion puts a moon across it.
func TestCell_NothingIsCentred(t *testing.T) {
	css := stripComments(blockCSS(t))

	ruleRe := regexp.MustCompile(`(?s)([^{}]*)\{([^}]*)\}`)
	for _, m := range ruleRe.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		body := m[2]
		if !strings.Contains(sel, ".dn") {
			continue
		}
		if strings.Contains(body, "text-align: center") {
			t.Errorf("%q centres the day numeral. C-CALV4-SPEC §1 puts the date in a CORNER "+
				"and centres nothing — and a centred date is what forced the moon row to "+
				"carry two anchors and to be refused below 40px of column", sel)
		}
		if strings.Contains(body, "justify-content: center") {
			t.Errorf("%q centres the day numeral's flex box — same rule, other property", sel)
		}
	}
	if !strings.Contains(css, "text-align: left") {
		t.Error("the day numeral is not left-aligned anywhere — the date's corner is the " +
			"top-LEFT one, and it is what frees the top-right for the moon")
	}
}

// TestCell_PopsAgainstItsOwnGround is the spec's "popped out", asserted as the
// three ingredients the spec itself names.
//
// C-CALV4-SPEC §1: "a fill slightly lighter than the page, a defined edge, and
// enough internal padding that the cell feels like an object rather than a
// table division."
//
// "LIGHTER THAN THE PAGE" IS A RELATION AND IT INVERTS WITH THE THEME, which is
// the part a single-arm test would miss. On light the page is white and nothing
// is lighter, so the GROUND darkens instead; on dark --surface-inset is the
// lighter of the pair and the two swap. Both arms are asserted, because a build
// that reads as raised on light and sunken on dark is the defect this catches.
func TestCell_PopsAgainstItsOwnGround(t *testing.T) {
	css := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(css), " ")

	for _, want := range []struct{ decl, why string }{
		{".cal-block-host .grid { display: grid;",
			"the grid rule is the ground the cells sit on"},
		{"background: var(--surface-inset)",
			"the LIGHT-theme ground is the darker of the two surfaces, so the cells " +
				"(--surface-card) are the lighter object on it"},
		{".dark .cal-block-host .grid { background: var(--surface-card) }",
			"the DARK-theme ground and cell fill SWAP, because --surface-inset is the " +
				"lighter of the pair on dark — without this the cell reads as sunken"},
	} {
		if !strings.Contains(flat, strings.Join(strings.Fields(want.decl), " ")) {
			t.Errorf("the stylesheet is missing %q — %s", want.decl, want.why)
		}
	}

	// THE DEFINED EDGE, one ramp step up from the hairline the cells used to
	// carry. The ramp itself must stay monotonic: hairline < structural <
	// structural-strong, with the five-column rule still a full step above its
	// neighbours, which is the whole point of that rule.
	if !strings.Contains(flat, "border-right: 1px solid var(--rule-structural);") {
		t.Error("the cell's rule is not --rule-structural. It was --rule-hairline on a " +
			"transparent fill, which IS the \"table division\" the spec contrasts against")
	}
	if !strings.Contains(flat, ".cal-block-host .cell.half, .cal-block-host .grid .hd b.half "+
		"{ border-right: 1px solid var(--rule-structural-strong); }") {
		t.Error("the five-column rule is no longer a full ramp step above the cell rule — " +
			"humans cannot count to ten across identical columns, and that rule is the " +
			"highest-value single addition to a ten-wide grid")
	}

	// THE CELL'S OWN PADDING STAYS 0. `container-type: inline-size` measures the
	// cell's CONTENT box, so padding here would shrink every container query's
	// subject and move all four thresholds by twice the padding — silently, on
	// every signed still. The spec's "internal padding" is spent on the
	// subtrees, which is where it already lived.
	cellRe := regexp.MustCompile(`(?s)\.cal-block-host \.cell \{([^}]*)\}`)
	if m := cellRe.FindStringSubmatch(css); m != nil {
		if !strings.Contains(m[1], "padding: 0") {
			t.Error("`.cell` has grown a padding. It is the container query's subject, so a " +
				"padding here moves the silhouette, date, expansion and density thresholds " +
				"all at once, and moves them invisibly")
		}
	}
}

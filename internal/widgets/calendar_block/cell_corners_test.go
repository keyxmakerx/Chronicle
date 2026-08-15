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
			"and the fixture's rows carry era data — so every dated cell an era covers " +
			"should be tinted")
	}

	// THE HUE GOES THROUGH bandToken's ALLOWLIST. A bare colour must never reach
	// a style attribute (identity_triple_test.go's rule, applied to the same
	// class of channel). The allowlist outlived the band row it was named for
	// (C-CALV4-TILES §3): the tint is the era's only surface now, so the token
	// gate is the only thing between a producer's string and a `style`.
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
	// NEVER TEXT. Since C-CALV4-TILES §3 the era's name is not in the grid AT
	// ALL — the caption row that used to print it once per week is deleted, and
	// the Nameplate's era badge is where the name lives. Repeating it in thirty
	// cells was always the noise MN-G4 ruled against for moons; with no band row
	// left to point at, a cell that grew era text would be the only place the
	// grid names an era, which is worse than the repetition ever was.
	for i, c := range cells {
		if strings.Contains(c, "data-era-label") || strings.Contains(c, "erartext") {
			t.Errorf("cell %d carries era TEXT (%s). The era's name belongs on the "+
				"Nameplate's era badge and nowhere in the month — C-CALV4-SPEC §2 and the "+
				"constraint it inherits: colour, never text, in the grid", i, c)
		}
	}
	// THE ERA IS NEVER AN EDGE *MEANING*, and that claim is narrower than it
	// used to be — narrowed on purpose rather than quietly weakened.
	//
	// WHAT IT MEANT BEFORE: no border may read --erahue at all, because the
	// edge already carries SELECTED and the pop, and a third edge meaning makes
	// all three ambiguous.
	//
	// WHAT IT MEANS NOW: no border may read --erahue DIRECTLY. Since the tile's
	// edge became --tile-rule — one pop step off --cellbase — a tinted tile's
	// edge does carry the era's hue, and that is deliberate: it is what makes a
	// tinted tile read as ONE soft object instead of a coloured field inside a
	// grey box. The hue arrives there as a property of the SURFACE, at the
	// surface's own lightness and pinned chroma, which is not an edge meaning;
	// `border-color: var(--erahue)` would be, and is still refused. The two are
	// distinguishable in exactly the way the regex below distinguishes them.
	css := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(css), " ")
	eraBorderRe := regexp.MustCompile(`border[^;{}]*var\(--erahue`)
	if m := eraBorderRe.FindString(flat); m != "" {
		t.Errorf("--erahue reaches a BORDER property DIRECTLY (%q). The operator's answer "+
			"was BACKGROUND: the edge already carries `selected` and the pop, and an era "+
			"drawn as an edge in its own right makes all three ambiguous. The era may reach "+
			"the tile's edge only the way it reaches the tile's fill — through --cellbase, "+
			"at the surface's lightness and the pinned soft chroma", m)
	}
	// It reaches the FILL, through --cellbase, and it has an OFF SWITCH that
	// needs no marker class: a cell with no --erahue makes --eratint
	// guaranteed-invalid, and every reader falls back to the plain fill.
	//
	// WHETHER THE OFF SWITCH WORKS is measured in cell_probe_test.go, on the
	// intercalary day — the one day surface no era covers — by reading its fill
	// back out of the layout. What is asserted here is only that the fallback
	// is WRITTEN, because a fallback nobody wrote resolves an un-tinted cell to
	// nothing at all, and "nothing" is a transparent cell that looks plausible.
	if !strings.Contains(flat, "var(--eratint, var(--surface-card))") &&
		!strings.Contains(flat, "var(--eratint,var(--surface-card))") {
		t.Error("the era tint's OFF SWITCH is missing. `var(--eratint, <the plain fill>)` is " +
			"what makes a cell with no era render its plain fill without needing a marker " +
			"class — and without it an un-tinted cell resolves to nothing at all")
	}
	// AND IT IS A HUE HELD AT THE SURFACE'S LIGHTNESS, not a mix towards the
	// hue. An oklch color-mix interpolates the HUE ANGLE from --surface-card's
	// hue 0, which is how a teal era rendered pink and two eras rendered 2/255
	// apart; it also drags lightness 8% towards an L≈0.55 ink, which is how the
	// tint cancelled the pop on light. Both are measured in cell_probe_test.go;
	// this refuses the specific construction that produced them.
	if regexp.MustCompile(`color-mix\([^)]*var\(--erahue`).MatchString(flat) {
		t.Error("--erahue is being color-mixed into the fill again. `color-mix(in oklch, …)` " +
			"interpolates the hue ANGLE and the LIGHTNESS: from a hue-0 white it rotates a few " +
			"degrees off white instead of adopting the era's hue, and it spends the whole " +
			"12-unit budget between the grid ground and the cell fill. Take the era's `h` and " +
			"pin `l` and `c` — see the note beside the rule")
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
			"eras off must get no era tint at all; a surface that survives its own " +
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
//
// TestCell_AvailabilitySlotIsReservedAndEmpty USED TO LIVE HERE AND IS DELETED,
// NOT MOVED — replaced by a measurement in cell_probe_test.go, which is a
// different test rather than the same one relocated.
//
// WHY IT HAD TO GO. It proved the reservation by counting
// `<span class="avail" aria-hidden="true"></span>` in the markup this package
// had just emitted, and proved that the event marks cleared it with
// `strings.Contains(flat, "bottom: calc(var(--avail-h)")`. Both questions are
// "does this file agree with itself", and both were answered yes while the
// event-mark strip above the slot was 0.00px wide and painted nothing at all.
// It was green on its first outing over a live regression — which is exactly
// the pattern the commit that wrote it had correctly diagnosed one paragraph
// earlier, in `wantRow := r.ColumnW >= MoonRowColWidthMin`.
//
// WHAT REPLACES IT asks a browser: does the slot have a rect at the cell's
// bottom edge, is it really empty, does the strip above it have WIDTH, are its
// segments INKED, and is the gap between them non-negative. Those are the
// claims; none of them can be answered by the markup or the stylesheet, and all
// four are answered per theme and per density.
//
// The slot's markup contract — one per dated cell, empty, no owner hue — is
// still asserted, in instrument_test's own rendering tests and by the probe's
// `availCells == datedCells` and `availInked == 0` arms. What is gone is the
// idea that counting our own spans was ever evidence.

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
//
// ── RE-POINTED AT THE TILE (C-CALV4-TILES §1) ──────────────────────────────
// This test used to pin two literal `border-right` strings, which is exactly
// what the ruled treatment was. The treatment is now a TILE — a rounded fill
// drawn inside the cell box with 1.5px of ground showing on every side — and
// the pins move with it: the pseudo-element, its inset, its radius, its rule,
// and the two things that make the ground SHOW.
//
// THE ONE PIN THAT MATTERS MOST IS AN ABSENCE. `.cell` must carry no background
// and no all-round border of its own. Either one closes the 3px gutter between
// two tiles — silently, because the cell would still look like a cell — and
// takes the micro-separation the operator asked for with it. The gutter is the
// whole point; a tile with no gap around it is just a rounded table division.
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

	// ── THE TILE IS A PSEUDO-ELEMENT, AND IT IS DRAWN INSIDE THE CELL ───────
	tileRe := regexp.MustCompile(`(?s)\.cal-block-host \.cell::before,\s*\.cal-block-host \.interc::before \{([^}]*)\}`)
	m := tileRe.FindStringSubmatch(css)
	if m == nil {
		t.Fatal("there is no `.cell::before, .interc::before` tile rule. The day cell's " +
			"separation is drawn INSIDE its own box precisely because .cell is the " +
			"container-query subject: a gap or a margin on the grid item costs the track " +
			"a pixel, and at a 366px host with a ten-day week that takes the column from " +
			"34.0px to 29.0px — under MoonSilhouetteColWidthMin, which turns the " +
			"always-visible moon back off on the operator's own phone")
	}
	tile := m[1]
	for _, want := range []struct{ decl, why string }{
		{"position: absolute",
			"the tile is out of flow, so it contributes nothing to the cell's layout"},
		{"inset: 1.5px",
			"1.5px on each of two neighbours is the reference's measured 3px of ground " +
				"between two cells — and it costs the container query NOTHING, because it " +
				"is inside the cell's own box"},
		{"z-index: -1",
			"the tile paints behind the cell's content and in front of the grid ground"},
		{"border-radius: var(--r-ctl)",
			"6px, the existing canon radius token — the reference measures ~6px and no " +
				"new scale value is introduced"},
		{"border: 1px solid var(--tile-rule)",
			"the reference's third step, and it is DERIVED rather than borrowed. It was " +
				"`var(--rule-structural)` — the general STRUCTURAL rule token — which " +
				"made the tile edge the loudest step in the grid: 36.1 from the ground " +
				"on light and 39.7 on dark, a 50/255 fill→rule step against the " +
				"reference's +14, inside a change whose stated purpose was softness. " +
				"--tile-rule is one pop step off --cellbase, so it also carries the era's " +
				"hue and a tinted tile reads as one soft object. The --rule-* ramp is " +
				"untouched because other surfaces read it, and the band is measured in a " +
				"real engine by TestCellProbe_TheTileRuleIsTheQuietestRuleInTheGrid"},
	} {
		if !strings.Contains(tile, want.decl) {
			t.Errorf("the tile rule is missing %q — %s", want.decl, want.why)
		}
	}
	// --r-ctl IS VERIFIED, NOT ASSUMED. The tile's radius is written as the token
	// because the recipe measured ~6px in the reference; if the canon scale ever
	// moved, the token would silently take the tile with it.
	if !strings.Contains(flat, "--r-ctl: 6px") {
		t.Error("--r-ctl is no longer 6px. The tile's radius is written as that token " +
			"because the reference measures ~6px; a moved token moves the tile silently")
	}

	// ── THE GROUND MUST SHOW, WHICH MEANS `.cell` PAINTS NOTHING ITSELF ─────
	cellRe := regexp.MustCompile(`(?s)\.cal-block-host \.cell \{([^}]*)\}`)
	cm := cellRe.FindStringSubmatch(css)
	if cm == nil {
		t.Fatal("no `.cal-block-host .cell` rule at all — every arm below is vacuous")
	}
	cell := cm[1]
	if strings.Contains(cell, "background") {
		t.Error("`.cell` declares a background again. Its box IS the grid track, so an " +
			"opaque fill here covers the 1.5px the tile insets on every side and the 3px " +
			"gutter between two tiles disappears. The fill belongs on ::before (§ANSWERED)")
	}
	for _, dead := range []string{"border-right: 1px solid var(--rule-structural);",
		"border-bottom: 1px solid var(--rule-structural);"} {
		if strings.Contains(cell, dead) {
			t.Errorf("`.cell` still carries %q. The shared hairline was the RULED treatment "+
				"the tile replaces, and it also costs the container-query subject a pixel "+
				"on every column", dead)
		}
	}
	if !strings.Contains(cell, "isolation: isolate") {
		t.Error("`.cell` is not a stacking context. Without one the tile's `z-index: -1` " +
			"escapes to the nearest ancestor context and paints BEHIND `.grid`'s ground, " +
			"i.e. nowhere — a cell with no fill at all, which reads as a working build " +
			"on a theme whose ground and fill are close together")
	}

	// THE FIVE-COLUMN RULE IS THE ONE BORDER THAT SURVIVES, and it is still a
	// full ramp step above the tile's own rule — the whole point of it.
	if !strings.Contains(flat, ".cal-block-host .cell.half, .cal-block-host .grid .hd b.half "+
		"{ border-right: 1px solid var(--rule-structural-strong); }") {
		t.Error("the five-column rule is no longer a full ramp step above the tile rule — " +
			"humans cannot count to ten across identical columns, and that rule is the " +
			"highest-value single addition to a ten-wide grid")
	}
	// `.cell.lastcol` existed only to cancel the shared right-hand hairline. With
	// no hairline it cancels nothing, and a rule that cancels nothing is a rule a
	// later hand has to reason about.
	if strings.Contains(flat, ".cal-block-host .cell.lastcol") {
		t.Error("`.cell.lastcol` still has a rule. It suppressed the shared border-right " +
			"on the last column; there is no shared border any more, so it is dead")
	}

	// THE CELL'S OWN PADDING STAYS 0. `container-type: inline-size` measures the
	// cell's CONTENT box, so padding here would shrink every container query's
	// subject and move all four thresholds by twice the padding — silently, on
	// every signed still. The spec's "internal padding" is spent on the
	// subtrees, which is where it already lived.
	if !strings.Contains(cell, "padding: 0") {
		t.Error("`.cell` has grown a padding. It is the container query's subject, so a " +
			"padding here moves the silhouette, date, expansion and density thresholds " +
			"all at once, and moves them invisibly")
	}
	if !strings.Contains(cell, "container-type: inline-size") {
		t.Error("`.cell` is no longer the density container — the whole sizing story " +
			"depends on it measuring the real column box")
	}
}

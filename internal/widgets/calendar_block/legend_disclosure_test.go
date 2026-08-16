// legend_disclosure_test.go — the legend OPENS, and it is a COLOUR key
// (C-CALV4-TILES §9.4).
//
// Two things changed about the legend and only two: how it opens, and what its
// tab is allowed to claim. Everything else — the entries, the swatches, the
// counts, the single pass over Rows[].Cells[].Marks that produces them — is
// untouched, and the producer-side count oracle
// (internal/plugins/calendar/block_count_oracle_test.go) still pins the exact
// bytes `<label> <span class="n">N</span>`. These tests assert the two changes
// and the fact that nothing else moved.
//
// WHAT IS ASSERTED HERE AND WHAT IS NOT. This file reads MARKUP and the
// STYLESHEET, which between them can prove that the control exists, that a rule
// reads it, and that the copy claims nothing about shape. They cannot prove that
// a thumb opens it or that a keyboard reaches it — a rule that is written and a
// finger that lands are different claims, and the package has been burned by
// the difference before (cell_probe_test.go's header). That half is
// legend_disclosure_probe_test.go, in a real browser, under a genuinely coarse
// pointer.
package calendar_block

import (
	"strings"
	"testing"
)

// ── the markup ──────────────────────────────────────────────────────────────

// TestLegend_OpensAsADisclosureAndKeepsItsEntries.
//
// THE DISCLOSURE IS A CLIPPED CHECKBOX PLUS ITS LABEL, and the pairing is the
// whole mechanism: `label[for]` is what a tap lands on and the input is what the
// keyboard lands on. A tab whose `for` did not resolve would look identical in
// review, render identically, and do nothing at all on either device.
//
// AND THE ENTRIES ARE STILL INSIDE IT. An assertion that only checked for the
// tab would pass on a build that shipped a disclosure over an empty box, which
// is the one-directional-guard failure sky_test.go names by example.
func TestLegend_OpensAsADisclosureAndKeepsItsEntries(t *testing.T) {
	d := fxAlmanac(t, true)
	body := legendOnly(t, d)

	pin := legendPinID(d)
	if pin == "" {
		t.Fatal("the disclosure control has no id, so no label can address it")
	}
	mustContain(t, body, `<input type="checkbox" class="legpin vhctl" id="`+pin+`">`,
		"the legend's disclosure control. CLIPPED, not display:none — a display-none input "+
			"is not focusable and the keyboard could never reach the thing the tab names")
	mustContain(t, body, `<label class="legtab" for="`+pin+`">`+legendTabLabel()+`</label>`,
		"the tab at rest, addressing the control by id. A label whose `for` does not "+
			"resolve renders identically and does nothing on tap or on keyboard")
	mustContain(t, body, `<div class="legbody">`,
		"the disclosed body — the entries need a container the stylesheet can close")

	// The zone's CONTENT is unchanged. This is the byte the producer-side count
	// oracle keys on; if the disclosure had rebuilt the entry markup on its way
	// past, the oracle would fail in a different package and read as a producer
	// bug rather than as this one.
	for _, want := range []string{
		`<span class="n">`,
		`class="lr `,
	} {
		mustContain(t, body, want,
			"the legend's entries must be untouched by the disclosure — same swatch, "+
				"same count, same single pass")
	}

	// EXACTLY ONE CONTROL. Two would sit in the DOM as two ids, `label[for]`
	// would resolve to the first, and the second tab would be inert.
	if n := strings.Count(body, `class="legpin`); n != 1 {
		t.Errorf("%d disclosure controls in one Block; there is one legend and one tab", n)
	}
}

// TestLegend_TheTabIDIsAPureFunctionOfTheData.
//
// `label[for]` resolves to the FIRST matching id in the DOCUMENT, so a constant
// id means every legend on a Bench of four Blocks opens the first Block's — the
// same defect moon_reach_probe_test.go hit with radio group names and reported
// as "the panel never opens". A pure function also survives an HTMX binding
// swap: the same Block re-rendered keeps the state the viewer just chose.
func TestLegend_TheTabIDIsAPureFunctionOfTheData(t *testing.T) {
	a := fxAlmanac(t, true)
	b := fxAlmanac(t, true)
	if legendPinID(a) != legendPinID(b) {
		t.Errorf("the same data produced two ids (%q, %q) — a re-render would lose the "+
			"viewer's open state", legendPinID(a), legendPinID(b))
	}

	other := fxAlmanac(t, true)
	other.CalendarSlug = a.CalendarSlug + "-second"
	if legendPinID(other) == legendPinID(a) {
		t.Error("two Blocks share one disclosure id — on a Bench every tab would open the " +
			"first Block's legend and the others would look broken")
	}

	hosted := fxAlmanac(t, true)
	hosted.Viewer.HostEntity = "ent-42"
	if legendPinID(hosted) == legendPinID(a) {
		t.Error("two Blocks of the same calendar on one page (one entity-hosted) share an " +
			"id — the identity is (CalendarSlug, HostEntity), exactly as tieGroupName and " +
			"dayPickGroupName are")
	}
}

// TestLegend_TheTabIsAColourKeyAndClaimsNothingAboutShape is §9.2's cost,
// asserted where it can actually be violated.
//
// The runes made the glyph decorative: it is chosen by :nth-child(), i.e. by
// POSITION IN THE DAY, so a four-event day draws four different runes and none
// of them means anything. blockMarkFor's guarantee — "a viewer who cannot
// separate the hues can still separate the marks" — is retired IN THE GRID, and
// the legend is the surface most likely to go on promising it, because a legend
// is where a reader goes to learn what a mark means.
//
// So the tab's copy is checked against the vocabulary that would make the
// promise. This is a copy assertion and it is the right shape for one: the
// defect is a sentence, not a rule.
func TestLegend_TheTabIsAColourKeyAndClaimsNothingAboutShape(t *testing.T) {
	tab := strings.ToLower(legendTabLabel())
	if tab == "" {
		t.Fatal("the tab has no copy at all — a disclosure with no name is not reachable")
	}
	for _, bad := range []string{"rune", "shape", "glyph", "symbol", "icon", "mark means", "sign"} {
		if strings.Contains(tab, bad) {
			t.Errorf("the legend tab says %q, which claims a mapping the grid does not "+
				"implement: since C-CALV4-TILES §9.2 the rune is chosen by position in the "+
				"day, so no shape carries a type. It is a COLOUR key", legendTabLabel())
		}
	}
	if !strings.Contains(tab, "colour") && !strings.Contains(tab, "color") {
		t.Errorf("the legend tab says %q and never names the axis it actually keys. The "+
			"swatch carries hue and the locked dash; the copy must say which", legendTabLabel())
	}

	// And the rendered zone must not smuggle the claim in somewhere else.
	body := strings.ToLower(legendOnly(t, fxAlmanac(t, true)))
	for _, bad := range []string{"rune", "glyph"} {
		if strings.Contains(body, bad) {
			t.Errorf("the legend zone's markup mentions %q — no rune means anything, and a "+
				"legend is exactly where a reader would believe it did", bad)
		}
	}
}

// TestLegend_NoMarksStillRendersNoZoneAtAll. The disclosure must not become the
// thing that survives an empty legend: a tab reading "Colour key" over nothing
// is an empty box saying "there is nothing here", which is worse than absence —
// absence is already this product's vocabulary for that.
func TestLegend_NoMarksStillRendersNoZoneAtAll(t *testing.T) {
	d := fxAlmanac(t, true)
	for ri := range d.Month.Rows {
		for ci := range d.Month.Rows[ri].Cells {
			d.Month.Rows[ri].Cells[ci].Marks = nil
		}
	}
	for i := range d.Month.Intercalary {
		d.Month.Intercalary[i].Marks = nil
	}
	body := legendOnly(t, d)
	for _, bad := range []string{`data-layer="legend"`, `class="legtab"`, `class="legpin`} {
		mustNotContain(t, body, bad,
			"a month with no visible marks renders no legend zone — not a tab, not a "+
				"control, not a heading")
	}
}

// ── the stylesheet ──────────────────────────────────────────────────────────

// TestCSS_LegendDisclosureIsReadableByPointerKeyboardAndThumb.
//
// The markup test above can see that a checkbox and a label exist. Only this can
// see whether anything READS them — and a control that is present, focusable and
// does nothing is worse than absent (the tie toggle's guard says so in as many
// words). Four openers, and each answers a device the others do not:
//
//	hover        a mouse, and gated on there BEING a hover — an ungated branch
//	             is the whole affordance on a desktop and none of it on a phone
//	:focus-visible  the keyboard, and NOT :focus-within: clicking a label focuses
//	             its input, so :focus-within would hold the zone open after the
//	             second tap un-checked the box, and the tab would visibly do
//	             nothing on every press after the first
//	:checked     the thumb, and the pin that survives the pointer leaving
//	switchboard  pointing at the layer row lights the section it controls
func TestCSS_LegendDisclosureIsReadableByPointerKeyboardAndThumb(t *testing.T) {
	code := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(code), " ")

	if !strings.Contains(flat, ".cal-block-host .legend > .legbody { display: none;") {
		t.Error("the legend's body is not closed at rest — the whole change is that it " +
			"OPENS, and a body that never closes is the surface that shipped before §9.4")
	}

	for _, o := range []struct{ sel, why string }{
		{".cal-block-host .legend:hover > .legbody",
			"a mouse must open it by pointing — that is the resting affordance"},
		{".cal-block-host .legend:has(> .legpin:focus-visible) > .legbody",
			"a keyboard must open it. :focus-within is REFUSED here: a tap focuses the " +
				"label's input, so :focus-within would keep the zone open after the second " +
				"tap closed it and the tab would be a control that visibly does nothing"},
		{".cal-block-host .legend:has(> .legpin:checked) > .legbody",
			"a thumb must open it, and the pin must survive the pointer leaving. Hover " +
				"alone strands a phone"},
		{".cal-block-host:has(.layerrow[data-layer-pick=\"legend\"]:hover) .legend > .legbody",
			"the switchboard's preview lights the section a row controls; with the legend " +
				"collapsed it would light a 22px tab and teach nothing"},
	} {
		if !strings.Contains(flat, o.sel) {
			t.Errorf("no rule opens the legend via %q — %s", o.sel, o.why)
		}
	}

	// THE HOVER BRANCH IS GATED. An ungated one is not a smaller version of the
	// same feature; it is the entire feature on one device class and none of it
	// on the other.
	inside, _, ok := splitAtRuleBlock(code, "@media (hover: hover)")
	if !ok {
		t.Fatal("the sheet has no `(hover: hover)` block — the legend's hover opener must " +
			"live inside one, or a touch device inherits an affordance it cannot use")
	}
	if !strings.Contains(strings.Join(strings.Fields(inside), " "),
		".cal-block-host .legend:hover > .legbody") {
		t.Error("the legend's hover opener is outside the hover-capability block")
	}

	// THE COARSE TARGET. Reported by the probe as a measurement; asserted here
	// as existence, because a deleted media block would make the probe measure a
	// fine pointer's geometry and report it as a touch device's.
	coarse, _, ok := splitAtRuleBlock(code, "@media (pointer: coarse)")
	if !ok {
		t.Fatal("the sheet declares no coarse-pointer block")
	}
	if !strings.Contains(coarse, ".legtab") {
		t.Error("the legend's tab takes no coarse-pointer target. A 22px control is under " +
			"the 44px floor this spec's own defect list cites, and the legend zone has the " +
			"block-axis room to meet it")
	}

	// NOTHING ABOUT THE DISCLOSURE MOVES. TestCSS_NoMotionAtAll bounds the whole
	// sheet; this bounds THIS surface by name, because "open" is the property
	// most likely to attract a transition from the next hand.
	for _, rule := range legendRuleBodies(code) {
		for _, bad := range []string{"transition", "animation"} {
			if strings.Contains(rule, bad) {
				t.Errorf("a legend rule declares %q. The month grid never moves and the "+
					"zones under it are laid out beneath it — a legend that grew would push "+
					"the illumination graph down on every hover", bad)
			}
		}
	}
}

// legendRuleBodies returns the declaration bodies of every rule whose selector
// names a legend surface. Comment-stripped input only.
func legendRuleBodies(code string) []string {
	var out []string
	for _, m := range preludeRe.FindAllStringSubmatchIndex(code, -1) {
		sel := strings.TrimSpace(code[m[4]:m[5]])
		if !strings.Contains(sel, ".legend") && !strings.Contains(sel, ".legtab") &&
			!strings.Contains(sel, ".legbody") && !strings.Contains(sel, ".legpin") {
			continue
		}
		rest := code[m[1]:]
		if end := strings.Index(rest, "}"); end >= 0 {
			out = append(out, rest[:end])
		}
	}
	return out
}

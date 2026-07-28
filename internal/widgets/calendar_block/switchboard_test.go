// switchboard_test.go — the layer sheet's render contract
// (C-CALV4-LAYERS-P9, W-F).
//
// The claims worth pinning here are the ones a screenshot cannot make and a
// reviewer cannot eyeball: that the registry is not extended, that the
// denominator does not vary by role, that every row posts the RESULTING SET,
// and that r54's invariant is enforced at the surface rather than only in a
// comment.
package calendar_block

import (
	"strings"
	"testing"
)

const prefsURL = "/campaigns/camp-1/calendar/prefs"

// withSwitchboard turns a fixture's Block into one whose producer resolved a
// live store: the flag and the endpoint set TOGETHER, because r54 pins them as
// one fact.
func withSwitchboard(d BlockData, enabled ...string) BlockData {
	d.Layers = LayerState{Enabled: enabled, HasSwitchboard: true, PersistURL: prefsURL}
	return d
}

// THE REGISTRY IS EIGHT ROWS AND THE SHAPE IS {k, n, where[, inside]}.
// LayerRows and LayerKeys are two orderings of one fact, so a key added to one
// and forgotten in the other must be a red build rather than a row that
// silently vanishes from the sheet.
func TestSwitchboard_RegistryMatchesLayerKeysExactly(t *testing.T) {
	if len(LayerRows) != len(LayerKeys) {
		t.Fatalf("LayerRows has %d entries, LayerKeys has %d — they are one registry",
			len(LayerRows), len(LayerKeys))
	}
	for i, r := range LayerRows {
		if r.Key != LayerKeys[i] {
			t.Errorf("row %d key = %q, LayerKeys[%d] = %q — the ORDER is the contract too "+
				"(the sheet's groups and the stored string both follow it)", i, r.Key, i, LayerKeys[i])
		}
		if r.Name == "" || r.Where == "" {
			t.Errorf("row %q has an empty Name/Where — `where` is the row's whole "+
				"explanation and is why no tooltip is needed", r.Key)
		}
	}
	// The three INSIDE layers change the month's own geometry (canon A8/L-M2)
	// and are the sheet's first group. Pinned by name: moving one across the
	// divide changes which law applies to it.
	inside := map[string]bool{}
	for _, r := range LayerRows {
		if r.Inside {
			inside[r.Key] = true
		}
	}
	for _, want := range []string{"moons", "eras", "weeknums"} {
		if !inside[want] {
			t.Errorf("%q must be an INSIDE layer — it changes the month's geometry", want)
		}
	}
	if len(inside) != 3 {
		t.Errorf("%d inside layers; the signed registry marks exactly three", len(inside))
	}
}

// The sheet exists only when the producer resolved a store, and the invoker
// tracks it. A rendered sheet with a disabled invoker is a surface nobody can
// reach; a live invoker with no sheet is a control that swallows a click.
func TestSwitchboard_AbsentWithoutAStore(t *testing.T) {
	body := flatten(render(t, fxHarptos(true)))
	if strings.Contains(body, `class="lsheet"`) {
		t.Error("the sheet rendered for a Block whose producer resolved no store")
	}
	if !strings.Contains(body, `class="icb layers" data-open-layers disabled`) {
		t.Error("without a store the ⋯ invoker must stay disabled — present so the " +
			"Nameplate's geometry is final, inert so it is not a dead control")
	}
	if strings.Contains(body, "popovertarget") {
		t.Error("a disabled invoker must not name a popover that does not exist")
	}
}

// r54's INVARIANT, enforced where it can actually hurt. A true flag with an
// empty URL would emit eight rows that post to the page they are already on.
func TestSwitchboard_NeverPostsToAnEmptyURL(t *testing.T) {
	d := fxHarptos(true)
	d.Layers = LayerState{Enabled: []string{"moons"}, HasSwitchboard: true} // no PersistURL
	body := flatten(render(t, d))
	if strings.Contains(body, `hx-post=""`) {
		t.Error("a row posts to the empty string — HasSwitchboard without PersistURL " +
			"renders a live-looking control that does nothing (r54's invariant, and " +
			"exactly the inert-control shape WG-spec V18 forbids)")
	}
}

// THE DENOMINATOR NEVER VARIES BY ROLE. Same eight rows, same "of 8", for
// owner, co-DM and player — only the on-set differs. Filtering the row set by
// what a viewer would actually get reintroduces `gmOnly` under another name,
// which REVIEW killed by name; [LYR-6a] signs the row staying.
func TestSwitchboard_DenominatorIsEightForEveryRole(t *testing.T) {
	for _, gm := range []bool{true, false} {
		body := flatten(render(t, withSwitchboard(fxHarptos(gm), "moons")))
		if !strings.Contains(body, "Layers · 1 of 8 on") {
			t.Errorf("gm=%v: heading missing or wrong — the denominator is the registry's "+
				"length and never a per-role subset", gm)
		}
		for _, r := range LayerRows {
			if !strings.Contains(body, `data-layer-pick="`+r.Key+`"`) {
				t.Errorf("gm=%v: row %q is missing from the sheet", gm, r.Key)
			}
		}
		if n := strings.Count(body, "data-layer-pick="); n != len(LayerRows) {
			t.Errorf("gm=%v: %d rows rendered, want %d", gm, n, len(LayerRows))
		}
	}
}

// The numerator IS the on-set, so the heading cannot drift from the switches.
func TestSwitchboard_HeadingCountsWhatIsOn(t *testing.T) {
	body := flatten(render(t, withSwitchboard(fxHarptos(true),
		"moons", "eras", "weeknums", "ledger", "shelf")))
	if !strings.Contains(body, "Layers · 5 of 8 on") {
		t.Error("the heading must count the enabled set")
	}
	// Counted on the ROWS, not on the document: the Ledger's own `.ltab` also
	// carries aria-pressed, and a document-wide count would silently absorb it.
	if n := strings.Count(body, `class="layerrow" data-layer-pick=`); n != len(LayerRows) {
		t.Fatalf("%d rows rendered, want %d", n, len(LayerRows))
	}
	on := 0
	for _, r := range LayerRows {
		if strings.Contains(body, `data-layer-pick="`+r.Key+`" aria-pressed="true"`) {
			on++
		}
	}
	if on != 5 {
		t.Errorf("%d rows pressed, want 5 — aria-pressed IS the state (canon A6)", on)
	}
}

// EVERY ROW POSTS THE RESULTING SET, NOT A DELTA. Idempotent under a double
// click, no read-modify-write on the server, and — the reason that decides it —
// the SET is what the seam test compares against what renders.
func TestSwitchboard_RowPostsTheResultingSet(t *testing.T) {
	d := withSwitchboard(fxHarptos(true), "moons", "ledger")
	body := flatten(render(t, d))

	// Turning a layer ON adds it, in registry order.
	if !strings.Contains(body, `hx-vals='{&#34;layers&#34;:&#34;moons,eras,ledger&#34;}'`) &&
		!strings.Contains(body, `hx-vals="{&#34;layers&#34;:&#34;moons,eras,ledger&#34;}"`) {
		t.Errorf("the `eras` row must post the set that RESULTS from turning it on, in "+
			"registry order; body did not contain it:\n%s", excerptAround(body, "eras"))
	}
	// Turning a layer OFF removes it and nothing else.
	if !strings.Contains(body, "moons,ledger") {
		t.Error("no row posts the unchanged pair — the set is not being composed from the " +
			"current state")
	}
	if strings.Contains(body, "js:") {
		t.Error("hx-vals must be plain JSON: boot.js disables eval globally, so a `js:` " +
			"payload is dead on arrival")
	}
	if !strings.Contains(body, `hx-swap="none"`) {
		t.Error("the answer is 204 with no body; a row that swaps something is expecting " +
			"a fragment this route deliberately never sends")
	}
	if strings.Count(body, `hx-post="`+prefsURL+`"`) != len(LayerRows) {
		t.Error("every row must post to the producer-supplied endpoint")
	}
}

// Turning the LAST layer off posts the empty set, which is a real choice — a
// bare month. If this posted nothing the viewer could never reach it.
func TestSwitchboard_LastLayerOffPostsTheBareMonth(t *testing.T) {
	body := flatten(render(t, withSwitchboard(fxHarptos(true), "moons")))
	if !strings.Contains(body, `layers:&#34;&#34;`) && !strings.Contains(body, `&#34;layers&#34;:&#34;&#34;`) {
		t.Errorf("the `moons` row must post an EMPTY set — a bare month is reachable, and "+
			"a default nobody can leave is not a default:\n%s", excerptAround(body, "moons"))
	}
}

// The row is a real, focusable, keyboard-reachable control — the signed
// <button data-layer-pick> primitive. REVIEW failed the proposed sheet for
// making it a <div>, which is also invisible to the keyboard.
func TestSwitchboard_RowsAreButtonsNotDivs(t *testing.T) {
	body := flatten(render(t, withSwitchboard(fxHarptos(true), "moons")))
	for _, r := range LayerRows {
		want := `<button type="button" class="layerrow" data-layer-pick="` + r.Key + `"`
		if !strings.Contains(body, want) {
			t.Errorf("row %q is not the signed <button data-layer-pick> primitive", r.Key)
		}
	}
}

// The sheet is a top-layer [popover] opened DECLARATIVELY, and the invoker
// names it. This is the only mechanism available: there is no JS in this
// package and there cannot be.
func TestSwitchboard_OpensWithoutJS(t *testing.T) {
	d := withSwitchboard(fxHarptos(true), "moons")
	body := flatten(render(t, d))
	id := layerSheetID(d)

	if !strings.Contains(body, `<div popover id="`+id+`" class="lsheet"`) {
		t.Error("the sheet must be a [popover] with a stable per-instance id")
	}
	if !strings.Contains(body, `popovertarget="`+id+`"`) {
		t.Error("the ⋯ invoker must open the sheet via popovertarget — declaratively, no JS")
	}
	for _, forbidden := range []string{"<script", "onclick=", "hx-on", "javascript:"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the switchboard emitted %q — boot.js sets allowScriptTags=false and "+
				"allowEval=false, so it would be dead code as well as a rule break", forbidden)
		}
	}
}

// Two Blocks on one page (the Bench composes four) must not share a sheet id or
// an anchor name: every ⋯ would open the first Block's sheet, and one shared
// anchor-name makes CSS anchor positioning ambiguous.
func TestSwitchboard_InstanceIdentityIsPerBlock(t *testing.T) {
	a := withSwitchboard(fxHarptos(true), "moons")
	b := withSwitchboard(fxGregorian(), "moons")
	if layerSheetID(a) == layerSheetID(b) {
		t.Error("two Blocks share a sheet id — every ⋯ would open the first one's sheet")
	}
	if layerAnchorName(a) == layerAnchorName(b) {
		t.Error("two Blocks share an anchor-name — CSS anchor positioning would be ambiguous " +
			"on a Bench, which routinely renders four")
	}
	body := flatten(render(t, a))
	if !strings.Contains(body, "anchor-name:"+layerAnchorName(a)) {
		t.Error("the invoker must carry its per-instance anchor-name")
	}
	if !strings.Contains(body, "position-anchor:"+layerAnchorName(a)) {
		t.Error("the sheet must point at its own invoker's anchor")
	}
}

// [LYR-10] SIGNED: the two-group sheet ships. The rubric lines are canon A8's
// two laws written where the viewer meets them, and the footer states the
// per-viewer promise the whole slice exists to keep.
func TestSwitchboard_ShipsTheTwoGroupSheet(t *testing.T) {
	body := flatten(render(t, withSwitchboard(fxHarptos(true), "moons")))
	for _, want := range []string{
		"In the month",
		"instant · no confirm, no animation",
		"Below the month",
		"opens a section",
		"Your view only — remembered per viewer.",
		"the default is the month with its moon phases and nothing else.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the sheet is missing %q", want)
		}
	}
}

// The sheet may not grow a second sky menu (L29: "The second menu is gone …
// The switchboard is now the only place layers are chosen"), and it may not
// mint a `needs backend` chip of its own — .badge.need means one thing.
func TestSwitchboard_GrowsNoSecondMenuAndNoChip(t *testing.T) {
	body := flatten(render(t, withSwitchboard(fxHarptos(true), "moons")))
	sheet := body[strings.Index(body, `class="lsheet"`):]
	if i := strings.Index(sheet, "</div> </div>"); i > 0 {
		sheet = sheet[:i]
	}
	for _, forbidden := range []string{"moonstyle", "data-sky", "badge need", "Apply"} {
		if strings.Contains(sheet, forbidden) {
			t.Errorf("the sheet contains %q — the switchboard is the ONLY place layers are "+
				"chosen, it mints no status chips, and an Apply button would be the "+
				"confirm its own rubric line forbids", forbidden)
		}
	}
}

// excerptAround gives a failure something readable to point at without ever
// slicing on a bare strings.Index result, which PANICS on a rename.
func excerptAround(body, needle string) string {
	i := strings.Index(body, needle)
	if i < 0 {
		return "(needle absent)"
	}
	lo, hi := i-160, i+160
	if lo < 0 {
		lo = 0
	}
	if hi > len(body) {
		hi = len(body)
	}
	return body[lo:hi]
}

// §8.4 — THE OBLIGATION THE SWITCHBOARD CREATES. `needs backend` is GM-facing
// honesty copy and never renders to a player. That was safe for three waves
// only because no player could enable `horizon`: DEF is moons-only and neither
// host seed carries it. A switchboard makes every key reachable by everybody,
// so the gate has to exist before the reachability does — which is why it lands
// in this commit rather than the next one.
//
// A PLAYER RENDER CONTAINS NO `needs backend` STRING ANYWHERE, asserted over
// the whole document rather than over the zone, because the claim is about the
// player's markup and not about one element.
func TestSwitchboard_PlayerNeverSeesANeedsBackendChip(t *testing.T) {
	player := withSwitchboard(fxHarptos(false),
		"moons", "eras", "weeknums", "ledger", "moongraph", "legend", "horizon", "shelf")
	body := flatten(render(t, player))

	if strings.Contains(body, "needs backend") {
		t.Errorf("a player enabled every layer and got GM-facing honesty copy:\n%s",
			excerptAround(body, "needs backend"))
	}
	if strings.Contains(body, `data-layer="horizon"`) {
		t.Error("the horizon zone rendered to a player — permission is ABSENCE, and " +
			"display:none is not absence")
	}
	// The ROW stays: the denominator never varies by role ([LYR-6a]).
	if !strings.Contains(body, `data-layer-pick="horizon"`) {
		t.Error("the horizon ROW must stay in a player's sheet — filtering the row set by " +
			"what a viewer would get reintroduces `gmOnly` under another name")
	}

	// The same Block for a GM still says what it cannot build.
	gm := withSwitchboard(fxHarptos(true), "horizon")
	if !strings.Contains(flatten(render(t, gm)), `data-layer="horizon"`) {
		t.Error("a GM must still get the horizon zone and its chip — the data does not " +
			"exist for anybody, and saying so is the honesty state")
	}
}

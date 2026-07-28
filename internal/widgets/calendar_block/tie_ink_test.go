package calendar_block

// tie_ink_test.go — pin r51: the tie toggle changes INK, never membership.
//
// This is the render-side half of the ruling in
// cordinator decisions/2026-07-27-calv4-tie-mark-emission.md. The producer's
// half (every viewer-visible mark emitted in BOTH modes, each flagged) is pinned
// in internal/plugins/calendar. Neither side can see the other, which is exactly
// how the original defect survived: the producer dropped untied marks, the
// renderer had no way to know, and both suites were green.
//
// What this file proves from here: given a BlockData whose marks differ only in
// Tied, flipping TieMode changes no element, no count and no geometry — only the
// stamped class. If that holds, a CSS-only toggle is implementable and no cell
// can change height when it flips.

import (
	"strings"
	"testing"
)

// countAll is a crude but honest element counter — crude on purpose, because a
// structural comparison that depended on parsing would be a second thing that
// can be wrong.
func countAll(body, needle string) int { return strings.Count(body, needle) }

// fxTieHosted returns the Harptos fixture hosted on an entity, with marks split
// between tied and untied so the flag is actually exercised.
func fxTieHosted(t *testing.T, mode string) BlockData {
	t.Helper()
	d := fxHarptos(true)
	d.Viewer.HostEntity = "ent-77"
	d.Viewer.TieMode = mode
	d.Viewer.TiedCount = 0
	d.Viewer.WholeCount = 0

	// Flag every other mark tied, so both classes appear.
	flip := false
	for r := range d.Month.Rows {
		for c := range d.Month.Rows[r].Cells {
			cell := &d.Month.Rows[r].Cells[c]
			for m := range cell.Marks {
				cell.Marks[m].Tied = flip
				if flip {
					d.Viewer.TiedCount++
					cell.Tied = true
				}
				d.Viewer.WholeCount++
				flip = !flip
			}
		}
	}
	return d
}

// TestTieMode_ChangesInkNotGeometry is the invariant the whole ruling exists for.
func TestTieMode_ChangesInkNotGeometry(t *testing.T) {
	tied := render(t, fxTieHosted(t, "tied"))
	whole := render(t, fxTieHosted(t, "whole"))

	for _, el := range []struct{ name, needle string }{
		{"day cells", `class="cell`},
		{"named chips", `class="chip`},
		{"underline segments", `class="ulseg`},
		{"week rows", `class="wk`},
	} {
		a, b := countAll(tied, el.needle), countAll(whole, el.needle)
		if a != b {
			t.Errorf("%s: %d in tied mode, %d in whole — flipping the toggle changed the "+
				"DOM. A cell that gains or loses a mark changes height, which breaks the "+
				"no-motion rule and makes the counts differenceable", el.name, a, b)
		}
	}

	// The ONLY difference should be the mode attribute and the ink class the
	// stylesheet keys off. Byte length may differ trivially ("tied" vs "untied"),
	// but not structurally. Take the absolute delta FIRST, then compare once, so
	// the threshold fires in BOTH directions — hanging the comparison off the
	// else of the normalisation silenced exactly the tied-exceeds-whole side,
	// the stop-and-flag direction of the r51 payload prediction (coordinator
	// ruling 2026-07-28 §3, stage 16).
	delta := len(whole) - len(tied)
	if delta < 0 {
		delta = -delta
	}
	if delta > len(tied)/50 {
		t.Errorf("rendered length differs by %d bytes (>2%%) — the two modes should differ "+
			"only by class names, not content", delta)
	}
}

// TestTieMode_StampsBothClasses proves the ink hook actually reaches the DOM.
// Without it the toggle is inert however correct the data is — which is the
// state the product shipped in.
func TestTieMode_StampsBothClasses(t *testing.T) {
	body := render(t, fxTieHosted(t, "tied"))

	if !strings.Contains(body, `data-tie-mode="tied"`) {
		t.Error(`no data-tie-mode="tied" on the Block — CSS has nothing to key the ink off`)
	}
	if !strings.Contains(body, " tied") {
		t.Error("no mark carries the tied class")
	}
	if !strings.Contains(body, " untied") {
		t.Error("no mark carries the untied class — with nothing to dim, the toggle is a " +
			"control that visibly does nothing")
	}
}

// TestTieMode_OffEntityPageEmitsNoToggle keeps the control entity-hosted. Off an
// entity page there is nothing to be tied to.
func TestTieMode_OffEntityPageEmitsNoToggle(t *testing.T) {
	body := render(t, fxHarptos(true))
	if strings.Contains(body, `class="seg tie"`) {
		t.Error("the tie toggle rendered off an entity page")
	}
	if strings.Contains(body, `data-tie-mode="tied"`) {
		t.Error("data-tie-mode is set off an entity page")
	}
}

// TestTieMode_HostedPageEmitsToggle is the regression that matters most: the
// control was computed correctly and then dropped on the floor for the whole of
// wave 1, because the producer zeroed HostEntity and the renderer gated on it.
func TestTieMode_HostedPageEmitsToggle(t *testing.T) {
	body := render(t, fxTieHosted(t, "tied"))
	if !strings.Contains(body, `class="seg tie"`) {
		t.Fatal("no tie toggle on an entity-hosted Block — this is the defect the whole " +
			"slice exists to close")
	}
	// Both counts are always shown; showing one would make them differenceable.
	if strings.Count(body, `class="tiebtn"`) != 2 {
		t.Error("the toggle must state BOTH counts, or the pair becomes an oracle")
	}
}

// ── the CSS-only control (C-CALV4-HOST-P3 §3) ───────────────────────────────

// TestTieToggle_IsAPureCSSControl. The toggle must be operable with ZERO
// JavaScript: a <script> inside an HTMX-swapped fragment never executes
// (boot.js:163 sets htmx.config.allowScriptTags = false), so a JS toggle would
// go dead after any binding swap — silently, and only in production.
//
// What makes it operable is the radio pair: two real inputs, one checked, each
// addressed by its own label. Everything else about the flip is a stylesheet
// rule, which is pinned separately in css_contract_test.go.
func TestTieToggle_IsAPureCSSControl(t *testing.T) {
	body := render(t, fxTieHosted(t, "tied"))

	if n := strings.Count(body, `class="tiepick"`); n != 2 {
		t.Fatalf("expected exactly 2 tie radios (tied + whole); got %d", n)
	}
	if n := strings.Count(body, `type="radio"`); n != 2 {
		t.Errorf("the tie options must be radios — a checkbox pair can reach a state that is neither mode; got %d", n)
	}
	for _, want := range []string{`data-tie-pick="tied"`, `data-tie-pick="whole"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s — the stylesheet's :has() rule keys on it", want)
		}
	}
	// Exactly one option is selected, and it is the server's mode.
	if n := strings.Count(body, " checked"); n != 1 {
		t.Errorf("exactly one tie option may be checked; got %d", n)
	}
	// Zero JS. Not "little": none.
	if strings.Contains(body, "<script") || strings.Contains(body, "onclick") {
		t.Error("the tie toggle must ship no script and no inline handler — a <script> in an " +
			"HTMX-swapped fragment never runs")
	}

	// Each label addresses its own input, so clicking the visible control
	// actually moves the state the stylesheet reads.
	for _, mode := range []string{"tied", "whole"} {
		id := tieInputID(fxTieHosted(t, "tied"), mode)
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("no radio with id %q", id)
		}
		if !strings.Contains(body, `for="`+id+`"`) {
			t.Errorf("no label bound to %q — the visible control would do nothing", id)
		}
	}

	// Flipping the mode moves `checked` and changes nothing else about the pair.
	whole := render(t, fxTieHosted(t, "whole"))
	if strings.Count(whole, " checked") != 1 {
		t.Error("whole mode must still check exactly one option")
	}
	// Order is checked by relative position, not by byte offset: `checked` moves
	// between the two inputs and legitimately shifts every offset after it.
	for _, s := range []string{body, whole} {
		tiedAt := strings.Index(s, `data-tie-pick="tied"`)
		wholeAt := strings.Index(s, `data-tie-pick="whole"`)
		if tiedAt < 0 || wholeAt < 0 || tiedAt > wholeAt {
			t.Errorf("the tied option must precede the whole option in both modes (tied@%d whole@%d)", tiedAt, wholeAt)
		}
	}
}

// TestTieToggle_GroupNameIsStableAndPerBlock. Two Blocks on one page must not
// share a radio group — one viewer act would flip both, and the second Block's
// mode would be decided by the first. The SAME Block re-rendered (an HTMX
// binding swap returns the same wrapper) must keep its group, or the swap
// silently discards the mode the viewer just chose.
func TestTieToggle_GroupNameIsStableAndPerBlock(t *testing.T) {
	a := fxTieHosted(t, "tied")
	a.CalendarSlug = "cal-a"
	a.Viewer.HostEntity = "ent-1"

	again := fxTieHosted(t, "whole") // same identity, other mode
	again.CalendarSlug = "cal-a"
	again.Viewer.HostEntity = "ent-1"
	if tieGroupName(a) != tieGroupName(again) {
		t.Error("the group name must survive a re-render, or an HTMX swap loses the viewer's mode")
	}

	otherCal := a
	otherCal.CalendarSlug = "cal-b"
	if tieGroupName(a) == tieGroupName(otherCal) {
		t.Error("two calendars on one page must not share a radio group")
	}
	otherHost := a
	otherHost.Viewer.HostEntity = "ent-2"
	if tieGroupName(a) == tieGroupName(otherHost) {
		t.Error("two hosts must not share a radio group")
	}

	// An identity that is not id-safe must still produce a usable token.
	odd := a
	odd.CalendarSlug = `a b"c<d`
	for _, bad := range []string{" ", `"`, "<", ">"} {
		if strings.Contains(tieGroupName(odd), bad) {
			t.Errorf("group name must be attribute-safe; %q leaked through", bad)
		}
	}
	empty := a
	empty.CalendarSlug = ""
	empty.Viewer.HostEntity = ""
	if strings.Contains(tieGroupName(empty), "--") {
		t.Error("an empty identity must still yield a non-degenerate token")
	}
}

// stub_chip_test.go — the `needs backend` chip is GATED, not unconditional.
//
// The pin's ruling (data.go, LedgerStub/ShelfStub): "NeedsBackend GATES the
// chip — the renderer must not emit it unconditionally. W-B sets it false when
// the Ledger is filled, and it must not have to edit the stub template to stop
// the chip rendering." These are renderer assertions on hand-written BlockData
// — exactly what a widget-side test may pin. What value the producer chooses
// to SET the flags to is the seam's business, asserted producer-side.
//
// BOTH ZONES ARE NOW FILLED, so this file's claim has turned over twice and
// both turns are EDITS, not breaks: W-B proved a filled Ledger carries no chip
// whatever its flag says, and W-E proves the same of the Shelf. The flags are
// deliberately NOT retired — they are the host's honesty switch for a zone a
// future host docks but cannot fill — so what is pinned is that a FILLED zone
// ignores its own flag, not that the flag stopped existing.
//
// The legend/horizon/moongraph zone chips (block.templ) are NOT asserted here:
// the pin defines no governing flag for them, so their gating is out of scope
// until a ruling assigns one. They are also untouched by wave 2 — the
// 2026-07-28 DEF/zone-chip ruling §2 forbids minting flags for zones no wave
// is named to fill.

package calendar_block

import (
	"strings"
	"testing"
)

// TestStubChips_AFilledZoneCarriesNoChipWhateverItsFlagSays.
//
// Renamed from TestStubChips_GatedOnNeedsBackend by C-CALV4-SHELF-P7, because
// the statement it makes is now the STRONGER one and the old name described
// the weaker: with both zones filled there is no flag setting that can put
// "needs backend" beside real content. A producer that was never updated
// cannot lie.
//
// THE ONE SURVIVING BLOCK-SIDE CHIP IS THE FILTERS PANEL'S, and it is not a
// zone chip: it is the honest state of one TAB inside a filled zone ([S2]),
// unconditional inside its own panel exactly as the 2026-07-28 ruling §2
// sanctions, and it renders to a GM only.
func TestStubChips_AFilledZoneCarriesNoChipWhateverItsFlagSays(t *testing.T) {
	const chip = `class="badge need">needs backend`

	// The two zone keys are ON (the zones are layer-owned and absent without
	// them) and no needzone key is, so the count isolates the two filled zones
	// plus whatever their content emits.
	gm := fxHarptos(true)
	gm.Layers = LayerState{Enabled: []string{"moons", "ledger", "shelf"}}

	// Both flags UP — the wave-1 state — on two zones that now have content.
	// Neither zone chip may render; the only chip left is the Filters panel's.
	body := render(t, gm)
	if n := strings.Count(body, chip); n != 1 {
		t.Errorf("both flags true on two FILLED zones: %d chips, want exactly one — the "+
			"Filters panel's ([S2]). A zone chip beside real content is a lie whichever "+
			"way the flag points", n)
	}
	mustContain(t, body, `data-spane="filters"`,
		"the surviving chip must be the Filters panel's, not a zone's")
	mustContain(t, body, `data-zone="ledger"`, "a filled Ledger still docks its zone")
	mustContain(t, body, `data-zone="shelf"`, "a filled Shelf still docks its zone")

	// Both flags DOWN — the wave-2 producer state. Identical result, which is
	// the point: the flags no longer decide anything for these two zones.
	gm.Ledger.NeedsBackend = false
	gm.Shelf.NeedsBackend = false
	down := render(t, gm)
	if n := strings.Count(down, chip); n != 1 {
		t.Errorf("both flags false: %d chips, want exactly one — the Filters panel's", n)
	}
	mustContain(t, down, `data-zone="ledger"`, "a Ledger with its flag down still docks its zone")
	mustContain(t, down, `data-zone="shelf"`, "a Shelf with its flag down still docks its zone")

	// A PLAYER RECEIVES NO CHIP IN ANY FORM. `needs backend` never renders to
	// a player (decisions/2026-07-27-needs-backend-audience.md), and [S10]
	// applies it one level down: the player gets no Filters TAB either, so the
	// tab cannot open on nothing.
	player := fxHarptos(false)
	player.Layers = LayerState{Enabled: []string{"moons", "ledger", "shelf"}}
	pb := render(t, player)
	if n := strings.Count(pb, chip); n != 0 {
		t.Errorf("a player received %d `needs backend` chips; for a player the surface simply "+
			"does not appear", n)
	}
	mustNotContain(t, pb, `data-spane="filters"`,
		"a player's Shelf has no Filters PANEL — absence, not a disabled control")
	mustNotContain(t, pb, `data-shelf-pick="filters"`,
		"a player's Shelf has no Filters TAB either ([S10]: two tabs for a player, three for a GM)")
	mustContain(t, pb, `data-zone="shelf"`, "the player's Shelf zone still docks")
}

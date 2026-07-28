// stub_chip_test.go — the `needs backend` chip is GATED, not unconditional.
//
// The pin's ruling (data.go, LedgerStub/ShelfStub): "NeedsBackend GATES the
// chip — the renderer must not emit it unconditionally. W-B sets it false when
// the Ledger is filled, and it must not have to edit the stub template to stop
// the chip rendering." These are renderer assertions on hand-written BlockData
// — exactly what a widget-side test may pin. What value the producer chooses
// to SET the flags to is the seam's business, asserted producer-side.
//
// The legend/horizon/moongraph zone chips (block.templ) are NOT asserted here:
// the pin defines no governing flag for them, so their gating is out of scope
// until a ruling assigns one.

package calendar_block

import (
	"strings"
	"testing"
)

// TestStubChips_GatedOnNeedsBackend: true → one chip per stub zone; false →
// no chip, but the zone STAYS DOCKED — the full-tier column arithmetic
// subtracts the Ledger's 300px unconditionally, so a filled zone that
// disappeared would flip density at the wrong host width.
func TestStubChips_GatedOnNeedsBackend(t *testing.T) {
	const chip = `class="badge need">needs backend`

	// The two zone keys are ON (the zones are layer-owned and absent without
	// them) and no needzone key is, so the count isolates the two stubs.
	d := fxHarptos(true)
	d.Layers = LayerState{Enabled: []string{"moons", "ledger", "shelf"}}
	if n := strings.Count(render(t, d), chip); n != 1 {
		t.Errorf("both flags true: %d chips; want exactly one — the Shelf's. The Ledger is "+
			"FILLED from wave 2 and carries no chip whatever its flag says", n)
	}

	// W-E fills the Shelf: flag false, chip gone, zone still docked.
	d.Shelf.NeedsBackend = false
	filled := render(t, d)
	mustContain(t, filled, `data-zone="ledger"`, "a filled Ledger still docks its zone")
	mustContain(t, filled, `data-zone="shelf"`, "a Shelf with its flag down still docks its zone")
	if n := strings.Count(filled, chip); n != 0 {
		t.Errorf("ShelfStub.NeedsBackend=false: %d chips still render; the flag gates the chip "+
			"(data.go ruling)", n)
	}

	// INVERTED BY C-CALV4-LEDGER-P6, not deleted. This half used to prove that
	// the LEDGER'S chip answered the Ledger's flag. Now that W-B has filled the
	// zone, the stronger statement is available and is the one worth pinning:
	// a filled Ledger carries no chip AT ALL, so a producer that was never
	// updated cannot leave "needs backend" sitting beside fourteen real rows.
	// The Shelf's flag still gates the Shelf's chip, independently.
	d.Ledger.NeedsBackend = true
	if n := strings.Count(render(t, d), chip); n != 0 {
		t.Errorf("LedgerStub.NeedsBackend=true on a FILLED Ledger produced %d chips; the zone "+
			"has content and a chip beside it would be a lie", n)
	}
	d.Shelf.NeedsBackend = true
	if n := strings.Count(render(t, d), chip); n != 1 {
		t.Errorf("Shelf=true: %d chips; want exactly the Shelf's", n)
	}
}

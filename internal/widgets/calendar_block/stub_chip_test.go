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

	// Wave 1's signed honesty state: both flags true, one chip per zone.
	// Layers hold no needzone keys so the count isolates the two stubs.
	d := fxHarptos(true)
	d.Layers = LayerState{Enabled: []string{"moons"}}
	if n := strings.Count(render(t, d), chip); n != 2 {
		t.Errorf("NeedsBackend=true on both stubs: %d chips; want one on the Ledger and one on the Shelf", n)
	}

	// W-B fills the Ledger, W-E fills the Shelf: flag false, chip gone,
	// zone still docked.
	d.Ledger.NeedsBackend = false
	d.Shelf.NeedsBackend = false
	filled := render(t, d)
	mustContain(t, filled, `data-zone="ledger"`, "a filled Ledger still docks its zone")
	mustContain(t, filled, `data-zone="shelf"`, "a filled Shelf still docks its zone")
	if n := strings.Count(filled, chip); n != 0 {
		t.Errorf("NeedsBackend=false on both stubs: %d chips still render; the flag gates the chip (data.go ruling)", n)
	}

	// The two zones gate independently — each chip answers only its own flag.
	d.Ledger.NeedsBackend = true
	if n := strings.Count(render(t, d), chip); n != 1 {
		t.Errorf("Ledger=true, Shelf=false: %d chips; want exactly the Ledger's", n)
	}
	d.Ledger.NeedsBackend = false
	d.Shelf.NeedsBackend = true
	if n := strings.Count(render(t, d), chip); n != 1 {
		t.Errorf("Ledger=false, Shelf=true: %d chips; want exactly the Shelf's", n)
	}
}

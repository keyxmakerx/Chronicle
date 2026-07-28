package calendar_block

// moons_badge_test.go — renderer behaviour of the Nameplate's declared-moon
// badge, on hand-written BlockData (seam discipline: whether the PRODUCER
// populates MoonsDeclared is asserted producer-side, in
// internal/plugins/calendar/block_seam_test.go).

import "testing"

// TestMoonsBadge_StatesTotalOnlyPastTheGridCeiling pins the r51 acceptance
// shape ("3 of 4 moons"): the badge renders IFF the calendar declares more
// moons than the grid's moonCap ceiling draws, and a calendar declaring three
// or fewer states nothing extra. The number comes from
// MonthGeometry.MoonsDeclared — the per-cell Moons are already capped
// (data.go) and cannot supply it.
func TestMoonsBadge_StatesTotalOnlyPastTheGridCeiling(t *testing.T) {
	// fxHarptos declares len(fxMoons)+1 = 4 moons; the grid draws moonCap = 3.
	over := render(t, fxHarptos(true))
	mustContain(t, over, `class="badge need moons"`,
		"a calendar declaring more moons than the grid draws must state the total")
	mustContain(t, over, ">3 of 4 moons<",
		"the badge states drawn-of-declared, the signed shape")

	at := fxHarptos(true)
	at.Month.MoonsDeclared = moonCap
	mustNotContain(t, render(t, at), "moons</span>",
		"a calendar declaring exactly the ceiling states nothing extra")

	zero := fxHarptos(true)
	zero.Month.MoonsDeclared = 0
	mustNotContain(t, render(t, zero), "moons</span>",
		"an unset total must never render a fabricated '3 of 0 moons'")

	// The badge is layer-gated with the discs it explains (the signed
	// MOONS_ON() gate): with the moons layer off the grid draws no moons at
	// all, and "3 of 4" would be a claim about a surface that is not there.
	off := fxHarptos(true)
	off.Layers.Enabled = []string{"eras", "weeknums", "ledger", "shelf"}
	mustNotContain(t, render(t, off), "moons</span>",
		"the declared-moon badge answers the moons layer key like the discs it explains")
}

// TestMoonsBadge_TailNamesTheAlmanacOnlyWhereItIsReachable is the restore
// C-CALV4-SEAM-P5 §4.8 booked BY NAME for this slice, with both of the two
// conditions stage 15 attached to it.
//
// The signed title is "N moons declared; the grid draws 3 — all of them are in
// the Almanac" (cv4:1653-1655). Stage 15 shipped the sentence WITHOUT its tail,
// deliberately, because the Almanac did not exist and a hover that points at an
// unbuilt surface is a small lie. W-E built it — so the tail returns, and it
// returns gated, because "the Almanac exists" and "this render draws one" are
// different claims.
//
// A Block with the Shelf hidden is not a hypothetical: it is the Bench's
// real-world Block (noShelf), which renders beside three others that do have
// one.
func TestMoonsBadge_TailNamesTheAlmanacOnlyWhereItIsReachable(t *testing.T) {
	const tail = "all of them are in the Almanac"

	// Reachable: the moons layer is on, the calendar declares four bodies and
	// the Shelf docks. The sentence is true, so it is printed.
	on := fxAlmanac(t, true)
	mustContain(t, render(t, on), tail,
		"with the Almanac reachable the signed tail is TRUE and is restored")
	mustContain(t, render(t, on), "4 moons declared; the grid draws 3",
		"the tail is an addition to the shipped sentence, not a replacement")

	// The HOST removed the zone — the Bench's real-world Block.
	hidden := fxAlmanac(t, true)
	hidden.Shelf.Hidden = true
	mustNotContain(t, render(t, hidden), tail,
		"a Block with no Shelf must not promise a surface it is not rendering")
	mustContain(t, render(t, hidden), "4 moons declared; the grid draws 3",
		"the ceiling is still explained; only the pointer to the Almanac drops")

	// The VIEWER turned the shelf layer off.
	off := fxAlmanac(t, true)
	off.Layers.Enabled = []string{"moons", "eras", "weeknums", "ledger"}
	mustNotContain(t, render(t, off), tail,
		"the shelf layer key is the viewer's half of reachable, and it gates the tail too")

	// The badge's own moons-layer gate is NOT weakened by any of this: with the
	// moons layer off there is no badge at all, so there is nothing to tail.
	noMoons := fxAlmanac(t, true)
	noMoons.Layers.Enabled = []string{"eras", "weeknums", "ledger", "shelf"}
	body := render(t, noMoons)
	mustNotContain(t, body, "moons</span>", "the badge answers the moons layer, as it always did")
	mustNotContain(t, body, tail, "and so does its tail")
}

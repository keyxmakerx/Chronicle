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

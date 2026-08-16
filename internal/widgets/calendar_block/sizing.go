// sizing.go — THE SIZING CONTRACT, EXPRESSED IN GO SO IT CAN BE TESTED.
//
// ────────────────────────────────────────────────────────────────────────────
// NOTHING IN THE RENDER PATH CALLS THIS FILE. That is deliberate.
//
// The signed mockup (cordinator mockups/calendar-v4.html) computes size class
// and density in JavaScript. On this platform that implementation is broken by
// construction: boot.js sets `htmx.config.allowScriptTags = false`, so a
// <script> inside an HTMX-swapped fragment never executes and a JS-sized Block
// silently renders at the wrong density after any swap. Size class and density
// are therefore CSS container queries (static/css/calendar-block.css), and the
// producer never decides a tier.
//
// This file exists so the contract's arithmetic is pinned by a Go test rather
// than by a comment: sizing_test.go reproduces the divergeSheet pair
// (calendar-v4.html:2504) exactly — Harptos 10 columns at host 1040 measures
// 68.2px and falls to underline; Gregorian 7 columns at the same host measures
// 97.4px and earns names.
//
// KNOWN CONTRACT DEFECT, CARRIED NOT FIXED: sizeSheet's caption
// (calendar-v4.html:2483) claims "10 columns at 100.2px" at host 1040. It is
// arithmetically wrong — ColWidth(TierFull, 1040, 10) is 68.2 — and the sheet's
// own rendered Block shows underline density, agreeing with the arithmetic and
// not with its caption. divergeSheet is authoritative (COMMON §1: the contract
// is immutable; an executor that believes a render is wrong stops and flags).
package calendar_block

// Size-class names. These are the four classes the Block's container queries
// switch on; they are strings here only so the test table reads like the
// mockup's.
const (
	TierFull    = "full"
	TierStd     = "std"
	TierMini    = "mini"
	TierSubmini = "submini"
)

// Size-class thresholds, in HOST pixels (calendar-v4.html:1423-1428). These are
// the same numbers the container queries in calendar-block.css use; the CSS is
// the implementation and this is the pin.
const (
	HostFullMin = 900
	HostStdMin  = 300
	HostMiniMin = 240
)

// NamedColWidthMin is the density flip, in MEASURED COLUMN pixels
// (calendar-v4.html:1438). Below it a cell carries a presence underline; at or
// above it a cell can honestly carry event names.
//
// In CSS this is `@container cal-cell (min-width: 84px)`, measured against the
// real rendered column rather than against the model below — see
// calendar-block.css §DENSITY for the ~1.2px delta that produces and why it is
// harmless (it moves the flip point by ~12px of host width, and every signed
// still lands on the same side of it under both).
const NamedColWidthMin = 84.0

// ── THE CELL'S THREE COLUMN THRESHOLDS ──────────────────────────────────────
//
// THERE USED TO BE ONE, AT 40px, AND ONE WAS THE DEFECT.
//
// The single `MoonRowColWidthMin = 40` gated the whole three-disc row: a column
// that could not hold three discs got NONE. That was defensible while the discs
// were an all-or-nothing ornament, and it stopped being defensible the moment
// C-CALV4-SPEC §4 made the primary body an ALWAYS-VISIBLE silhouette.
//
// It was also, measured against the operator's actual calendar, already wrong.
// Harptos is a TEN-day week. Its column measures 30.0px at a 360px phone,
// 33.0px at 390px and 37.0px at 430px — every one of them under 40 — so the
// operator's phone drew no moons at all, at any width, on the only calendar
// they use. The census below is the measurement; both columns of it are
// re-measured in a browser by moon_reach_probe_test.go.
//
//	viewport   7-day column   10-day column
//	   360px       43.3            30.0
//	   390px       47.6            33.0
//	   430px       53.3            37.0
//	   768px       62.7            43.6
//	  1024px       99.3            67.2
//	  1280px       93.0            62.8
//	  1440px      115.9            78.8
//	  1920px      121.0            82.4
//
// (1024 exceeds 1280 because the pinned 256px sidebar takes layout width from
// the `md` breakpoint up while .cal-bench's 1180px cap has not yet bitten.)
//
// MoonSilhouetteColWidthMin is the width the ALWAYS-VISIBLE primary silhouette
// needs. In CSS it is `@container cal-cell (min-width: 30px)`.
//
// WHY 30, AND WHY IT IS A REAL BOUNDARY RATHER THAN A FORMALITY. It is the STD
// tier's column FLOOR: `.grid` declares `min-width: calc(var(--week-len) *
// 30px)`, so a std-tier column is 30px before the grid overflows its host
// instead of shrinking further, and the 360px / ten-day census row measures
// exactly 30.0px — that floor being hit. Every viewport a person can hold is at
// or above it, which is what makes "always visible" true where the operator
// lives.
//
// The MINI tier rewrites that floor to 24px (§SIZE CLASS), and that is the case
// this threshold degrades. A ten-column month in a 240–300px host measures
// 24.0–28.3px and cannot hold a date and a moon on one line at all, so it draws
// no moon — in a tier that already drops the weekday header, the era bands, the
// dogear and the event names for exactly the same reason. A SEVEN-column month
// at the same tier reaches 40.4px, clears this threshold and keeps its
// silhouette.
//
// Written as an unconditional baseline instead, an 8px disc would land across
// the date of every ten-day mini-tier Block — trading "no moon" for "a moon on
// top of the date", which is the worse of the two defects.
const MoonSilhouetteColWidthMin = 30.0

// CellCompactColWidthMin is where the DATE gets its full type size back. In CSS
// it is `@container cal-cell (min-width: 35px)`.
//
// WHY 35. The date sits at the cell's left and the silhouette at its right, so
// both fit only when
//
//	left padding 3 + date ink 16.7 + clearance 2 + disc 10 + right inset 4 = 35.7
//
// Every figure there is measured in the browser rather than assumed. The date's
// ink is 16.7px — two digits at 12px/600 — and NOT the ~14px an earlier pass of
// this very file guessed. The census leaves a clean gap around 35.7: 33.0px
// (ten-day at a 390px phone) is the widest column that cannot afford both and
// 37.0px (ten-day at 430px) the narrowest that can, so 35 is that gap's midpoint
// — the same method the retired 40px number was derived by.
//
// THE TODAY STAMP WAS A TERM IN THIS ARITHMETIC AND IS NOT ANY MORE
// (C-CALV4-TILES §9.1). It was a FILLED DISC, 18.0px against two digits' ~13px —
// the widest thing a date could be, which is how the first build of this cell
// put a moon across day 14 while day 3 looked fine, and why three stamp
// diameters (13 / 14 / 22px) used to step with the column. Today is now the
// NUMERAL ITSELF in --today-ink at weight 750: the disc read as a redaction over
// the date, and an inked numeral is exactly as wide as the same numeral in body
// ink. So today widens nothing, all three diameters are deleted from the
// stylesheet, and 16.7px is the whole of the date's inline cost at every
// density. The 35.7 above does not move — it never counted the stamp.
//
// BELOW IT THE DATE STEPS DOWN, NOT THE MOON, and that ordering is the spec's
// rather than a convenience: C-CALV4-SPEC §4 makes the silhouette
// unconditional, so at 30–33px it is the date's type size that yields (10px).
// The mini tier already sets that precedent at 11px.
const CellCompactColWidthMin = 35.0

// MoonExpandColWidthMin is the width the HOVER/FOCUS EXPANSION needs — up to
// three discs plus the `+` when the calendar declares more. In CSS it is
// `@container cal-cell (min-width: 75px)`.
//
// WHY 75. Measured, and the first attempt at it was wrong in a way only the
// browser could show:
//
//	3 discs × 10px           30.0
//	2 gaps  ×  2.5px          5.0
//	the trailing gap          2.5
//	the `+` glyph             7.5   (9px/700 in the Block's stack)
//	                        ─────
//	the expanded row         45.0   ← measured, not summed
//	right inset               4.0
//	clearance                 2.0
//	the date's ink           16.7   (12px/600, two digits)
//	left padding              3.0
//	                        ─────
//	                         70.7
//
// A first pass put this at 62, on the arithmetic for a row with NO `+` (35.0px
// measured), and the probe immediately found the expanded row lying across the
// date at 62.8px and 67.2px — both ten-day weeks, by 82px² and 38px². The `+`
// is not optional decoration: it costs 10px of inline space, and the threshold
// has to cover the widest form the row can take rather than the commonest.
//
// The census leaves the gap 67.2px → 78.8px around 70.7, and 75 sits inside it
// with 4.3px of margin over the requirement and 3.8px under the next real
// measurement.
//
// BELOW IT THE SILHOUETTE STILL RESTS THERE AND THE PANEL STILL OPENS IN ONE
// TAP. That is what makes this a degradation rather than the old defect wearing
// a new number: the information has a reachable home at every width, which is
// exactly the test the 40px query failed.
//
// NOTHING RESIZES AT THIS THRESHOLD. The disc diameter steps at
// CellCompactColWidthMin instead, so revealing two more discs never changes the
// size of the one already under the pointer — which would be movement, in a
// sheet that forbids it, arriving through a hover rule.
//
// None of the three is derived from NamedColWidthMin. They answer different
// questions and a change to one must not silently move the others.
const MoonExpandColWidthMin = 75.0

// Chrome the full-tier column arithmetic subtracts (calendar-v4.html:1429-1434).
//
//   - instPad      — the instrument's own horizontal padding.
//   - ledgerDock   — the docked Ledger. Subtracted UNCONDITIONALLY at full tier,
//     which is exactly why the full-tier form of ColWidth may not be reused at
//     std/mini: there the Ledger is not beside the month. Wave 1 docks the
//     Ledger zone at full tier as a `needs backend` stub, so the subtraction is
//     real from the first commit.
//   - blockPad     — the Block's own border/padding budget.
//   - weekGutter   — the week-number gutter, present only above 480px of host.
const (
	instPad         = 24
	ledgerDock      = 300
	blockPad        = 16
	weekGutter      = 18
	gutterHostMin   = 480
	stdInstrumentPx = 220
)

// SizeClass maps a host width to a size class (calendar-v4.html:1423).
func SizeClass(hostW int) string {
	switch {
	case hostW >= HostFullMin:
		return TierFull
	case hostW >= HostStdMin:
		return TierStd
	case hostW >= HostMiniMin:
		return TierMini
	default:
		return TierSubmini
	}
}

// ColWidth is the contract's model of one column's measured width
// (calendar-v4.html:1429).
//
// The full-tier form subtracts the docked Ledger; the std form does not,
// because at std the Ledger sits BELOW the month rather than beside it. Reusing
// the full-tier form at std would under-report every column by 30px per column
// on a ten-day week.
func ColWidth(tier string, hostW, week int) float64 {
	if week <= 0 {
		return 0
	}
	gutter := 0
	if hostW > gutterHostMin {
		gutter = weekGutter
	}
	switch tier {
	case TierFull:
		return float64(hostW-instPad-ledgerDock-blockPad-gutter) / float64(week)
	case TierStd:
		return float64(hostW-blockPad-gutter) / float64(week)
	default:
		return float64(hostW-blockPad) / float64(week)
	}
}

// IsNamed answers the contract's density question for a given host.
//
// DELIBERATE IMPLEMENTATION DIVERGENCE (flagged in the PR, per COMMON §9): the
// mockup's isNamed() carries a SECOND clause — `instrument / rowCount >= 68`,
// where `instrument` is a hardcoded 470px at full tier against a MEASURED 563px
// (canon amendments §D; wrong by ~93px since design-notes §9 deviation 4). It is
// a known, booked, deliberately-unfixed defect that errs safe.
//
// The CSS retires that clause rather than inheriting it. A container query
// measures the real column, and vertical fit is settled by real layout —
// `.block.full{max-height:780px}` plus the instrument's own overflow — instead of
// by a constant that was never right. Correcting the constant to 563 was
// explicitly refused: it would flip renders and desynchronise the signed stills.
//
// This function keeps the mockup's two-clause form so the divergence is VISIBLE
// and testable (see TestIsNamed_RetiredRowHeightClause), using the mockup's own
// 470/220 constants. The CSS implements only the first clause.
func IsNamed(tier string, hostW, week, rowCount int) bool {
	if tier == TierMini || tier == TierSubmini {
		return false
	}
	if ColWidth(tier, hostW, week) < NamedColWidthMin {
		return false
	}
	return mockupRowHeightClause(tier, rowCount)
}

// IsNamedCSS is what the shipped stylesheet actually implements: the measured
// column-width clause alone. Kept beside IsNamed so the two can be diffed by a
// test rather than by eye.
func IsNamedCSS(tier string, hostW, week int) bool {
	if tier == TierMini || tier == TierSubmini {
		return false
	}
	return ColWidth(tier, hostW, week) >= NamedColWidthMin
}

// mockupRowHeightClause is calendar-v4.html:1439-1440, verbatim constants and
// all. It is referenced only by IsNamed and by the divergence test.
func mockupRowHeightClause(tier string, rowCount int) bool {
	instrument := stdInstrumentPx
	if tier == TierFull {
		instrument = 470 // the booked defect; measured is 563 (canon §D)
	}
	if rowCount <= 0 {
		rowCount = 3
	}
	return float64(instrument)/float64(rowCount) >= 68
}

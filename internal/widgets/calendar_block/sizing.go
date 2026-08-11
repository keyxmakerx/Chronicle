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

// MoonRowColWidthMin is the SECOND density threshold — the one the per-day moon
// discs get for themselves, in MEASURED COLUMN pixels. In CSS it is
// `@container cal-cell (min-width: 40px)`.
//
// WHY THERE ARE TWO. The discs shipped behind NamedColWidthMin, sharing one
// query with the named-event subtree they happened to be nested inside, and
// that gate was never right for them: three phase discs measure 35.0px of
// inline space and an event NAME measures a great deal more. The consequence
// was not a cramped cell but an absent feature — a census of twenty real
// viewport/week-length pairs (moon_reach_probe_test.go) found the discs painted
// in six, none of them a phone, and none of them a ten-day week at any monitor
// size, because .cal-bench caps the Block at 1180px and a ten-day column tops
// out at 82.4px there.
//
// WHY 40. It is read off that census rather than chosen: 37.0px is the widest
// column that must NOT draw the row (a ten-day week on a 430px phone) and
// 43.3px the narrowest that must (a seven-day week on a 360px phone), so 40 is
// the midpoint of the only gap the measurements leave. It independently clears
// the row's own measured 35.0px with 2.5px of clearance each side.
//
// It is deliberately NOT derived from NamedColWidthMin. The two answer
// different questions and a future change to one must not silently move the
// other.
const MoonRowColWidthMin = 40.0

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

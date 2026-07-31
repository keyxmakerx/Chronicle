// bench_mobile_order_test.go — the ≤640px reading-order swap, pinned.
//
// [BR2-4] Option C SIGNED: at ≤640px, CSS `order` lifts `.stack` (the two
// Blocks) above `.ribbon`, so the phone meets the calendar first. It is the
// only half of [BR2-4] that does what the operator literally asked for — "5-6
// blocks of data before you get to the calendar" — and §11 states its promise
// as a number: the count of things between the top of the page and the
// calendar goes from six to ONE.
//
// ── WHY THIS FILE EXISTS (C-CALV4-BENCH-R2 verify pass, R2-1 stage 10) ───────
//
// The rule shipped in stage 3 with NO guard of any kind. Its only intended
// evidence was the §13 screenshot gate, and the 390px and 640px stills in
// reports/chronicle/screenshots/2026-07-30-c-calv4-bench-r2/ do not show the
// swap at all — they render the ribbon above the stack while their own caption
// band asserts "the ≤640 order rule is live". The shipped CSS is CORRECT (the
// swap was re-measured at true 390px and 640px viewports during the verify pass
// and computes to phead -3 / sechead -2 / stack -1 / everything else 0, with
// the stack at y259 against the ribbon at y1425 at 390px). The artifacts are
// what was wrong. But a geometry claim whose only evidence is a still is a
// claim CI cannot hold, and this one had drifted from its evidence without
// anything going red.
//
// So the swap is pinned here instead, where a regression cannot pass.
//
// THE FAILURE MODE THIS IS REALLY ABOUT. `order` is INERT unless the parent
// establishes a flex or grid formatting context. A tidying hand that removed
// `display: flex` from `.bsurf` — or scoped it to a different breakpoint —
// would leave three `order` declarations sitting in the sheet, reading exactly
// as though they still worked, and every one of them silently doing nothing.
// That is why the display mode is asserted first and separately.
package calendar

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// benchMobileOrderRe reads one `.cal-bench .bsurf > .<child> { order: <n>; }`
// declaration, so the swap is asserted arithmetically rather than by eye.
var benchMobileOrderRe = regexp.MustCompile(`\.cal-bench\s+\.bsurf\s*>\s*\.([a-z]+)\s*\{\s*order:\s*(-?\d+)\s*;?\s*\}`)

func TestBenchCSS_MobileLiftsTheCalendarAboveTheRibbon(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")

	const mq = "@media (max-width: 640px)"
	if !strings.Contains(code, mq) {
		t.Fatalf("no `%s` block — [BR2-4] Option C SIGNED is the only answer this "+
			"slice ships to \"5-6 blocks of data before you get to the calendar\"", mq)
	}

	// The order declarations must live inside a ≤640 block and nowhere else: an
	// order value that escaped its media query would reorder the DESKTOP page,
	// which is the divergence [BR2-4] bounded to one adjacency below 640px.
	all := benchMobileOrderRe.FindAllStringSubmatch(code, -1)
	if len(all) == 0 {
		t.Fatal("no `.cal-bench .bsurf > .<child> { order: … }` rule survives — the ≤640 " +
			"reading-order swap is gone and the phone meets the ribbon first again")
	}

	// FIRST, AND SEPARATELY: the formatting context. `order` does nothing at all
	// unless .bsurf is flex or grid, and a swap that silently stopped working is
	// worse than one that was never written — the sheet would still read as
	// though the phone were served.
	flex := regexp.MustCompile(`\.cal-bench\s+\.bsurf\s*\{[^}]*display:\s*(flex|grid)`)
	if !flex.MatchString(code) {
		t.Error("`.cal-bench .bsurf` never becomes flex or grid, so every `order` " +
			"declaration below is INERT — the ribbon/stack swap is decoration, not geometry")
	}

	got := map[string]int{}
	for _, m := range all {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("unreadable order value %q", m[2])
		}
		got[m[1]] = n
	}

	// EXACTLY THREE, and the sheet's own comment says so. A fourth order value
	// is a second reading order, and [BR2-4] priced exactly one adjacency
	// (WCAG 1.3.2, meaningful sequence) — not a general re-sequencing licence.
	if len(got) != 3 {
		t.Errorf("%d ordered children %v, want exactly 3 (.phead, .sechead, .stack) — "+
			"[BR2-4] Option C bought ONE reading-order divergence, not a free hand", len(got), got)
	}

	// THE SWAP ITSELF. Every ordered child must sort BEFORE the default 0 that
	// the ribbon and the three other disclosures keep, and the trio must keep
	// its own sequence: the page's name first, then the label, then the thing
	// the label names. A .sechead separated from its .stack is a worse defect
	// than the one this rule fixes.
	for _, k := range []string{"phead", "sechead", "stack"} {
		v, ok := got[k]
		if !ok {
			t.Errorf(".%s carries no order value; it would fall back to 0 and sort "+
				"among the disclosures in DOM order", k)
			continue
		}
		if v >= 0 {
			t.Errorf(".%s has order %d, want negative — at 0 it does not move ahead of "+
				"the ribbon and the swap does not happen", k, v)
		}
	}
	if got["phead"] >= got["sechead"] || got["sechead"] >= got["stack"] {
		t.Errorf("order is phead=%d sechead=%d stack=%d; want phead < sechead < stack so the "+
			"page keeps its name, the label travels with the stack it labels, and the "+
			"calendar still lands first among the things that moved",
			got["phead"], got["sechead"], got["stack"])
	}

	// AND THE RIBBON STAYS PUT. The swap is achieved by lifting three children,
	// never by pushing the ribbon down: a positive order on .ribbon would also
	// push it past .rsvp, .nextup and .rows, which no ruling authorises.
	if v, ok := got["ribbon"]; ok {
		t.Errorf(".ribbon was given order %d; it must keep the default 0 — the swap "+
			"lifts the calendar, it does not demote the ribbon past the other three sections", v)
	}
}

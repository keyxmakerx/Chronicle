// availability_strip_test.go — THE STRIP IS BUILT AND IT IS DORMANT
// (C-CALV4-TILES §8.4 / §8.9 / §9.5).
//
// The strip is a TIME AXIS for the day: its width is the day, a span's position
// says when and its length says how long, one lane per player under one accent
// lane carrying the overlap. All of that now exists in the stylesheet, complete
// — and NOTHING may paint from it, because there is no data.
//
// WHY DORMANT IS THE HARD PART TO TEST, AND WHY IT GETS ITS OWN FILE. A feature
// that cannot run is normally proved by its absence; this one is proved by its
// PRESENCE plus the absence of any path from real data to a painted pixel.
// `DayCell` has no availability field, `block_projection.go` fetches none, and
// the windows live in the sessions plugin's `availability_exceptions` keyed to
// REAL dates — §8.6's fantasy-date blocker is unresolved, so on the operator's
// own calendar there is nothing to draw and a lane drawn anyway would be a
// fabricated figure.
//
// Three separate things hold that, deliberately, because one would be an
// accident: the geometry switch is on `.cell.rsvp`, a class no markup emits;
// `.lane` matches no element; and --m0/--m1 carry no fallback, so an unset span
// has no width rather than a full-day one. Each is asserted below, and the
// PIXEL half — that a full render of the current fixtures paints zero lane
// pixels — is availability_probe_test.go, in a real engine.
package calendar_block

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── the markup ──────────────────────────────────────────────────────────────

// TestAvailability_NoFixtureEmitsALaneOrAnRSVPDay.
//
// The whole dormancy rests on two class names never being emitted. This walks
// every fixture in the package, at both roles, and looks for either. A single
// producer that started stamping `rsvp` would light the strip on live data with
// no data behind it, which is the one outcome §9.5 forbids by name.
func TestAvailability_NoFixtureEmitsALaneOrAnRSVPDay(t *testing.T) {
	fixtures := map[string]BlockData{
		"almanac · GM":     fxAlmanac(t, true),
		"almanac · player": fxAlmanac(t, false),
		"harptos · GM":     fxHarptos(true),
		"harptos · player": fxHarptos(false),
		"gregorian":        fxGregorian(),
		"dwarven · fault":  fxDwarvenFault(),
		"eras off":         fxErasOff(t),
		"long month":       fxLongMonth(t),
	}
	for name, d := range fixtures {
		t.Run(name, func(t *testing.T) {
			body := render(t, d)
			// The subject must exist, or the two absences below are vacuous:
			// a Block that stopped emitting the slot entirely would pass an
			// absence-only guard while having deleted the reservation.
			if !strings.Contains(body, `class="avail"`) {
				t.Fatal("no availability slot in this render at all — the reservation is " +
					"unconditional by design, and without it the assertions below prove nothing")
			}
			for _, bad := range []string{`class="lane`, ` lane"`, `rsvp`} {
				if strings.Contains(body, bad) {
					t.Errorf("the render emits %q. The strip is BUILT AND DORMANT: DayCell "+
						"carries no availability field, block_projection.go fetches none, and "+
						"the windows are keyed to REAL dates behind §8.6's blocker. A lane "+
						"drawn from nothing is a fabricated figure", bad)
				}
			}
		})
	}
}

// ── the stylesheet ──────────────────────────────────────────────────────────

// TestCSS_AvailabilityStripIsCompleteAndCannotPaintYet.
//
// TWO DIRECTIONS, and both are needed. An absence-only guard passes on a build
// that never shipped the strip; a presence-only guard passes on a build that
// ships it painting placeholder spans. §9.5 asks for exactly one of the four
// possible states — built AND inert — so both halves are asserted here.
func TestCSS_AvailabilityStripIsCompleteAndCannotPaintYet(t *testing.T) {
	code := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(code), " ")

	// ── BUILT ────────────────────────────────────────────────────────────
	for _, part := range []struct{ want, why string }{
		{".cal-block-host .cell.rsvp .avail",
			"the strip's own geometry: the lane stack, inset to the tile's inner edge"},
		{".cal-block-host .cell .lane {",
			"one lane per player — the track their window is measured against"},
		{".cal-block-host .cell .lane > i {",
			"the window itself, positioned on the day"},
		{".cal-block-host .cell .lane.all {",
			"the overlap lane. 'When can we all play' is the only question this surface " +
				"exists to answer, so the intersection is DRAWN rather than eyeballed"},
		{".cal-block-host .cell .lane.all.none > i { display: none",
			"nobody overlaps: the lane stays, empty. A missing lane reads as 'no data' " +
				"and an empty one as 'no window', and those are different answers"},
	} {
		if !strings.Contains(flat, part.want) {
			t.Errorf("the strip is missing %q — %s", part.want, part.why)
		}
	}

	// FLUSH LANES, and this is the operator's own correction: "no space between
	// them because they are colored." A gutter is a separator doing a job the
	// hue already does, and it spends a quarter of a 12px strip on nothing.
	stripRule := cssRuleBody(code, ".cal-block-host .cell.rsvp .avail")
	if stripRule == "" {
		t.Fatal("no strip rule to read")
	}
	if !strings.Contains(strings.Join(strings.Fields(stripRule), " "), "gap: 0") {
		t.Error("the lane stack does not declare `gap: 0`. The lanes ABUT — a 1px rule " +
			"between two coloured lanes is a separator doing a job the hue already does")
	}
	// THE TRACK IS SQUARE. A pill track between two flush neighbours prints a
	// light notch at each end, which reads as exactly the gutter just removed.
	laneRule := strings.Join(strings.Fields(cssRuleBody(code, ".cal-block-host .cell .lane")), " ")
	if !strings.Contains(laneRule, "border-radius: 0") {
		t.Errorf("the per-lane track is not square (%q). A rounded track between two flush "+
			"neighbours prints a notch at each end that reads as the gap the operator "+
			"asked to remove", laneRule)
	}

	// ── IT CANNOT PAINT ──────────────────────────────────────────────────
	//
	// 1. The geometry switch is gated on a class nothing emits. Written on
	//    `.avail` itself it would replace the reservation cell_probe_test.go
	//    measures — an empty box at the cell's inner bottom edge — with a
	//    zero-height flex container, and the strip would be "built" by having
	//    deleted what it was meant to fill.
	if strings.Contains(flat, ".cal-block-host .avail { position: absolute; left: 0; right: 0; bottom: 0; height: var(--avail-h)") == false {
		t.Error("the empty reservation's own rule has changed. The strip must render " +
			"EXACTLY as it does today on current data; cell_probe_test.go measures that " +
			"box's height, its offset from the cell's inner bottom edge and its width")
	}
	// 2. --m0/--m1 CARRY NO FALLBACK. /schedule's `.sc-off` writes
	//    `var(--m0, 0%)` / `var(--m1, 100%)` and can afford to, because it
	//    always computes the pair. Here a defaulted span would paint a
	//    FULL-DAY bar out of no data at all.
	spanRule := strings.Join(strings.Fields(cssRuleBody(code, ".cal-block-host .cell .lane > i")), " ")
	for _, bad := range []string{"var(--m0,", "var(--m1,"} {
		if strings.Contains(spanRule, bad) {
			t.Errorf("the span declares a fallback (%q in %q). Unset, the declaration must "+
				"be invalid at computed-value time so left/right fall back to `auto` and the "+
				"span has no width. A defaulted span paints a full-day window out of nothing, "+
				"which is the one thing this surface must never draw", bad, spanRule)
		}
	}
	if !strings.Contains(spanRule, "left: var(--m0)") || !strings.Contains(spanRule, "var(--m1)") {
		t.Errorf("the span does not ride --m0/--m1 (%q). That is the SAME contract "+
			"/schedule's `.sc-off` ships — one span primitive in the product, not two "+
			"(C-CALV4-DRAG-RESCHEDULE.md:172)", spanRule)
	}

	// ── THE DAY IS THE TARGET, NOT THE LANE (§8.7) ───────────────────────
	//
	// A 3px lane against a 44px floor is not a control, and a third hit region
	// inside one cell breaks the two-target rule. Tapping the day already opens
	// the Ledger and the detail lands there.
	availRule := strings.Join(strings.Fields(cssRuleBody(code, ".cal-block-host .avail")), " ")
	if !strings.Contains(availRule, "pointer-events: none") {
		t.Errorf("the strip is not pointer-events: none (%q). §8.7: the bar is a MARK and "+
			"the DAY is the target — a 4px hit area against a 44px floor is not a control, "+
			"and a third target in one cell is what the two-target rule refuses", availRule)
	}

	// ── THE PHONE COLLAPSE, ON THE LADDER'S EXISTING RUNG ────────────────
	//
	// Below 75px the player lanes drop and the overlap alone survives: three
	// 3px lanes plus the overlap need the 85px cell, and at 34px the overlap is
	// the one line a GM can act on. Written as the sheet's existing min-width
	// rung rather than a `max-width: 74px` twin, so the boundary is spelled
	// once — the same reasoning TestCSS_TheFiveColumnRuleHasExactlyTwoOwners
	// applies to a rule declared "twice at two strengths".
	if !strings.Contains(flat, ".cal-block-host .cell .lane:not(.all) { display: none") {
		t.Error("the player lanes are not dropped by default — the phone is the baseline " +
			"in this sheet and the wide cell is the enhancement")
	}
	rung, _, ok := splitAtRuleBlock(code, "@container cal-cell (min-width: 75px)")
	if !ok {
		t.Fatal("the 75px rung is gone")
	}
	if !strings.Contains(strings.Join(strings.Fields(rung), " "),
		".cal-block-host .cell .lane:not(.all) { display: block }") {
		t.Error("the player lanes are not restored at the 75px rung. Below it the strip is " +
			"the overlap alone (§8.9); above it, one lane per player")
	}
	if strings.Contains(code, "@container cal-cell (max-width: 74px)") {
		t.Error("the 75px boundary is spelled a second time as `max-width: 74px`. One " +
			"boundary, one number, one place — two spellings is how they come to disagree")
	}

	// ── AND NOTHING ABOUT IT MOVES ───────────────────────────────────────
	for _, sel := range []string{
		".cal-block-host .cell.rsvp .avail",
		".cal-block-host .cell .lane",
		".cal-block-host .cell .lane > i",
		".cal-block-host .cell .lane.all",
	} {
		body := cssRuleBody(code, sel)
		for _, bad := range []string{"transition", "animation"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s declares %q — the month grid never moves, and a lane that "+
					"animated into place would encode a magnitude the data does not have",
					sel, bad)
			}
		}
	}
}

// TestCSS_TheSpanPrimitiveIsSharedWithSchedule. C-CALV4-DRAG-RESCHEDULE.md:172
// warns against growing a SECOND span primitive, and §8.5 answers it by naming
// /schedule's `.sc-off` as the one to reuse. This reads BOTH sheets, because the
// claim is about two files agreeing and a single-file assertion cannot see it.
func TestCSS_TheSpanPrimitiveIsSharedWithSchedule(t *testing.T) {
	block := stripComments(blockCSS(t))
	schedPath := filepath.Join(filepath.Dir(blockCSSPath(t)), "calendar-schedule.css")
	raw, err := os.ReadFile(schedPath) //nolint:gosec // repo file, path derived from the block sheet's
	if err != nil {
		t.Fatalf("read %s: %v", schedPath, err)
	}
	sched := string(raw)

	if !strings.Contains(sched, "--m0") || !strings.Contains(sched, "--m1") {
		t.Fatalf("%s no longer positions its spans on --m0/--m1 — the Block's strip was "+
			"built on that contract precisely so there is one span primitive in the "+
			"product and not two", schedPath)
	}
	for _, want := range []string{"--m0", "--m1"} {
		if !strings.Contains(block, want) {
			t.Errorf("the Block's availability strip does not use %s. It is the SAME "+
				"percentage-of-the-day contract /schedule already ships; a second one would "+
				"be a second thing for the server to compute and a second thing to be wrong",
				want)
		}
	}
	// Both position the span the same way — left from --m0, right from the
	// complement of --m1. A strip that used width instead would drift the
	// moment one of the two learned about continuation caps.
	if !strings.Contains(strings.Join(strings.Fields(sched), " "), "right: calc(100% - var(--m1") {
		t.Error("/schedule no longer closes its span with `calc(100% - var(--m1…))`; the " +
			"Block's strip mirrors that construction and the two must not drift")
	}
}

// cssRuleBody returns the declaration body of the FIRST rule whose prelude is
// exactly sel (whitespace-normalised). Comment-stripped input only.
func cssRuleBody(code, sel string) string {
	want := strings.Join(strings.Fields(sel), " ")
	for _, m := range preludeRe.FindAllStringSubmatchIndex(code, -1) {
		if strings.Join(strings.Fields(code[m[4]:m[5]]), " ") != want {
			continue
		}
		rest := code[m[1]:]
		if end := strings.Index(rest, "}"); end >= 0 {
			return rest[:end]
		}
	}
	return ""
}

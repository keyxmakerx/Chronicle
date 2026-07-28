// ledger_test.go — Zone C's renderer behaviour, on hand-written BlockData.
//
// WHAT THIS FILE MAY AND MAY NOT ASSERT. It is a WIDGET-side test, so its
// fixtures are written by its own author: it can prove "given this BlockData,
// the renderer emits that", and it cannot prove anything about what a producer
// CHOSE to put in the BlockData. Every count claim, every permission claim and
// every "this number is reproducible from that viewer's own set" claim lives in
// package calendar instead (block_count_oracle_test.go, block_seam_test.go),
// for the reason block_seam_test.go's header sets out at length.
package calendar_block

import (
	"strings"
	"testing"
)

// ── the CSS-only day pick (§1, CTS-1 = Option A) ────────────────────────────

// fxLongMonth is a month PAST the ladder's bound: 45 ordinary days and 9
// intercalary ones, against a bound of 40 + 8. Nothing else in the suite
// reaches past it, and the over-bound behaviour is a signed ruling (CTS-2), not
// an implementation detail.
func fxLongMonth(t *testing.T) BlockData {
	t.Helper()
	d := fxHarptos(true)
	d.Month.Days = 45
	week := d.Month.WeekLen
	d.Month.Rows = nil
	for r := 0; r*week < d.Month.Days; r++ {
		row := WeekRow{Index: r}
		for c := 1; c <= week; c++ {
			day := r*week + c
			cell := DayCell{Col: c, Half: c == 5}
			if day <= d.Month.Days {
				cell.Day = day
			}
			row.Cells = append(row.Cells, cell)
		}
		d.Month.Rows = append(d.Month.Rows, row)
	}
	d.Month.RowCount = len(d.Month.Rows)
	d.Month.Intercalary = nil
	for i := 1; i <= 9; i++ {
		d.Month.Intercalary = append(d.Month.Intercalary,
			IntercalaryDay{Name: "Feast " + intText(i), Day: i})
	}
	return d
}

// TestDayPick_IsAPureCSSControl. Day answering must be operable with ZERO
// JavaScript and zero routes: a <script> inside an HTMX-swapped fragment never
// executes (boot.js:163), and the docked Ledger's whole promise is that
// choosing a day repaints a panel already on screen — a fetch would put latency
// inside that promise.
//
// What makes it operable is one radio group per Block, one radio per
// selectable day, each addressed by its own stretched label. Everything else is
// a stylesheet rule, pinned in css_contract_test.go.
func TestDayPick_IsAPureCSSControl(t *testing.T) {
	d := fxHarptos(true)
	body := render(t, d)

	// One control per day of a 30-day month, plus one per intercalary day.
	wantPicks := d.Month.Days + len(d.Month.Intercalary)
	if n := strings.Count(body, `class="daypick"`); n != wantPicks {
		t.Errorf("%d day radios for %d selectable days", n, wantPicks)
	}
	if n := strings.Count(body, `class="dsel"`); n != wantPicks {
		t.Errorf("%d stretched labels for %d radios — every radio needs a hit target", n, wantPicks)
	}

	// ONE TAB STOP. Every day option shares one radio group name, so the Block
	// takes a single tab stop and the arrow keys move the selection between
	// days — which is how a KEYBOARD answers, not just a pointer.
	name := dayPickGroupName(d)
	if n := strings.Count(body, `name="`+name+`"`); n != wantPicks {
		t.Errorf("the day options do not all share the group %q — two groups would be two "+
			"tab stops and two independent selections", name)
	}

	// Guard B3: a control's attribute ends in -pick and never reuses an <html>
	// state-marker name.
	if !strings.Contains(body, `data-day-pick="1"`) {
		t.Error("the ladder keys on data-day-pick; without it every rule in the sheet is inert")
	}
	for _, marker := range []string{"data-theme", "data-view", "data-role",
		"data-colour", "data-moonstyle", "data-ready"} {
		if strings.Contains(body, marker+`="`) {
			t.Errorf("guard B3: the day control reuses the <html> state marker %q", marker)
		}
	}

	// NOTHING IS CHECKED at render. "No day chosen" is the default, and the
	// `✕ all` option exists to RETURN to it — a radio cannot be un-checked by
	// another radio, which is why that head slot is reserved rather than
	// conditional.
	if strings.Contains(body, `class="daypick" data-day-pick="all" name="`+name+`" id="`+
		dayPickInputID(d, dayPickAll)+`" checked`) {
		t.Error("no day option may be pre-checked: the server's answer is the whole month")
	}

	// Zero JS. Not "little": none.
	if strings.Contains(body, "<script") || strings.Contains(body, "onclick") {
		t.Error("day answering must ship no script and no inline handler")
	}
}

// TestDayPick_GroupNameIsAPureFunctionOfTheData. Two Blocks on one page (the
// Bench composes four) must not share a radio group, or they fight over one
// piece of state; but the SAME Block re-rendered by an HTMX binding swap must
// keep the same name, or the swapped-in fragment loses the day the viewer just
// chose. Only a pure function of the data satisfies both — a package counter
// satisfies the first and breaks the second, and differs between servers.
func TestDayPick_GroupNameIsAPureFunctionOfTheData(t *testing.T) {
	a := fxHarptos(true)
	b := fxHarptos(true)
	if dayPickGroupName(a) != dayPickGroupName(b) {
		t.Error("the same Block rendered twice changed its group name — an HTMX swap would " +
			"drop the viewer's chosen day")
	}
	other := fxGregorian()
	if dayPickGroupName(a) == dayPickGroupName(other) {
		t.Error("two different calendars share a radio group — on the Bench they would fight " +
			"over one selection")
	}
	hosted := fxHarptos(true)
	hosted.Viewer.HostEntity = "ent-9"
	if dayPickGroupName(a) == dayPickGroupName(hosted) {
		t.Error("the same calendar hosted on an entity page shares its group with the " +
			"unhosted one — two Blocks for the same calendar can be on one page")
	}
}

// TestDayPick_IntercalaryDaysAnswerToo. The intercalary key namespace is `iN`,
// not `N`, so a ladder keyed on the ordinal alone would make Midwinter silently
// stop answering — guard B4's exact failure mode one level up. This is the
// "one intercalary day demonstrably answers" acceptance line.
func TestDayPick_IntercalaryDaysAnswerToo(t *testing.T) {
	body := render(t, fxHarptos(true))
	for _, want := range []string{
		`data-day-ord="i1"`,     // the ladder's partner key on the intercalary row
		`data-day-pick="i1"`,    // its own selection control
		`data-day="harptos-of-imix-i1"`, // the ANSWER key, still in its own namespace
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the intercalary row is missing %q — it would list in the Ledger and "+
				"never answer", want)
		}
	}
	// And it is NOT confusable with ordinary day 1.
	if strings.Contains(body, `class="interc" style="grid-column:2/-1" data-day="harptos-of-imix-1"`) {
		t.Error("an intercalary day claimed the ordinary day's key — Midwinter 1 is not Deepwinter 1")
	}
}

// TestDayPick_StopsAtTheLadderBound pins CTS-2's over-bound ruling: past the
// bound a day carries NO selection control at all — no radio, no label, no dead
// affordance. The Ledger still lists the day; only the answering stops.
//
// The alternative the ruling refused was a control that is present, focusable
// and silently does nothing, which is worse than an absent one (WG-spec V18: a
// `title` is not an honesty mechanism).
func TestDayPick_StopsAtTheLadderBound(t *testing.T) {
	body := render(t, fxLongMonth(t))

	if !strings.Contains(body, `data-day-pick="`+intText(answerLadderDays)+`"`) {
		t.Errorf("day %d is the last day INSIDE the bound and must still answer", answerLadderDays)
	}
	if strings.Contains(body, `data-day-pick="`+intText(answerLadderDays+1)+`"`) {
		t.Errorf("day %d is past the bound and must carry no control at all — the sheet has "+
			"no rule that could read it", answerLadderDays+1)
	}
	if !strings.Contains(body, `data-day-pick="i`+intText(answerLadderIntercalary)+`"`) {
		t.Errorf("intercalary day %d is the last one inside the bound", answerLadderIntercalary)
	}
	if strings.Contains(body, `data-day-pick="i`+intText(answerLadderIntercalary+1)+`"`) {
		t.Errorf("intercalary day %d is past the bound", answerLadderIntercalary+1)
	}
	// The day itself is still DATED and still rendered: the bound removes the
	// control, not the day.
	if !strings.Contains(body, `data-day-ord="`+intText(answerLadderDays+1)+`"`) {
		t.Errorf("day %d must still render and still carry its keys", answerLadderDays+1)
	}
}

// TestDayPick_AbsentWithoutADockedLedger. A Block with no Ledger has nothing
// for a chosen day to repaint and no home for the `✕ all` option that
// deselects it, so the controls would be a radio group with no way out. The
// gate is the same predicate block.templ uses for the zone (ledgerDocked), in
// one place, so the two cannot drift.
func TestDayPick_AbsentWithoutADockedLedger(t *testing.T) {
	off := fxHarptos(true)
	off.Layers.Enabled = []string{"moons"} // DEF: no ledger key
	if body := render(t, off); strings.Contains(body, `class="daypick"`) {
		t.Error("a Block whose viewer has the Ledger layer off must ship no day controls")
	}

	hidden := fxHarptos(true)
	hidden.Ledger.Hidden = true // the HOST removed the zone
	if body := render(t, hidden); strings.Contains(body, `class="daypick"`) {
		t.Error("a Block whose HOST removed the Ledger must ship no day controls either")
	}
}

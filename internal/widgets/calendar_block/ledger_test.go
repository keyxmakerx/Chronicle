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
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	// One control per day of a 30-day month, plus one per intercalary day,
	// plus the group's explicit "none" option in the Ledger head.
	wantDays := d.Month.Days + len(d.Month.Intercalary)
	if n := strings.Count(body, `class="daypick"`); n != wantDays+1 {
		t.Errorf("%d day radios for %d selectable days plus the `all` option", n, wantDays)
	}
	if n := strings.Count(body, `class="dsel"`); n != wantDays {
		t.Errorf("%d stretched labels for %d day radios — every day needs a hit target", n, wantDays)
	}
	// `✕ all` is the "none" option's label, and it is a RESERVED SLOT: always
	// in the DOM, toggled by visibility, so .lhead can never wrap.
	if n := strings.Count(body, `class="badge lclear"`); n != 1 {
		t.Errorf("%d clear controls; the reserved head slot is exactly one", n)
	}

	// ONE TAB STOP. Every day option shares one radio group name, so the Block
	// takes a single tab stop and the arrow keys move the selection between
	// days — which is how a KEYBOARD answers, not just a pointer.
	name := dayPickGroupName(d)
	if n := strings.Count(body, `name="`+name+`"`); n != wantDays+1 {
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

// ── Zone C's content (§3) ───────────────────────────────────────────────────

// fxLedgerMarks is a hand-written month whose marks exercise every branch of
// the signed row: a dm_only mark, a restricted-audience mark, an ordinary one,
// a typeless one and an untimed one.
func fxLedgerMarks(t *testing.T) BlockData {
	t.Helper()
	d := fxHarptos(true)
	for r := range d.Month.Rows {
		for c := range d.Month.Rows[r].Cells {
			d.Month.Rows[r].Cells[c].Marks = nil
		}
	}
	set := func(day int, marks ...Mark) {
		t.Helper()
		for r := range d.Month.Rows {
			for c := range d.Month.Rows[r].Cells {
				if d.Month.Rows[r].Cells[c].Day == day {
					d.Month.Rows[r].Cells[c].Marks = marks
					return
				}
			}
		}
		t.Fatalf("fixture drift: no cell for day %d", day)
	}
	set(4, Mark{EventID: "ev-open", Title: "Frost fair", Axis: "var(--ev-festival)",
		Pattern: "p4", Glyph: "✦", Named: true, Time: "10:00", AxisLabel: "Festival"})
	set(2, Mark{EventID: "ev-dm", Title: "Barrow scouting", Axis: "var(--ev-quest)",
		Pattern: "p2", Glyph: "▲", Named: true, AxisLabel: "Quest",
		Audience: &AudienceMark{Label: "GM only", Restricted: false}})
	set(9, Mark{EventID: "ev-restricted", Title: "Nissa's recital", Axis: "var(--ev-social)",
		Pattern: "p1", Glyph: "◆", Named: true,
		Audience: &AudienceMark{Label: "Restricted", Restricted: true}})
	d.Month.Intercalary = []IntercalaryDay{{Name: "Midwinter", Day: 1,
		Marks: []Mark{{EventID: "ev-mid", Title: "Midwinter rite", Axis: "var(--ev-celestial)",
			Pattern: "p6", Glyph: "☾", Named: true}}}}
	return d
}

// TestLedger_ListsByOrdinalDayFromTheCellsThemselves. The Ledger is reassembled
// from the cells the grid already draws (r52 §5 refused a parallel Ledger.Rows
// list) — so this pins that the walk is in ORDINAL order, covers the
// intercalary row, and produces exactly one row per mark. A second pass over
// anything would be visible here as a duplicate or a gap.
func TestLedger_ListsByOrdinalDayFromTheCellsThemselves(t *testing.T) {
	d := fxLedgerMarks(t)
	body := render(t, d)

	if n := strings.Count(body, `class="lrow `); n != 4 {
		t.Fatalf("%d Ledger rows for 4 marks", n)
	}
	// Ordinal order: day 2, then 4, then 9, then the intercalary row LAST.
	order := []string{`data-lday="2"`, `data-lday="4"`, `data-lday="9"`, `data-lday="i1"`}
	at := -1
	for _, want := range order {
		i := strings.Index(body, want)
		if i < 0 {
			t.Fatalf("no Ledger row carries %s", want)
		}
		if i < at {
			t.Errorf("%s is out of ordinal order — the Ledger lists by ordinal day", want)
		}
		at = i
	}
	// Both keys on every row: the ANSWER key a partner surface matches, and the
	// bare LADDER key the one static sheet can filter with.
	if !strings.Contains(body, `data-day="harptos-of-imix-4" data-lday="4"`) {
		t.Error("a Ledger row must carry BOTH its answer key and its ladder key")
	}
	// The zone marker the seam suite's layer table keys on must survive verbatim.
	if !strings.Contains(body, `<div class="ledger" data-zone="ledger">`) {
		t.Error(`data-zone="ledger" must stay on the outer div — the enabled-layer seam test ` +
			`silently stops proving the gate without it`)
	}
}

// TestLedger_GoldRailSplitsOnDmOnlyLikeTheGridsDogear. Audience.Restricted is
// the DISCRIMINATOR, not a synonym for "hidden": dm_only draws the gold notch
// in the grid and the gold rail here; a visibility_rules restriction draws the
// diamond in the grid and the audience chip alone here. A Ledger that drew the
// gold rail on a restricted row would be the identical defect one layer down
// from the one SEAM-P5 stage 4 fixed.
func TestLedger_GoldRailSplitsOnDmOnlyLikeTheGridsDogear(t *testing.T) {
	body := render(t, fxLedgerMarks(t))

	if n := strings.Count(body, `class="gr"`); n != 1 {
		t.Errorf("%d gold rails; exactly the ONE dm_only row may draw it", n)
	}
	if n := strings.Count(body, `class="badge gm">GM<`); n != 1 {
		t.Errorf("%d `GM` badges; the badge and the rail are one condition, not two", n)
	}
	// The rail and the badge belong to the dm_only row, not the restricted one.
	dm := ledgerRowFragment(t, body, "ev-dm")
	if !strings.Contains(dm, `class="gr"`) || !strings.Contains(dm, `>GM<`) {
		t.Error("the dm_only row must carry the gold rail AND the GM badge")
	}
	restricted := ledgerRowFragment(t, body, "ev-restricted")
	if strings.Contains(restricted, `class="gr"`) || strings.Contains(restricted, `>GM<`) {
		t.Error("a restricted-audience row is not a dm_only row: no gold rail, no GM badge")
	}
	// The audience CHIP renders for both and says which. `GM only` / `Restricted`
	// is the pin's own wave-1 ruling; the composed `◈ Wardens · Rell` audience
	// does not exist on main and is W-G's.
	if !strings.Contains(dm, `class="audchip">GM only<`) {
		t.Error("the dm_only row's audience chip must read `GM only`")
	}
	if !strings.Contains(restricted, `class="audchip">Restricted<`) {
		t.Error("the restricted row's audience chip must read `Restricted`")
	}
	// A player's Block carries no AudienceMark at all — permission is ABSENCE.
	player := fxLedgerMarks(t)
	for r := range player.Month.Rows {
		for c := range player.Month.Rows[r].Cells {
			for m := range player.Month.Rows[r].Cells[c].Marks {
				player.Month.Rows[r].Cells[c].Marks[m].Audience = nil
			}
		}
	}
	pb := render(t, player)
	for _, bad := range []string{`class="gr"`, `class="badge gm"`, `class="audchip"`} {
		if strings.Contains(pb, bad) {
			t.Errorf("a Block with no AudienceMark still emitted %s", bad)
		}
	}
}

// ledgerRowFragment returns the markup of one Ledger row, located by event id.
// It uses strings.Index only after checking it is non-negative — never as a
// slice bound directly, which PANICS on a rename (COMMON §3).
func ledgerRowFragment(t *testing.T, body, eventID string) string {
	t.Helper()
	// Scope to Zone C first: data-event-id is also on the grid's chips, and the
	// grid comes first in the document.
	zone := strings.Index(body, `<div class="ledger" data-zone="ledger">`)
	if zone < 0 {
		t.Fatal("no Ledger zone in the rendered Block")
	}
	body = body[zone:]
	start := strings.Index(body, `data-event-id="`+eventID+`"`)
	if start < 0 {
		t.Fatalf("no Ledger row for event %q", eventID)
	}
	rest := body[start:]
	if end := strings.Index(rest, `<div class="lrow`); end > 0 {
		return rest[:end]
	}
	return rest
}

// TestLedger_AbsentSegmentsDropRatherThanPrintEmpty is r52's own acceptance
// line: an untimed event drops the .tm segment and a typeless event drops the
// meta segment. NEITHER prints an empty element, and neither prints a dangling
// separator — the blockSyncStrings idiom, which is also why the signed meta
// line's owner segment is omitted rather than printed as "quest · " (CTS-5).
func TestLedger_AbsentSegmentsDropRatherThanPrintEmpty(t *testing.T) {
	body := render(t, fxLedgerMarks(t))

	timed := ledgerRowFragment(t, body, "ev-open")
	if !strings.Contains(timed, `>10:00<`) {
		t.Error("a timed event must print its time")
	}
	if !strings.Contains(timed, `class="mt">festival<`) {
		t.Error("the meta line prints the type LOWERCASED and alone (no owner segment in wave 2)")
	}
	if strings.Contains(timed, "festival ·") || strings.Contains(timed, "· </span>") {
		t.Error("no dangling separator: the omitted owner segment must leave no `·` behind")
	}

	untimed := ledgerRowFragment(t, body, "ev-dm")
	if strings.Contains(untimed, `class="tm`) {
		t.Error("an untimed event must emit NO .tm element, not an empty one")
	}

	typeless := ledgerRowFragment(t, body, "ev-restricted")
	if strings.Contains(typeless, `class="mt"`) {
		t.Error("a typeless event must emit NO .mt element, not an empty one")
	}

	// RSVP surfaces are W-G; the Bench's RSVP panel already carries the chip.
	if strings.Contains(body, "RSVP") {
		t.Error("the Ledger meta line must not print an RSVP segment in wave 2")
	}
}

// TestLedger_TimeTreatmentFollowsTheCalendarNotTheEvent. L15 (design notes §9
// dev 5): a real-world time is zone-labelled and an in-world time never is, and
// the distinction is a property of the CALENDAR. BlockData.IsRealWorld is the
// single fact that decides it — which is exactly why r52 added no per-mark
// real-world flag: a second copy of one fact can disagree with itself, and the
// disagreement IS L15's forbidden case.
func TestLedger_TimeTreatmentFollowsTheCalendarNotTheEvent(t *testing.T) {
	inWorld := fxLedgerMarks(t)
	if body := render(t, inWorld); !strings.Contains(body, `class="tm mono">10:00<`) {
		t.Error("an in-world calendar's Ledger times are .tm.mono and never zone-labelled")
	}

	realWorld := fxLedgerMarks(t)
	realWorld.IsRealWorld = true
	body := render(t, realWorld)
	if !strings.Contains(body, `class="tm">10:00<`) {
		t.Error("a real-world calendar's Ledger times are plain .tm")
	}
	if strings.Contains(body, `class="tm mono"`) {
		t.Error("one calendar cannot render both treatments — that is the disagreement L15 forbids")
	}
}

// TestLedger_HeadAndZeroStatesAreServerRendered. CSS cannot compute
// "3 Deepwinter · 1 event", so every selectable day's head line — and every
// EMPTY day's own "nothing here" line — is rendered on every pass and revealed
// by the ladder. The two empty strings are different claims and both are
// signed: "No events in Deepwinter" says the month is empty, "Nothing on 3
// Deepwinter" says the chosen day is.
func TestLedger_HeadAndZeroStatesAreServerRendered(t *testing.T) {
	d := fxLedgerMarks(t)
	body := render(t, d)

	if !strings.Contains(body, `class="lctx-all">Deepwinter · 4 events<`) {
		t.Error("the unselected head must state the month and its total")
	}
	if !strings.Contains(body, `data-lday="4">4 Deepwinter · 1 event<`) {
		t.Error("day 4 carries one mark; its head line must say so, with the shipped " +
			"eventCountLabel pluralisation")
	}
	if !strings.Contains(body, `data-lday="i1">Midwinter · 1 event<`) {
		t.Error("an intercalary day's head line names the day, not an ordinal in the month")
	}
	// An empty day gets its own zero line; a day with marks does not.
	if !strings.Contains(body, `class="lzero lday" data-lday="3">Nothing on 3 Deepwinter.<`) {
		t.Error("an empty day must carry its own selected-empty line")
	}
	if strings.Contains(body, `data-lday="4">Nothing on 4 Deepwinter.<`) {
		t.Error("a day WITH marks must not also carry a zero line — the ladder would show both")
	}

	// A month with nothing in it: the unselected empty state, and its second
	// sentence, which is the reason the Ledger renders in the fault case.
	empty := fxLedgerMarks(t)
	for r := range empty.Month.Rows {
		for c := range empty.Month.Rows[r].Cells {
			empty.Month.Rows[r].Cells[c].Marks = nil
		}
	}
	empty.Month.Intercalary = []IntercalaryDay{{Name: "Midwinter", Day: 1}}
	eb := render(t, empty)
	if !strings.Contains(eb, "No events in Deepwinter.") {
		t.Error("the unselected empty state names the MONTH")
	}
	if !strings.Contains(eb, "works before eras are defined") {
		t.Error("the second sentence is load-bearing: it is why the Ledger renders in the " +
			"fault case at all")
	}
}

// TestLedger_RendersInTheFaultCase. A calendar that cannot resolve a date
// prints its fault WHERE THE DATE WOULD GO and emits no date element — and
// STILL lists its events, because listing by ordinal day needs no era, no epoch
// and no reckoning. cv4:1754 says so in the empty state's own second sentence.
func TestLedger_RendersInTheFaultCase(t *testing.T) {
	d := fxLedgerMarks(t)
	d.DateLabel, d.EraLabel = "", ""
	d.Fault = "Needs eras — 0 eras defined, dates cannot resolve"
	body := render(t, d)

	mustContain(t, body, `class="fault"`, "the fault prints where the date would go")
	mustNotContain(t, body, `class="iw mono"`, "no date element is emitted in the fault case")
	if n := strings.Count(body, `class="lrow `); n != 4 {
		t.Errorf("the Ledger listed %d of 4 rows on a calendar that cannot resolve a date; "+
			"ordinal listing needs no era", n)
	}
	if !strings.Contains(body, `data-lday="i1"`) {
		t.Error("the intercalary day lists in the fault case too")
	}
}

// TestLedger_TabStripCarriesMonthAlone pins CTS-7. The signed std still shows
// four tabs; the other three panels are W-E's Shelf, and three tabs that do
// nothing are three inert controls. A single pressed tab is the truthful
// statement that there is currently one panel.
func TestLedger_TabStripCarriesMonthAlone(t *testing.T) {
	body := render(t, fxHarptos(true))
	if n := strings.Count(body, `class="ltab"`); n != 1 {
		t.Errorf("%d tabs in the strip; wave 2 emits `Month` alone (CTS-7)", n)
	}
	if !strings.Contains(body, `class="ltab" aria-pressed="true">Month<`) {
		t.Error("the one tab is the current panel and says so")
	}
	for _, unbuilt := range []string{">Upcoming<", ">Filters<", ">Almanac<"} {
		if strings.Contains(body, unbuilt) {
			t.Errorf("%s is W-E's panel; a tab without its panel is an inert control", unbuilt)
		}
	}
	// W-F's two std-head surfaces stay OUT rather than shipping inert.
	if strings.Contains(body, "colour:") {
		t.Error("the `colour: <axis>` picker is W-F's per-viewer preference store")
	}
}

// ── the invariance the docked Ledger promises, measured in a real engine ────

// ledgerProbeReading is one host box's measurement.
type ledgerProbeReading struct {
	Host        int     `json:"host"`
	BlockHeight float64 `json:"blockHeight"`
	RowsHeight  float64 `json:"rowsHeight"`
	VisibleRows int     `json:"visibleRows"`
	HeadText    string  `json:"headText"`
	ClearShown  bool    `json:"clearShown"`
	HeadWrapped bool    `json:"headWrapped"`
	TabsShown   bool    `json:"tabsShown"`
	LedgerTop   float64 `json:"ledgerTop"`
	LedgerBot   float64 `json:"ledgerBot"`
	RowsBot     float64 `json:"rowsBot"`
	ShelfTop    float64 `json:"shelfTop"`
	ShelfBot    float64 `json:"shelfBot"`
	BlockBot    float64 `json:"blockBot"`
	ScrollH     float64 `json:"scrollH"`
	ClientH     float64 `json:"clientH"`
	ShelfCapVis bool    `json:"shelfCapVis"`

	// RowDisplay is the SET of computed `display` values across every visible
	// .lrow, sorted and comma-joined — "flex" when the row is the signed row,
	// anything else when a rule reached it that should not have.
	//
	// It exists because the height probe above CANNOT SEE A COLLAPSE. .lrow
	// carries a fixed height (48px full / 44px std), so a row whose display
	// flips to `block` stacks its children into a pile INSIDE a box of exactly
	// the right height: every geometric reading in this struct stays identical
	// and the component is visibly broken. That is not hypothetical — it is
	// what shipped, and what this reading now makes impossible to ship again.
	RowDisplay string `json:"rowDisplay"`

	// Overprint is true when any visible row's time string is drawn over any
	// visible day numeral. It is the collapse's SYMPTOM measured independently
	// of its cause: a stacked row is ~70px of content inside a 48px box, so it
	// spills onto the numeral of the row beneath it. A future rule that breaks
	// the row some other way is caught here even if `display` still reads flex.
	Overprint bool `json:"overprint"`
}

const ledgerProbeScript = `
function(root){
  var r = function(el){ return el ? el.getBoundingClientRect() : null };
  var block  = root.querySelector('.block');
  var rows   = root.querySelector('.lrows');
  var head   = root.querySelector('.lhead');
  var ledger = root.querySelector('[data-zone="ledger"]');
  var shelf  = root.querySelector('[data-zone="shelf"]');
  var clear  = root.querySelector('.lclear');
  var tabs   = root.querySelector('.ltabs');
  var vis = function(el){ var b = r(el); return !!b && b.width > 0 && b.height > 0 };
  var shown = 0, ctx = '';
  // The row's own SHAPE, read from the engine rather than inferred from the
  // sheet: a fixed-height row that has stopped being a flex row measures the
  // same as one that has not, so the display value is read directly and the
  // time/day overprint the collapse produces is measured beside it.
  var disp = {}, overprint = false, numerals = [], times = [];
  [].slice.call(root.querySelectorAll('.lrow')).forEach(function(el){
    if (!vis(el)) return;
    shown++;
    disp[getComputedStyle(el).display] = true;
    // The row's height is FIXED, so a row whose content stopped fitting spills
    // into the row below it rather than growing. That is where the collapse
    // actually became visible: a stacked row's time string landed on the NEXT
    // row's day numeral. So the two are collected across the whole list and
    // intersected pairwise, not compared within one row.
    [].slice.call(el.querySelectorAll('.dg')).forEach(function(n){ numerals.push(r(n)) });
    [].slice.call(el.querySelectorAll('.tm')).forEach(function(n){ times.push(r(n)) });
  });
  numerals.forEach(function(a){
    times.forEach(function(b){
      if (a.right > b.left + 0.5 && b.right > a.left + 0.5 &&
          a.bottom > b.top + 0.5 && b.bottom > a.top + 0.5) overprint = true;
    });
  });
  [].slice.call(root.querySelectorAll('.lctx-all,.lctx')).forEach(function(el){
    if (vis(el)) ctx = el.textContent.trim();
  });
  // .lhead must never wrap: two children on different rows IS the wrap.
  var wrapped = false;
  if (head) {
    var top = null;
    [].slice.call(head.children).forEach(function(el){
      var b = r(el);
      if (!b || b.height === 0) return;
      if (getComputedStyle(el).position === 'absolute') return;
      if (top === null) top = b.top;
      else if (Math.abs(b.top - top) > 6) wrapped = true;
    });
  }
  return {
    host: Math.round(r(root).width),
    blockHeight: r(block).height,
    rowsHeight: rows ? r(rows).height : 0,
    visibleRows: shown,
    headText: ctx,
    clearShown: !!(clear && getComputedStyle(clear).visibility === 'visible'),
    headWrapped: wrapped,
    tabsShown: vis(tabs),
    ledgerTop: ledger ? r(ledger).top : 0,
    ledgerBot: ledger ? r(ledger).bottom : 0,
    rowsBot: rows ? r(rows).bottom : 0,
    shelfTop: shelf ? r(shelf).top : 0,
    shelfBot: shelf ? r(shelf).bottom : 0,
    blockBot: r(block).bottom,
    scrollH: block.scrollHeight,
    clientH: block.clientHeight,
    shelfCapVis: (function(){
      var cap = shelf && shelf.querySelector('.cap');
      if (!cap) return false;
      var b = r(cap), bb = r(block);
      return b.height > 0 && b.top >= bb.top - 1 && b.bottom <= bb.bottom + 1;
    })(),
    rowDisplay: Object.keys(disp).sort().join(','),
    overprint: overprint
  };
}`

// runLedgerProbe lays every host out in one wide window and reads them back in
// a single Chromium run — the same harness shape as the sizing probe.
func runLedgerProbe(t *testing.T, chrome string, boxes []string) []ledgerProbeReading {
	t.Helper()
	css := blockCSS(t)
	var body strings.Builder
	for i, markup := range boxes {
		fmt.Fprintf(&body, `<div class="probe-host" id="h%d" style="%s">%s</div>`,
			i, ledgerProbeWidths[i], markup)
	}
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + body.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + ledgerProbeScript + `;` +
		`var out=[].slice.call(document.querySelectorAll('.probe-host')).map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "ledger-probe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-gpu",
		"--hide-scrollbars", "--window-size=1600,1400", "--virtual-time-budget=5000",
		"--dump-dom", "file://"+path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []ledgerProbeReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	return readings
}

// ledgerProbeWidths is filled by each probe before it builds its boxes.
var ledgerProbeWidths []string

// ledgerProbeBoxes renders the same Block twice per host width — once as the
// server sends it (no day chosen) and once with a day radio CHECKED, which is
// exactly the DOM state a click produces. There is no JS to simulate.
func ledgerProbeBoxes(t *testing.T, widths []int, d BlockData, day string) ([]string, []string) {
	t.Helper()
	base := stripLink(render(t, d))
	group := dayPickGroupName(d)
	var boxes, styles []string
	for i, w := range widths {
		// Radios sharing a name are ONE GROUP document-wide, so every box gets
		// its own suffix — otherwise only the last `checked` in the page
		// survives and every earlier box measures as unselected, which would
		// make the whole probe pass vacuously.
		unsel := strings.ReplaceAll(base, `"`+group+`"`, fmt.Sprintf(`"%s-p%da"`, group, i))
		unsel = strings.ReplaceAll(unsel, `"`+group+`-`, fmt.Sprintf(`"%s-p%da-`, group, i))
		sel := strings.ReplaceAll(base, `"`+group+`"`, fmt.Sprintf(`"%s-p%db"`, group, i))
		sel = strings.ReplaceAll(sel, `"`+group+`-`, fmt.Sprintf(`"%s-p%db-`, group, i))
		picked := strings.Replace(sel,
			`data-day-pick="`+day+`" name=`, `data-day-pick="`+day+`" checked name=`, 1)
		if picked == sel {
			t.Fatalf("could not check day %q in the rendered markup — the probe would measure "+
				"two identical DOMs and pass vacuously", day)
		}
		boxes = append(boxes, unsel, picked)
		styles = append(styles, fmt.Sprintf("width:%dpx", w), fmt.Sprintf("width:%dpx", w))
	}
	return boxes, styles
}

// TestProbe_LedgerHeightIsInvariantUnderSelection is the "nothing reflows"
// promise made FALSIFIABLE.
//
// The docked Ledger's whole premise is that choosing a day repaints a panel
// that was already on screen — so the Block must declare the SAME HEIGHT
// selected and unselected. .lrows{min-height:176px} is the device that makes it
// true when a selection filters fourteen rows down to one, and a device nobody
// measures is a device that quietly stops working. Only a real engine can
// answer this: it is a question about flexbox, container queries and a
// min-height interacting, and no string assertion can see it.
func TestProbe_LedgerHeightIsInvariantUnderSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	// Full tier, and BOTH std host widths the two production hosts measure at:
	// 420px on the entity page and 358px on the Bench (CTS-8).
	widths := []int{1232, 420, 358}
	boxes, styles := ledgerProbeBoxes(t, widths, fxHarptos(true), "5")
	ledgerProbeWidths = styles
	readings := runLedgerProbe(t, chrome, boxes)

	for i, w := range widths {
		unsel, sel := readings[2*i], readings[2*i+1]
		t.Logf("host %dpx — unselected: block %.1fpx, rows %.1fpx, %d rows, head %q · "+
			"selected: block %.1fpx, rows %.1fpx, %d rows, head %q",
			w, unsel.BlockHeight, unsel.RowsHeight, unsel.VisibleRows, unsel.HeadText,
			sel.BlockHeight, sel.RowsHeight, sel.VisibleRows, sel.HeadText)

		if math.Abs(unsel.BlockHeight-sel.BlockHeight) > 1 {
			t.Errorf("host %dpx: the Block is %.1fpx unselected and %.1fpx selected — "+
				"choosing a day reflowed the component, which is the one thing the docked "+
				"Ledger exists to prevent", w, unsel.BlockHeight, sel.BlockHeight)
		}
		if sel.VisibleRows >= unsel.VisibleRows {
			t.Errorf("host %dpx: %d rows visible selected vs %d unselected — the ladder did "+
				"not filter, so the invariance above is vacuous", w, sel.VisibleRows, unsel.VisibleRows)
		}
		if unsel.HeadText == sel.HeadText {
			t.Errorf("host %dpx: the head says %q either way — the panel did not repaint",
				w, unsel.HeadText)
		}
		if !sel.ClearShown {
			t.Errorf("host %dpx: `✕ all` must become visible when a day is chosen, or the "+
				"viewer has no way back to the month", w)
		}
		if unsel.ClearShown {
			t.Errorf("host %dpx: `✕ all` must be INVISIBLE (not absent) with no day chosen", w)
		}
		if unsel.HeadWrapped || sel.HeadWrapped {
			t.Errorf("host %dpx: .lhead wrapped — design notes §10 defect 5, the jolt on the "+
				"commonest interaction in the component", w)
		}

		// THE ROW MUST STILL BE THE ROW AFTER A DAY IS CHOSEN. The signed row is
		// one flex line — day ordinal · rail · gold rail · glyph · name · chips ·
		// meta · right-aligned time — and every reading above is blind to its
		// collapse, because .lrow's height is fixed: a row that stops being a
		// flex container piles its children up INSIDE a box of exactly the right
		// height and every geometric invariant still holds.
		//
		// This is the one that shipped: the ladder's reveal rule was written for
		// the two surfaces that are display:none at rest and matched on a class
		// token the row also carried, so choosing a day turned all six matching
		// rows into stacks with the time drawn over the numeral. Measured, not
		// inferred — a string assertion over the sheet could not see it and did
		// not.
		for _, r := range []struct {
			when string
			read ledgerProbeReading
		}{{"unselected", unsel}, {"selected", sel}} {
			if r.read.RowDisplay != "flex" {
				t.Errorf("host %dpx, %s: visible Ledger rows compute display=%q, not \"flex\" — "+
					"the row is a single flex line and a rule has reached it. Choosing a day "+
					"may change WHICH rows are listed and nothing else about them.",
					w, r.when, r.read.RowDisplay)
			}
			if r.read.Overprint {
				t.Errorf("host %dpx, %s: a Ledger time string is drawn over a day numeral — "+
					"the rows have stopped laying out along one line each and are spilling "+
					"past their own fixed height into the row below", w, r.when)
			}
		}
	}

	// The strip is a std affordance: present at std, absent at full.
	if readings[0].TabsShown {
		t.Error("the panel strip must not render at full tier")
	}
	if !readings[2].TabsShown || !readings[4].TabsShown {
		t.Error("the panel strip is the std tier's Ledger head furniture")
	}
}

// TestProbe_StdTierFilledLedgerDoesNotCollide is CTS-8's measurement, and it is
// a STOP-AND-FLAG gate rather than a licence to drop a host layer key.
//
// entity_calendar_block_test.go:374-375 and bench.go:513-521 record that at std
// tier an extra needzone row "stacks into the docked Ledger and the Shelf, and
// the Ledger and Shelf headers visibly overlap" — the measurement that booked
// `moongraph` and `horizon` out of both host layer sets. IT WAS TAKEN AGAINST
// STUBS. Filling the Ledger makes it taller, so it is re-taken here at both
// production host widths, with the Ledger full.
//
// If this fails, the answer is NOT to drop a key from entityBlockLayers() or
// benchBlockLayers(): those are HOST-P3's and BENCH-P4's pinned files, and a
// Block that quietly stops docking its Ledger on the entity page is the failure
// the layer registry exists to make visible.
func TestProbe_StdTierFilledLedgerDoesNotCollide(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	// The production host layer sets: moons · eras · weeknums · ledger · shelf.
	d := fxHarptos(true)
	d.Layers = LayerState{Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"}}
	d.Ledger = LedgerStub{NeedsBackend: false}
	d.Shelf = ShelfStub{NeedsBackend: true}

	widths := []int{420, 358}
	boxes, styles := ledgerProbeBoxes(t, widths, d, "5")
	ledgerProbeWidths = styles
	readings := runLedgerProbe(t, chrome, boxes)

	for i, w := range widths {
		for _, r := range []ledgerProbeReading{readings[2*i], readings[2*i+1]} {
			t.Logf("std %dpx — block %.1fpx (content %.0f in %.0f) · ledger %.1f→%.1f · "+
				"shelf %.1f→%.1f · rows %.1fpx · shelf caption visible=%v",
				w, r.BlockHeight, r.ScrollH, r.ClientH, r.LedgerTop, r.LedgerBot,
				r.ShelfTop, r.ShelfBot, r.RowsHeight, r.ShelfCapVis)
			// CONTENT overlap, not box overlap. The two zone BOXES abut exactly
			// by construction (the Ledger takes the flex share above the
			// Shelf), so comparing their rects can never catch anything. What
			// collides is the rows box overflowing its own zone: a min-height
			// the std Block cannot afford keeps its declared size and lands on
			// top of the Shelf. Measured: 36px of bleed with a 176px floor.
			if r.ShelfTop > 0 && r.RowsBot-r.ShelfTop > 1 {
				t.Errorf("std %dpx: the Ledger's rows box ends at %.1f and the Shelf starts at "+
					"%.1f — the filled Ledger is ON TOP of the Shelf. STOP AND FLAG (CTS-8): "+
					"the answer is a host-layer or std-geometry decision for the coordinator, "+
					"never a silent key drop from entityBlockLayers()/benchBlockLayers().",
					w, r.RowsBot, r.ShelfTop)
			}
			if r.LedgerBot-r.ShelfTop > 1 {
				t.Errorf("std %dpx: the Ledger zone's own box (ends %.1f) overlaps the Shelf's "+
					"(starts %.1f)", w, r.LedgerBot, r.ShelfTop)
			}
			if r.ScrollH-r.ClientH > 1 {
				t.Errorf("std %dpx: %0.f px of content in a %0.f px box — the Block is clipping "+
					"a zone it declared room for", w, r.ScrollH, r.ClientH)
			}
			if r.ShelfBot > r.BlockBot+1 {
				t.Errorf("std %dpx: the Shelf's box ends at %.1f, past the Block's own %.1f — "+
					"the zone is pushed out of the component", w, r.ShelfBot, r.BlockBot)
			}
		}
	}
}

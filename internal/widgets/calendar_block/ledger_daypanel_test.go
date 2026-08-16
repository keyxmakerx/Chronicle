// ledger_daypanel_test.go — the Ledger's DAY PANEL (calendar-v4 refinement,
// stage 3), and the two questions the operator's brief asks by name:
//
//	is anything here reachable ONLY by hovering?
//	does the create door carry the day card's gate, or a second one?
//
// The geometric half — the panel appears on TAP, sits below the month at 390px
// and does not steal the rows' space — is measured in a real engine next door
// (ledger_daypanel_probe_test.go). A string assertion cannot see any of it.
//
// NO NEW CSS OR TEMPL FILE WAS ADDED BY THIS STAGE, so guard B4's filename glob
// (`*calendar_block*|*bench*|*daycard*|*schedule*|*theater*`) loses no coverage:
// the panel lives in ledger.templ and static/css/calendar-block.css, both of
// which the glob already reads. This file is Go, which the guard does not scan
// at all — it scans *.css, *.templ and *.html.

package calendar_block

import (
	"regexp"
	"strings"
	"testing"
)

// fxDayPanel is the Almanac fixture (four declared moons, so the panel's moon
// line has something plural to fold) with the Ledger's create door lit.
func fxDayPanel(t *testing.T, canCreate bool) BlockData {
	t.Helper()
	d := fxAlmanac(t, true)
	d.Ledger.CanCreate = canCreate
	return d
}

// dayPanelSlice cuts one day's panel out of a render. Bounds are CHECKED, never
// bare (COMMON §3): a raw strings.Index used as a slice bound panics on a
// rename instead of failing with a sentence.
func dayPanelSlice(t *testing.T, body, ord string) string {
	t.Helper()
	open := strings.Index(body, `<div class="ldp lday" data-lday="`+ord+`">`)
	if open < 0 {
		t.Fatalf("no day panel for day %q in the render", ord)
	}
	rest := body[open:]
	// The panel is the only `.ldp` and its own closing tag is the fourth
	// `</div>`-ish boundary; slicing to the NEXT panel (or to the rows' first
	// `.lzero`/`.lrow`) is the checkable bound.
	end := strings.Index(rest[1:], `<div class="ldp lday"`)
	if end < 0 {
		end = strings.Index(rest[1:], `class="lrow`)
	}
	if end < 0 {
		end = strings.Index(rest[1:], `class="lzero`)
	}
	if end < 0 {
		t.Fatalf("the day panel for %q never ends — no following panel, row or zero line", ord)
	}
	return rest[:end+1]
}

// TestDayPanel_IsRenderedForEverySelectableDayAndHiddenAtRest.
//
// The panel rides the SAME contract the head's context lines and the per-day
// empty states already ride: the server renders one per selectable day and the
// generated ladder reveals exactly one. It has to be server-rendered because
// CSS cannot compute "4 Deepwinter, 1492" — the same sentence that put the
// context lines in the markup in the first place.
func TestDayPanel_IsRenderedForEverySelectableDayAndHiddenAtRest(t *testing.T) {
	d := fxDayPanel(t, true)
	body := render(t, d)
	v := newLedgerView(d)

	if len(v.Ctx) == 0 {
		t.Fatal("the fixture produced no selectable days; every assertion below would be vacuous")
	}
	got := strings.Count(body, `<div class="ldp lday" data-lday="`)
	if got != len(v.Ctx) {
		t.Errorf("%d day panels for %d selectable days — one per day, or the ladder reveals "+
			"nothing for the days it skipped", got, len(v.Ctx))
	}
	// Hidden at rest is the SHEET's job, and it is the half a markup test can
	// still check: the panel must carry the `.lday` reveal token, because that
	// is the token the sheet's `display:none` and the ladder's `display:revert`
	// both key on.
	for _, c := range v.Ctx {
		want := `<div class="ldp lday" data-lday="` + c.Ord + `">`
		if !strings.Contains(body, want) {
			t.Errorf("day %q has a ledgerContext but no panel in the markup", c.Ord)
		}
	}
	css := blockCSS(t)
	if !strings.Contains(stripComments(css), ".ldp {\n  display: none;") {
		t.Error("`.ldp` must be display:none at rest — every panel is emitted on every render " +
			"and exactly one is revealed by the ladder")
	}
}

// TestDayPanel_CarriesTheDateTheWeekdayAndTheMoon.
//
// The four facts the reference's TODAY panel carries, and the two it must DROP
// rather than guess.
func TestDayPanel_CarriesTheDateTheWeekdayAndTheMoon(t *testing.T) {
	d := fxDayPanel(t, true)
	body := render(t, d)

	p := dayPanelSlice(t, body, "4")
	// The date line names the day, the month and the RESOLVED YEAR — the fact
	// MonthGeometry already carries. It is not BlockData.DateLabel, which is
	// today's era-aware line and says nothing about a chosen day.
	if !strings.Contains(p, `class="ldn">4 `+d.Month.Name+`, `+intText(d.Month.Year)+`<`) {
		t.Errorf("the panel's date header must read `4 %s, %d`; panel was:\n%s",
			d.Month.Name, d.Month.Year, p)
	}
	// The weekday comes from the CELL's own column against the declared names,
	// never from an ordinal modulo anything.
	if !strings.Contains(p, `class="lds">`) {
		t.Error("the panel must carry the weekday sub-line for an ordinary day")
	}
	// EVERY declared body, not one: stage 2's ruling, one surface down.
	if !strings.Contains(p, `class="ldm"`) {
		t.Fatalf("the panel carries no moon line; panel was:\n%s", p)
	}
	for _, m := range d.Month.Almanac {
		if !strings.Contains(p, m.Name) {
			t.Errorf("declared moon %q is missing from the panel's phase line — the grid's "+
				"three-moon ceiling is only legitimate because the register carries every "+
				"body (L21's second half)", m.Name)
		}
	}

	// THE INTERCALARY DAY DROPS BOTH, and the reason is the same fact twice:
	// it is outside every tenday AND outside the Almanac register's ordinal
	// index, so a weekday or a phase printed here would be the neighbouring
	// ordinary day's answer wearing this day's name.
	ip := dayPanelSlice(t, body, "i1")
	if strings.Contains(ip, `class="lds">`) {
		t.Errorf("an intercalary day printed a weekday; it sits outside every tenday:\n%s", ip)
	}
	if strings.Contains(ip, `class="ldm"`) {
		t.Errorf("an intercalary day printed a moon phase; the Almanac register is indexed by "+
			"ordinal day of the RENDERED month, so this would be Deepwinter 1's "+
			"illumination under Midwinter's name:\n%s", ip)
	}
}

// TestDayPanel_MoonLineIsTheAlmanacsOwnArithmetic. One derivation, two
// surfaces: if the panel and the Tonight readout ever disagreed about a day's
// illumination, one of them would be lying and neither would say which.
func TestDayPanel_MoonLineIsTheAlmanacsOwnArithmetic(t *testing.T) {
	d := fxDayPanel(t, true)
	const day = 4
	for _, m := range d.Month.Almanac {
		a, ok := almanacDayAt(m, day)
		if !ok {
			t.Fatalf("fixture drift: %s has no entry for day %d", m.Name, day)
		}
		want := m.Name + " " + intText(almanacIllumPct(a)) + "% " + a.Phase
		if !strings.Contains(ledgerMoonLine(d, day), want) {
			t.Errorf("the panel's moon line does not carry %q — it must be almanacDayAt + "+
				"almanacIllumPct, the SAME functions the Tonight readout and the Month lane "+
				"share, and not a second derivation", want)
		}
	}
	// A Block with no register (no Shelf, no sky) prints no line rather than an
	// empty one — the blockSyncStrings idiom.
	bare := fxHarptos(true)
	bare.Layers = LayerState{Enabled: []string{"moons", "ledger"}}
	if got := ledgerMoonLine(bare, 4); got != "" {
		t.Errorf("a Block with no Almanac register produced a moon line %q; the row drops", got)
	}
}

// TestDayPanel_CreateDoorIsTheDayCardsControlAndItsGate.
//
// THE GATE IS MARKUP-LEVEL AND IT IS ABSENCE. A viewer below the authoring
// floor gets no button, no disabled state and no `title` explaining one — the
// house idiom (WG-spec V18), and the same one daycard.templ's `+ New event`
// already obeys. This is the half that would silently invert: a door rendered
// unconditionally still LOOKS gated because the server would refuse the write.
func TestDayPanel_CreateDoorIsTheDayCardsControlAndItsGate(t *testing.T) {
	with := render(t, fxDayPanel(t, true))
	without := render(t, fxDayPanel(t, false))

	if n := strings.Count(with, `data-dc-new`); n == 0 {
		t.Fatal("CanCreate is set and the Ledger drew no create door")
	}
	if n := strings.Count(without, `data-dc-new`); n != 0 {
		t.Errorf("%d create doors rendered for a viewer without the authoring floor — "+
			"permission is ABSENCE, not a disabled control", n)
	}
	for _, leak := range []string{"New event", "ldnew"} {
		if strings.Contains(without, leak) {
			t.Errorf("%q reached a viewer who cannot create; the gate is the markup, so "+
				"nothing about the control may survive it", leak)
		}
	}

	// IT IS THE DAY CARD'S CONTROL, NOT A SECOND ONE: same handle, therefore
	// the same delegated listener, the same editor and the same shipped route.
	// A new handle here would be a second create path to keep in step.
	p := dayPanelSlice(t, with, "4")
	if !strings.Contains(p, `data-dc-new`) {
		t.Error("the door must carry `data-dc-new` — daycard.templ's own handle, which " +
			"calendar_daycard.js's delegated listener already reads")
	}
	// AND IT NAMES ITS OWN DAY. Without this the module falls back to whatever
	// day the card was last opened on, and a viewer who moved the selection by
	// keyboard creates on a stale date.
	v := newLedgerView(fxDayPanel(t, true))
	var key string
	for _, c := range v.Ctx {
		if c.Ord == "4" {
			key = c.Key
		}
	}
	if key == "" {
		t.Fatal("fixture drift: day 4 has no ANSWER key")
	}
	if !strings.Contains(p, `data-day="`+key+`"`) {
		t.Errorf("the door must name its own day (%q) in the ANSWER key namespace; panel was:\n%s",
			key, p)
	}
	// The intercalary door names the OTHER namespace. Midwinter 1 is not
	// Deepwinter 1, and a door keyed on the bare ordinal would create the event
	// in the wrong month entirely.
	ip := dayPanelSlice(t, with, "i1")
	if !strings.Contains(ip, `data-day="`+intercalaryKey(fxDayPanel(t, true).CalendarSlug, 1)+`"`) {
		t.Errorf("the intercalary door must name the intercalaryKey namespace; panel was:\n%s", ip)
	}
}

// TestDayPanel_DoorIsGatedOnTheDockedLedgerToo. CanCreate alone is not enough:
// the door opens the day card's editor, and a Block with no docked Ledger has
// no panel to draw it in. Both facts, one predicate, one place.
func TestDayPanel_DoorIsGatedOnTheDockedLedgerToo(t *testing.T) {
	d := fxDayPanel(t, true)
	d.Layers = LayerState{Enabled: []string{"moons"}} // no ledger key
	if ledgerCanCreate(d) {
		t.Error("ledgerCanCreate is true with the Ledger undocked — the door would be a " +
			"control with no zone to live in")
	}
	hidden := fxDayPanel(t, true)
	hidden.Ledger.Hidden = true
	if ledgerCanCreate(hidden) {
		t.Error("ledgerCanCreate is true with the host's Ledger hidden")
	}
	if !ledgerCanCreate(fxDayPanel(t, true)) {
		t.Error("ledgerCanCreate is false on a docked, create-capable Block — the door " +
			"would never render at all and every assertion above would be vacuous")
	}
}

// TestDayPanel_NothingIsReachableOnlyByHovering.
//
// The operator is usually on a phone. This is the brief's item 3 turned into a
// test rather than a claim.
//
// The audit has two halves and both are checkable here:
//
//  1. the panel is revealed by SELECTION, not by hover. The ladder's reveal
//     rule keys on `.daypick:checked`, which a tap on the stretched `.dsel`
//     label sets — the `:hover`/`:focus-within` selectors in the ladder belong
//     to rule 3, the ANSWER wash, which carries no information of its own.
//  2. nothing INSIDE the panel is hover-gated. No rule anywhere in the sheet
//     may make an `.ldp` descendant appear on `:hover`.
func TestDayPanel_NothingIsReachableOnlyByHovering(t *testing.T) {
	code := stripComments(blockCSS(t))

	// (1) every generated reveal rule for the panel is keyed on :checked.
	revealRe := regexp.MustCompile(`(?m)^([^\n{]*\.ldp\.lday\[[^\n{]*)$`)
	found := 0
	for _, m := range revealRe.FindAllStringSubmatch(answerLadderCSS(), -1) {
		found++
		sel := m[1]
		if !strings.Contains(sel, ":checked") {
			t.Errorf("the panel's reveal rule %q is not keyed on :checked — the panel would "+
				"be reachable by something other than choosing the day", sel)
		}
		if strings.Contains(sel, ":hover") {
			t.Errorf("the panel's reveal rule %q is keyed on :hover — on a phone there is no "+
				"hover, so the panel would be unreachable", sel)
		}
	}
	if found == 0 {
		t.Fatal("no generated reveal rule names the day panel; the assertions above are vacuous")
	}

	// (2) no hand-written rule in the sheet reveals an .ldp descendant on hover.
	ruleRe := regexp.MustCompile(`(?s)([^{}]*)\{([^}]*)\}`)
	for _, m := range ruleRe.FindAllStringSubmatch(code, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		if !strings.Contains(sel, ".ldp") || !strings.Contains(sel, ":hover") {
			continue
		}
		// A hover rule may re-INK the door (background/colour). It may never
		// change what EXISTS.
		for _, forbidden := range []string{"display:", "visibility:", "content-visibility:", "opacity:"} {
			if strings.Contains(strings.ReplaceAll(body, " ", ""), strings.ReplaceAll(forbidden, " ", "")) {
				t.Errorf("%q changes %s on hover — on a phone that content is unreachable. "+
					"Hover may re-ink; it may never be the only way to something.", sel, forbidden)
			}
		}
	}
}

// TestDayPanel_AddsNoMotionAndNoAccentToTheMarksLayer is the standing pair of
// laws restated over this stage's own selectors, so a future edit to THIS
// section fails with a sentence about THIS section rather than with the
// sheet-wide guard's.
func TestDayPanel_AddsNoMotionAndNoAccentToTheMarksLayer(t *testing.T) {
	code := stripComments(blockCSS(t))
	ruleRe := regexp.MustCompile(`(?s)([^{}]*)\{([^}]*)\}`)
	for _, m := range ruleRe.FindAllStringSubmatch(code, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		if !strings.Contains(sel, ".ldp") && !strings.Contains(sel, ".ldn") &&
			!strings.Contains(sel, ".ldm") && !strings.Contains(sel, ".ldnew") {
			continue
		}
		for _, bad := range []string{"transition", "animation"} {
			if strings.Contains(body, bad) {
				t.Errorf("%q declares %s — the grid and its Ledger are at ZERO MOTION; "+
					"hover and selection change instantly, and a `:hover` rule with no "+
					"transition IS the design", sel, bad)
			}
		}
		// The moon line is ACHROMATIC BY LAW: the sky may never borrow the
		// event colour axis, and --accent is chrome.
		if strings.Contains(sel, ".ldm") {
			for _, bad := range []string{"var(--axis)", "var(--accent)", "var(--cal)"} {
				if strings.Contains(body, bad) {
					t.Errorf("the panel's moon line references %s in %q — the sky is "+
						"ACHROMATIC and --accent is chrome", bad, sel)
				}
			}
		}
	}
}

// TestDayPanel_TheLedgerStillListsAndStillFiltersUnderneathIt. The panel is an
// ADDITION to the docked column, not a replacement for it: stage 3 finishes the
// Ledger and must not quietly become the drawer canon amendment A3 struck.
func TestDayPanel_TheLedgerStillListsAndStillFiltersUnderneathIt(t *testing.T) {
	d := fxLedgerMarks(t)
	d.Ledger.CanCreate = true
	body := render(t, d)

	if n := strings.Count(body, `class="lrow `); n != 4 {
		t.Errorf("the Ledger listed %d of 4 rows with the panel in place; the panel adds a "+
			"header, it does not take the list's job", n)
	}
	if !strings.Contains(body, `class="lctx-all">Deepwinter · 4 events<`) {
		t.Error("the unselected head still states the month and its total")
	}
	if !strings.Contains(body, `class="lzero lday" data-lday="3">Nothing on 3 Deepwinter.<`) {
		t.Error("the per-day empty state survives — it is a different claim from the panel's " +
			"date header and both are signed")
	}
}

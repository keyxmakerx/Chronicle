// block_test.go — the render contract.
//
// These assert on OUTPUT, which means they cannot see whether the stylesheet
// actually defines the container queries the markup depends on. That gap is
// closed by two other files: css_contract_test.go reads the sheet, and
// container_query_probe_test.go asks a real browser. Green here is necessary and
// not sufficient — it is precisely the bug class #568 chased.
package calendar_block

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

func render(t *testing.T, d BlockData) string {
	t.Helper()
	var sb strings.Builder
	if err := Block(d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render Block: %v", err)
	}
	return sb.String()
}

// flatten collapses whitespace so a substring pin survives reformatting. Copied
// from the calendar_v2 editor-drawer pattern (COMMON §3): never a bare
// strings.Index result used as a slice bound, which PANICS on a rename instead
// of failing cleanly.
var wsRe = regexp.MustCompile(`\s+`)

func flatten(s string) string { return wsRe.ReplaceAllString(s, " ") }

func mustContain(t *testing.T, body, want, why string) {
	t.Helper()
	if !strings.Contains(flatten(body), want) {
		t.Errorf("missing %q — %s", want, why)
	}
}

func mustNotContain(t *testing.T, body, bad, why string) {
	t.Helper()
	if strings.Contains(flatten(body), bad) {
		t.Errorf("must not contain %q — %s", bad, why)
	}
}

// ── guard B4: every dated cell and row carries its ANSWER key ───────────────

// TestDataDay_OnEveryDatedCellAndRow is the one guard that catches a BEHAVIOURAL
// regression rather than a stylistic one. A surface that forgets data-day simply
// stops answering, which reads to the operator as "the calendar feels dead over
// here" and is invisible in code review. Nothing consumes it in wave 1; W-B does.
func TestDataDay_OnEveryDatedCellAndRow(t *testing.T) {
	d := fxHarptos(true)
	body := render(t, d)

	for day := 1; day <= d.Month.Days; day++ {
		want := `data-day="` + dayKey(d.CalendarSlug, day) + `"`
		if !strings.Contains(body, want) {
			t.Errorf("day %d has no %s", day, want)
		}
	}
	// The intercalary row is a DATED row too, and its key must not collide with
	// the ordinary day of the same ordinal — Midwinter 1 is not Deepwinter 1.
	mustContain(t, body, `data-day="harptos-of-imix-i1"`,
		"the intercalary row must carry a distinct data-day")

	// Out-of-range cells are NOT dated and must not claim to be.
	g := render(t, fxGregorian())
	lead := strings.Count(g, `class="cell out"`)
	if lead == 0 {
		t.Error("the Gregorian fixture has a 3-day lead and 4 trailing cells; none rendered as .out")
	}
	if n := strings.Count(g, `data-day="real-world-0"`); n != 0 {
		t.Errorf("an out-of-range cell emitted a zero data-day %d times", n)
	}
}

// ── the fault ───────────────────────────────────────────────────────────────

// TestFault_ReplacesTheDateEntirely. Not a zero, not an em dash, not a
// placeholder — and NO date element at all beside it.
func TestFault_ReplacesTheDateEntirely(t *testing.T) {
	body := render(t, fxDwarvenFault())
	mustContain(t, body, `class="fault"`, "the fault must print where the date would go")
	mustContain(t, body, "Needs eras — 0 eras defined, dates cannot resolve", "the fault text")
	mustNotContain(t, body, `class="iw mono"`,
		"a faulted Block must emit NO date element, not an empty or placeholder one")

	// And the healthy case still prints its date.
	ok := render(t, fxHarptos(true))
	mustContain(t, ok, `class="iw mono"`, "a healthy Block prints its date")
	mustNotContain(t, ok, `class="fault"`, "a healthy Block has no fault line")
}

// ── the sync pill ───────────────────────────────────────────────────────────

// TestSyncPill_BothStringsAndTheDenominator. Both strings are always emitted so
// a container query can choose; the denominator can never drop, whatever the
// producer hands over.
func TestSyncPill_BothStringsAndTheDenominator(t *testing.T) {
	body := render(t, fxHarptos(true))
	mustContain(t, body, `class="sync-full"`, "the wide string must always be in the DOM")
	mustContain(t, body, `class="sync-compact"`, "the compact string must always be in the DOM")
	mustContain(t, body, "In sync · Foundry · pushed 2m ago · 1 of 4 linked", "the signed full string")
	mustContain(t, body, "In sync · 1 of 4", "the signed compact string")
	mustContain(t, body, `class="sync s-ok"`, "the state class")

	// A producer that loses the denominator gets it back rather than shipping
	// the lie. This is the exact failure the operator hit.
	broken := fxHarptos(true)
	broken.Sync.Full = "In sync"
	broken.Sync.Compact = "In sync"
	repaired := render(t, broken)
	if n := strings.Count(repaired, "1 of 4 linked"); n != 2 {
		t.Errorf("a denominator-less pill must be repaired in BOTH strings; got %d occurrences", n)
	}

	// The two dot-less states.
	for _, tc := range []struct{ state, wantClass string }{
		{"pause", "s-pause"}, {"none", "s-pause"},
	} {
		d := fxHarptos(true)
		d.Sync = SyncPill{State: tc.state, Linked: 0, Total: 4}
		out := render(t, d)
		mustContain(t, out, `class="sync `+tc.wantClass+`"`, "state "+tc.state)
		mustContain(t, out, "0 of 4 linked", "the denominator survives state "+tc.state)
		if strings.Contains(out, `class="sync `+tc.wantClass+`" title="`+tc.state+`"><span class="d"`) {
			t.Errorf("state %q must not lead with a state dot", tc.state)
		}
	}
	// …and one that does.
	drift := fxHarptos(true)
	drift.Sync = SyncPill{State: "drift", Full: "Drifted · 3 days · 1 of 4 linked", Compact: "Drifted · 1 of 4", Linked: 1, Total: 4}
	mustContain(t, render(t, drift), `class="sync s-drift" title="drift"><span class="d"`, "drift leads with its dot")
}

// ── the instrument ──────────────────────────────────────────────────────────

// TestGrid_TenDayWeeksAreNative. --week-len drives every column count; a literal
// week length never reaches the markup.
func TestGrid_TenDayWeeksAreNative(t *testing.T) {
	h := render(t, fxHarptos(true))
	mustContain(t, h, "--week-len:10", "the Harptos Block declares its own week length")
	mustContain(t, h, `data-week-len="10"`, "the grid states its week length for the probe")

	g := render(t, fxGregorian())
	mustContain(t, g, "--week-len:7", "the Gregorian Block declares seven")
	mustContain(t, g, `data-week-len="7"`, "the grid states its week length for the probe")

	// One component, two week lengths, identical zone markup.
	for _, zone := range []string{`class="np"`, `class="grid"`, `data-zone="ledger"`} {
		if !strings.Contains(h, zone) || !strings.Contains(g, zone) {
			t.Errorf("%s must render for BOTH week lengths — it is one component", zone)
		}
	}
}

// TestFiveColumnRule_ReachesCellHeaderAndBand. Humans cannot count to ten across
// identical columns; 5+5 is instant.
func TestFiveColumnRule_ReachesCellHeaderAndBand(t *testing.T) {
	d := fxHarptos(true)
	body := render(t, d)

	if got := halfColumn(d.Month); got != 5 {
		t.Fatalf("halfColumn = %d; the fixture puts the ramp step after column 5", got)
	}
	// header
	mustContain(t, body, `<b class="half" title="Nym">`, "the weekday header carries the rule")
	// cells — one per row
	if n := strings.Count(body, `class="cell half"`); n != len(d.Month.Rows) {
		t.Errorf("%d cells carry the half rule; want one per week row (%d)", n, len(d.Month.Rows))
	}
	// band row — the deviation the dispatch asked for. The mockup DECLARES
	// .band.half and never applies it; a ramp step that stops at the era band is
	// a visible seam.
	if n := strings.Count(body, `class="halfrule"`); n != len(d.Month.Rows) {
		t.Errorf("%d band rows carry the five-column ruler; want one per week row (%d)", n, len(d.Month.Rows))
	}
	mustContain(t, body, `class="halfrule" style="grid-column:6/span 1;"`,
		"the ruler sits on the boundary after column 5, stepping over the gutter track")

	// A seven-day week has no ramp step at all.
	g := fxGregorian()
	if got := halfColumn(g.Month); got != 0 {
		t.Errorf("halfColumn on a seven-day week = %d; want 0", got)
	}
	mustNotContain(t, render(t, g), `class="halfrule"`, "a seven-day week needs no counting aid")
}

// TestEraBands_PerWeekRow. An era spanning days 1..17 cannot be one band across
// three rows — `span 17` silently wrecks the grid.
func TestEraBands_PerWeekRow(t *testing.T) {
	d := fxHarptos(true)
	body := render(t, d)

	if n := strings.Count(body, `class="bands"`); n != len(d.Month.Rows) {
		t.Errorf("%d band rows; want one per week row (%d)", n, len(d.Month.Rows))
	}
	for _, span := range []string{"span 10", "span 7", "span 3"} {
		mustContain(t, body, span, "the era is re-cut per row, never spanned across rows")
	}
	if strings.Contains(body, "span 17") || strings.Contains(body, "span 13") {
		t.Error("an era was spanned across week rows — that silently wrecks the subgrid")
	}

	// --bandhue comes from the ERA, not from the calendar. Two eras, two hues,
	// and neither is the calendar's own --cal-harptos.
	mustContain(t, body, "--bandhue:oklch(0.55 0.12 200)", "era 1's own hue")
	mustContain(t, body, "--bandhue:oklch(0.62 0.10 70)", "era 2's own hue")
	if strings.Contains(body, "--bandhue:var(--cal-harptos)") {
		t.Error("--bandhue is hardcoded to the calendar hue — that is calendar-v4.html:1566's defect")
	}

	// the editorial rule at the MID-MONTH boundary, once
	if n := strings.Count(body, "band edge"); n != 1 {
		t.Errorf("%d edge rules; the fixture has exactly one mid-month era boundary", n)
	}
}

// TestSeasonSuffix_Row0Only (§9 deviation 3). The season is also stated in the
// Nameplate; repeating it above every week row turns the era band into a caption
// instead of a boundary.
func TestSeasonSuffix_Row0Only(t *testing.T) {
	d := fxHarptos(true)
	// Deliberately give EVERY row a suffix: the renderer, not the producer, is
	// what must enforce row 0.
	for ri := range d.Month.Rows {
		for bi := range d.Month.Rows[ri].Bands {
			d.Month.Rows[ri].Bands[bi].Suffix = "Long Night"
		}
	}
	body := render(t, d)
	if n := strings.Count(body, `class="sfx"`); n != 1 {
		t.Errorf("%d season suffixes rendered; the season belongs to row 0 only", n)
	}
	mustContain(t, body, `<span class="sfx">· Long Night</span>`, "the suffix form")
}

// TestIntercalary_ClearsTheGutter. The row that a 30-day / ten-day-week month
// has no cell for.
func TestIntercalary_ClearsTheGutter(t *testing.T) {
	body := render(t, fxHarptos(true))
	mustContain(t, body, `class="interc-row"`, "the intercalary row is emitted")
	mustContain(t, body, `class="interc" style="grid-column:2/-1"`,
		"the intercalary row is overridden inline so it clears the week gutter")
	mustContain(t, body, "Midwinter", "the intercalary day is named")

	// A calendar with no intercalary days emits no row.
	mustNotContain(t, render(t, fxGregorian()), `class="interc-row"`,
		"a Gregorian month has no day outside its weeks")
}

// TestCell_ExactlyTwoSubtrees. Named chips and an underline — never four tiers
// of markup. A tier the producer cannot see is a tier that silently rots.
func TestCell_ExactlyTwoSubtrees(t *testing.T) {
	d := fxHarptos(true)
	body := render(t, d)

	named := strings.Count(body, `class="cnamed"`)
	under := strings.Count(body, `class="cunder"`)
	if named != d.Month.Days || under != d.Month.Days {
		t.Errorf("dated cells: %d named subtrees, %d underline subtrees; want %d of each",
			named, under, d.Month.Days)
	}
	// Day 5 carries six events: 2 chips + "+4 more" in the named subtree, and a
	// three-segment underline whose third segment is neutral, in the other.
	mustContain(t, body, "+4 more", "six events on day 5 → two chips and a +4 more row")
	mustContain(t, body, `class="ulseg rest"`, "a fourth event turns the third segment neutral")
	if n := strings.Count(body, `class="ul"`); n == 0 {
		t.Error("no underline rendered at all")
	}
}

// TestMarks_PatternAndAxisAreAlwaysValid. Colour is never load-bearing, so a
// broken axis must still leave a mark with a legible pattern.
func TestMarks_PatternAndAxisAreAlwaysValid(t *testing.T) {
	d := fxHarptos(true)
	// Poison one mark with values that would be a CSS injection if unvalidated.
	d.Month.Rows[0].Cells[2].Marks[0].Axis = `red;} body{display:none} .x{`
	d.Month.Rows[0].Cells[2].Marks[0].Pattern = "p99"
	body := render(t, d)

	mustNotContain(t, body, "body{display:none}", "an axis value may never escape its declaration")
	mustContain(t, body, "--axis:"+AxisFallback, "an unrecognised axis falls back to the neutral grey")
	if strings.Contains(body, "p99") {
		t.Error("an unrecognised pattern reached the class list")
	}

	// Every rendered pattern class is one of the eight locked ones.
	classRe := regexp.MustCompile(`class="(?:rail|ulseg) (p[0-9]+)"`)
	for _, m := range classRe.FindAllStringSubmatch(body, -1) {
		ok := false
		for _, p := range PatternKeys {
			if m[1] == p {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("pattern %q is not one of the eight locked patterns", m[1])
		}
	}
}

// TestPermissionIsAbsence. A player receives no dm_only row: no placeholder, no
// ghost, no greyed entry, and no gold notch hinting that one exists.
func TestPermissionIsAbsence(t *testing.T) {
	gm := render(t, fxHarptos(true))
	player := render(t, fxHarptos(false))

	mustContain(t, gm, `class="dogear"`, "the GM sees the hidden-event notch")
	mustContain(t, gm, `class="audmark"`, "the GM sees the restricted-audience diamond")
	mustNotContain(t, player, `class="dogear"`, "a player must see no trace of a hidden event")

	for _, hidden := range []string{"Barrow scouting", "Into the Barrow", "Warden’s writ due"} {
		if strings.Contains(player, hidden) {
			t.Errorf("a dm_only event (%q) reached a player's DOM", hidden)
		}
		if !strings.Contains(gm, hidden) {
			t.Errorf("the GM is missing %q", hidden)
		}
	}
}

// TestMoons_CappedAndDerived. However many moons a calendar declares, the grid
// draws at most three so the month can never grow with the fiction — and the
// terminator follows from the illumination, so a crescent cannot disagree with
// its own fill.
func TestMoons_CappedAndDerived(t *testing.T) {
	d := fxHarptos(true)
	d.Month.Rows[0].Cells[0].Moons = append(d.Month.Rows[0].Cells[0].Moons,
		MoonDisc{Name: "Sable", Illum: 0.67})
	body := render(t, d)

	firstCell := body[strings.Index(body, `data-day="harptos-of-imix-1"`):]
	firstCell = firstCell[:strings.Index(firstCell, `data-day="harptos-of-imix-2"`)]
	if n := strings.Count(firstCell, `<i class="ph`); n != 3 {
		t.Errorf("day 1 drew %d moon discs; the grid's ceiling is 3 and the rest are named "+
			"in the Almanac", n)
	}

	// The terminator is |cos 2πφ| — full width at BOTH turns and zero at the
	// quarters — and its FILL is what makes new an empty ring and full a solid
	// disc. That is why it can be derived from Illum alone and can never
	// disagree with the disc it sits on.
	if got := moonStyle(MoonDisc{Illum: 0}); !strings.Contains(got, "--term:10.0px;--termfill:var(--surface-card)") {
		t.Errorf("new moon = %q; want a full-width SUBTRACTING terminator (an empty ring)", got)
	}
	if got := moonStyle(MoonDisc{Illum: 1}); !strings.Contains(got, "--term:10.0px;--termfill:var(--rule-structural-strong)") {
		t.Errorf("full moon = %q; want a full-width ADDING terminator (a solid disc)", got)
	}
	if got := moonStyle(MoonDisc{Illum: 0.5}); !strings.Contains(got, "--term:0.0px") {
		t.Errorf("quarter moon = %q; want exactly half, i.e. no terminator ellipse", got)
	}
	// gibbous ADDS, crescent SUBTRACTS
	if !strings.Contains(moonStyle(MoonDisc{Illum: 0.8}), "--termfill:var(--rule-structural-strong)") {
		t.Error("a gibbous terminator must ADD")
	}
	if !strings.Contains(moonStyle(MoonDisc{Illum: 0.2}), "--termfill:var(--surface-card)") {
		t.Error("a crescent terminator must SUBTRACT")
	}
}

// ── zones C and D ───────────────────────────────────────────────────────────

// TestStubs_DockedAtTheRightSizeWithTheChip. The chip is a SIGNED honesty state,
// not a shortcut. What matters for wave 1 is that the Ledger zone is present:
// the full-tier column arithmetic subtracts its 300px unconditionally, so a
// Block that skipped it would flip density at the wrong host width.
func TestStubs_DockedAtTheRightSizeWithTheChip(t *testing.T) {
	body := render(t, fxHarptos(true))
	mustContain(t, body, `data-zone="ledger"`, "the Ledger zone is docked")
	mustContain(t, body, `data-zone="shelf"`, "the Shelf zone is at the foot")
	if n := strings.Count(body, `class="badge need">needs backend`); n != 2 {
		t.Errorf("%d `needs backend` chips; want one on the Ledger and one on the Shelf", n)
	}

	// Hidden means REMOVED, not collapsed: the declared heights are invariant.
	g := render(t, fxGregorian())
	mustNotContain(t, g, `data-zone="shelf"`, "ShelfStub.Hidden must remove the zone")
	mustContain(t, g, `data-zone="ledger"`, "the real-world Block still docks its Ledger")

	noLedger := fxHarptos(true)
	noLedger.Ledger.Hidden = true
	mustNotContain(t, render(t, noLedger), `data-zone="ledger"`, "LedgerStub.Hidden must remove the zone")
}

// ── layers ──────────────────────────────────────────────────────────────────

// TestLayers_DefaultIsMoonsAndTheInvokerIsInert.
func TestLayers_DefaultIsMoonsAndTheInvokerIsInert(t *testing.T) {
	d := fxHarptos(true)
	d.Layers = LayerState{Enabled: []string{"moons"}} // DEF
	body := render(t, d)

	mustContain(t, body, "data-open-layers", "the ⋯ invoker is emitted")
	mustContain(t, body, "disabled", "the switchboard is W-F, so the invoker renders inert")
	mustContain(t, body, `class="ph`, "the default surface is a month with its moon phases")
	mustNotContain(t, body, `class="bands"`, "eras are off by default")
	mustNotContain(t, body, "data-weeknums", "week numbers are off by default")
	mustNotContain(t, body, `class="wknum"`, "week numbers are off by default")
	// …and NOTHING else (data.go: DEF = ["moons"]). The Ledger and Shelf zones
	// are layer-owned and used to render past the registry.
	mustNotContain(t, body, `data-zone="ledger"`, "the ledger layer is off by default")
	mustNotContain(t, body, `data-zone="shelf"`, "the shelf layer is off by default")

	// Switching them on is the producer's decision and the renderer honours it.
	d.Layers = LayerState{Enabled: []string{"moons", "eras", "weeknums"}, HasSwitchboard: true}
	on := render(t, d)
	mustContain(t, on, `class="bands"`, "the eras layer draws its bands")
	// The gutter's WIDTH is a container query (the mockup drops it below 481px of
	// host, which the producer cannot measure); the markup states the viewer's
	// layer choice and nothing more. Writing --gut inline would beat the query.
	mustContain(t, on, "data-weeknums", "the weeknums layer is stated as a fact on the grid")
	mustNotContain(t, on, "--gut:", "the gutter width must never be written inline")
	mustContain(t, on, `<b class="wknum">W1</b>`, "…and labels it")
	if strings.Contains(flatten(on), `data-open-layers disabled`) {
		t.Error("with a switchboard the invoker must not be inert")
	}

	// The three zones wave 1 cannot fill render their zone with the chip rather
	// than fabricating content — and ONLY what the registry names renders: with
	// neither zone key nor "moons" enabled, the stubs and the discs leave too.
	// (This count was 5 when the Ledger and Shelf rendered past the registry;
	// inverted, not deleted, when the layer gate landed.)
	d.Layers = LayerState{Enabled: []string{"legend", "horizon", "moongraph"}}
	zones := render(t, d)
	for _, k := range []string{"legend", "horizon", "moongraph"} {
		mustContain(t, zones, `data-layer="`+k+`"`, k+"'s zone is reserved at the right place")
	}
	if n := strings.Count(zones, `class="badge need">needs backend`); n != 3 {
		t.Errorf("%d `needs backend` chips; want exactly the 3 enabled layer zones", n)
	}
	mustNotContain(t, zones, `data-zone="ledger"`, "the ledger layer is not enabled here")
	mustNotContain(t, zones, `data-zone="shelf"`, "the shelf layer is not enabled here")
	mustNotContain(t, zones, `class="phrow"`, "the moons layer is not enabled here; the discs must leave")
}

// ── no motion ───────────────────────────────────────────────────────────────

// TestNoMotion_InTheMarkup. The stylesheet is checked separately; this catches
// an inline transition, which is the sneakier of the two.
func TestNoMotion_InTheMarkup(t *testing.T) {
	for name, d := range map[string]BlockData{
		"harptos": fxHarptos(true), "player": fxHarptos(false),
		"gregorian": fxGregorian(), "fault": fxDwarvenFault(),
	} {
		body := flatten(render(t, d))
		for _, bad := range []string{"transition", "animation", "@keyframes", "will-change"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s: markup contains %q — there is no motion inside the Block in wave 1", name, bad)
			}
		}
	}
}

// ── plugin agnosticism + asset versioning ───────────────────────────────────

// TestStylesheet_IsVersioned. A bare href="/static/…" in any .templ fails
// TestTemplatesUseAssetURL repo-wide; this additionally proves the emitted URL
// actually carries its digest, so an HTMX-swapped Block cannot pick up a
// build-old sheet.
func TestStylesheet_IsVersioned(t *testing.T) {
	body := render(t, fxHarptos(true))
	mustContain(t, body, StylesheetPath+"?v=", "the sheet must be cache-busted")
	if !strings.Contains(body, `rel="stylesheet"`) {
		t.Error("the Block must carry its own stylesheet: an HTMX-swapped fragment cannot add to <head>")
	}
}

// TestTieMode_IsAnAttributeNotAClass. W-C owns the CSS-only toggle; it needs one
// attribute to flip, and no Go decision to undo. TieMode changes ink LEVEL only —
// no cell may grow, move, or leave the DOM when it flips.
func TestTieMode_IsAnAttributeNotAClass(t *testing.T) {
	d := fxHarptos(true)
	d.Viewer.HostEntity = "ent-77"
	d.Viewer.TiedCount = 4
	d.Viewer.WholeCount = 14

	whole := render(t, d)
	mustContain(t, whole, `data-tie-mode="whole"`, "the mode is an attribute on the Block")
	mustContain(t, whole, "Tied 4", "both counts are stated")
	mustContain(t, whole, "Whole calendar 14", "both counts are stated")

	d.Viewer.TieMode = "tied"
	tied := render(t, d)
	mustContain(t, tied, `data-tie-mode="tied"`, "flipping the mode changes one attribute")
	if strings.Count(tied, `data-tied`) < 2 {
		t.Error("tied days must be marked as a FACT, independent of the current mode")
	}
	// The DOM is otherwise identical: the toggle may not add or remove a cell.
	//
	// PIN REFRESHED at C-CALV4-HOST-P3 §3, intent unchanged. The selected state
	// used to be `aria-pressed` on two spans; the toggle is now a real radio
	// pair, so the state that legitimately moves is the `checked` attribute.
	// Everything else this assertion protects — no cell added, removed or
	// re-ordered — is untouched.
	stripMode := func(s string) string {
		return strings.NewReplacer(`data-tie-mode="tied"`, "", `data-tie-mode="whole"`, "",
			` checked`, "").Replace(s)
	}
	if stripMode(whole) != stripMode(tied) {
		t.Error("flipping the tie mode changed the DOM beyond the mode attribute and the " +
			"checked radio — ink LEVEL only, or the counts become differenceable")
	}

	// Off an entity page there is no tie control at all.
	mustNotContain(t, render(t, fxHarptos(true)), `class="seg tie"`,
		"the tie toggle is entity-hosted only")
}

// TestMarkCap_AgreesWithTheProducer pins the seam between the producer's
// per-cell ceiling and this renderer's split of it.
//
// WHY THIS EXISTS. C-CALV4-SPINE-P2 landed BEFORE this render tier, so the two
// halves of the contract were written by chats that could not see each other.
// The producer keeps the FULL viewer-visible mark list and reports the overflow
// (`blockCapMarks`, block_projection.go); this renderer decides how many chips
// that means. Both must arrive at "three chips, or two plus a +n more row, never
// both" from opposite directions, or the cell stops being a fixed 84px.
//
// It is pinned HERE, from the renderer's side, because this package does not own
// internal/plugins/** and must not add a test there. The producer's own cap
// constants are 3 / 2 (block_geometry.go:64-65); this table is what those
// numbers have to mean once they reach a cell.
func TestMarkCap_AgreesWithTheProducer(t *testing.T) {
	// producerSplit mirrors blockCapMarks: never trims, reports the overflow.
	producerSplit := func(n int) ([]Mark, int) {
		marks := make([]Mark, n)
		for i := range marks {
			marks[i] = Mark{Title: "e", Axis: "var(--ev-social)", Pattern: "p1"}
		}
		if n <= namedChipCap {
			return marks, 0
		}
		return marks, n - (namedChipCap - 1)
	}

	for n := 0; n <= 8; n++ {
		marks, more := producerSplit(n)
		c := DayCell{Day: 1, Col: 1, Marks: marks, MoreCount: more}

		chips, overflow := len(chipsFor(c)), moreCount(c)

		// The invariant: chips + the folded tail always accounts for every mark,
		// and a cell never draws three chips AND an overflow row.
		if overflow > 0 && chips != namedChipCap-1 {
			t.Errorf("%d marks: %d chips beside a +%d more row; the signed cell draws two, "+
				"never three, when it also carries the affordance", n, chips, overflow)
		}
		if overflow == 0 && chips != n {
			t.Errorf("%d marks: %d chips and no overflow row — a mark went missing", n, chips)
		}
		if chips+overflow != n {
			t.Errorf("%d marks: %d chips + %d folded = %d; the counts must reconcile or the "+
				"'+n more' lies", n, chips, overflow, chips+overflow)
		}

		// The underline half of the same cell must still see real marks: a
		// producer that trimmed to the chip cap would silently cost it segments.
		segs := len(underlineSegs(c))
		if want := min(n, underlineCap); segs != want {
			t.Errorf("%d marks: underline drew %d segments, want %d — the producer must NOT "+
				"trim Marks, because density is a container query and it cannot know which "+
				"subtree will be visible", n, segs, want)
		}
		if n > underlineCap && !underlineRestAt(c, segs-1) {
			t.Errorf("%d marks: the last underline segment must turn neutral", n)
		}
	}
}

// TestUnderlineRest_MoreCountIsOverlapping pins the §3.4 ruling (data.go:
// MoreCount is OVERLAPPING, not additive) on the renderer's other counting
// site. The day total is len(Marks), so the neutral overflow segment may only
// appear when REAL marks are off the bar — MoreCount describes the chip fold,
// not extra events beyond the list. A renderer that adds it turns the last
// segment neutral on a fully-shown day, claiming events that do not exist.
//
// This lives widget-side because the seam cannot reach it: the producer only
// sets MoreCount > 0 when len(Marks) already exceeds the underline cap, the one
// region where the additive and overlapping formulas agree.
func TestUnderlineRest_MoreCountIsOverlapping(t *testing.T) {
	// Three marks, three segments: the whole day is on the bar. No segment may
	// turn neutral, whatever chip fold the cell declares.
	folded := DayCell{Day: 1, Col: 1,
		Marks:     []Mark{{Title: "a"}, {Title: "b"}, {Title: "c"}},
		MoreCount: 2,
	}
	for i := range underlineSegs(folded) {
		if underlineRestAt(folded, i) {
			t.Errorf("segment %d turned neutral on a fully-shown day — MoreCount folds "+
				"chips, it never adds events beyond len(Marks)", i)
		}
	}

	// Four marks, three segments: one real mark is off the bar, so the last
	// segment MUST still turn neutral — the fix may not overshoot into
	// never-neutral.
	over := DayCell{Day: 2, Col: 2,
		Marks: []Mark{{Title: "a"}, {Title: "b"}, {Title: "c"}, {Title: "d"}},
	}
	if !underlineRestAt(over, len(underlineSegs(over))-1) {
		t.Error("the last segment must turn neutral when len(Marks) exceeds the segments shown")
	}
}

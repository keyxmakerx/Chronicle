package calendar_block

// shelf_test.go — Zone D's RENDERER behaviour, on hand-written BlockData.
//
// SEAM DISCIPLINE. Everything here is a claim about what this package draws
// GIVEN a BlockData. Whether the PRODUCER fills the Almanac register, flips the
// zone's flag or counts the viewer's own set is asserted in package calendar
// (block_almanac_test.go, block_seam_test.go, block_count_oracle_test.go),
// because a test whose author writes the BlockData cannot make any of those
// claims without them being vacuous.

import (
	"regexp"
	"strings"
	"testing"
)

// fxShelf is a full-tier GM Block with both zone layers on.
func fxShelf(t *testing.T, gm bool) BlockData {
	t.Helper()
	d := fxHarptos(gm)
	d.Layers = LayerState{Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"}}
	return d
}

// shelfZone slices the render to Zone D's markup. Bounds are checked rather
// than bare (COMMON §3).
func shelfZone(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `data-zone="shelf"`)
	if i < 0 {
		t.Fatal(`no data-zone="shelf" in the render — seamLayerSurfaces keys on that exact marker`)
	}
	return body[i:]
}

// TestShelf_KeepsItsZoneMarkerAndItsSignedDOMOrder.
//
// data-zone="shelf" IS THE CONTRACT, not a convenience:
// TestSeam_EnabledLayerSetMatchesWhatRenders keys the whole layer-registry
// proof on it, so a rename would turn that test green while proving nothing.
// The file changed name and the body was replaced; the marker did not move.
//
// The signed DOM order is the strip then the body (cv4:1774-1801). It is
// asserted by RELATIVE POSITION rather than by a byte-exact fragment, so
// reformatting the template cannot red CI with no behaviour change.
func TestShelf_KeepsItsZoneMarkerAndItsSignedDOMOrder(t *testing.T) {
	z := shelfZone(t, render(t, fxShelf(t, true)))

	strip := strings.Index(z, `class="st"`)
	body := strings.Index(z, `class="sp2"`)
	if strip < 0 || body < 0 {
		t.Fatalf("the Shelf must carry both its strip and its body (.st %d, .sp2 %d)", strip, body)
	}
	if strip > body {
		t.Error("the strip must precede the body — the signed zone is a strip over a scroller")
	}
	// The tabs live in the strip, the panels in the body. A panel that drifted
	// into the strip would break the 34px geometry the Block's declared
	// heights depend on.
	firstPane := strings.Index(z, `data-spane=`)
	if firstPane < body {
		t.Error("a panel is rendered outside .sp2 — the 132px scroller is what bounds the zone")
	}
}

// TestShelf_HiddenRemovesTheZoneRatherThanCollapsingIt.
//
// The Bench's real-world Block renders with noShelf. A hidden Shelf must
// actually REMOVE the zone rather than collapse it, or the Block's declared
// heights stop being invariant — the sentence shelf_stub.templ carried into
// wave 2 and the reason the branch exists at all.
func TestShelf_HiddenRemovesTheZoneRatherThanCollapsingIt(t *testing.T) {
	d := fxShelf(t, true)
	d.Shelf.Hidden = true
	body := render(t, d)

	mustNotContain(t, body, `data-zone="shelf"`,
		"a hidden Shelf must REMOVE the zone; a zero-height one still contributes a border and a box")
	mustNotContain(t, body, `class="st"`, "the strip goes with the zone")
	mustNotContain(t, body, `data-spane=`, "no panel survives a removed zone")
	// The Ledger is untouched by the Shelf's removal: they are separate zones
	// and the full-tier column arithmetic still subtracts the Ledger's 300px.
	mustContain(t, body, `data-zone="ledger"`, "removing the Shelf must not remove the Ledger")

	// The LAYER key removes it too, and by a different route.
	off := fxShelf(t, true)
	off.Layers.Enabled = []string{"moons", "ledger"}
	mustNotContain(t, render(t, off), `data-zone="shelf"`,
		"the shelf layer key gates the zone as the registry says it does")
}

// TestShelf_UpcomingReusesTheLedgersRowPrimitiveVerbatim.
//
// THE SHELF'S ROW PRIMITIVE **IS** THE LEDGER'S (cv4:1794, :1798 — the signed
// panel calls ledgerRow(e, 'full', i)). This is the acceptance line the
// dispatch asks to be able to grep: there is no second row implementation, so
// the Shelf's rows carry the same class, the same ANSWER key, the same gold
// rail / audience split and the same tie ink as the Ledger's.
//
// The check is structural rather than a source-text grep: every row in Zone D
// must be byte-identical to a row that also appears in Zone C. A forked
// primitive would have to reproduce the whole row to pass, at which point it is
// not a fork.
func TestShelf_UpcomingReusesTheLedgersRowPrimitiveVerbatim(t *testing.T) {
	body := render(t, fxShelf(t, true))
	i := strings.Index(body, `data-zone="shelf"`)
	if i < 0 {
		t.Fatal("no Shelf zone")
	}
	ledger, shelf := body[:i], body[i:]

	// A Ledger row contains no nested <div> — every child is a <span> or an
	// <i> — so the first `</div>` after the opening tag closes it.
	rowRe := regexp.MustCompile(`(?s)<div class="lrow.*?</div>`)
	shelfRows := rowRe.FindAllString(shelf, -1)
	if len(shelfRows) == 0 {
		t.Fatal("the Upcoming panel listed no rows — the comparison would be vacuous")
	}
	for _, r := range shelfRows {
		if !strings.Contains(ledger, r) {
			t.Errorf("a Shelf row is not byte-identical to any Ledger row — the Shelf's row "+
				"primitive IS the Ledger's, not an adaptation of it:\n%s", r)
		}
	}
}

// TestShelf_EveryUpcomingRowCarriesItsAnswerKey is guard B4 inside Zone D.
//
// A dated surface that forgets data-day simply stops answering, and that is
// invisible in code review — which is exactly why the guard exists. The key is
// the dayKey namespace ("<slug>-<day>"), the same one the grid and the Ledger
// use, because a partner surface anywhere in the product matches on it.
func TestShelf_EveryUpcomingRowCarriesItsAnswerKey(t *testing.T) {
	d := fxShelf(t, true)
	z := shelfZone(t, render(t, d))

	rows := strings.Count(z, `class="lrow `)
	if rows == 0 {
		t.Fatal("no Upcoming rows — the key assertion would be vacuous")
	}
	keyed := regexp.MustCompile(`class="lrow[^"]*" data-day="` + regexp.QuoteMeta(d.CalendarSlug) + `-\d+"`)
	if n := len(keyed.FindAllString(z, -1)); n != rows {
		t.Errorf("%d of %d Upcoming rows carry a data-day in the dayKey namespace — a keyless "+
			"row reds guard B4 and, worse, stops the Ledger answering it", n, rows)
	}
	// No row may claim an INTERCALARY key: an intercalary day's ordinal is its
	// own scale and comparing it against TodayDay would order it wrongly, so
	// the panel leaves those days to the Ledger.
	mustNotContain(t, z, `-i1"`,
		"the Upcoming panel is ordinal-day-scoped; an intercalary ordinal is a different scale")
}

// TestShelf_UpcomingIsMonthScopedAndCappedAtFour is [S1], SIGNED, at the
// renderer.
//
// TODAY + UP TO FOUR LATER DAYS, ON ONE CALENDAR. The cap is normative because
// `.sp2` is a fixed 132px and the cap is what keeps its scroller from becoming
// the panel. When the cap bites, the panel SAYS SO — a list that stops without
// saying it stopped is the same class of omission as a count printed with no
// denominator.
func TestShelf_UpcomingIsMonthScopedAndCappedAtFour(t *testing.T) {
	d := fxShelf(t, true)
	v := newShelfUpcoming(d)

	if len(v.Later) > shelfUpcomingCap {
		t.Errorf("the Upcoming panel listed %d later rows; the signed cap is %d",
			len(v.Later), shelfUpcomingCap)
	}
	for _, l := range v.Today {
		if l.Day != d.Month.TodayDay {
			t.Errorf("a Today row sits on day %d, not %d", l.Day, d.Month.TodayDay)
		}
	}
	for _, l := range v.Later {
		if l.Day <= d.Month.TodayDay {
			t.Errorf("a This-month row sits on day %d, at or before today (%d)", l.Day, d.Month.TodayDay)
		}
	}

	z := shelfZone(t, render(t, d))
	mustContain(t, z, ">Today<", "the signed panel opens on a Today band")
	mustContain(t, z, ">This month<", "and a This month band")

	// THE CAP MUST ACTUALLY BITE SOMEWHERE, or every assertion above passes
	// vacuously: the shared fixture's month carries only three events after
	// today, so it can never exercise the bound it is supposed to pin. A
	// deliberately dense month is built here for that one purpose.
	dense := fxShelf(t, true)
	spare := dense.Month.Rows[0].Cells[2].Marks
	if len(spare) == 0 {
		t.Fatal("the fixture's first row carries no mark to clone")
	}
	for r := range dense.Month.Rows {
		for c := range dense.Month.Rows[r].Cells {
			cell := &dense.Month.Rows[r].Cells[c]
			if cell.Day > dense.Month.TodayDay {
				cell.Marks = append(cell.Marks, spare[0])
			}
		}
	}
	dv := newShelfUpcoming(dense)
	if !dv.Truncated {
		t.Fatal("a month with more than four later events did not trip the cap — the bound " +
			"this test exists to pin is not being reached")
	}
	if len(dv.Later) != shelfUpcomingCap {
		t.Errorf("a capped panel listed %d later rows, want exactly %d", len(dv.Later), shelfUpcomingCap)
	}
	mustContain(t, shelfZone(t, render(t, dense)), "the Ledger lists the whole month",
		"a capped list must SAY it capped — a list that stops silently is the same class "+
			"of omission as a count printed with no denominator")
}

// TestShelf_UpcomingZeroStatesAreTwoDifferentClaims.
//
// "Nothing today" and "Today is not in Deepwinter" are different statements,
// and printing the first for the second would assert the viewer is looking at
// the current month when they are not. It is the same split ledgerZeroAll and
// ledgerZeroDay make one zone up, and it is signed there for the same reason.
func TestShelf_UpcomingZeroStatesAreTwoDifferentClaims(t *testing.T) {
	// A month with no events at all, and today inside it.
	empty := fxShelf(t, true)
	for r := range empty.Month.Rows {
		for c := range empty.Month.Rows[r].Cells {
			empty.Month.Rows[r].Cells[c].Marks = nil
		}
	}
	for i := range empty.Month.Intercalary {
		empty.Month.Intercalary[i].Marks = nil
	}
	z := shelfZone(t, render(t, empty))
	mustContain(t, z, "Nothing today.", "an empty today says so")
	mustContain(t, z, "Nothing further in Deepwinter.", "and an empty rest-of-month names the month")

	// The same month, but today is elsewhere.
	away := empty
	away.Month.TodayDay = 0
	z2 := shelfZone(t, render(t, away))
	mustContain(t, z2, "Today is not in Deepwinter.",
		"a month that does not contain today must say that, not 'nothing today'")
	mustNotContain(t, z2, "Nothing today.",
		"'nothing today' asserts the viewer is looking at the current month")
}

// TestShelf_TabsAreCSSOnlyAndExactlyOneIsPressed is [S7], SIGNED.
//
// A bound Block renders under context.Background(), so the mockup's `?alm=`
// query param is unreachable; per-viewer persistence is W-F's store, which does
// not exist; and there is no JS in this package. The tabs are therefore the
// same radio mechanism the tie toggle proved. Exactly one must be checked, or
// the zone renders with no panel at all.
func TestShelf_TabsAreCSSOnlyAndExactlyOneIsPressed(t *testing.T) {
	d := fxShelf(t, true)
	body := render(t, d)
	z := shelfZone(t, body)

	tabs := strings.Count(z, `class="shelfpick"`)
	if tabs < 2 {
		t.Fatalf("the Shelf has %d tabs; a GM's Shelf carries at least Upcoming and Filters", tabs)
	}
	checked := 0
	for _, frag := range strings.Split(z, "<input ") {
		if strings.Contains(frag, `class="shelfpick"`) && strings.Contains(frag, " checked") {
			checked++
		}
	}
	if checked != 1 {
		t.Errorf("%d Shelf tabs are checked, want exactly 1 — a zone with no pressed tab "+
			"renders no panel, and two would render two", checked)
	}
	// Every tab addresses its own input, or the visible control does nothing.
	for _, key := range []string{shelfTabUpcoming, shelfTabFilters} {
		id := shelfPickInputID(d, key)
		mustContain(t, z, `id="`+id+`"`, "the "+key+" tab needs its own input")
		mustContain(t, z, `for="`+id+`"`, "and a label bound to it")
	}
	// Zero JS, zero handlers. Not "little": none.
	if strings.Contains(z, "<script") || strings.Contains(z, "onclick") {
		t.Error("the Shelf must ship no script and no inline handler — a <script> inside an " +
			"HTMX-swapped fragment never runs")
	}
	// Guard B3: controls end in `-pick` and never reuse an <html> state marker.
	mustContain(t, z, `data-shelf-pick="`, "the tab control is `-pick`-suffixed (guard B3)")
	mustNotContain(t, z, `data-moonstyle`, "no control may reuse an <html> state-marker noun")
}

// TestShelf_TabGroupIsAPureFunctionOfTheData. The Bench composes four Blocks on
// one page: two sharing a radio name would fight over one piece of state, while
// the SAME Block re-rendered by an HTMX binding swap must keep the name or the
// viewer's chosen tab is lost in the swap. A counter satisfies the first and
// breaks the second.
func TestShelf_TabGroupIsAPureFunctionOfTheData(t *testing.T) {
	a := fxShelf(t, true)
	b := fxShelf(t, true)
	if shelfPickGroupName(a) != shelfPickGroupName(b) {
		t.Error("the same data produced two group names — an HTMX swap would lose the chosen tab")
	}
	other := fxShelf(t, true)
	other.CalendarSlug = "other-cal"
	if shelfPickGroupName(a) == shelfPickGroupName(other) {
		t.Error("two calendars share a radio group — two Blocks on the Bench would fight over it")
	}
	hosted := fxShelf(t, true)
	hosted.Viewer.HostEntity = "ent-77"
	if shelfPickGroupName(a) == shelfPickGroupName(hosted) {
		t.Error("the same calendar on two hosts shares a group — the entity page and the " +
			"Bench would share one tab")
	}
}

// TestShelf_PlayerGetsNoFiltersTab is [S10], SIGNED: a 2-tab Shelf for players
// and a 3-tab Shelf for the GM.
//
// It is the 2026-07-27 needs-backend-audience ruling applied one level down —
// "for a player the zone simply does not appear" — and it is ABSENCE, not a
// disabled control. Flagged in the books as what it is: the first per-role
// difference inside a CHROME STRIP rather than inside content.
func TestShelf_PlayerGetsNoFiltersTab(t *testing.T) {
	player := shelfZone(t, render(t, fxShelf(t, false)))

	mustNotContain(t, player, `data-shelf-pick="filters"`,
		"a player receives no Filters tab — absence, not a disabled control")
	mustNotContain(t, player, `data-spane="filters"`, "and no Filters panel")
	mustNotContain(t, player, "needs backend",
		"a `needs backend` chip never renders to a player")
	mustNotContain(t, player, "disabled",
		"the difference is ABSENCE; a disabled control would advertise the surface it withholds")
	// The player's Shelf is still a real Shelf.
	mustContain(t, player, `data-shelf-pick="upcoming"`, "the player keeps Upcoming")
	mustContain(t, player, ">Today<", "and its bands")

	gm := shelfZone(t, render(t, fxShelf(t, true)))
	mustContain(t, gm, `data-shelf-pick="filters"`, "the GM keeps Filters")
}

// TestShelf_FiltersShipsTheTabAndNotTheEngine is [S2], SIGNED.
//
// No filter state, no query, no persistence and no preference store exists in
// the repo. So the panel is ONE CHIP: no controls, no inert chips, no
// fabricated state, and no pin field. An inert chip row would be exactly the
// fabricated state `needs backend` exists to replace.
func TestShelf_FiltersShipsTheTabAndNotTheEngine(t *testing.T) {
	z := shelfZone(t, render(t, fxShelf(t, true)))

	mustContain(t, z, `class="badge need">needs backend`,
		"the Filters panel states the honest thing about itself")
	for _, fabricated := range []string{
		`type="checkbox"`, `<select`, `data-filter`, "Type ", "Owner ", "Hidden ",
	} {
		if strings.Contains(z, fabricated) {
			t.Errorf("the Filters panel emitted %q — with no engine behind it, a control is "+
				"fabricated state ([S2]: the tab ships, the engine does not)", fabricated)
		}
	}
}

// TestShelf_ShipsNoSkyPickerAndNoFiltersBadge — two pre-authorised divergences
// from the signed stills ([S8], SIGNED), asserted so a later hand cannot
// "restore" either one by reading a render rather than the contract.
//
//   - the `sky: graph | words | moons | off` segmented control appears in
//     v4-sky-almanac-month.png and v4-sky-almanac-moons.png and has NO EMITTER
//     anywhere in the signed HTML. Those stills are round-7/8 output; design
//     notes L29 (round 11) struck the control: "the second menu is gone …
//     the switchboard is now the only place layers are chosen".
//   - `filters 2` is a number with nothing behind it, which is the exact shape
//     the sync-pill ruling forbids. It returns WITH the engine.
func TestShelf_ShipsNoSkyPickerAndNoFiltersBadge(t *testing.T) {
	z := shelfZone(t, render(t, fxShelf(t, true)))

	for _, stale := range []string{"data-moonpick", ">graph<", ">words<", "sky:"} {
		if strings.Contains(z, stale) {
			t.Errorf("the stale sky picker reached the strip (%q) — design notes L29 struck it", stale)
		}
	}
	if regexp.MustCompile(`filters \d`).MatchString(z) {
		t.Error("the `filters N` badge rendered — a count with no engine behind it is the " +
			"shape `needs backend` exists to replace ([S4])")
	}
	// And no `Shelf` caption: the signed strip carries its tabs and nothing else.
	mustNotContain(t, z, `class="cap">Shelf<`,
		"the caption named a zone that had no content to name itself; the tabs name it now")
}

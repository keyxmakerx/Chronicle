// bench_test.go — C-CALV4-BENCH-P4.
//
// Three things are pinned here, and they are the three the dispatch calls
// load-bearing:
//
//  1. PERMISSION IS ABSENCE. The three GM ribbon tiles and the warnrow are not
//     in a player's DOM. Not greyed, not disabled, not rendered-then-hidden.
//  2. THE PROPORTION RULE. One primary Block, one real-world Block, then rows.
//     No calendar count and no width turns a row into a panel.
//  3. THE HONESTY STATES. The fault prints where the date would go and the row
//     carries no date element; design-ahead tiles carry the signed chip and
//     never a fabricated zero; the sync denominator never drops.
//
// PIN DISCIPLINE (COMMON §3): every assertion flattens nothing and uses
// strings.Contains / strings.Count. Nothing here uses a bare strings.Index
// result as a slice bound — that PANICS on a rename instead of failing cleanly,
// which is how calendar_v2_design_pass1_test.go:147 behaves and why it must
// never be copied.
package calendar

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// --- fixtures ---------------------------------------------------------------

// benchFxHarptos is a resolvable in-world calendar: 2 months, a 10-day week,
// the campaign default.
func benchFxHarptos() Calendar {
	return Calendar{
		ID: "cal-harptos", CampaignID: "camp-1", Name: "Harptos of Imix",
		Mode: ModeFantasy, IsDefault: true,
		CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 14,
		Months: []Month{
			{Name: "Deepwinter", Days: 30, SortOrder: 0},
			{Name: "Thawrun", Days: 30, SortOrder: 1},
		},
		Weekdays: []Weekday{
			{Name: "Sar"}, {Name: "Mol"}, {Name: "Zor"}, {Name: "Wir"}, {Name: "Nym"},
			{Name: "Lyr"}, {Name: "Tam"}, {Name: "Kes"}, {Name: "Vel"}, {Name: "Odd"},
		},
	}
}

// benchFxGregorian is the real-world calendar the second Block renders.
func benchFxGregorian() Calendar {
	c := Calendar{
		ID: "cal-real", CampaignID: "camp-1", Name: "Real world / Gregorian",
		Mode: ModeRealLife, SortOrder: 1,
		CurrentYear: 2026, CurrentMonth: 7, CurrentDay: 25,
	}
	for i, n := range []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"} {
		days := 31
		switch n {
		case "April", "June", "September", "November":
			days = 30
		case "February":
			days = 28
		}
		c.Months = append(c.Months, Month{Name: n, Days: days, SortOrder: i})
	}
	for _, n := range []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"} {
		c.Weekdays = append(c.Weekdays, Weekday{Name: n})
	}
	return c
}

// benchFxElven is a healthy subordinate calendar.
func benchFxElven() Calendar {
	return Calendar{
		ID: "cal-elven", CampaignID: "camp-1", Name: "Elven Reckoning",
		Mode: ModeFantasy, SortOrder: 2,
		CurrentYear: 218, CurrentMonth: 1, CurrentDay: 9,
		Months:   []Month{{Name: "Sithrel", Days: 40, SortOrder: 0}},
		Weekdays: []Weekday{{Name: "One"}, {Name: "Two"}, {Name: "Three"}, {Name: "Four"}},
	}
}

// benchFxDwarven is the MISCONFIGURED calendar: zero months, so its date
// cannot resolve. It is the shipped analogue of the signed Dwarven warnrow.
func benchFxDwarven() Calendar {
	return Calendar{
		ID: "cal-dwarven", CampaignID: "camp-1", Name: "Dwarven Deep-count",
		Mode: ModeFantasy, SortOrder: 3,
		CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
	}
}

func benchFxAll() []Calendar {
	return []Calendar{benchFxHarptos(), benchFxGregorian(), benchFxElven(), benchFxDwarven()}
}

// benchFxData builds a BenchData the way buildBench does, minus the IO. The
// Blocks are projected through the REAL projection so the DOM under test is the
// DOM production renders.
func benchFxData(isGM, isOwner bool) BenchData {
	cals := benchFxAll()
	if !isGM {
		// A player never receives the misconfigured calendar's row, and this
		// fixture keeps the calendar itself in the list so the count line is
		// exercised for both roles.
		cals = cals[:3]
	}
	primary, realWorld, rows := benchClassify(cals, "cal-harptos")
	role := 1
	if isGM {
		role = 3
	}
	viewer := BlockViewer{UserID: "u-1", Role: role}
	data := BenchData{
		CampaignID: "camp-1", CampaignName: "Imix",
		IsGM: isGM, IsOwner: isOwner, ShowNewSlot: isOwner,
		CalendarCount: len(cals),
		NeedsSetup:    benchNeedsSetup(cals),
		Rsvp:          benchRsvpPanel(),
		NextUp:        benchFxNextUp(cals, isGM),
	}
	if primary != nil {
		data.Primary = benchFxBlock(primary, viewer, false)
	}
	if realWorld != nil {
		data.RealWorld = benchFxBlock(realWorld, viewer, true)
	}
	data.Rows = benchRows(rows, "cal-harptos", "camp-1", isGM)
	data.Ribbon = benchRibbon(benchRibbonInput{
		IsGM: isGM, CampaignID: "camp-1", Primary: primary, Block: data.Primary,
		NextUp:    data.NextUp,
		Sync:      calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4, Full: "In sync · 1 of 4 linked"},
		Attention: benchAttentionRows(cals, "camp-1"),
	})
	return data
}

func benchFxBlock(cal *Calendar, viewer BlockViewer, noShelf bool) *BenchBlock {
	d := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: viewer, MonthIndex: cal.CurrentMonth - 1, Year: cal.CurrentYear,
		Sync:        calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4, Full: "In sync · 1 of 4 linked", Compact: "In sync · 1 of 4"},
		ShelfHidden: noShelf, MoonCap: benchMoonCap,
	})
	d.Layers = benchBlockLayers()
	return &BenchBlock{Data: d, Manage: benchManage(cal, "cal-harptos", "camp-1")}
}

// benchFxNextUp builds an index the way UpcomingAcrossCalendars would, with one
// dm_only row so the GM/player split is exercised end to end.
func benchFxNextUp(cals []Calendar, isGM bool) BenchNextUp {
	harptos := cals[0]
	var rows []BlockUpcoming
	rows = append(rows, BlockUpcoming{
		Event:    Event{ID: "e-vigil", Name: "Emberfall Vigil", Year: 1523, Month: 1, Day: 14},
		Calendar: &harptos, Date: BlockDate{Year: 1523, Month: 1, Day: 14}, DaysUntil: 0,
	})
	if isGM {
		rows = append(rows, BlockUpcoming{
			Event: Event{ID: "e-writ", Name: "Warden's writ due", Visibility: "dm_only",
				Year: 1523, Month: 1, Day: 26},
			Calendar: &harptos, Date: BlockDate{Year: 1523, Month: 1, Day: 26}, DaysUntil: 12,
		})
	}
	return benchNextUp(rows, isGM, "camp-1")
}

func renderBench(t *testing.T, data BenchData) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: data.CampaignID, Name: data.CampaignName}}
	var sb strings.Builder
	if err := BenchPage(cc, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render bench: %v", err)
	}
	return sb.String()
}

// --- 1. permission is absence ----------------------------------------------

// The three GM tiles are ABSENT from a player's DOM. This is the dispatch's §3
// acceptance line and it is asserted on the MARKERS, not on the copy, so a
// wording change cannot silently retire the guarantee.
func TestBench_PlayerRibbonOmitsGMTilesFromTheDOM(t *testing.T) {
	gm := renderBench(t, benchFxData(true, true))
	for _, want := range []string{
		`data-bench-tile="today"`, `data-bench-tile="nextup"`, `data-bench-tile="session"`,
		`data-bench-tile="sync"`, `data-bench-tile="attention"`, `data-bench-tile="horizon"`,
	} {
		if !strings.Contains(gm, want) {
			t.Errorf("GM ribbon missing %q", want)
		}
	}

	player := renderBench(t, benchFxData(false, false))
	for _, want := range []string{
		`data-bench-tile="today"`, `data-bench-tile="nextup"`, `data-bench-tile="session"`,
	} {
		if !strings.Contains(player, want) {
			t.Errorf("player ribbon missing %q", want)
		}
	}
	for _, forbidden := range []string{
		`data-bench-tile="sync"`, `data-bench-tile="attention"`, `data-bench-tile="horizon"`,
		// Not greyed, not disabled, not rendered-then-hidden: the tiles' own
		// copy must not survive either.
		"Needs attention", "Horizon", "the denominator is the point",
	} {
		if strings.Contains(player, forbidden) {
			t.Errorf("player DOM must not contain %q — permission is ABSENCE", forbidden)
		}
	}
}

// A player's ribbon is exactly three tiles. Counting is the half of the
// guarantee that a marker-presence assertion cannot make.
func TestBench_PlayerRibbonIsExactlyThreeTiles(t *testing.T) {
	if n := strings.Count(renderBench(t, benchFxData(false, false)), "data-bench-tile="); n != 3 {
		t.Errorf("player ribbon has %d tiles; want exactly 3", n)
	}
	if n := strings.Count(renderBench(t, benchFxData(true, true)), "data-bench-tile="); n != 6 {
		t.Errorf("GM ribbon has %d tiles; want exactly 6", n)
	}
}

// THE WARN TREATMENT IS GM-ONLY, THE ROW IS NOT. A player whose own calendar is
// misconfigured must still see that the calendar exists — a row that vanished
// silently is the worst of the three possible answers — but gets no diagnosis
// and no setup affordance, and no date element either way.
func TestBench_WarnTreatmentIsGMOnlyButTheRowIsNot(t *testing.T) {
	gmRows := benchRows([]*Calendar{ptrCal(benchFxDwarven())}, "", "camp-1", true)
	if len(gmRows) != 1 || !gmRows[0].Warn {
		t.Fatalf("a GM should get the warnrow; got %+v", gmRows)
	}
	if gmRows[0].Name != "" || gmRows[0].Fault == "" {
		t.Errorf("the GM's fault takes the NAME's slot; got %+v", gmRows[0])
	}

	playerRows := benchRows([]*Calendar{ptrCal(benchFxDwarven())}, "", "camp-1", false)
	if len(playerRows) != 1 {
		t.Fatalf("a player must still see the calendar exists; got %d rows", len(playerRows))
	}
	p := playerRows[0]
	if p.Warn || p.Fault != "" {
		t.Errorf("a player gets no diagnosis and no warn treatment; got %+v", p)
	}
	if p.Name == "" {
		t.Error("a player's row still names the calendar")
	}
	if p.DateLabel != "" {
		t.Errorf("neither role gets a date element on an unresolvable calendar; got %q", p.DateLabel)
	}
}

// --- 2. the proportion rule -------------------------------------------------

// One primary Block, one real-world Block, everything else a ROW — whatever the
// calendar count. This is the rule that stops the Bench decaying back into the
// card grid it replaces.
func TestBench_ProportionRuleHoldsAtEveryCalendarCount(t *testing.T) {
	cases := []struct {
		name     string
		cals     []Calendar
		wantPrim string
		wantReal string
		wantRows int
	}{
		{"one in-world calendar", []Calendar{benchFxHarptos()}, "cal-harptos", "", 0},
		{"in-world + real world", []Calendar{benchFxHarptos(), benchFxGregorian()}, "cal-harptos", "cal-real", 0},
		{"the signed four", benchFxAll(), "cal-harptos", "cal-real", 2},
		{"two real-world calendars promote only one",
			[]Calendar{benchFxGregorian(), {ID: "cal-real2", Name: "Other real", Mode: ModeRealLife}},
			"cal-real", "cal-real2", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary, realWorld, rows := benchClassify(tc.cals, "")
			if primary == nil || primary.ID != tc.wantPrim {
				t.Fatalf("primary = %v; want %q", primary, tc.wantPrim)
			}
			switch {
			case tc.wantReal == "" && realWorld != nil:
				t.Errorf("real-world = %q; want none", realWorld.ID)
			case tc.wantReal != "" && (realWorld == nil || realWorld.ID != tc.wantReal):
				t.Errorf("real-world = %v; want %q", realWorld, tc.wantReal)
			}
			if len(rows) != tc.wantRows {
				t.Errorf("rows = %d; want %d", len(rows), tc.wantRows)
			}
		})
	}
}

// The rendered stack holds at most two Blocks and the rest are rows — asserted
// on the composed HTML, because the classification being right does not prove
// the template obeyed it.
func TestBench_StackRendersTwoBlocksAndTheRestAsRows(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	if n := strings.Count(html, "data-bench-block="); n != 2 {
		t.Errorf("stack rendered %d Blocks; the proportion rule allows exactly 2", n)
	}
	if !strings.Contains(html, `data-bench-block="primary"`) || !strings.Contains(html, `data-bench-block="real-world"`) {
		t.Error("the two stack Blocks must be labelled primary and real-world")
	}
	if n := strings.Count(html, "data-bench-row"); n != 2 {
		t.Errorf("rendered %d subordinate rows; the four-calendar fixture has 2", n)
	}
	// The real-world Block renders noShelf; the primary keeps its Shelf.
	if !strings.Contains(html, "data-cal-block") {
		t.Error("the Bench must render real Blocks, not a placeholder")
	}
}

// The contract's DOM order is exact, and order is a thing a marker-presence
// test cannot see. Indexes are compared only after each marker is proven
// present, so a rename fails cleanly rather than panicking on a -1.
func TestBench_ContractDOMOrder(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	order := []string{
		`class="phead"`,
		"data-bench-ribbon",
		"The bench",
		"data-bench-stack",
		"data-bench-rsvp",
		"data-bench-nextup",
		`id="cal-dash-grid"`,
		"data-bench-caption",
	}
	prev := -1
	for _, marker := range order {
		i := strings.Index(html, marker)
		if i < 0 {
			t.Fatalf("bench DOM is missing %q", marker)
		}
		if i <= prev {
			t.Errorf("%q appears out of contract order", marker)
		}
		prev = i
	}
}

// --- 3. the honesty states --------------------------------------------------

// THE FAULT PRINTS WHERE THE DATE WOULD GO, and the row carries NO date element
// at all — the discriminator the signed Dwarven warnrow turns on.
func TestBench_WarnrowPrintsItsFaultAndNoDate(t *testing.T) {
	rows := benchRows([]*Calendar{ptrCal(benchFxDwarven()), ptrCal(benchFxElven())}, "", "camp-1", true)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows; got %d", len(rows))
	}
	warn, healthy := rows[0], rows[1]
	if !warn.Warn || warn.Fault == "" {
		t.Fatalf("the monthless calendar must raise a fault; got %+v", warn)
	}
	if warn.Name != "" {
		t.Errorf("the fault takes the NAME's slot; name was %q", warn.Name)
	}
	if warn.DateLabel != "" {
		t.Errorf("a faulted row emits NO date — not a zero, not a placeholder; got %q", warn.DateLabel)
	}
	if healthy.DateLabel == "" || healthy.Fault != "" {
		t.Errorf("a resolvable calendar prints its in-world date; got %+v", healthy)
	}

	grid := benchGridSection(t, renderBench(t, benchFxData(true, true)))
	if !strings.Contains(grid, `class="fault"`) {
		t.Error("the rendered warnrow must carry the fault element")
	}
	// The warnrow must not carry an .iw date element. Counted inside the row
	// grid only: the Block's own nameplate uses the same .iw mono date element,
	// and a page-wide count would silently pass whatever the rows did.
	if n := strings.Count(grid, `class="iw mono"`); n != 1 {
		t.Errorf("the four-calendar fixture has ONE healthy subordinate row, so one date element in the grid; got %d", n)
	}
	if n := strings.Count(grid, `class="fault"`); n != 1 {
		t.Errorf("exactly one row faults in the fixture; got %d", n)
	}
}

// benchGridSection returns the #cal-dash-grid section of a rendered Bench. The
// index result is checked before it is used as a bound — a bare strings.Index
// slice bound PANICS on a rename instead of failing cleanly (COMMON §3).
func benchGridSection(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `id="cal-dash-grid"`)
	if start < 0 {
		t.Fatal("rendered bench has no #cal-dash-grid section")
	}
	rest := html[start:]
	if end := strings.Index(rest, "data-bench-caption"); end >= 0 {
		return rest[:end]
	}
	return rest
}

// Design-ahead surfaces carry the signed chip, never a fabricated number. The
// mockup's "RSVP 3 / 5" and "9 days fogged" are exactly the numbers that must
// not appear.
func TestBench_DesignAheadTilesChipRatherThanFabricate(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	if n := strings.Count(html, `class="badge need">needs backend`); n < 4 {
		t.Errorf("the session, sync and horizon tiles plus the RSVP panel header all carry the chip; got %d", n)
	}
	for _, fabricated := range []string{"RSVP 3 / 5", "9 days fogged", "2 RSVPs unanswered"} {
		if strings.Contains(html, fabricated) {
			t.Errorf("the Bench must not print the mockup's fabricated %q", fabricated)
		}
	}
	// The RSVP panel draws its header and nothing it cannot answer.
	if !strings.Contains(html, "data-bench-rsvp") {
		t.Error("the RSVP panel header must render")
	}
	if strings.Contains(html, "recommended window") {
		t.Error("the RSVP panel must not draw a recommended window it cannot compute")
	}
}

// THE SYNC DENOMINATOR NEVER DROPS. Every state keeps the total; only the
// numerator and the state change.
func TestBench_SyncTileDenominatorNeverDrops(t *testing.T) {
	for _, state := range []string{blockSyncStateOK, blockSyncStateDrift, blockSyncStateBad,
		blockSyncStatePause, blockSyncStateNone} {
		tile := benchSyncTile(calblock.SyncPill{State: state, Linked: 0, Total: 4})
		if !strings.Contains(tile.Detail, "of 4") {
			t.Errorf("state %q dropped the denominator: %q", state, tile.Detail)
		}
		if !tile.NeedsBackend {
			t.Errorf("state %q must keep the chip — per-calendar linkage is defined, not queried", state)
		}
	}
	// The defined numerator: 1 when a module is connected, else 0.
	if got := benchSyncTile(calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4}).Detail; !strings.HasPrefix(got, "1 of 4") {
		t.Errorf("connected numerator = %q; want the defined '1 of 4'", got)
	}
}

// The attention instrument does not vanish when healthy.
func TestBench_AttentionTileStatesAllClear(t *testing.T) {
	clear := benchAttentionTile(nil)
	if clear.Headline != "all clear" || clear.Tone != "ok" {
		t.Errorf("a healthy campaign gets the all-clear tile; got %+v", clear)
	}
	rows := benchAttentionRows([]Calendar{benchFxHarptos(), benchFxDwarven()}, "camp-1")
	if len(rows) != 1 || !strings.Contains(rows[0].Label, "Dwarven Deep-count") {
		t.Fatalf("the misconfigured calendar should be the one attention row; got %+v", rows)
	}
	if got := benchAttentionTile(rows).Headline; got != "1 item" {
		t.Errorf("attention headline = %q; want '1 item'", got)
	}
}

// --- NEXT UP ----------------------------------------------------------------

// Every count is computed post-filter, per viewer. The mockup's hardcoded
// "all 11" tail is the leak class this must not port.
func TestBench_NextUpCountsAreViewerComputed(t *testing.T) {
	gm := benchFxData(true, true)
	player := benchFxData(false, false)
	if gm.NextUp.Total != 2 {
		t.Errorf("GM index total = %d; want 2", gm.NextUp.Total)
	}
	if player.NextUp.Total != 1 {
		t.Errorf("player index total = %d; want 1 (the dm_only row never reaches them)", player.NextUp.Total)
	}
	html := renderBench(t, player)
	if strings.Contains(html, "Warden's writ due") || strings.Contains(html, "Warden&#39;s writ due") {
		t.Error("a dm_only event's NAME must never reach a player's index")
	}
	if strings.Contains(html, "all 11") {
		t.Error("the mockup's hardcoded 'all 11' tail must not be ported")
	}
	// The GM's row carries the gold GM badge; the player's DOM has no such badge.
	if !strings.Contains(renderBench(t, gm), `class="badge gm"`) {
		t.Error("a dm_only index row should carry the GM badge for a GM")
	}
	if strings.Contains(html, `class="badge gm"`) {
		t.Error("a player's index must carry no GM badge — permission is absence")
	}
}

// benchSortKeys feeds the shipped ?sort=nextevent comparator from the
// VIEWER-FILTERED index, so the leaky UpcomingByCalendar path is not reopened.
func TestBench_SortKeysComeFromTheFilteredIndex(t *testing.T) {
	h := benchFxHarptos()
	e := benchFxElven()
	keys := benchSortKeys([]BlockUpcoming{
		{Event: Event{ID: "a", Name: "First"}, Calendar: &h, Date: BlockDate{Year: 1523, Month: 1, Day: 14}},
		{Event: Event{ID: "b", Name: "Second"}, Calendar: &h, Date: BlockDate{Year: 1523, Month: 1, Day: 20}},
		{Event: Event{ID: "c", Name: "Elvish"}, Calendar: &e, Date: BlockDate{Year: 218, Month: 1, Day: 30}},
	})
	if len(keys) != 2 {
		t.Fatalf("one key per calendar; got %d", len(keys))
	}
	if keys["cal-harptos"].Next == nil || keys["cal-harptos"].Next.Day != 14 {
		t.Errorf("only the SOONEST row per calendar feeds the comparator; got %+v", keys["cal-harptos"].Next)
	}
	if benchSortKeys(nil) != nil {
		t.Error("an empty index produces no keys, so the default order stands")
	}
}

// --- the ANSWER key ---------------------------------------------------------

// data-day is emitted in wave 1 and consumed in W-B. A surface that forgets it
// simply stops answering later, and that is invisible in review (guard B4).
func TestBench_DatedSurfacesCarryTheAnswerKey(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	// Every element carrying a data-cell / data-row marker carries data-day.
	for _, marker := range []string{"data-cell=", "data-row="} {
		for _, frag := range strings.Split(html, marker)[1:] {
			// The attribute pair is emitted adjacently by bench.templ; look
			// inside the remainder of this tag only.
			tag := frag
			if end := strings.Index(tag, ">"); end >= 0 {
				tag = tag[:end]
			}
			if !strings.Contains(tag, "data-day=") {
				t.Errorf("a %s surface carries no data-day ANSWER key: …%s…", marker, tag)
			}
		}
	}
	if !strings.Contains(html, `data-day="cal-harptos-14"`) {
		t.Error("the Today tile's tick rule should key each day as <calendar>-<day>")
	}
}

// --- the stylesheet ---------------------------------------------------------

func benchCSS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "static", "css", "calendar-bench.css"))
	if err != nil {
		t.Fatalf("read calendar-bench.css: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("calendar-bench.css is empty")
	}
	return string(body)
}

var benchCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// TestBenchCSS_NoMotionAtAll is the sibling of the Block's
// TestCSS_NoMotionAtAll, for the same reason and with the same wording: COMMON
// §6.5 forbids motion inside the Block in wave 1, and a Bench that animated the
// chrome AROUND a motionless Block would break the same rule from the outside.
func TestBenchCSS_NoMotionAtAll(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	for _, bad := range []string{
		"transition", "animation", "@keyframes", "will-change",
		"@starting-style", "view-transition",
	} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-bench.css contains %q — nothing on the Bench moves in wave 1", bad)
		}
	}
}

// Every selector is scoped under .cal-bench. An unlayered sheet outranks the
// app's layered CSS, so a bare `.badge` rule in here would silently restyle the
// whole product — which is exactly the class of bug the Block's sheet carries
// the same warning about.
func TestBenchCSS_EverySelectorIsScoped(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")
	for _, line := range strings.Split(code, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || !strings.HasSuffix(l, "{") || strings.HasPrefix(l, "@") || strings.HasPrefix(l, "}") {
			continue
		}
		sel := strings.TrimSpace(strings.TrimSuffix(l, "{"))
		if sel == "" {
			continue
		}
		if !strings.Contains(sel, ".cal-bench") {
			t.Errorf("unscoped selector in calendar-bench.css: %q", sel)
		}
	}
}

// The sheet must actually define the tokens the markup names. This is the gap
// #568 fell into: every DOM assertion stayed green while the stylesheet dropped
// what the markup depended on.
func TestBenchCSS_DefinesWhatTheMarkupNames(t *testing.T) {
	code := benchCSS(t)
	for _, want := range []string{
		".cal-bench .ribbon", ".cal-bench .tile", ".cal-bench .stack",
		".cal-bench .calrow", ".cal-bench .calrow.warnrow", ".cal-bench .newslot",
		".cal-bench .nextup", ".cal-bench .tick", ".cal-bench .badge.need",
		"--cal-harptos", "--cal-real", "--cal-elven", "--cal-dwarven",
		".cal-bench .p1", ".cal-bench .p8",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("calendar-bench.css does not define %q", want)
		}
	}
	// The proportion rule, in geometry: .stack is a column at EVERY width, so no
	// media query may hand it a row direction or a multi-column grid.
	for _, forbidden := range []string{"flex-direction: row", "flex-direction:row"} {
		if strings.Contains(code, ".cal-bench .stack "+forbidden) {
			t.Errorf("the stack must stay a column — %q would make two Blocks two identical panels", forbidden)
		}
	}
}

// ptrCal is a fixture helper: benchRows takes pointers into a caller-owned
// slice, and a test that took &benchFxDwarven() directly would not compile.
func ptrCal(c Calendar) *Calendar { return &c }

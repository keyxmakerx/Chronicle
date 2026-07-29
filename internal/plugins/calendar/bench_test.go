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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

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

// --- the RSVP panel's fixture (C-CALV4-RSVP-P8, WG-9) -----------------------
//
// SIGNED: 5 in the campaign · 3 answered · 1 DEPARTED member with a stored row.
// It lives HERE, beside the rest of the Bench fixture, rather than in the oracle
// file, because the oracle must EXTEND this suite and not fork it — and because
// the panel now renders on every Bench assertion, so the fixture is shared.
//
// The roster mirrors the signed contract's own cast (cv4 OWNERS) so a render
// against it can be read beside v4-bench-desktop-light.png and
// v4-bench-player-light.png: Kael owns the campaign, Nissa holds a co-DM grant,
// and Rell has NO stored zone — the state that must print the repair and a
// literally empty clock.
const (
	benchFxDepartedID   = "u-departed"
	benchFxDepartedName = "Ghost of Campaigns Past"
)

func benchFxRoster() []BenchRosterMember {
	return []BenchRosterMember{
		{UserID: "u-kael", Name: "Kael", Role: "Owner", IsOwner: true, TZ: "America/Chicago"},
		{UserID: "u-bryn", Name: "Bryn", Role: "Player", TZ: "America/New_York"},
		{UserID: "u-nissa", Name: "Nissa", Role: "Scribe", IsCoDM: true, TZ: "Europe/London"},
		{UserID: "u-rell", Name: "Rell", Role: "Player"},
		{UserID: "u-tam", Name: "Tam", Role: "Player", TZ: "America/Los_Angeles"},
	}
}

// benchFxRsvpInitials mirrors what the lanes label their rows with, so the
// permission assertion can look for lane traces in a player's DOM by content
// rather than only by marker.
func benchFxRsvpInitials() []string {
	out := make([]string, 0, 5)
	for _, m := range benchFxRoster() {
		out = append(out, benchRsvpInitials(m.Name))
	}
	return out
}

// benchFxAvail is the week overlay. Saturday (column 5) carries the week's peak
// — all five free for three contiguous hours — so the derived window has a
// single unambiguous answer that the oracle can recompute.
func benchFxAvail(includeDetail bool) *BenchAvailability {
	dates := []string{"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23",
		"2026-07-24", "2026-07-25", "2026-07-26"}
	a := &BenchAvailability{WeekStart: dates[0], WithPattern: 5}
	for i, d := range dates {
		free := make([]int, 24)
		for h := 18; h < 23; h++ {
			free[h] = 2
			if i >= 4 {
				free[h] = 3
			}
		}
		if i == 5 {
			free[19], free[20], free[21] = 5, 5, 5
		}
		a.Days = append(a.Days, BenchAvailabilityDay{Date: d, Free: free})
	}
	if includeDetail {
		a.FreeDays = map[string][]bool{
			"u-kael":  {false, true, false, false, true, true, true},
			"u-bryn":  {false, false, false, false, true, true, false},
			"u-nissa": {true, false, true, false, true, true, false},
			"u-rell":  {false, false, false, false, false, true, false},
			"u-tam":   {false, true, false, false, true, true, true},
		}
	}
	return a
}

// benchFxRsvpInput is the signed fixture as the builder takes it.
//
// THE DEPARTED ROW IS THE LOAD-BEARING CASE: u-departed holds a stored `yes`
// and is NOT in the roster. Every number the panel prints must be identical
// with and without it.
func benchFxRsvpInput(isGM bool) benchRsvpInput {
	loc, _ := time.LoadLocation("America/Chicago")
	return benchRsvpInput{
		IsGM:       isGM,
		ViewerID:   "u-kael",
		CampaignID: "camp-1",
		Roster:     benchFxRoster(),
		Avail:      benchFxAvail(isGM),
		// The default world for every pre-P8B fixture: a mail server IS
		// configured and this campaign has never been asked, so the ask control
		// is LIVE and adds no caption. The three states get their own fixture
		// (benchFxDataRsvpAsk) rather than perturbing every existing render.
		CSRFToken:      "fx-csrf",
		MailConfigured: true,
		AskState:       ScheduleAskState{Ready: true},
		Answers: map[string]string{
			"u-kael":          RSVPYes,
			"u-nissa":         RSVPYes,
			"u-rell":          RSVPNo,
			benchFxDepartedID: RSVPYes,
		},
		Session: &BenchRsvpSession{
			Name:     "Session 41",
			Instant:  time.Date(2026, 7, 25, 19, 0, 0, 0, loc),
			Anchored: true,
		},
		ViewerZone:       "America/Chicago",
		ViewerZoneSource: "member",
		WeekLabel:        "20 Jul 2026",
	}
}

// benchFxHonesty* are the panel's three honesty states, shot as evidence
// because each is a case where the panel could plausibly have invented
// something instead: no answers, not enough availability to rank, and no
// session to answer.
type benchFxHonestyState int

const (
	benchFxHonestyNoAnswers benchFxHonestyState = iota
	benchFxHonestyThin
	benchFxHonestyNoSession
)

func benchFxDataRsvpHonesty(state benchFxHonestyState) BenchData {
	in := benchFxRsvpInput(true)
	switch state {
	case benchFxHonestyNoAnswers:
		in.Answers = map[string]string{}
	case benchFxHonestyThin:
		in.Avail.WithPattern = benchRsvpQuorum - 1
	case benchFxHonestyNoSession:
		in.Session = nil
	}
	d := benchFxData(true, true)
	d.Rsvp = benchRsvpBuild(in)
	return d
}

// benchFxDataRsvp is benchFxData with the RSVP panel FILLED. It is a separate
// entry point rather than a flag on benchFxData so the wave-1 assertions that
// deliberately exercise the panel's UNFILLED state keep doing so.
func benchFxDataRsvp(isGM, isOwner bool) BenchData {
	in := benchFxRsvpInput(isGM)
	d := benchFxData(isGM, isOwner)
	d.Rsvp = benchRsvpBuild(in)
	// The ribbon's session tile is fed from the SAME resolution the panel is,
	// exactly as buildBench does it — so a fixture render exercises the live
	// tile rather than the not-yet-reading one, and the tile's tally and the
	// panel's tally are visibly the same number.
	answered, _ := benchRsvpTally(in.Roster, in.Answers)
	d.Ribbon = benchRibbon(benchRibbonInput{
		IsGM: isGM, CampaignID: "camp-1", NextUp: d.NextUp,
		Sync:      calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4, Full: "In sync · 1 of 4 linked"},
		Attention: benchAttentionRows(benchFxAll(), "camp-1"),
		Session: &benchSessionTileInput{
			IsGM: isGM, CampaignID: "camp-1", CalendarID: "cal-real", EventID: "evt-41",
			Name: in.Session.Name, When: benchRsvpWhen(in.Session.DaysUntil),
			Answered: answered, Total: len(in.Roster),
			MyStatus: in.Answers[in.ViewerID], CSRFToken: "fx-csrf",
		},
	})
	return d
}

// benchFxData builds a BenchData the way buildBench does, minus the IO. The
// Blocks are projected through the REAL projection so the DOM under test is the
// DOM production renders. Its RSVP panel is the UNFILLED state — the one a
// campaign that has entered nothing gets; benchFxDataRsvp is the filled twin.
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
	d.Layers = benchBlockLayers(blockLayerPrefs{})
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

	// RE-PINNED DELIBERATELY, THREE TIMES NOW, AND IT IS AN ENUMERATION RATHER
	// THAN A NUMBER ([S12], SIGNED AS MODIFIED). It began as `>= 4` — a floor a
	// Block-side chip could satisfy on the Bench's behalf, and in fact did:
	// on wave-1 main it was met by FIVE, four Bench-side plus one Block-side
	// zone chip. A floor another surface can meet stops proving anything.
	//
	// C-CALV4-LEDGER-P6 §9 made it an exact count of the Bench's OWN chips,
	// subtracting the Block-side ones first. C-CALV4-SHELF-P7 kept that shape.
	// C-CALV4-RSVP-P8 re-enumerated because it RETIRED one chip and MOVED
	// another. C-CALV4-RSVP-P8B RE-ENUMERATES AGAIN, and this time the count
	// really does move — four to three — because the thing one of them was
	// waiting for now exists. That is the reason this is a list and not a
	// number: the number changing is not the finding, WHICH entry left is.
	//
	// THE SURVIVING CHIPS, EVERY ONE, BY SITE. The count below is a
	// consequence of this list and must never be nudged without editing it:
	//
	//   BENCH-SIDE (this test's subject) — four, ALL GM-TIER:
	//     1. bench.go benchSessionTile  — the tile does not read the shipped
	//        RSVP store yet. GM-only as of P8: the chip is build status, and
	//        build status never reaches a player
	//        (decisions/2026-07-27-needs-backend-audience.md). The count
	//        oracle caught wave 1 rendering it at every role.
	//     2. bench.go benchSyncTile     — the transport has no per-calendar
	//        linkage to report
	//     3. bench.go benchAttentionTile's sibling, the horizon tile — there is
	//        no queryable knowledge horizon (COMMON §6.1)
	//
	//   RETIRED BY P8B, and it is the point of THAT slice:
	//     · benchSessionTile's Nudge action — the fourth chip in this list until
	//       this slice. It was WG-5's visible chip beside a disabled control,
	//       and its stated reason was "there is no reminder endpoint".
	//       POST /campaigns/:id/calendar/ask is that endpoint. The control is
	//       not made live here, though: it is RETIRED ([PB-5] item 3), because a
	//       "Nudge" on a tile headlined "Not scheduled here yet" never had a
	//       referent, and the one live ask control lives in the RSVP panel where
	//       the roster it mails is. benchSessionTileLive's Nudge is retired for
	//       the sibling reason ([PB-5] item 2) — two live buttons mailing one
	//       roster from one page is a double-send affordance — but it never
	//       appeared in this fixture's count, because that tile only renders
	//       once the panel resolves a session.
	//
	//   RETIRED BY P8, and it is the point of THAT slice:
	//     · the RSVP PANEL HEADER's chip. It was drawn when the whole panel was
	//       design-ahead; the panel is now backed, and a panel-level chip over a
	//       filled panel is the same lie class as a green sync pill with no
	//       denominator, inverted (WG-8). The chip moved to the two controls
	//       inside it that really are unbacked.
	//
	//   BENCH-SIDE BUT NOT IN THIS FIXTURE: the RSVP panel's Propose and Nudge
	//   chips, which only render once the panel is FILLED — benchFxDataRsvp is
	//   the fixture that exercises them, and TestBenchRsvp_* below counts them.
	//
	//   BLOCK-SIDE (subtracted) — one:
	//     5. calendar_block/shelf.templ, shelfFiltersPanel — the Filters TAB's
	//        panel, unconditional inside a FILLED zone ([S2]: the tab ships,
	//        the engine does not; W-F is the named filling wave). It renders
	//        once here because the Bench's real-world Block carries noShelf,
	//        and to a GM only.
	//
	//   GONE, and both by a producer flag rather than a template edit:
	//     · the Ledger zone chip (W-B, C-CALV4-LEDGER-P6)
	//     · the Shelf zone chip  (W-E, C-CALV4-SHELF-P7)
	//
	//   NOT ON THE BENCH AT ALL: the legend / horizon / moongraph zone chips
	//   (block.templ). benchBlockLayers enables none of those keys, so their
	//   unconditional chips never render here.
	const chip = `class="badge need">needs backend`
	const filtersChip = `data-spane="filters"><span ` + chip
	benchOwn := strings.Count(html, chip) - strings.Count(html, filtersChip)
	if benchOwn != 3 {
		t.Errorf("the Bench's own chips = %d, want 3 (session tile · sync · horizon); "+
			"Block-side chips are subtracted and must not stand in for them. If this "+
			"dropped to 2, a chip was retired without its entry above being struck; "+
			"if it rose to 4, the retired Nudge came back over a backend that exists",
			benchOwn)
	}
	// The Ledger's chip is gone because W-B FILLED the zone, and the Shelf's
	// because W-E did. Inverted, not deleted: a chip beside real rows is a lie
	// whichever zone it sits in.
	if !strings.Contains(html, `data-zone="ledger"`) {
		t.Error("the Bench's Blocks must still dock the Ledger — the full-tier column " +
			"arithmetic subtracts its 300px unconditionally")
	}
	if !strings.Contains(html, `data-zone="shelf"`) {
		t.Error("the Bench's Blocks must still dock the Shelf")
	}
	const shelfZoneChip = `data-zone="shelf"><div class="st"`
	if !strings.Contains(html, shelfZoneChip) {
		t.Error("the Shelf's strip must open on its tabs")
	}
	if n := strings.Count(html, filtersChip); n != 1 {
		t.Errorf("the Filters panel's chip renders %d times, want 1 — the real-world Block "+
			"carries noShelf and a player receives no Filters panel at all", n)
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
	// INVERTED, NOT DELETED (C-CALV4-RSVP-P8 / WG-3). The wave-1 assertion was
	// `must not draw a recommended window it cannot compute`, and it was right
	// for as long as there was no per-hour free count to compute one from. There
	// is one now, so the rule it was defending — the panel never states a
	// ranking it has no data for — is asserted at its two real edges instead:
	// an EMPTY panel still draws nothing, and a filled one is asserted below
	// (TestBenchRsvp_*) to carry the permanent `derived · not stored` chip and
	// to refuse below quorum. Deleting it would have retired the rule with it.
	if strings.Contains(html, "Most free:") {
		t.Error("the UNFILLED RSVP panel must not draw a window — it has no availability to rank")
	}
	if strings.Contains(html, "derived · not stored") {
		t.Error("the UNFILLED RSVP panel must not carry the derived-window chip")
	}
}

// --- the RSVP panel (C-CALV4-RSVP-P8 Part A) --------------------------------

// The signed DOM ships in the signed ORDER. The class names ARE the contract
// (cv4:2205-2263), and preflight measured every one of them at zero occurrences
// across both shipped calendar-v4 sheets — this panel body is built from
// nothing, so there is no prior markup to inherit the order from.
func TestBenchRsvp_SignedDOMInSignedOrder(t *testing.T) {
	html := renderBench(t, benchFxDataRsvp(true, true))
	order := []string{
		`class="rsvp surf"`, `class="lhead"`, `class="top"`, `class="ov"`,
		`class="ovwrap"`, `class="ovgrid"`, `class="ovhead"`,
		`class="lane"`, `class="who"`, `class="swatch`, `class="slot`,
		`class="lane dens-row"`, `class="dens"`, `class="recbr"`,
		`class="side"`, `class="hl"`, `class="rec"`, `class="btns"`,
		`class="mtable"`, `class="mrow"`, `class="nm"`, `class="ro"`, `class="lt`, `class="rs`,
	}
	at := 0
	for _, want := range order {
		i := strings.Index(html[at:], want)
		if i < 0 {
			t.Fatalf("the signed panel is missing %q (or it is out of order after %d)", want, at)
		}
		at += i
	}
}

// EVERY SWATCH CARRIES ITS PATTERN CLASS. Colour is never load-bearing alone:
// the pattern is the greyscale identity channel, which is the whole reason
// OverlayMember.Color (ten hex values, one channel) is ignored. The
// v4-proposed mockup ships a pattern-less swatch; that is a defect and this
// asserts it was not inherited.
func TestBenchRsvp_EverySwatchCarriesItsPattern(t *testing.T) {
	html := renderBench(t, benchFxDataRsvp(true, true))
	total := strings.Count(html, `class="rsvp surf"`)
	if total == 0 {
		t.Fatal("no RSVP panel rendered")
	}
	swatches := strings.Count(html, `class="swatch`)
	patterned := 0
	for _, p := range []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"} {
		patterned += strings.Count(html, `class="swatch `+p+`"`)
	}
	if swatches == 0 || swatches != patterned {
		t.Errorf("%d swatches, %d of them patterned — the pattern class is not optional",
			swatches, patterned)
	}
	// The identity pair is keyed to the stable roster index and stays unique
	// past the eighth member, hue repeating while the pattern steps.
	h0, p0 := benchRsvpIdentity(0)
	h8, p8 := benchRsvpIdentity(8)
	if h0 != h8 {
		t.Errorf("hue should repeat at index 8: %q vs %q", h0, h8)
	}
	if p0 == p8 {
		t.Errorf("pattern must STEP at index 8, both were %q — index 8 would be index 0's twin", p0)
	}
}

// The zone-less member is a FIRST-CLASS STATE: the repair renders, the clock is
// LITERALLY EMPTY, and neither is a dash, a `--:--` or a UTC guess.
func TestBenchRsvp_ZonelessMemberGetsTheRepairAndNoClock(t *testing.T) {
	p := benchRsvpBuild(benchFxRsvpInput(true))
	var rell BenchRsvpMember
	for _, m := range p.Members {
		if m.Name == "Rell" {
			rell = m
		}
	}
	if rell.Name == "" {
		t.Fatal("the fixture's zone-less member is gone")
	}
	if rell.Zone != "" || rell.LocalTime != "" {
		t.Errorf("a zone-less member got zone=%q clock=%q; both must be empty", rell.Zone, rell.LocalTime)
	}
	if rell.AskHref == "" {
		t.Error("the `Ask →` repair must have somewhere to go")
	}
	html := renderBench(t, benchFxDataRsvp(false, false))
	for _, want := range []string{`class="badge warn">zone not set`, `Ask →`} {
		if !strings.Contains(html, want) {
			t.Errorf("a player's DOM is missing the zone repair %q — the repair may never be "+
				"the thing that disappears", want)
		}
	}
	for _, forbidden := range []string{"--:--", ">—<span", "UTC guess"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("a zone-less clock printed %q instead of nothing", forbidden)
		}
	}
}

// Abbreviations are real, DST-correct and carry the full identifier in `title`
// — one fact at two densities. `+1d` and the antisocial ink are drawn in every
// state, because those are the two cases where getting it wrong wakes somebody
// at 5am.
func TestBenchRsvp_ZoneAbbreviationsAndTheNextDayBadge(t *testing.T) {
	p := benchRsvpBuild(benchFxRsvpInput(true))
	byName := map[string]BenchRsvpMember{}
	for _, m := range p.Members {
		byName[m.Name] = m
	}
	if got := byName["Kael"].Zone; got != "CDT" {
		t.Errorf("Kael's zone chip = %q, want the DST-correct CDT", got)
	}
	if got := byName["Kael"].ZoneTitle; got != "America/Chicago" {
		t.Errorf("the full identifier must ride in title; got %q", got)
	}
	// 19:00 CDT is 01:00 the NEXT day in London — the signed player render's own
	// case, and the one a member reads wrong without the badge.
	n := byName["Nissa"]
	if n.LocalTime != "01:00" || !n.NextDay || !n.Antisocial {
		t.Errorf("Nissa: clock=%q nextDay=%v antisocial=%v; want 01:00 with +1d and warn ink",
			n.LocalTime, n.NextDay, n.Antisocial)
	}
	if byName["Kael"].NextDay || byName["Kael"].Antisocial {
		t.Error("19:00 in the viewer's own zone is neither next-day nor antisocial")
	}
}

// The co-DM is marked with `.badge.gm`'s third signed string (WG-4), and the
// role column prints the truth rather than the retired two-value vocabulary.
func TestBenchRsvp_CoDMIsMarkedAndRolesAreTheTruth(t *testing.T) {
	html := renderBench(t, benchFxDataRsvp(false, false))
	if !strings.Contains(html, `class="badge gm">co-DM`) {
		t.Error("the co-DM marker is missing — a co-DM labelled a plain player on a " +
			"permission surface is the defect WG-4 exists to close")
	}
	for _, role := range []string{">Owner<", ">Scribe<", ">Player<"} {
		if !strings.Contains(html, role) {
			t.Errorf("role column is missing %q — Role.DisplayName() is the vocabulary", role)
		}
	}
	if strings.Contains(html, `class="ro">DM<`) {
		t.Error("the retired roleLabel vocabulary is back")
	}
}

// The panel's captions carry the facts the numbers cannot state about
// themselves, including §6's mandated sentence about the stored tally.
func TestBenchRsvp_CaptionsStateWhyTheCountsAreRecomputed(t *testing.T) {
	html := renderBench(t, benchFxDataRsvp(true, true))
	for _, want := range []string{
		"recomputed from these rows, not from the stored tally",
		"still counts people who have left the campaign",
		"the Director included",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the panel does not say %q", want)
		}
	}
}

// The derived window ships with its PERMANENT chip, and Propose stays inert
// beside it with a VISIBLE one (WG-3 + WG-5). The readout is real, the action
// is not, and the gap between them is stated.
func TestBenchRsvp_DerivedWindowShipsChippedAndProposeStaysInert(t *testing.T) {
	html := renderBench(t, benchFxDataRsvp(true, true))
	if !strings.Contains(html, "derived · not stored") {
		t.Error("the derived window must carry its permanent chip")
	}
	if !strings.Contains(html, "does not know what is already on the calendar") {
		t.Error("the window must name what it cannot include (ledger #16)")
	}
	if !strings.Contains(html, `class="inert"`) {
		t.Error("an inert control must carry a visible chip, not a title alone (WG-5)")
	}
	// C-CALV4-RSVP-P8B SHRANK THIS SENTENCE and the pin follows it, intent
	// preserved: the reason must still be visible rather than title-only, but it
	// now names Propose ALONE. The old wording ended "…and RSVP mail fans out
	// only at the moment collection is switched on", which stopped being true
	// the moment this slice shipped the ask endpoint — a sentence that still
	// said it would be a NEW false honesty state.
	if !strings.Contains(html, "Propose is inert") {
		t.Error("the specific reason must be VISIBLE, not only in title")
	}
	if strings.Contains(html, "Propose and Nudge are inert") ||
		strings.Contains(html, "fans out only at the moment") {
		t.Error("ActionsWhy still describes Nudge as inert after the endpoint shipped")
	}
	// The chip beside them is the literal "needs backend" and never the reason,
	// so `.badge.need` cannot be diluted.
	if strings.Contains(html, `class="badge need">no reminder endpoint`) ||
		strings.Contains(html, `class="badge need">propose`) {
		t.Error(".badge.need was diluted — its text is always the literal `needs backend`")
	}
}

// PART B IS NOT BUILT. No `.sc-` class, no /schedule link, no Verdict, Matrix,
// Roster or Painter. This is a bound, so it is pinned rather than trusted.
func TestBenchRsvp_PartBIsNotBuilt(t *testing.T) {
	for _, gm := range []bool{true, false} {
		html := renderBench(t, benchFxDataRsvp(gm, gm))
		for _, forbidden := range []string{`class="sc-`, ` sc-`, `/schedule`, "cal-schedule"} {
			if strings.Contains(html, forbidden) {
				t.Errorf("gm=%v Part B leaked into Part A: %q", gm, forbidden)
			}
		}
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
		// The RSVP panel body (C-CALV4-RSVP-P8). Preflight measured every one of
		// these at ZERO occurrences before this slice — the panel is built from
		// nothing, so this pin is the only thing standing between the markup and
		// a sheet that silently drops what it depends on (the #568 gap).
		".cal-bench .rsvp", ".cal-bench .rsvp .ovgrid", ".cal-bench .rsvp .lane",
		".cal-bench .rsvp .lane .slot.free", ".cal-bench .rsvp .lane .dens",
		".cal-bench .rsvp .recbr", ".cal-bench .rsvp .swatch",
		".cal-bench .rsvp .mrow", ".cal-bench .rsvp .mrow .lt.small",
		".cal-bench .rsvp .side", ".cal-bench .rsvp .inert",
		"--own-1", "--own-8",
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

// --- the fidelity evidence generator ---------------------------------------
//
// Not a test: a tool that happens to live in a _test.go file so it can reuse
// the fixtures and the REAL templ output rather than re-implementing either.
// Same shape as the Block's own screenshot_gen_test.go, and inert unless
// BENCH_SCREENSHOTS names an output directory:
//
//	BENCH_SCREENSHOTS=/tmp/shots go test ./internal/plugins/calendar/ -run BenchScreenshots
//
// It renders benchSurface — the page WITHOUT the app shell — because the shell
// needs the Tailwind build and a live server, and what is under review here is
// the Bench's own sheet. The widths are the contract's own:
// BENCH_HOST = min(1232, VW - (VW <= 640 ? 32 : 48)).

type benchShot struct {
	file    string
	title   string
	caption string
	dark    bool
	// w/h are the browser window; host is the Bench's own content width, written
	// explicitly so the arithmetic is readable off the image rather than taken
	// on trust. CHROMIUM CLAMPS ITS WINDOW WIDTH TO 500 CSS px, so a 390px
	// phone cannot be simulated by --window-size alone — the phone shots use a
	// 500px window (which still satisfies the <=640px media queries, exactly as
	// a real 390px viewport does) and give the Bench its real 358px box.
	w, h, host int
	data       BenchData
}

func TestGenerateBenchScreenshots(t *testing.T) {
	outDir := os.Getenv("BENCH_SCREENSHOTS")
	if outDir == "" {
		t.Skip("bench screenshot generator: set BENCH_SCREENSHOTS=<dir> to run")
	}
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("bench screenshot generator: no Chromium binary found (set CHROMIUM_BIN)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	const gmCap = "GM · four calendars, one misconfigured · viewport 1280px → BENCH_HOST 1232px"
	const mobileCap = "GM · the phone reading · BENCH_HOST 358px, i.e. a 390px viewport — the stack is still a column, the rows are still rows"
	shots := []benchShot{
		{file: "01-bench-desktop-light.png", title: "The Bench · desktop · GM · light",
			caption: gmCap, w: 1280, h: 2700, host: 1232, data: benchFxData(true, true)},
		{file: "02-bench-desktop-dark.png", title: "The Bench · desktop · GM · dark",
			caption: gmCap, dark: true, w: 1280, h: 2700, host: 1232, data: benchFxData(true, true)},
		{file: "03-bench-mobile-light.png", title: "The Bench · phone · GM · light",
			caption: mobileCap, w: 500, h: 3600, host: 358, data: benchFxData(true, true)},
		{file: "04-bench-mobile-dark.png", title: "The Bench · phone · GM · dark",
			caption: mobileCap, dark: true, w: 500, h: 3600, host: 358, data: benchFxData(true, true)},
		{file: "05-bench-player-light.png", title: "The Bench · desktop · PLAYER · light",
			caption: "permission is ABSENCE — Sync, Needs attention and Horizon are not in this DOM, and no gap hints that they exist",
			w:       1280, h: 2400, host: 1232, data: benchFxData(false, false)},
		{file: "06-bench-empty-player-light.png", title: "The Bench · a player with nothing shared",
			caption: "the calm empty state, with no create affordance a player would meet a 403 on",
			w:       1280, h: 700, host: 1232, data: BenchData{CampaignID: "camp-1", CampaignName: "Imix"}},

		// C-CALV4-RSVP-P8 Part A. The pair at 07/08 is THE PERMISSION PROOF and
		// must be read side by side at the same width: the Director's panel
		// carries five availability lanes and two chipped Director controls, and
		// the player's carries neither — while both carry the identical member
		// table, density row and 3 / 5, because answers, roles, zones and local
		// clocks are party-visible and only the LANES are owner / co-DM only.
		{file: "07-rsvp-panel-gm-light.png", title: "RSVP panel · desktop · DIRECTOR · light",
			caption: "GM · lanes, the derived window with its permanent `derived · not stored` chip, and two chipped inert controls",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvp(true, true)},
		{file: "08-rsvp-panel-player-light.png", title: "RSVP panel · desktop · PLAYER · light — THE PERMISSION PROOF",
			caption: "same width, same panel: NO lanes and NO Director controls, but the same density denominator, the same 3 / 5 and the same full member table",
			w:       1280, h: 2600, host: 1232, data: benchFxDataRsvp(false, false)},
		{file: "09-rsvp-panel-gm-dark.png", title: "RSVP panel · desktop · DIRECTOR · dark",
			caption: "the identity hues carry their own dark ramp; the pattern channel is unchanged, because a pattern does not have a theme",
			dark:    true, w: 1280, h: 3000, host: 1232, data: benchFxDataRsvp(true, true)},
		{file: "10-rsvp-panel-mobile-light.png", title: "RSVP panel · phone · PLAYER · light",
			caption: "BENCH_HOST 358px — the `zone not set` + `Ask →` repair SURVIVES at this width; the repair may never be the thing that disappears",
			w:       500, h: 3600, host: 358, data: benchFxDataRsvp(false, false)},
		{file: "11-rsvp-panel-mobile-dark.png", title: "RSVP panel · phone · PLAYER · dark",
			caption: "BENCH_HOST 358px, dark — same reflow, same surviving repair",
			dark:    true, w: 500, h: 3600, host: 358, data: benchFxDataRsvp(false, false)},
		{file: "12-rsvp-honesty-nobody-answered.png", title: "RSVP panel · nobody has answered",
			caption: "every member silent, the tally 0 / 5 — the panel states it rather than dropping the line",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvpHonesty(benchFxHonestyNoAnswers)},
		{file: "13-rsvp-honesty-below-quorum.png", title: "RSVP panel · not enough saved availability",
			caption: "the derived window REFUSES to rank below three members, and says so — a ranking from two people's data is a guess wearing a number",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvpHonesty(benchFxHonestyThin)},
		{file: "14-rsvp-honesty-no-session.png", title: "RSVP panel · no session is collecting RSVPs",
			caption: "the roster, the zones and the density are all real; there is simply nothing to answer, and no clock is invented for it",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvpHonesty(benchFxHonestyNoSession)},
		{file: "15-rsvp-panel-unfilled.png", title: "RSVP panel · a campaign that has entered nothing",
			caption: "the corrected copy: the STORAGE exists — what is empty is this campaign's data, and the panel no longer claims otherwise",
			w:       1280, h: 900, host: 1232, data: benchFxData(true, true)},

		// C-CALV4-RSVP-P8B. THE ASK CONTROL IN ALL THREE STATES, plus the
		// player's proof that none of them reaches them. 16/17/18 must be read
		// as a set: the control is live only when it can actually send, and
		// each refusal states its own reason where the Director can see it
		// rather than behind a click.
		{file: "16-ask-askable-light.png", title: "The ask · ASKABLE · DIRECTOR · light",
			caption: "a configured mail server and no recent ask: Nudge is LIVE, no chip, no badge, and the foot says nothing — silence is the true state",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvpAskShot(true, benchFxAskNever)},
		{file: "17-ask-cooling-down-light.png", title: "The ask · COOLING DOWN · DIRECTOR · light",
			caption: "asked 2 hours ago: disabled, and the panel's foot says when and how long — a limit whose only expression is an error page is a limit the operator hits blind",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvpAskShot(true, benchFxAskCooling)},
		{file: "18-ask-no-smtp-light.png", title: "The ask · EMAIL NOT CONFIGURED · DIRECTOR · light",
			caption: "no mail server: disabled with `.badge.warn` (NOT `.badge.need` — the endpoint exists, the operator has not configured SMTP), and ledger item 11's sentence verbatim in the foot",
			w:       1280, h: 3000, host: 1232, data: benchFxDataRsvpAskShot(false, benchFxAskNever)},
		{file: "19-ask-askable-dark.png", title: "The ask · ASKABLE · DIRECTOR · dark",
			caption: "the same live control in dark", dark: true,
			w: 1280, h: 3000, host: 1232, data: benchFxDataRsvpAskShot(true, benchFxAskNever)},
		{file: "20-ask-cooling-down-dark.png", title: "The ask · COOLING DOWN · DIRECTOR · dark",
			caption: "the cooldown line in dark", dark: true,
			w: 1280, h: 3000, host: 1232, data: benchFxDataRsvpAskShot(true, benchFxAskCooling)},
		{file: "21-ask-no-smtp-dark.png", title: "The ask · EMAIL NOT CONFIGURED · DIRECTOR · dark",
			caption: "the warn badge in dark, where a `.badge.need` would be indistinguishable from a build gap if the class had been diluted", dark: true,
			w: 1280, h: 3000, host: 1232, data: benchFxDataRsvpAskShot(false, benchFxAskNever)},
		{file: "22-ask-mobile-light.png", title: "The ask · phone · DIRECTOR · light",
			caption: "BENCH_HOST 358px — the control and its reason both survive the phone reading; the reason may never be the thing that disappears",
			w:       500, h: 3600, host: 358, data: benchFxDataRsvpAskShot(false, benchFxAskNever)},
		{file: "23-ask-player-light.png", title: "The ask · PLAYER · light — THE ABSENCE PROOF",
			caption: "same width, same panel, same unconfigured server: NO control, NO cooldown line, NO SMTP sentence. Deployment status is build status for audience purposes, and a player has no control to explain",
			w:       1280, h: 2600, host: 1232, data: benchFxDataRsvpAskShot(false, benchFxAskPlayer)},
	}

	css := benchCSS(t) + benchBlockSheet(t)
	for _, s := range shots {
		t.Run(s.file, func(t *testing.T) {
			var sb strings.Builder
			if err := benchSurface(s.data).Render(context.Background(), &sb); err != nil {
				t.Fatalf("render bench surface: %v", err)
			}
			page := benchShotPage(s, css, benchStripLinks(sb.String()))
			dir := t.TempDir()
			src := filepath.Join(dir, "shot.html")
			if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
				t.Fatalf("write page: %v", err)
			}
			out := filepath.Join(outDir, s.file)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, chrome,
				"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
				"--force-device-scale-factor=2",
				fmt.Sprintf("--window-size=%d,%d", s.w, s.h),
				"--virtual-time-budget=4000",
				"--screenshot="+out, "file://"+src,
			)
			if combined, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("chromium screenshot: %v\n%s", err, combined)
			}
			if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
				t.Fatalf("screenshot %s was not written", out)
			}
		})
	}
}

// benchBlockSheet inlines the BLOCK's stylesheet too: the Bench's two Blocks
// are real calendar_block components and a shot without their sheet would be a
// shot of unstyled markup.
func benchBlockSheet(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "static", "css", "calendar-block.css"))
	if err != nil {
		t.Fatalf("read calendar-block.css: %v", err)
	}
	return string(body)
}

var benchLinkRe = regexp.MustCompile(`<link[^>]*>`)

// benchStripLinks removes the AssetURL <link> elements: file:// cannot resolve
// /static/, and inlining the sheets guarantees the shot is of THESE stylesheets
// rather than of a stale build artefact.
func benchStripLinks(markup string) string { return benchLinkRe.ReplaceAllString(markup, "") }

func benchShotPage(s benchShot, css, body string) string {
	cls := ""
	if s.dark {
		cls = ` class="dark"`
	}
	pad := "20px"
	if s.w <= 640 {
		pad = "16px"
	}
	return `<!doctype html><html` + cls + `><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:"Inter",ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`html.dark body{background:oklch(0.165 0.010 265);color:oklch(0.975 0.002 265)}` +
		`.shot-wrap{padding:` + pad + `}` +
		fmt.Sprintf(`.cal-bench{width:%dpx}`, s.host) +
		`h1{font-size:18px;line-height:1.2;margin:0 0 4px;letter-spacing:-.02em}` +
		`.shot-cap{font-size:11.5px;line-height:1.5;margin:0 0 14px;opacity:.72}` +
		css +
		`</style></head><body><div class="shot-wrap">` +
		`<h1>` + s.title + `</h1><p class="shot-cap">` + s.caption + `</p>` +
		`<div class="cal-bench">` + body + `</div>` +
		`</div></body></html>`
}

// benchFindChromium locates a headless Chromium the same way the Block's
// container-query probe does.
func benchFindChromium() string {
	if p := os.Getenv("CHROMIUM_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, pattern := range []string{
		"/opt/pw-browsers/chromium-*/chrome-linux/chrome",
		filepath.Join(os.Getenv("HOME"), ".cache/ms-playwright/chromium-*/chrome-linux/chrome"),
	} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	return ""
}

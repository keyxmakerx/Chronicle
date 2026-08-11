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
	"sort"
	"strconv"
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
		Attention: benchAttentionRows(benchFxAll(), "camp-1", nil),
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
		// The month cursor's trio, exactly where buildBench puts it — on the
		// PRIMARY Block only (C-CALV4-GAMEREADY [GR-1]/[GR-2]). The fixture
		// carries it so every assertion in this file judges the DOM production
		// renders, cursor and all.
		data.Primary.Nav = benchNav(primary, data.Primary.Data, "camp-1")
	}
	if realWorld != nil {
		data.RealWorld = benchFxBlock(realWorld, viewer, true)
	}
	data.Rows = benchRows(rows, "cal-harptos", "camp-1", isGM)
	// The day card's payload, exactly where buildBench puts it — beside the
	// Block projection and from nothing else (C-CALV4-DAYCARD, R2-2a). The
	// fixture carries it so every assertion in this file judges the DOM
	// production renders, card and all, rather than a Bench that quietly
	// predates it.
	data.DayCard = DayCardMount{
		CanCreate: isOwner, CanAuthorDmOnly: isOwner, CanDelete: isOwner,
		CanRestrict: isOwner,
		CampaignID:  "camp-1",
	}
	// The Owner-only audience roster rides the same wrapper ([ER-3] SIGNED,
	// C-CALV4-EDITOR-R2b). benchFxRoster is the same three people the RSVP
	// panel's fixture prints, so a player render can be asserted to carry NO
	// member name at all rather than merely to carry no roster key.
	data.DayCardJSON = dayCardPayloadJSON(
		dayCardSeed{
			CanAuthor: data.DayCard.CanCreate, CanRestrict: data.DayCard.CanRestrict,
			Roster: benchFxRoster(),
		},
		dayCardSource{Block: data.Primary, Calendar: primary},
		dayCardSource{Block: data.RealWorld, Calendar: realWorld})
	data.Ribbon = benchRibbon(benchRibbonInput{
		IsGM: isGM, CampaignID: "camp-1", Primary: primary, Block: data.Primary,
		NextUp:    data.NextUp,
		Sync:      calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4, Full: "In sync · 1 of 4 linked"},
		Attention: benchAttentionRows(cals, "camp-1", nil),
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
	//   ADDED BY C-CALV4-EDITOR-R2b stage 2, and this is the fourth
	//   re-enumeration:
	//     4. daycard.templ's DAY-OF-WEEK MULTI-SELECT, inside the event
	//        editor's recurrence block. `recurrence_day_of_week` exists as a
	//        column (migration 011) and EXPANSION IGNORES IT ENTIRELY — OccursOn
	//        is base-anchored (internal/plugins/calendar/.ai.md:963) and
	//        eventEditorRecord does not carry it — so the control is real, the
	//        storage is real, and the behaviour is not. That is precisely what
	//        `.badge.need` means and precisely why it is not the `year`
	//        treatment: `year` is not an accepted recurrence_type at all and
	//        does not ship, chipped or otherwise.
	//        GM-TIER BY CONSTRUCTION, which is the rule this list exists to
	//        keep: bench.templ renders the editor only for CanCreate, so the
	//        chip cannot reach a player and
	//        TestDayCard_APlayerBenchCarriesNoAuthoringDOMAndNoHonestyChip
	//        asserts the zero rather than trusting the construction.
	//        The editor's OTHER two chips — the `day` and moon recurrence units
	//        — are built by the module from the calendar's own derived week and
	//        are pinned in test/js/daycard_editor_requests.test.mjs, which is
	//        the honest split rather than a Go test asserting about JS.
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
	if benchOwn != 4 {
		t.Errorf("the Bench's own chips = %d, want 4 (session tile · sync · horizon · "+
			"the editor's day-of-week multi-select); Block-side chips are subtracted "+
			"and must not stand in for them. If this dropped to 3, a chip was retired "+
			"without its entry above being struck; if it rose to 5, either the retired "+
			"Nudge came back over a backend that exists or the editor chipped a field "+
			"the API already writes", benchOwn)
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
	// AMENDED — C-CALV4-GAMEREADY §8 [GR-15]. This block used to assert that a
	// PLAYER's DOM contained `Ask →`, which pinned the defect: the link went to
	// `/campaigns/:id/settings/members`, a route that has never existed, and
	// even a working member roster is not a Player's affordance — they were
	// being told to ask somebody else about a page they cannot act on. The
	// claim the original was making — "the repair may never be the thing that
	// disappears on the smallest screen" — is preserved and now asserted where
	// it belongs, on the GM's render; the Player's render asserts the ABSENCE
	// that is the fix. Both directions, so neither can drift.
	if rell.AskHref != "/campaigns/camp-1/members" {
		t.Errorf("a GM's zone repair points at %q; it must be the campaign's real member roster",
			rell.AskHref)
	}
	if rell.AskLabel != "Ask →" {
		t.Errorf("a GM's repair on someone else's row says %q; want %q", rell.AskLabel, "Ask →")
	}
	gmHTML := renderBench(t, benchFxDataRsvp(true, false))
	for _, want := range []string{`class="badge warn">zone not set`, `Ask →`, `/campaigns/camp-1/members`} {
		if !strings.Contains(gmHTML, want) {
			t.Errorf("a GM's DOM is missing the zone repair %q — the repair may never be "+
				"the thing that disappears", want)
		}
	}
	if strings.Contains(gmHTML, "/settings/members") {
		t.Error("the dead `/campaigns/:id/settings/members` href is back; it 404s")
	}
	html := renderBench(t, benchFxDataRsvp(false, false))
	if !strings.Contains(html, `class="badge warn">zone not set`) {
		t.Error("a player still sees WHICH members have no zone — only the repair is re-audienced")
	}
	if strings.Contains(html, "Ask →") {
		t.Error("a player was shown `Ask →` on another member's row: an affordance " +
			"rendered to an audience that cannot use it, which is the [GR-15] defect")
	}
	// AND THE VIEWER'S OWN ROW REPAIRS ITSELF. u-kael is the viewer; make them
	// the zone-less one and the control must become the one thing they can
	// actually do.
	own := benchFxRsvpInput(false)
	for i := range own.Roster {
		if own.Roster[i].UserID == own.ViewerID {
			own.Roster[i].TZ = ""
		}
	}
	var mine BenchRsvpMember
	for _, m := range benchRsvpBuild(own).Members {
		if m.Name == "Kael" {
			mine = m
		}
	}
	if mine.AskHref != "/account" || mine.AskLabel != "Set your zone →" {
		t.Errorf("the viewer's own zone-less row got %q / %q; want /account and `Set your zone →` — "+
			"telling somebody to ASK for the one field only they can set is the third fault",
			mine.AskHref, mine.AskLabel)
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

// PART B TOUCHES THE BENCH ONLY AS THE DOOR — the INVERTED form of Part A's
// `TestBenchRsvp_PartBIsNotBuilt`, which asserted that no `.sc-` class, no
// `/schedule` link and no `cal-schedule` root had leaked back here.
//
// THE TRIPWIRE IS INVERTED RATHER THAN DELETED, which is the standing rule: it
// held a real bound in Part A (a link to a 404 is worse than no link) and it
// holds the REMAINING half of that bound now that the page exists. Exactly one
// of the four forbidden strings became legal — the href — and the other three
// are MORE load-bearing than before, not less: the Bench does not load
// calendar-schedule.css, so an `.sc-` class or a `cal-schedule` root appearing
// in this DOM would be markup styled by a sheet that is not on the page.
func TestBenchRsvp_PartBTouchesTheBenchOnlyAsTheDoor(t *testing.T) {
	for _, gm := range []bool{true, false} {
		html := renderBench(t, benchFxDataRsvp(gm, gm))
		for _, forbidden := range []string{`class="sc-`, ` sc-`, "cal-schedule"} {
			if strings.Contains(html, forbidden) {
				t.Errorf("gm=%v Part B's namespace leaked onto the Bench: %q", gm, forbidden)
			}
		}
	}
}

// THE DOOR TO THE SCHEDULE. Part B ships `GET /campaigns/:id/schedule`, and the
// dispatch (§10) puts its entrance here rather than in the nav: WG-2's signed
// ruling keeps the nav pointing at `/availability` and books the retirement as
// its own slice, so the ONE way into the new page is the panel that has been
// called `RSVP · Schedule` since the signed contract drew it (cv4:2233).
//
// THE TITLE IS THE LINK, and that is why the signed render does not move: the
// anchor keeps the signed class, the signed words and the signed ink, and only
// grows an underline under the pointer. Part A could not do this because the
// page 404'd; the page exists now.
//
// IT IS A DOOR AT EVERY ROLE. The route's floor is Player+, so gating the link
// to a Director would hide the painter from the exact people whose availability
// the panel is complaining it does not have.
func TestBenchRsvp_PanelTitleIsTheDoorToTheSchedule(t *testing.T) {
	const door = `href="/campaigns/camp-1/schedule"`
	for _, tc := range []struct {
		name string
		data BenchData
	}{
		// BOTH STATES, and the unfilled one is the load-bearing case: a
		// campaign where nobody has saved availability is precisely the
		// campaign that needs the painter, and its panel is the one with
		// nothing else in it to click.
		{"director · filled", benchFxDataRsvp(true, true)},
		{"player · filled", benchFxDataRsvp(false, false)},
		{"director · unfilled", benchFxData(true, true)},
		{"player · unfilled", benchFxData(false, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBench(t, tc.data)
			if n := strings.Count(html, door); n != 1 {
				t.Fatalf("the panel opens %d doors to /schedule, want exactly 1", n)
			}
			// The signed words and the signed class survive the element change.
			if !strings.Contains(html, `class="t"`+" "+door) &&
				!strings.Contains(html, door+` class="t"`) {
				t.Error("the door is not the panel's own `.t` title")
			}
			if !strings.Contains(html, "RSVP · Schedule") {
				t.Error("the signed title words did not survive becoming a link")
			}
		})
	}
}

// A DOOR WITH NO CAMPAIGN IS NOT A DOOR. `/campaigns//schedule` is a 404 with a
// friendlier shape, so the title degrades to the span it has always been rather
// than to a broken link.
func TestBenchRsvp_TheDoorNeedsACampaignToOpenOnto(t *testing.T) {
	data := benchFxDataRsvp(true, true)
	data.CampaignID = ""
	html := renderBench(t, data)
	if strings.Contains(html, "/schedule") {
		t.Error("a campaign-less Bench still printed a link to /schedule")
	}
	if !strings.Contains(html, "RSVP · Schedule") {
		t.Error("the title vanished with the link — it must degrade to a span")
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
	rows := benchAttentionRows([]Calendar{benchFxHarptos(), benchFxDwarven()}, "camp-1", nil)
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

// TestBenchCSS_NoMotionAtAll was a BLANKET SUBSTRING BAN in wave 1 — "nothing
// on the Bench moves" — and it is now an ALLOWLIST.
//
// IT WAS INVERTED, NEVER DELETED, by C-CALV4-BENCH-R2 slice R2-1 under [BR2-2]
// SIGNED, following the precedent the Block's own sheet set when
// TestCSS_NoMotionAtAll became an allowlist in LEDGER-P6 stage 5. The name is
// kept on purpose: a reader who greps for the wave-1 rule must land on the
// document that changed it, not on a hole where it used to be.
//
// THE NEW CLAIM, in one sentence: the Bench moves in exactly one way, for
// exactly one reason, and every part of that way is enumerated here.
// decisions/2026-07-29-motion-disclosure-register.md is the signed law — one
// grammar (clip-reveal + opacity), two durations with close faster than open,
// reduced motion INSTANT rather than shortened, and the Block's interior still.
//
// Widening this allowlist widens EVERYTHING, because
// tools/check-v2-motion-discipline.sh does not police static/css/ at all (its
// scope is *.templ / *.css INSIDE internal/plugins/{calendar,timeline,
// ai_workspace,campaigns}, and it bans only `transition: all`). This test is the
// only thing standing here.
func TestBenchCSS_NoMotionAtAll(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")

	// STILL REFUSED, exactly as the Block refuses them. The register is a
	// TRANSITION family; a keyframe here would be a second grammar.
	for _, bad := range []string{
		"animation", "@keyframes", "will-change", "@starting-style", "view-transition",
	} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-bench.css contains %q — the register is a transition family "+
				"and a second grammar is a signature, not a tuning", bad)
		}
	}

	// CLAUSE 1, AND THE SENTENCE THAT DESCRIBES THE TWISTY. The register bans
	// `transform` on a disclosure (nothing slides, nothing bounces, the content
	// reveals inside its own box), and the shipped twisty is a CONTENT SWAP —
	// `content: "▸"` / `content: "▾"` on [open] — rather than a rotated glyph.
	//
	// This assertion exists because the sheet's own comment claimed a rotation
	// for one wave while the rules were a swap (verifier finding 3, R2-1
	// follow-up), which is the same defect stage 4 of this slice existed to
	// delete from entity_calendar_block.go — a prose claim nothing could
	// contradict. It is now enforceable.
	//
	// SCOPED TO THE DISCLOSURE, not to the sheet. A blanket ban would be a
	// false claim: `text-transform` is typography and appears throughout, and
	// `.rsvp .recbr b` carries a static `translateX(-50%)` from wave 1 that
	// centres a label and is never transitioned. What clause 1 forbids is a
	// transform ON THE SECTION, and that is what is checked.
	for _, rule := range benchCSSRules(code) {
		if !strings.Contains(rule[0], ".disc") && !strings.Contains(rule[0], "summary") {
			continue
		}
		if benchTransformRe.MatchString(rule[1]) {
			t.Errorf("`%s` declares `transform` — the register's clause 1 forbids it on a "+
				"disclosure, and the twisty is a CONTENT SWAP (▸ / ▾ on [open]), not a "+
				"rotation. The sheet's own comment says so.", rule[0])
		}
	}
	// …and the swap it is instead of is present, so the assertion above is
	// proving a mechanism rather than an absence.
	for _, want := range []string{`content: "▸"`, `content: "▾"`} {
		if !strings.Contains(code, want) {
			t.Errorf("the twisty's %s is gone; a summary with no visible "+
				"\"there is more here\" cue is the clause-5 failure", want)
		}
	}

	// CLAUSE 3 IS STRUCTURAL, not a shortened duration: the whole motion block
	// lives inside ONE `prefers-reduced-motion: no-preference` wrapper, so
	// outside it there is no rule at all and native <details> behaviour stands —
	// instant and complete. Same construction the Block's clause 2 demands.
	const guard = "@media (prefers-reduced-motion: no-preference)"
	if n := strings.Count(code, guard); n != 1 {
		t.Fatalf("%d `%s` blocks, want exactly 1 — clause 3 is proven by there being "+
			"nowhere else for a transition to live", n, guard)
	}
	motion := benchCSSBlock(t, code, guard)
	if outside := strings.Count(code, "transition") - strings.Count(motion, "transition"); outside != 0 {
		t.Errorf("%d `transition` declarations sit OUTSIDE the reduced-motion wrapper; "+
			"under prefers-reduced-motion they would still run", outside)
	}

	// THE ALLOWLIST: three properties, and no fourth — EXCEPT under the one
	// named carve-out class, which gets four and no fifth.
	//
	// Guard B1 in particular — --dash and --gap are the greyscale identity
	// channel and are NEVER animated, under any prelude.
	//
	// ── THE CARVE-OUT CLAUSE — amended deliberately, by name ──────────────
	//
	// C-CALV4-EDITOR-R2b, under the OPERATOR's signature of 2026-08-01
	// (decisions/2026-08-01-operator-signatures-wz1-sky-editor.md §3) and
	// [ER-6] / [ER-7] SIGNED.
	//
	// WHAT IT LIFTS AND WHAT IT DOES NOT. The signature lifts exactly one
	// thing — [DC-7]'s "register-only in this slice" — so the day card may
	// morph into the editor as its own NAMED motion signature. It does not
	// repeal the register: the carve-out is an addition on top of it, the base
	// grammar is unchanged, and every other surface in R2b still moves under
	// the three properties above.
	//
	// THE OLD CLAIM: every transitioned property in this sheet is one of
	// block-size · opacity · content-visibility.
	// THE NEW CLAIM: that is still true of EVERY RULE WHOSE PRELUDE DOES NOT
	// NAME THE CARVE-OUT CLASS, and rules that do name it may additionally
	// transition inline-size and translate — the two properties a GEOMETRIC
	// morph needs and a scale would have avoided.
	//
	// WHY THE NEW CLAIM IS NOT WEAKER. The widening is keyed to a class that
	// TestBenchCSS_TheNamedCarveOutsAreExactlyTwo independently pins to two
	// files product-wide, so it cannot be borrowed by a second surface without
	// that guard going red. `transform` is still refused everywhere, including
	// under the carve-out — `translate` is named on its own precisely so the
	// allowlist can admit the movement without admitting a scale. The five
	// refusal strings and the `--disc-*` count assert UNCHANGED, above and
	// below this block.
	//
	// MUTATION-TESTED: moving the morph's transition onto a rule whose prelude
	// does not name the class turns this red; adding `transform` under the
	// class turns this red.
	//
	// ── THE SKY BAND'S CLAUSE — amended deliberately, by name ─────────────
	//
	// C-CALV4-SKY slice R2-5, under the OPERATOR's signature of 2026-08-07
	// ([SKY-2]) and [SKY-3] SIGNED. The SECOND named carve-out, and the shape
	// is deliberately the morph's: a per-class widening keyed to `.skygrow`,
	// pinned to two files by the monopoly guard below.
	//
	// SEVEN PROPERTIES AND NO EIGHTH. grid-template-rows · opacity ·
	// inline-size · block-size · gap · --seal-solid · --seal-fade. Two of them
	// (opacity, block-size) are already on the BASE allowlist and are not
	// restated here; what the class buys is the other five. (An EIGHTH was
	// amended in by name — `content-visibility`, for the close — and it needed
	// no widening HERE because it was already on the base allowlist; see the
	// amendment block in TestBenchCSS_TheSkyCarveOutIsPinned below.) `transform` is
	// still refused, including `scale`. `color` is NOT on the list — the caret
	// is a CONTENT SWAP ([SKY-12]) and an eighth property arriving because
	// `color` was quietly kept is exactly the failure that ruling names.
	//
	// AND NO THIRD DURATION. The mock's six duration tokens were a SEQUENCING
	// of the register's existing two totals; they ship as four --sky-phase-*
	// tokens whose sums are asserted equal to --disc-open and --disc-close in
	// the clause below. The `--disc-*` count assertion stays at exactly 3,
	// BYTE-UNCHANGED, which is the proof no third total was smuggled in.
	const carveOut = ".edmorph"
	const skyCarveOut = ".skygrow"
	base := map[string]bool{"block-size": true, "opacity": true, "content-visibility": true}
	morph := map[string]bool{"inline-size": true, "translate": true}
	sky := map[string]bool{
		"grid-template-rows": true, "inline-size": true, "gap": true,
		"--seal-solid": true, "--seal-fade": true,
	}
	for _, rule := range benchCSSRules(motion) {
		carved := strings.Contains(rule[0], carveOut)
		skyed := strings.Contains(rule[0], skyCarveOut)
		for _, decl := range benchTransitionDecls(rule[1]) {
			for _, prop := range decl {
				if base[prop] || (carved && morph[prop]) || (skyed && sky[prop]) {
					continue
				}
				if skyed {
					t.Errorf("`%s` names the sky's carve-out and transitions %q, which is "+
						"on neither the base allowlist nor the sky's seven "+
						"(grid-template-rows · opacity · inline-size · block-size · gap · "+
						"--seal-solid · --seal-fade) — [SKY-3] SIGNED names seven and no "+
						"eighth, and `color` came off the list when the caret became a "+
						"content swap", rule[0], prop)
					continue
				}
				if carved {
					t.Errorf("`%s` names the carve-out and transitions %q, which is on "+
						"neither the base allowlist nor the morph's four "+
						"(inline-size · block-size · translate · opacity) — [ER-6] SIGNED "+
						"names four properties and no fifth", rule[0], prop)
					continue
				}
				t.Errorf("transitioned property %q is not on the allowlist "+
					"(block-size · opacity · content-visibility) — [BR2-2] SIGNED. The "+
					"editor-morph carve-out widens it ONLY for rules whose prelude names "+
					"%s, and the sky's ONLY for %s", prop, carveOut, skyCarveOut)
			}
		}
	}
	// …and the parser really saw the rules, so the loop above is proving a
	// property set rather than an empty iteration.
	if n := len(benchTransitionDecls(motion)); n < 3 {
		t.Fatalf("only %d transition declarations found inside the wrapper; the rule "+
			"parser stopped reading and every assertion above is vacuous", n)
	}

	// CLAUSE 2, ENFORCEABLE RATHER THAN DECORATIVE: exactly two durations, and
	// CLOSE IS FASTER THAN OPEN. Leaving must never feel slower than arriving.
	// A third duration anywhere is a signature and a STOP-AND-FLAG.
	open, ok := benchCSSDurationMS(code, "--disc-open")
	if !ok {
		t.Fatal("--disc-open is not defined")
	}
	closeMS, ok := benchCSSDurationMS(code, "--disc-close")
	if !ok {
		t.Fatal("--disc-close is not defined")
	}
	if closeMS >= open {
		t.Errorf("--disc-close (%dms) is not faster than --disc-open (%dms) — "+
			"register clause 2: leaving must never feel slower than arriving", closeMS, open)
	}
	if n := len(regexp.MustCompile(`--disc-(open|close|ease)\s*:`).FindAllString(code, -1)); n != 3 {
		t.Errorf("the register declares %d --disc-* tokens; want exactly 3 "+
			"(--disc-open, --disc-close, --disc-ease). A third DURATION is a signature", n)
	}

	// ── THE DAYCARD CLAUSE — amended deliberately, by name ─────────────────
	//
	// C-CALV4-DAYCARD slice R2-2a, [DC-6] SIGNED. The allowlist above was not
	// widened by a byte to admit the day card: the card reuses the same three
	// transitionable properties, the same two durations and the same easing,
	// inside the same single no-preference wrapper. What is added here is the
	// claim that it DOES — because "the card consumes the register" is exactly
	// the kind of sentence that decays into a second grammar the moment nothing
	// checks it, and DC-6's own words are that a second register section
	// anywhere is laundering and is refused.
	//
	// A guard nudged until green stops proving anything, so this clause fails
	// in BOTH directions: if the card's rules vanish (the reveal was quietly
	// deleted) and if they appear outside the wrapper (reduced motion would
	// then animate).
	const cardRule = ".cal-bench .cal-daycard .dcbox"
	if !strings.Contains(motion, cardRule) {
		t.Errorf("the day card's reveal is not inside %q — R2-2a consumes THIS register, "+
			"and a card that animated anywhere else would be the second grammar [DC-6] "+
			"refuses", guard)
	}
	if strings.Count(code, cardRule) != strings.Count(motion, cardRule) {
		t.Errorf("a `%s` rule sits OUTSIDE the reduced-motion wrapper; under "+
			"prefers-reduced-motion the card must open INSTANTLY AND COMPLETELY, which "+
			"is structural (no rule at all) and never a shortened duration", cardRule)
	}

	// ── THE THEATER CLAUSE — amended deliberately, by name ─────────────────
	//
	// C-CALV4-THEATER slice R2-3, [TH-3] SIGNED. Like the day card's clause
	// above, the allowlist was not widened by a byte to admit the theater: it
	// reuses the same two transitionable properties, the same two durations and
	// the same easing, inside the same single no-preference wrapper.
	//
	// IT IS LOAD-BEARING IN A WAY THE CARD'S WAS NOT. calendar-theater.css is
	// FORBIDDEN to declare a transition (TestTheaterCSS_CarriesNoMotionOfItsOwn),
	// so these two rules are the ONLY place the theater's reveal can exist. The
	// slice was drafted believing /schedule's `class="cal-bench cal-schedule"`
	// was a precedent for consuming this register from outside the Bench; it is
	// not — /schedule inherits the TOKENS and consumes zero RULES, and its own
	// guard bans `--disc-open` there. Carrying the class alone would have
	// shipped the theater three tokens and no motion at all, with every guard in
	// this file green.
	//
	// So this clause fails in BOTH directions, exactly as the card's does: if
	// the theater's rules vanish (the reveal was quietly deleted, or a later
	// hand "tidied" a class this sheet does not otherwise mention) and if they
	// appear outside the wrapper (reduced motion would then animate).
	const theaterRule = ".cal-bench.cal-theater .tbox"
	if !strings.Contains(motion, theaterRule) {
		t.Errorf("the theater's reveal is not inside %q — calendar-theater.css may declare no "+
			"transition at all, so this is the ONLY place the motion can live and its absence "+
			"means the theater opens instantly at every motion setting", guard)
	}
	if strings.Count(code, theaterRule) != strings.Count(motion, theaterRule) {
		t.Errorf("a `%s` rule sits OUTSIDE the reduced-motion wrapper; under "+
			"prefers-reduced-motion the theater must open INSTANTLY AND COMPLETELY, which is "+
			"structural (no rule at all) and never a shortened duration", theaterRule)
	}
	// …and the OPEN state really overrides the duration, which is what makes
	// leaving faster than arriving on this surface too.
	if !strings.Contains(motion, theaterRule+".tbopen") {
		t.Error("the theater has no open-state rule, so its reveal would run at --disc-close " +
			"in both directions and arriving would feel like leaving")
	}
	if !strings.Contains(motion, ".cal-bench .cal-daycard.dcopen .dcbox") {
		t.Error("the card declares a closed state and no open one — the reveal has no " +
			"endpoint and the register's 200ms open cannot run")
	}

	// ── THE MORPH'S OWN TWO-DIRECTIONAL CLAUSE ────────────────────────────
	//
	// The same shape the DAYCARD clause above uses, for the same reason: it
	// fails if the morph's rules VANISH (the carve-out was quietly deleted and
	// the operator's signed ask stopped shipping) and if they ESCAPE the
	// wrapper (under prefers-reduced-motion the morph would then run, and the
	// signature says instant AND complete).
	const morphRule = ".cal-bench .cal-dayeditor.edmorph"
	if !strings.Contains(motion, morphRule) {
		t.Errorf("the editor morph is not inside %q — the carve-out is a named addition "+
			"INSIDE the one register section ([DC-6]'s singularity clause survives it), "+
			"and a morph declared anywhere else would be the second grammar", guard)
	}
	if strings.Count(code, morphRule) != strings.Count(motion, morphRule) {
		t.Errorf("a `%s` rule sits OUTSIDE the reduced-motion wrapper; the operator's "+
			"signature says the morph must be instant AND COMPLETE under reduced motion, "+
			"which is structural (no rule at all) and never a shortened duration", morphRule)
	}
	// CLOSE FASTER THAN OPEN, STRUCTURALLY, in the register's own idiom: the
	// base rule carries --disc-close and the OPEN state overrides the duration.
	// Clause 2 was not named by the carve-out, so clause 2 binds.
	if !strings.Contains(motion, ".cal-bench .cal-dayeditor.edmorph.dcopen") {
		t.Error("the morph declares no open-state duration override — leaving would " +
			"then take exactly as long as arriving, which is register clause 2's " +
			"whole subject")
	}

	// ── THE SKY BAND'S OWN TWO-DIRECTIONAL CLAUSE ─────────────────────────
	//
	// The same shape, for the same reason: it fails if the sky's rules VANISH
	// (the carve-out was quietly deleted and the operator's amended clause 4
	// stopped buying anything) and if they ESCAPE the wrapper (under
	// prefers-reduced-motion the band would then animate, and [SKY-4] says
	// instant AND complete, structurally).
	const skyRule = ".cal-bench .cal-block-host .skygrow .skpane"
	if !strings.Contains(motion, skyRule) {
		t.Errorf("the sky's pane is not inside %q — the carve-out is a named addition "+
			"INSIDE the one register section and a band declared anywhere else would "+
			"be the second grammar", guard)
	}
	// THE ESCAPE DIRECTION IS COUNTED ON TRANSITIONS, NOT ON PRELUDES, and that
	// difference is load-bearing rather than pedantic. The morph's class exists
	// ONLY while the morph is in flight, so its prelude appears nowhere but the
	// wrapper and a prelude count is exact for it. The sky's class is on a
	// resting element that also has to be LAID OUT — `.skpane` is a 0fr grid
	// row outside the wrapper, which is the clip-reveal's box and not motion —
	// so counting its prelude would red the structural rule the reveal needs.
	// What must never escape is a TRANSITION.
	skyTransitions := func(css string) int {
		n := 0
		for _, rule := range benchCSSRules(css) {
			if strings.Contains(rule[0], ".skygrow") {
				n += len(benchTransitionDecls(rule[1]))
			}
		}
		return n
	}
	if outside, inside := skyTransitions(code), skyTransitions(motion); outside != inside {
		t.Errorf("%d of the sky's %d transition declarations sit OUTSIDE the "+
			"reduced-motion wrapper; [SKY-4] makes reduced motion STRUCTURAL — no rule "+
			"at all — and never a shortened duration or a `reduce` override",
			outside-inside, outside)
	}
	// CLOSE FASTER THAN OPEN, STRUCTURALLY, in the register's own idiom: the
	// base rules carry the CLOSE timing and the [open] state overrides it.
	if !strings.Contains(motion, ".cal-bench .cal-block-host .skygrow[open] .skpane") {
		t.Error("the sky declares a closed pane and no open one — the reveal has no " +
			"endpoint and the register's 200ms open cannot run")
	}
	if !strings.Contains(motion, ".cal-bench .cal-block-host .skygrow[open] {") &&
		!strings.Contains(motion, ".cal-bench .cal-block-host .skygrow[open],") {
		t.Error("the sky declares no open-state duration override on its own box — the " +
			"seal's sweep would then take exactly as long to leave as to arrive")
	}

	// AND THE SEQUENCING TOKENS SUM TO THE TWO TOTALS, which is the whole
	// reason a third DURATION was not bought ([SKY-3] SIGNED). The mock split
	// its open 150/50 and its close 24/136; those are phases of 200ms and
	// 160ms, not new durations, and the only way that sentence can be trusted
	// is arithmetically. CHANGING THIS ASSERTION IS A STOP-AND-FLAG, NOT A FIX.
	phase := func(token string) int {
		t.Helper()
		m := regexp.MustCompile(token + `\s*:\s*(\d+)ms`).FindStringSubmatch(code)
		if m == nil {
			t.Fatalf("%s is not defined — the sky's sequencing is undeclared", token)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("%s is not a whole number of ms", token)
		}
		return n
	}
	if got := phase("--sky-phase-box-open") + phase("--sky-phase-reveal-open"); got != open {
		t.Errorf("the sky's open phases sum to %dms, but --disc-open is %dms — the four "+
			"--sky-phase-* tokens are a SEQUENCING of the register's two totals and a "+
			"sum that does not match is a third duration wearing a phase's name",
			got, open)
	}
	if got := phase("--sky-phase-reveal-close") + phase("--sky-phase-box-close"); got != closeMS {
		t.Errorf("the sky's close phases sum to %dms, but --disc-close is %dms", got, closeMS)
	}
}

// TestBenchCSS_TheNamedCarveOutsAreExactlyTwo is the carve-outs' MONOPOLY guard
// ([ER-7] SIGNED, C-CALV4-EDITOR-R2b).
//
// RENAMED BY C-CALV4-SKY (R2-5) UNDER THE OPERATOR'S SIGNATURE OF 2026-08-07
// ([SKY-2]). It was TestBenchCSS_TheEditorMorphIsTheOnlyCarveOut, and that name
// stated a monopoly the product no longer has: THERE ARE NOW TWO NAMED
// CARVE-OUTS — the editor morph and the sky band — AND A THIRD REQUIRES AN
// OPERATOR SIGNATURE. Renaming it is not cosmetic debt-paying: a guard whose
// name still claims a monopoly it lost is how the next hand learns the wrong
// law, and this file's own history is that a comment outlived its rules for a
// whole wave before anyone noticed.
//
// WHY A SEPARATE GUARD RATHER THAN ANOTHER CLAUSE. The clause above widens an
// allowlist for rules that name a carve-out class. That widening is only safe
// if the class itself cannot spread — otherwise the next surface that wants a
// geometric transition simply adds the class to its own rule and the allowlist
// admits it silently. So each class is pinned as narrowly as its properties are.
//
// THREE CLAIMS, and the third is the operator's signature made mechanical:
//
//  1. EACH CARVE-OUT'S PROPERTIES APPEAR ONLY UNDER ITS OWN CLASS, and only in
//     this sheet. `translate` is the morph's alone; grid-template-rows, gap,
//     --seal-solid and --seal-fade are the sky's alone; `inline-size` is the
//     one property both geometric surfaces need and is therefore owned by both
//     and by nothing else.
//
//  2. EACH CLASS APPEARS IN EXACTLY THE FILES ITS CARVE-OUT NAMES —
//     established by WALKING the repository's authored source, not by
//     consulting a list of files someone already suspected. Test files are
//     excluded by construction: a guard that counted its own assertions would
//     be counting itself, and a test cannot add a transition to a shipped
//     surface. See benchCarveOutMentions for exactly what is walked and what
//     is skipped.
//
//  3. NO RULE NAMING A CARVE-OUT CLASS ALSO NAMES `.benchblock`,
//     `.cal-block-host`, `.block`, `.lrow` or `[data-cell]` — bound 4 of the
//     editor signature, "never touch the Block's interior", checked rather than
//     promised — EXCEPT for the classes on the PER-CLASS EXEMPTION LIST.
//
//     THE FORBIDDEN-ANCESTOR LIST ITSELF IS NOT RELAXED BY A BYTE, and that is
//     the whole point of the exemption's shape. The operator's 2026-08-07
//     answer seated the sky's band INSIDE the Block, which clause 4 forbade;
//     the answer was to amend clause 4 BY NAME for the sky's band and to add
//     `.skygrow` to this list — never to remove an entry from the ancestor
//     list, which would have repealed clause 4 for every surface in the
//     product. EVERY OTHER RULE STAYS EXACTLY AS CONSTRAINED AS IT WAS.
//     Widening the ancestor list is still the unrecoverable act it always was;
//     a THIRD exemption is a NEW OPERATOR SIGNATURE, not a dev call.
//
// MUTATION-TESTED, C-CALV4-SKY STAGE 8, BOTH DIRECTIONS OF THE AMENDMENT, EACH
// REVERTED:
//
//	· a sky rule authored OUTSIDE `.skygrow` (`.cal-bench .cal-block-host .skb`
//	  carrying the same transition) turns claim 1 red — the exemption is keyed
//	  to the CLASS, not to the surface, not to the sheet, not to the word
//	  "sky".
//	· REMOVING `.skygrow` from the per-class exemption turns the sky's own rule
//	  red on claim 3 — which proves the exemption is load-bearing rather than
//	  decorative. A guard that passed with the exemption deleted would not be
//	  guarding anything.
//
// MUTATION-TESTED, STAGE 7, EACH REVERTED:
//
//	· `el.classList.add("edmorph")` appended to
//	  internal/plugins/calendar/static/js/gm_panel.js — a REAL third consumer
//	  one directory away — turns claim 2 red, BY NAME, on this test. Against
//	  the eight-file allowlist this replaced, the whole calendar package stayed
//	  GREEN, which is why the walk exists.
//	· `.cal-sched .edmorph { … }` appended to static/css/calendar-schedule.css
//	  turns claim 2 red on THIS test. Against the allowlist it went red only on
//	  that sheet's own scoping guard — the right answer for the wrong reason,
//	  which is indistinguishable from luck.
//	· `.cal-block-host` added to the morph's prelude turns claim 3 red.
func TestBenchCSS_TheNamedCarveOutsAreExactlyTwo(t *testing.T) {
	// THE TWO NAMED CARVE-OUTS, AS DATA — so the claims below iterate a list
	// rather than hard-coding one class, and so adding a third is visibly an
	// EDIT TO A SIGNED REGISTER rather than one more `strings.Contains`.
	type carveOutSpec struct {
		class string // the CSS class, with its dot
		bare  string // the bare token, as the repository walk sees it
		props []string
		files map[string]bool
		// interiorExempt is the PER-CLASS EXEMPTION on claim 3, and it is the
		// only thing the operator's 2026-08-07 answer added. The
		// forbidden-ancestor list itself is untouched.
		interiorExempt bool
	}
	carveOuts := []carveOutSpec{{
		class: ".edmorph", bare: "edmorph",
		props: []string{"inline-size", "translate"},
		files: map[string]bool{
			"static/css/calendar-bench.css":                           true,
			"internal/plugins/calendar/static/js/calendar_daycard.js": true,
		},
	}, {
		// THE SKY BAND. Its class rides the <details> the widget renders and
		// the sheet that styles it — TWO files, and no module, because the
		// whole surface is a native disclosure with NO JAVASCRIPT AT ALL.
		class: ".skygrow", bare: "skygrow",
		props: []string{"inline-size", "grid-template-rows", "gap", "--seal-solid", "--seal-fade"},
		files: map[string]bool{
			"static/css/calendar-bench.css":               true,
			"internal/widgets/calendar_block/block.templ": true,
		},
		interiorExempt: true,
	}}

	const carveOut = ".edmorph"
	const bare = "edmorph"
	css := benchCommentRe.ReplaceAllString(benchCSS(t), " ")

	// (1) each carve-out's properties, only under a class that owns them, only
	// here. `inline-size` is owned by BOTH geometric surfaces and by nothing
	// else, so the check is "some owner names it", never "this one does".
	owners := map[string][]string{}
	for _, c := range carveOuts {
		for _, p := range c.props {
			owners[p] = append(owners[p], c.class)
		}
	}
	for _, rule := range benchCSSRules(css) {
		for _, decl := range benchTransitionDecls(rule[1]) {
			for _, prop := range decl {
				classes, owned := owners[prop]
				if !owned {
					continue
				}
				named := false
				for _, cls := range classes {
					if strings.Contains(rule[0], cls) {
						named = true
					}
				}
				if !named {
					t.Errorf("`%s` transitions %q without naming %v — the geometric "+
						"properties belong to the NAMED carve-outs and a third surface "+
						"taking one is a third signature nobody signed", rule[0], prop, classes)
				}
			}
		}
	}

	// (2) EXACTLY TWO FILES PRODUCT-WIDE, AND "PRODUCT-WIDE" MEANS THE
	// REPOSITORY, NOT A LIST.
	//
	// FIXED FORWARD AT STAGE 7, AND THE PREVIOUS CUT IS THE REASON THIS
	// PARAGRAPH IS LONG. This claim used to iterate a HARDCODED EIGHT-FILE
	// ALLOWLIST while its comment said "product-wide" and "adding `.edmorph` to
	// a third file turns claim 2 red." It did not: adding the class to
	// internal/plugins/calendar/static/js/gm_panel.js — a real consumer, one
	// directory away, outside the list — left the entire calendar package green.
	// A closed allowlist can only ever find the files someone already thought
	// of, which is precisely the set that does not need finding.
	//
	// [ER-7]'s stated rationale is exactly this risk, in its own words: the
	// allowlist widening "is only safe if the class itself cannot spread —
	// otherwise the next surface that wants a geometric transition simply adds
	// the class to its own rule and the allowlist admits it silently." A guard
	// that cannot see the next surface cannot enforce that.
	//
	// It now WALKS the repository's own source — every .css, .js, .templ and
	// non-generated .go under the tracked source roots — and names every file
	// that mentions the class. Vendor, node_modules, .git, build output and
	// _templ.go generations are skipped because they are not authored here; the
	// test files that assert ABOUT the carve-out are skipped for the obvious
	// reason that they must name it to check it.
	for _, c := range carveOuts {
		found := map[string]bool{}
		for _, path := range benchCarveOutMentions(t, c.bare) {
			found[path] = true
		}
		if len(found) == 0 {
			t.Fatalf("the repository walk found no mention of %q at all — the walk is "+
				"not reading the tree, and every claim here would pass vacuously", c.bare)
		}
		for path := range found {
			if !c.files[path] {
				t.Errorf("%s names %q — that carve-out lives in exactly %d places, and a "+
					"further one is a second consumer of a signature that names ONE "+
					"surface", path, c.bare, len(c.files))
			}
		}
		for path := range c.files {
			if !found[path] {
				t.Errorf("%s no longer names %q — each carve-out is an operator's signed "+
					"ask and the reason its slice exists in the shape it does", path, c.bare)
			}
		}
	}

	// (3) BOUND 4, MECHANICALLY: a carve-out reads its OWN box and nothing
	// else, and no rule of it reaches into the Block.
	//
	// THE PER-CLASS EXEMPTION, AND WHY IT IS SHAPED LIKE THIS. The sky's band
	// seats INSIDE the Block by the operator's own 2026-08-07 answer, so its
	// rules necessarily name `.cal-block-host`. The answer was to exempt THAT
	// CLASS by name — never to remove an entry from the list below, which would
	// have repealed clause 4 for every surface in the product at once. The list
	// is therefore byte-identical to what it was before the amendment, and it
	// is written here rather than in the loop so a reader can see that it is.
	forbiddenAncestors := []string{".benchblock", ".cal-block-host", ".block", ".lrow", "[data-cell]"}
	for _, c := range carveOuts {
		if c.interiorExempt {
			continue
		}
		for _, rule := range benchCSSRules(css) {
			if !strings.Contains(rule[0], c.class) {
				continue
			}
			for _, inside := range forbiddenAncestors {
				if strings.Contains(rule[0], inside) {
					t.Errorf("`%s` names the carve-out %q AND %q — the signature's last "+
						"clause is \"never touch the Block's interior\", and a carve-out "+
						"that selected into it would be animating the one subtree this "+
						"whole arc is fenced around. Only `.skygrow` is exempt, and only "+
						"because the operator amended clause 4 by name on 2026-08-07",
						rule[0], c.class, inside)
				}
			}
		}
	}
	_ = carveOut
	_ = bare
}

// TestBenchCSS_TheSkyCarveOutIsPinned is the SKY BAND's own guard — C-CALV4-SKY
// slice R2-5, [SKY-3] SIGNED — written to the same three-claim shape as the
// monopoly guard above and asserting the things that guard cannot.
//
// WHY IT IS SEPARATE FROM THE MONOPOLY GUARD RATHER THAN A CLAUSE INSIDE IT.
// The monopoly guard answers "can this class spread?"; this one answers "is the
// sky's carve-out still the exact seven properties, two totals and one
// construction the operator signed?" Those are different questions with
// different failure modes, and folding the second into the first would give one
// test two reasons to be red — which is how a guard gets nudged until green.
//
// THREE CLAIMS:
//
//  1. THE PROPERTIES ARE THE SIGNED SEVEN PLUS THE ONE AMENDED EIGHTH AND THERE
//     IS NO NINTH, and every one of them is transitioned only under `.skygrow`. `transform` is refused
//     everywhere on this surface, INCLUDING `scale` — which is named on its own
//     because "no transform" is the sentence people read past.
//  2. THE CLASS IS THE MARKUP'S AND THE SHEET'S, established by the same
//     repository walk the monopoly guard uses. There is NO third file, and in
//     particular no JS module: the whole surface is a native <details>, and a
//     script would be DELETED anyway inside an HTMX-swapped fragment.
//  3. THE CONSTRUCTION IS [SKY-4]'s: no second `no-preference` wrapper, no
//     `prefers-reduced-motion: reduce` block, and no `!important` anywhere in
//     the sky's rules. The mock reaches the same OUTCOME by the exact inverse
//     construction and it does not port.
//
// MUTATION-TESTED IN ALL THREE DIRECTIONS, C-CALV4-SKY STAGE 8, EACH REVERTED:
//
//	· adding `transform: scale(1.02)` under `.skygrow[open]` reds claim 1
//	· adding `color` under `.skygrow[open]` reds claim 1's "no ninth" arm, and
//	  deleting the `::details-content` rules reds its "one going missing" arm
//	  (calv4 fix R1 — the amendment that made the seven eight)
//	· adding `.skygrow` to a third authored file reds claim 2
//	· adding a `@media (prefers-reduced-motion: reduce)` block reds claim 3
//	  (and TestBenchCSS_NoMotionAtAll's wrapper count fatals beside it, which
//	  is the belt to this braces)
func TestBenchCSS_TheSkyCarveOutIsPinned(t *testing.T) {
	const skyClass = ".skygrow"
	const skyBare = "skygrow"
	raw := benchCSS(t)
	css := benchCommentRe.ReplaceAllString(raw, " ")

	// (1) THE SEVEN, AND NO EIGHTH.
	//
	// It is asserted as a SET EQUALITY rather than as a subset check, because a
	// property quietly DISAPPEARING is as much a defect as one arriving: the
	// discs no longer growing, or the mask no longer sweeping, is the carve-out
	// the operator signed silently ceasing to exist while every other test
	// stays green.
	//
	// ── THE EIGHTH, AMENDED BY NAME — calv4 fix R1, item 2 ────────────────
	//
	// [SKY-3] SIGNED said "seven properties and no eighth". This is the eighth,
	// and it is `content-visibility`, once, on `::details-content`.
	//
	// WHY THE AMENDMENT IS NOT THE THING THIS GUARD EXISTS TO REFUSE. The seven
	// are properties the sky ANIMATES; this one animates nothing and cannot —
	// it is discrete, it is transitioned `allow-discrete`, and its entire
	// function is to keep the subtree RENDERED so the other seven can be seen.
	// Without it the signed close does not exist: measured by
	// sky_close_probe_test.go before the rule landed, the sky went 152.9px →
	// 40.0px (C1) and 263.1px → 32.0px (C3) in a SINGLE FRAME, with
	// ::details-content already computing `content-visibility: hidden` in the
	// very frame `open` was removed, while the signed 160ms grid-template-rows
	// transition ran correctly and invisibly.
	//
	// IT IS THE SHEET'S OWN EXISTING IDIOM, NOT A NEW ONE. The identical
	// declaration is already carried by `.cal-bench .disc::details-content`
	// some 300 lines above the sky's rules, every other disclosure on the page
	// uses it, and `content-visibility` was already on the monopoly guard's
	// BASE allowlist for exactly this reason — which is why the monopoly guard
	// stayed green through this change and only THIS set-equality went red.
	// The sky was authored as a `.skpane` grid-row clip-reveal and was the one
	// disclosure in the product that closed by cutting.
	//
	// NO THIRD TOTAL. --disc-close out, --disc-open in, matching the register's
	// structure exactly; the `--disc-*` count assertion is byte-unchanged.
	//
	// MUTATION-TESTED BOTH WAYS, calv4 fix R1, each reverted:
	//   · deleting the `::details-content` rules reds this guard's
	//     "no longer transitions" arm AND sky_close_probe_test.go
	//   · adding a NINTH (`color` under `.skygrow[open]`) reds the "not one of
	//     the signed eight" arm, so the list is still a list and not a door
	want := map[string]bool{
		"grid-template-rows": true, "opacity": true, "inline-size": true,
		"block-size": true, "gap": true, "--seal-solid": true, "--seal-fade": true,
		"content-visibility": true,
	}
	got := map[string]bool{}
	for _, rule := range benchCSSRules(css) {
		if !strings.Contains(rule[0], skyClass) {
			continue
		}
		for _, decl := range benchTransitionDecls(rule[1]) {
			for _, prop := range decl {
				got[prop] = true
			}
		}
	}
	if len(got) == 0 {
		t.Fatal("the sky transitions nothing at all — the carve-out the operator " +
			"amended clause 4 to admit has stopped existing, and every claim below " +
			"would pass vacuously")
	}
	for prop := range got {
		if !want[prop] {
			t.Errorf("the sky transitions %q, which is not one of the signed eight "+
				"(grid-template-rows · opacity · inline-size · block-size · gap · "+
				"--seal-solid · --seal-fade · content-visibility) — [SKY-3] named seven "+
				"and the close's `content-visibility` was amended in as the eighth, BY "+
				"NAME. A ninth needs the same treatment: a named amendment and a "+
				"mutation test, not a quiet addition to this map", prop)
		}
	}
	for prop := range want {
		if !got[prop] {
			t.Errorf("the sky no longer transitions %q — the signed eight are a LIST, "+
				"and one going missing is the carve-out shrinking without a signature. "+
				"For `content-visibility` in particular, losing it does not shrink the "+
				"motion: it makes the whole close invisible, which is what "+
				"sky_close_probe_test.go measures", prop)
		}
	}
	// `transform` is refused, and `scale` is named on its own.
	for _, rule := range benchCSSRules(css) {
		if !strings.Contains(rule[0], skyClass) {
			continue
		}
		// benchTransformRe, not a substring: `text-transform` is TYPOGRAPHY and
		// the season word and every lane label are uppercase by design. A
		// blanket substring ban would be red on day one and would then be
		// "fixed" by deleting the ban, which is the failure mode this guard is
		// meant to be immune to.
		if benchTransformRe.MatchString(rule[1]) {
			t.Errorf("`%s` declares `transform` — it is refused on this surface exactly "+
				"as it is under the morph, and the caret is a content swap rather than "+
				"a rotation ([SKY-12])", rule[0])
		}
		if strings.Contains(rule[1], "scale(") {
			t.Errorf("`%s` declares a `scale()` — [SKY-3] refuses transform INCLUDING "+
				"scale, and scale is named on its own because \"no transform\" is the "+
				"sentence people read past", rule[0])
		}
	}

	// (2) TWO FILES, WALKED. The sheet that declares the transition and the
	// template that renders the class. No module: there is no JS in the Block's
	// package and there structurally cannot be.
	found := map[string]bool{}
	for _, path := range benchCarveOutMentions(t, skyBare) {
		found[path] = true
	}
	wantFiles := map[string]bool{
		"static/css/calendar-bench.css":               true,
		"internal/widgets/calendar_block/block.templ": true,
	}
	if len(found) == 0 {
		t.Fatal("the repository walk found no mention of the sky's class at all")
	}
	for path := range found {
		if !wantFiles[path] {
			t.Errorf("%s names %q — the sky's carve-out lives in exactly two places, and "+
				"a third would be a second surface borrowing a signature that names ONE "+
				"band, on ONE Block, on ONE surface", path, skyBare)
		}
	}
	for path := range wantFiles {
		if !found[path] {
			t.Errorf("%s no longer names %q — one half of the pair went missing, and a "+
				"sheet with no markup or markup with no sheet is the #568 gap", path, skyBare)
		}
	}

	// (3) THE CONSTRUCTION IS STRUCTURAL ([SKY-4]). Read from the RAW sheet,
	// comments and all, because a `!important` inside a comment is still a
	// sentence someone will copy.
	if strings.Contains(css, "prefers-reduced-motion: reduce") ||
		strings.Contains(css, "prefers-reduced-motion:reduce") {
		t.Error("the sheet carries a `prefers-reduced-motion: reduce` block — the mock's " +
			"reduce-and-override construction is a MOCK convenience for its own " +
			"data-motion switch and does not port; reduced motion here is the ABSENCE " +
			"of a rule, which is why there is exactly one no-preference wrapper")
	}
	for _, rule := range benchCSSRules(css) {
		if strings.Contains(rule[0], skyClass) && strings.Contains(rule[1], "!important") {
			t.Errorf("`%s` uses !important — [SKY-4] forbids it anywhere in this slice, "+
				"because an override is precisely what reduced motion must NOT be", rule[0])
		}
	}
}

// benchCarveOutMentions walks the repository's authored source and returns every
// file that names the carve-out class, repo-relative and slash-separated.
//
// WHAT IT READS: the four source roots this product's front end actually lives
// in — `static/`, `internal/`, `cmd/` and `tools/` — over `.css`, `.js`, `.mjs`,
// `.templ` and `.go`. WHAT IT SKIPS, and why each skip is safe:
//
//	· `node_modules`, `vendor`, `.git`, `dist`, `bin`, `tmp` — not authored here.
//	· `*_templ.go` — GENERATED from a `.templ` that the walk already reads, so a
//	  class in one is a class in the other and counting both would report two
//	  files for one authoring decision.
//	· `*_test.go` and `test/js/*.test.mjs` — a test that ASSERTS about the
//	  carve-out must name it. Excluding assertions from a monopoly count is not
//	  a loophole: a test file cannot add a transition to a shipped surface.
//
// The point of a walk over a list is that it finds the file nobody thought of.
// That is the whole difference between this and the eight-name allowlist it
// replaced.
func benchCarveOutMentions(t *testing.T, needle string) []string {
	t.Helper()
	root := repoRootFrom(t)
	skipDir := map[string]bool{
		"node_modules": true, "vendor": true, ".git": true,
		"dist": true, "bin": true, "tmp": true,
	}
	exts := map[string]bool{
		".css": true, ".js": true, ".mjs": true, ".templ": true, ".go": true,
	}
	var out []string
	for _, top := range []string{"static", "internal", "cmd", "tools", "test"} {
		base := filepath.Join(root, top)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if skipDir[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			name := info.Name()
			if !exts[strings.ToLower(filepath.Ext(name))] {
				return nil
			}
			if strings.HasSuffix(name, "_templ.go") ||
				strings.HasSuffix(name, "_test.go") ||
				strings.HasSuffix(name, ".test.mjs") {
				return nil
			}
			body, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if !strings.Contains(string(body), needle) {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}
	sort.Strings(out)
	return out
}

// TestBenchProse_MotionClaimsMatchTheSheet pins the two SENTENCES that describe
// this slice's motion budget to the sheet they describe.
//
// WHY A TEST FOR PROSE. Stage 8 of R2-1 rewrote a comment that had described the
// twisty as a rotation the rules never shipped, and pinned it — and the verifier
// then found TWO MORE claims of the same class, in files this slice had edited
// three times: bench.templ's header still said "calendar-bench.css defines no
// transition, no animation and no @keyframes, and its contract test says so"
// (both clauses died in stage 3), and calendar-bench.css's RSVP header said "the
// Bench sheet is under tools/check-v2-motion-discipline.sh" (it never has been).
// The second is worse than a stale sentence: it names a guard as the safety net
// under the exact allowlist that [BR2-2] warned has no guard, so a reader
// widening the allowlist would believe CI was watching. This test derives both
// facts instead of trusting either sentence.
func TestBenchProse_MotionClaimsMatchTheSheet(t *testing.T) {
	css := benchCSS(t)
	templ := readRepoFile(t, "internal/plugins/calendar/bench.templ")
	sources := map[string]string{"static/css/calendar-bench.css": css, "internal/plugins/calendar/bench.templ": templ}

	// THE DERIVED FACT, not an assumption: the sheet really does transition
	// something. If a later slice removes the disclosure motion entirely, this
	// fails FIRST and tells the reader the blanket denials below are allowed
	// back — the test never silently forbids a true sentence.
	if !benchTransitionRe.MatchString(benchCommentRe.ReplaceAllString(css, " ")) {
		t.Fatal("calendar-bench.css declares no `transition:` at all — the register was " +
			"removed, and the wave-1 blanket denials this test forbids would now be TRUE. " +
			"Re-read decisions/2026-07-29-motion-disclosure-register.md before deleting this.")
	}

	// (a) NO SHEET-WIDE DENIAL. Scoped denials stay legal and are used just
	// below the line this caught ("this panel adds no transition…" is true of
	// that panel); what is banned is a claim about the SHEET, which is false.
	denial := regexp.MustCompile(`(?is)(calendar-bench\.css|the bench sheet|this sheet)[^.]{0,160}?\bno (transition|animation|@keyframes)`)
	for path, src := range sources {
		if m := denial.FindString(src); m != "" {
			t.Errorf("%s claims the SHEET has no motion — %q — but it transitions "+
				"block-size/opacity/content-visibility on `.cal-bench .disc::details-content` "+
				"under [BR2-2] SIGNED. Scope the sentence or delete it", path, strings.Join(strings.Fields(m), " "))
		}
	}

	// (b) THE GUARD IS NAMED HONESTLY OR NOT AT ALL. tools/check-v2-motion-
	// discipline.sh scopes to internal/plugins/{calendar,timeline,ai_workspace,
	// campaigns} and filters to `${scope}/*.templ` / `${scope}/*.css`, so
	// static/css/ has never been inside it. Any mention of it near this sheet
	// must carry the negation; the sanctioned phrasing is "does not police",
	// deliberately one string so a new mention has to come here and read this.
	guard := regexp.MustCompile(`does not police`)
	for path, src := range sources {
		for _, idx := range regexp.MustCompile(`check-v2-motion-discipline`).FindAllStringIndex(src, -1) {
			lo, hi := idx[0]-240, idx[1]+240
			if lo < 0 {
				lo = 0
			}
			if hi > len(src) {
				hi = len(src)
			}
			if !guard.MatchString(src[lo:hi]) {
				t.Errorf("%s names check-v2-motion-discipline.sh without saying it DOES NOT "+
					"POLICE static/css/ — the guard's scope is internal/plugins/*, so naming it "+
					"here promises an enforcement that has never run. The enforcement is "+
					"TestBenchCSS_NoMotionAtAll", path)
			}
		}
	}

	// (c) MECHANISM, NOT ABSENCE. The header that used to deny motion must now
	// point at the document that authorised it, so the next reader lands on the
	// law rather than on a hole where the wave-1 rule used to be — the same
	// reason TestBenchCSS_NoMotionAtAll kept its name when it was inverted.
	for _, want := range []string{"2026-07-29-motion-disclosure-register.md", "TestBenchCSS_NoMotionAtAll"} {
		if !strings.Contains(templ, want) {
			t.Errorf("bench.templ's header does not cite %q — a header that describes the "+
				"motion budget must name the law and the enforcement, or it decays again", want)
		}
	}
}

// benchCSSBlock returns the body of the at-rule that starts with `prelude`,
// brace-matched. The index is checked before it is used as a bound: a bare
// strings.Index slice bound PANICS on a rename instead of failing cleanly
// (COMMON §3).
func benchCSSBlock(t *testing.T, code, prelude string) string {
	t.Helper()
	i := strings.Index(code, prelude)
	if i < 0 {
		t.Fatalf("no %q block in calendar-bench.css", prelude)
	}
	rest := code[i:]
	open := strings.Index(rest, "{")
	if open < 0 {
		t.Fatalf("%q has no body", prelude)
	}
	depth := 0
	for j := open; j < len(rest); j++ {
		switch rest[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[open+1 : j]
			}
		}
	}
	t.Fatalf("%q body is unterminated", prelude)
	return ""
}

// benchTransformRe matches a `transform:` declaration and NOT `text-transform:`
// — the character before the word must not be a hyphen or a word character.
var benchTransformRe = regexp.MustCompile(`(^|[^-\w])transform\s*:`)

// benchCSSRules yields (prelude, body) for every brace-delimited rule in the
// sheet, including rules nested inside at-rules.
//
// It is exact for LEAF rules, which is what every caller wants: the prelude is
// the text since the previous brace, and the body is that rule's own
// declarations. An at-rule's own "body" comes out as the tail after its last
// nested rule, which is harmless here because at-rule preludes
// (@media, @supports) never name a selector this test asks about.
func benchCSSRules(code string) [][2]string {
	var out [][2]string
	var stack []string
	start := 0
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '{':
			stack = append(stack, strings.TrimSpace(code[start:i]))
			start = i + 1
		case '}':
			if len(stack) > 0 {
				out = append(out, [2]string{stack[len(stack)-1], code[start:i]})
				stack = stack[:len(stack)-1]
			}
			start = i + 1
		}
	}
	return out
}

var benchTransitionRe = regexp.MustCompile(`transition\s*:\s*([^;}]*)`)

// benchTransitionDecls returns, per `transition:` declaration, the property
// names it names. A transition's first token is the property; `allow-discrete`
// and duration/easing values are not properties and are skipped.
func benchTransitionDecls(css string) [][]string {
	var out [][]string
	for _, m := range benchTransitionRe.FindAllStringSubmatch(css, -1) {
		var props []string
		for _, part := range strings.Split(m[1], ",") {
			fields := strings.Fields(strings.TrimSpace(part))
			if len(fields) == 0 {
				continue
			}
			props = append(props, fields[0])
		}
		out = append(out, props)
	}
	return out
}

var benchDurationRe = regexp.MustCompile(`(--disc-(?:open|close))\s*:\s*(\d+)ms`)

// benchCSSDurationMS reads one --disc-* duration token as a number, so the
// close < open clause is asserted arithmetically rather than by eye.
func benchCSSDurationMS(css, token string) (int, bool) {
	for _, m := range benchDurationRe.FindAllStringSubmatch(css, -1) {
		if m[1] != token {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// Every selector is scoped under .cal-bench. An unlayered sheet outranks the
// app's layered CSS, so a bare `.badge` rule in here would silently restyle the
// whole product — which is exactly the class of bug the Block's sheet carries
// the same warning about.
//
// It reads the sheet BY BRACE, not by line — the same DC2-SCOPEGUARD-LINEFORM
// fix-forward daycard_test.go already took, applied here to the last line-form
// scanner left in the repo. The line-form version only inspected lines ENDING
// in `{`, and never split a comma list, so it examined 113 of this sheet's 180
// rules and 113 of its 190 comma-list members: a one-liner `.badge { … }` and a
// `.badge,` member of a multi-selector prelude were both invisible to it.
// Measured, every one of the 67 unexamined rules was correctly scoped, which is
// precisely why the hole would have stayed invisible until it wasn't.
func TestBenchCSS_EverySelectorIsScoped(t *testing.T) {
	sels := cssSelectors(benchCommentRe.ReplaceAllString(benchCSS(t), " "))
	if len(sels) < 150 {
		t.Fatalf("only %d selectors found; the parser stopped reading the sheet", len(sels))
	}
	for _, sel := range sels {
		// A prelude is a comma-separated LIST; each member carries the scope on
		// its own, so the list has to be split before it is judged.
		for _, part := range strings.Split(sel, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			// Contains, not HasPrefix. The sheet's dark-mode rules are written
			// `.dark .cal-bench …` — legitimately scoped, and a HasPrefix form
			// would redden both of them today. The scope root just has to be
			// somewhere in the compound.
			if !strings.Contains(p, ".cal-bench") {
				t.Errorf("unscoped selector in calendar-bench.css: %q (in prelude %q)", p, sel)
			}
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
		// The disclosure register and the measure (C-CALV4-BENCH-R2 slice R2-1).
		// bench.templ names .disc, <summary> and the four data-bench-disc keys;
		// this is the pin that stops the sheet dropping what that markup depends
		// on — the #568 gap, and the reason .disc and summary are exactly the
		// generic nouns TestBenchCSS_EverySelectorIsScoped exists for.
		".cal-bench .disc", ".cal-bench .disc > summary", ".cal-bench .bsurf",
		".cal-bench .disc::details-content",
		"--disc-open", "--disc-close", "--disc-ease",
		"--bench-measure",
		// The day card's reveal (C-CALV4-DAYCARD, R2-2a). bench.templ mounts
		// the scaffold and calendar_daycard.js toggles .dcopen on it; without
		// these two rules the card would appear instantly at every motion
		// setting and the register would have one consumer fewer than its own
		// comment claims.
		".cal-bench .cal-daycard .dcbox", ".cal-bench .cal-daycard.dcopen .dcbox",
		// THE BLOCK THEATER's reveal (C-CALV4-THEATER, R2-3). theater.templ
		// mounts the scaffold and calendar_theater.js toggles .tbopen on its
		// .tbox; without these two rules the theater would appear instantly at
		// every motion setting while calendar-theater.css — which is FORBIDDEN
		// to declare a transition — stayed green and said nothing. That is the
		// #568 gap with the register's monopoly on top of it, which is why the
		// pin is here rather than in the satellite sheet's own guard.
		".cal-bench.cal-theater .tbox", ".cal-bench.cal-theater .tbox.tbopen",
		// The §8 two-column RSVP treatment.
		".cal-bench .rsvp .mtable",
		// THE SKY HEADER (C-CALV4-SKY, R2-5). block.templ names all of these
		// and this sheet is the ONLY place they are defined — the band seats in
		// the Block but its CSS is the Bench's ([SKY-2]), so a tidying hand in
		// calendar-block.css cannot see that anything depends on them. That is
		// the #568 gap with an extra package boundary in the way, which is why
		// the pin is long rather than token.
		".cal-bench .cal-block-host .skygrow",
		".cal-bench .cal-block-host .skygrow::before",
		".cal-bench .cal-block-host .skygrow > summary.skyhdr",
		".cal-bench .cal-block-host .skygrow .skdiscs",
		".cal-bench .cal-block-host .skygrow .skb",
		".cal-bench .cal-block-host .skygrow .sksp",
		".cal-bench .cal-block-host .skygrow .sktime",
		".cal-bench .cal-block-host .skygrow .skseason",
		".cal-bench .cal-block-host .skygrow .skcaret",
		".cal-bench .cal-block-host .skygrow .skpane",
		".cal-bench .cal-block-host .skygrow .skpane-in",
		".cal-bench .cal-block-host .skygrow .skpane-pad",
		".cal-bench .cal-block-host .skygrow .skhead",
		".cal-bench .cal-block-host .skygrow .sktabs",
		".cal-bench .cal-block-host .skygrow .skypick",
		".cal-bench .cal-block-host .skygrow .skybtn",
		".cal-bench .cal-block-host .skygrow .skypane-t",
		".cal-bench .cal-block-host .skygrow .skyrow",
		// THE FOUR ALIGNED COLUMNS OF THE SIGNED TONIGHT ROW. Added in the
		// C-CALV4-SKY fix round: the first pass shipped a two-item flex and the
		// stills show four columns, so these three are as load-bearing as the
		// name column already was. `.nx` is DECLARED here and switched on at C1
		// — the base rule is the seal's `display:none`.
		".cal-bench .cal-block-host .skygrow .skyrow .ph",
		".cal-bench .cal-block-host .skygrow .skyrow .il",
		".cal-bench .cal-block-host .skygrow .skyrow .nx",
		".cal-bench .cal-block-host .skygrow .skynote",
		// The sub-head's tight position under the bold head. Without it the
		// muted line inherits the footnote's 8px and reads as a third block
		// rather than as part of the header stack.
		".cal-bench .cal-block-host .skygrow .skhead + .skynote",
		".cal-bench .cal-block-host .skygrow .skylanes",
		".cal-bench .cal-block-host .skygrow .skylane",
		// The nameplate's hairline goes so the two read as ONE HEADER STACK —
		// the stills' own words, and the thing a reader would restore first if
		// nothing said it was deliberate.
		".cal-bench .cal-block-host .block:has(.skygrow) .np",
		// The C1/C3 density switch, on the Block's EXISTING boundary. A fifth
		// breakpoint number is a STOP-AND-FLAG, so the number is pinned here.
		"@container cal-block (min-width: 481px)",
		// The seal's two registered lengths. Unregistered, the mask JUMPS
		// instead of sweeping, and a jump at 0ms is the pop the carve-out
		// exists to avoid.
		"@property --seal-solid",
		"@property --seal-fade",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("calendar-bench.css does not define %q", want)
		}
	}
	// THE INNER MEASURES SURVIVE THE PAGE CAP. .note's 88ch and .caption's 104ch
	// are measures on RUNNING TEXT and the page cap does not subsume them — at
	// 1180px, 104ch is still the tighter bound. A tidying hand that reads them as
	// redundant after --bench-measure landed would widen both back out.
	for _, want := range []string{"max-width: 88ch", "max-width: 104ch"} {
		if !strings.Contains(code, want) {
			t.Errorf("calendar-bench.css lost %q — the page cap does not subsume an inner text measure", want)
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
		`<div class="cal-bench"><div class="` + benchShotSurfaceClass + `" data-bench-surface>` +
		body +
		`</div></div>` +
		`</div></body></html>`
}

// ── C-CALV4-MOBILE [MOB-10] — THE CAMERA WAS POINTED AT A PAGE PRODUCTION
//
//	DOES NOT RENDER ────────────────────────────────────────────────────────
//
// benchShotPage wrapped benchSurface(...) in `<div class="cal-bench">` and
// stopped there. bench.templ emits an INNER `<div class="bsurf"
// data-bench-surface>` (bench.templ ~:100), and every ≤640 reading-order rule
// [BR2-4] signed is written `.cal-bench .bsurf > .phead|.sechead|.stack`
// (calendar-bench.css ~:1539-1545). With the inner wrapper missing those
// selectors matched NOTHING, so every phone shot this rig ever took — 03, 04,
// 10, 11 and 22, the entire phone evidence set — showed the ribbon above the
// calendars while the product shows the calendars first.
//
// THE PRODUCT WAS RIGHT AND THE CAMERA WAS WRONG, and that is why a 41px
// Ledger reached a live build: the only pictures anyone had of the phone Bench
// were pictures of a different layout. No acceptance row in the MOBILE slice
// may be gated on an artefact taken before this line landed.
//
// benchShotSurfaceClass is a named constant rather than a literal so
// TestBenchShotPage_StandsInTheProductsOwnSurface can pin the camera's wrapper
// against the sheet's own selector prefix instead of against a copy of it.
const benchShotSurfaceClass = "bsurf"

// benchOrderDeclRe matches a flex `order:` declaration and NOT `border:`,
// `scroll-behavior` or any other word ending in "order".
var benchOrderDeclRe = regexp.MustCompile(`(^|[^-\w])order\s*:\s*-?\d`)

// TestBenchShotPage_StandsInTheProductsOwnSurface derives the ancestor chain
// the ≤640 reading-order rules require FROM THE SHEET and asserts the shot rig
// builds that chain. It does not compare against a copy of the class name: a
// rename of `.bsurf` in bench.templ + calendar-bench.css must red this test
// rather than silently pass while the camera drifts again.
//
// RED WITHOUT THE FIX: with benchShotPage emitting only `<div class="cal-bench">`
// the derived chain `.cal-bench .bsurf` has no `bsurf` to find and the run
// fails naming the missing wrapper.
func TestBenchShotPage_StandsInTheProductsOwnSurface(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(benchCSS(t), " ")

	// Every ordering rule the sheet ships, by its own selector. The sheet
	// carries three separate `@media (max-width: 640px)` blocks, so this reads
	// the whole file rather than the first one it finds.
	var chains []string
	for _, r := range benchCSSRules(code) {
		if !benchOrderDeclRe.MatchString(r[1]) {
			continue
		}
		sel := strings.TrimSpace(r[0])
		cut := strings.Index(sel, ">")
		if cut < 0 {
			continue
		}
		chains = append(chains, strings.TrimSpace(sel[:cut]))
	}
	if len(chains) == 0 {
		t.Fatal("no `order:` rule found in the ≤640 block — [BR2-4] ships three of them, " +
			"so either the reading order was retired or this test is reading the wrong block")
	}

	page := benchShotPage(
		benchShot{title: "t", caption: "c", w: 390, h: 800, host: 358},
		"", `<div class="stack"></div>`)

	for _, chain := range chains {
		// ".cal-bench .bsurf" → ["cal-bench", "bsurf"], in order.
		at := 0
		for _, part := range strings.Fields(chain) {
			cls := strings.TrimPrefix(part, ".")
			needle := `class="` + cls + `"`
			i := strings.Index(page[at:], needle)
			if i < 0 {
				t.Fatalf("the shot rig does not build %q: no %s at or after offset %d.\n"+
					"[MOB-10]: with the chain broken every ≤640 ordering rule matches nothing "+
					"and every phone artefact this rig produces is of a layout production does "+
					"not render — which is how a 41px Ledger reached a live build.",
					chain, needle, at)
			}
			at += i + len(needle)
		}
	}
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

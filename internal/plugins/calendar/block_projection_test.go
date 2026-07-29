package calendar

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// blockEvent builds a minimal event on a day of the fixture month.
func blockEvent(id string, day int, visibility string) Event {
	return Event{
		ID: id, CalendarID: "cal-harptos", Name: "Event " + id,
		Year: 1523, Month: 1, Day: day, Visibility: visibility,
	}
}

// blockProjectionFixture is the signed entity-host scenario: a month carrying
// events the viewer may see and one the viewer may not, some tied to the host
// entity and some not.
func blockProjectionFixture() (*Calendar, []Event, map[string]bool) {
	cal := blockTenDayCal()
	cal.EventCategories = []EventCategory{
		{ID: 1, CalendarID: cal.ID, Slug: "social", Name: "Social", Icon: "◆", Color: "#3b82f6"},
	}
	social := "social"
	events := []Event{
		blockEvent("tied-visible-1", 3, "everyone"),
		blockEvent("tied-visible-2", 8, "everyone"),
		blockEvent("untied-visible", 12, "everyone"),
		blockEvent("tied-hidden", 5, "dm_only"), // a player must never learn this exists
	}
	events[0].Category = &social
	tied := map[string]bool{"tied-visible-1": true, "tied-visible-2": true, "tied-hidden": true}
	return cal, events, tied
}

func blockCopyEvents(in []Event) []Event {
	out := make([]Event, len(in))
	copy(out, in)
	return out
}

// --- THE COUNT-ORACLE GUARD -------------------------------------------------

// TestBlockCountsAreNotAnOracle is the guard the projection exists for.
//
// The tie toggle shows two numbers side by side. If one is computed before the
// viewer filter and the other after, their DIFFERENCE is the number of events
// the viewer is not permitted to know about — the toggle becomes an oracle that
// leaks the existence of hidden content without ever rendering it. The signed
// mockup's own tiedCount is exactly that shape and is deliberately not ported.
//
// The assertion is not "the numbers are right" but "the hidden event is
// invisible in the pair": every number a player receives is computed from the
// player's own visible set, so nothing about `tied-hidden` can be recovered.
func TestBlockCountsAreNotAnOracle(t *testing.T) {
	cal, events, tied := blockProjectionFixture()

	player := projectBlock(BlockProjectionInput{
		Calendar:     cal,
		Events:       blockCopyEvents(events),
		Viewer:       BlockViewer{UserID: "u-player", Role: permissions.RolePlayer, HostEntity: "ent-1", TieMode: "tied"},
		MonthIndex:   0,
		Year:         1523,
		TiedEventIDs: tied,
	})
	gm := projectBlock(BlockProjectionInput{
		Calendar:     cal,
		Events:       blockCopyEvents(events),
		Viewer:       BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner, HostEntity: "ent-1", TieMode: "tied"},
		MonthIndex:   0,
		Year:         1523,
		TiedEventIDs: tied,
	})

	if player.Viewer.TiedCount != 2 || player.Viewer.WholeCount != 3 {
		t.Fatalf("player counts = tied %d / whole %d, want 2 / 3 — the dm_only event "+
			"must contribute to NEITHER number", player.Viewer.TiedCount, player.Viewer.WholeCount)
	}
	if gm.Viewer.TiedCount != 3 || gm.Viewer.WholeCount != 4 {
		t.Fatalf("GM counts = tied %d / whole %d, want 3 / 4", gm.Viewer.TiedCount, gm.Viewer.WholeCount)
	}

	// The oracle test proper: the player's two numbers must be reproducible from
	// the player's own visible set alone. If either had been computed pre-filter,
	// one of them would carry the hidden event and this would fail.
	visibleToPlayer := blockCopyEvents(events)
	visibleToPlayer = filterEventsByUser(visibleToPlayer, permissions.RolePlayer, "u-player")
	wantWhole := len(visibleToPlayer)
	wantTied := 0
	for i := range visibleToPlayer {
		if tied[visibleToPlayer[i].ID] {
			wantTied++
		}
	}
	if player.Viewer.WholeCount != wantWhole || player.Viewer.TiedCount != wantTied {
		t.Fatalf("player pair (%d,%d) is not derivable from the player's own visible set "+
			"(%d,%d) — one of the counts saw an unfiltered slice",
			player.Viewer.TiedCount, player.Viewer.WholeCount, wantTied, wantWhole)
	}

	// And the hidden event never reaches the DOM data at all — permission is
	// ABSENCE, not a greyed placeholder or a "+1".
	for _, row := range player.Month.Rows {
		for _, cell := range row.Cells {
			for _, m := range cell.Marks {
				if m.Title == "Event tied-hidden" {
					t.Fatal("a dm_only event's title reached a player's marks")
				}
			}
			if cell.Day == 5 && len(cell.Marks) != 0 {
				t.Fatalf("day 5 carries %d marks for a player; the only event there is dm_only",
					len(cell.Marks))
			}
		}
	}
}

// TestFilterEventsByUserMutatesTheCallerSlice pins the trap the one-pass rule
// exists to avoid (COMMON §7). filterEventsByUser compacts through
// `events[:0]`, so the caller's backing array is rewritten in place: a SECOND
// filtering pass over the original slice header reads compacted-then-stale
// elements. This documents the hazard next to the code that avoids it.
func TestFilterEventsByUserMutatesTheCallerSlice(t *testing.T) {
	original := []Event{
		blockEvent("a", 1, "everyone"),
		blockEvent("b", 2, "dm_only"),
		blockEvent("c", 3, "everyone"),
	}
	first := filterEventsByUser(original, permissions.RolePlayer, "u1")
	if len(first) != 2 {
		t.Fatalf("first pass returned %d, want 2", len(first))
	}
	// The caller's slice header still has length 3, but element 1 has been
	// overwritten with what used to be element 2.
	if original[1].ID != "c" {
		t.Fatalf("expected the backing array to be compacted in place; original[1].ID = %q",
			original[1].ID)
	}
	// A second pass over the same header therefore counts "c" twice — which is
	// precisely how a tie/whole pair computed in two passes goes wrong.
	second := filterEventsByUser(original, permissions.RolePlayer, "u1")
	if len(second) != 3 {
		t.Fatalf("second pass over the mutated header returned %d; the corruption this "+
			"test documents has changed shape and the projection's one-pass rule needs "+
			"re-checking", len(second))
	}
}

// --- tie toggle -------------------------------------------------------------

// TestBlockTieModeChangesInkNotGeometry pins canon A8 / L-M1: flipping the
// toggle may change what a cell says, never what it IS. No cell may grow, move
// or leave the DOM, and both counts stay put — that is what makes the toggle
// legal under the no-motion rule and non-differenceable.
func TestBlockTieModeChangesInkNotGeometry(t *testing.T) {
	cal, events, tied := blockProjectionFixture()
	viewer := BlockViewer{UserID: "u-player", Role: permissions.RolePlayer, HostEntity: "ent-1"}

	viewer.TieMode = "tied"
	tiedView := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: viewer, MonthIndex: 0, Year: 1523, TiedEventIDs: tied})
	viewer.TieMode = "whole"
	wholeView := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: viewer, MonthIndex: 0, Year: 1523, TiedEventIDs: tied})

	if tiedView.Viewer.TiedCount != wholeView.Viewer.TiedCount ||
		tiedView.Viewer.WholeCount != wholeView.Viewer.WholeCount {
		t.Fatal("the counts moved when the toggle flipped; both are always shown, so both " +
			"must be mode-independent")
	}
	if len(tiedView.Month.Rows) != len(wholeView.Month.Rows) {
		t.Fatal("the row count changed when the toggle flipped")
	}
	for r := range tiedView.Month.Rows {
		a, b := tiedView.Month.Rows[r].Cells, wholeView.Month.Rows[r].Cells
		if len(a) != len(b) {
			t.Fatalf("row %d cell count changed: %d vs %d", r, len(a), len(b))
		}
		for c := range a {
			if a[c].Day != b[c].Day || a[c].Col != b[c].Col || a[c].Tied != b[c].Tied {
				t.Fatalf("row %d col %d: a cell's identity changed with the toggle", r, c)
			}
		}
	}
	// INVERTED at pin r51. This block used to assert that tied mode drew ZERO
	// marks on the untied day 12 — it pinned the defect as correct behaviour,
	// reconciling it with the no-motion law above by reading "cell" structurally:
	// the cell element survived, so nothing "left the DOM".
	//
	// That reading holds in isolation and fails in composition. A cell with one
	// mark and a cell with none do not render at the same height, and CSS cannot
	// restore a mark the server never sent — which made the CSS-only tie toggle
	// unimplementable. Membership is not a function of TieMode; ink is.
	dayMarks := func(d calblock.BlockData, day int) []calblock.Mark {
		for _, row := range d.Month.Rows {
			for _, cell := range row.Cells {
				if cell.Day == day {
					return cell.Marks
				}
			}
		}
		return nil
	}

	// Every day draws the same marks in both modes. This is the whole ruling.
	for _, row := range tiedView.Month.Rows {
		for _, cell := range row.Cells {
			if cell.Day == 0 {
				continue
			}
			a, b := dayMarks(tiedView, cell.Day), dayMarks(wholeView, cell.Day)
			if len(a) != len(b) {
				t.Fatalf("day %d: tied mode drew %d marks, whole mode %d — TieMode must "+
					"change ink, never membership, or the toggle cannot be CSS-only and the "+
					"cell changes height when it flips", cell.Day, len(a), len(b))
			}
			for i := range a {
				if a[i].EventID != b[i].EventID {
					t.Fatalf("day %d mark %d: %q vs %q — mark ORDER must also be "+
						"mode-independent", cell.Day, i, a[i].EventID, b[i].EventID)
				}
			}
		}
	}

	// And the flag carries the distinction the drop used to carry.
	untied := dayMarks(tiedView, 12)
	if len(untied) != 1 {
		t.Fatalf("day 12 drew %d marks, want 1 — the fixture's untied event must still "+
			"reach the cell", len(untied))
	}
	if untied[0].Tied {
		t.Error("day 12's event is not tied to the host entity, but Mark.Tied is true — " +
			"the renderer would ink it as tied")
	}
	tiedDay := dayMarks(tiedView, 3)
	if len(tiedDay) != 1 || !tiedDay[0].Tied {
		t.Errorf("day 3 carries the tied event; got %d marks, Tied=%v", len(tiedDay),
			len(tiedDay) == 1 && tiedDay[0].Tied)
	}
}

func TestBlockTieModeFallsBackToWholeOffAnEntityPage(t *testing.T) {
	cal, events, _ := blockProjectionFixture()
	got := projectBlock(BlockProjectionInput{
		Calendar: cal, Events: blockCopyEvents(events),
		Viewer:     BlockViewer{UserID: "u", Role: permissions.RoleOwner, TieMode: "tied"},
		MonthIndex: 0, Year: 1523,
	})
	if got.Viewer.TieMode != "whole" {
		t.Fatalf("TieMode = %q off an entity page, want whole — there is nothing to tie to",
			got.Viewer.TieMode)
	}
	if got.Viewer.TiedCount != 0 {
		t.Fatalf("TiedCount = %d off an entity page, want 0", got.Viewer.TiedCount)
	}
}

// TestBlockTiesIncludeTheDirectEntityColumn pins that BOTH tie primitives count:
// the event's own entity_id column and the entity_event_links table.
func TestBlockTiesIncludeTheDirectEntityColumn(t *testing.T) {
	cal := blockTenDayCal()
	host := "ent-1"
	e := blockEvent("direct", 4, "everyone")
	e.EntityID = &host
	got := projectBlock(BlockProjectionInput{
		Calendar: cal, Events: []Event{e},
		Viewer:     BlockViewer{UserID: "u", Role: permissions.RoleOwner, HostEntity: host, TieMode: "tied"},
		MonthIndex: 0, Year: 1523,
	})
	if got.Viewer.TiedCount != 1 {
		t.Fatalf("TiedCount = %d; an event whose entity_id IS the host is tied", got.Viewer.TiedCount)
	}
}

// --- the wave-1 rulings -----------------------------------------------------

// TestBlockFoggedIsAlwaysFalse pins ruling COMMON §6.1: there is no queryable
// knowledge horizon on main, so no producer may pretend there is.
func TestBlockFoggedIsAlwaysFalse(t *testing.T) {
	cal, events, _ := blockProjectionFixture()
	got := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})
	for _, row := range got.Month.Rows {
		for _, cell := range row.Cells {
			if cell.Fogged {
				t.Fatal("a cell reported Fogged; wave 1 has no fog backend and must render " +
					"the `needs backend` chip instead of inventing a horizon")
			}
		}
	}
}

// TestBlockAudienceMarkUsesOnlyWhatExists pins ruling COMMON §6.2: composed
// tag+member audiences do not exist on main (no member_tags table; the shipped
// people primitive is campaign_groups), so the gold marks are populated ONLY
// from visibility == dm_only or a visibility_rules restriction.
//
// EXTENDED at P5 §3.3 to pin the DISCRIMINATOR, not just the labels. This test
// originally checked Label alone, which is why the producer could set
// Restricted true on both branches — drawing the diamond on every dm_only day
// and the dogear nowhere — without a single test going red. The ruling
// (data.go, AudienceMark): Restricted == false → the gold DOGEAR (dm_only);
// Restricted == true → the gold DIAMOND (a visibility_rules restriction).
func TestBlockAudienceMarkUsesOnlyWhatExists(t *testing.T) {
	cal := blockTenDayCal()
	restricted := blockEvent("restricted", 6, "everyone")
	restricted.VisibilityRules = blockStrPtr(`{"allowed_users":["u-gm","u-player"]}`)
	events := []Event{
		blockEvent("plain", 2, "everyone"),
		blockEvent("gmonly", 4, "dm_only"),
		restricted,
	}

	gm := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})
	labels := map[string]string{}
	restrictedFlag := map[string]bool{}
	for _, row := range gm.Month.Rows {
		for _, cell := range row.Cells {
			for _, m := range cell.Marks {
				if m.Audience != nil {
					labels[m.Title] = m.Audience.Label
					restrictedFlag[m.Title] = m.Audience.Restricted
				} else {
					labels[m.Title] = ""
				}
			}
		}
	}
	if labels["Event plain"] != "" {
		t.Fatalf("an unrestricted event got audience %q", labels["Event plain"])
	}
	if labels["Event gmonly"] != "GM only" {
		t.Fatalf("dm_only audience = %q, want \"GM only\"", labels["Event gmonly"])
	}
	if labels["Event restricted"] != "Restricted" {
		t.Fatalf("visibility_rules audience = %q, want \"Restricted\"", labels["Event restricted"])
	}
	if restrictedFlag["Event gmonly"] {
		t.Fatal("dm_only set Restricted = true — the renderer draws the restricted-audience " +
			"diamond and the dogear renders nowhere in the product")
	}
	if !restrictedFlag["Event restricted"] {
		t.Fatal("a visibility_rules restriction set Restricted = false — the diamond is its mark")
	}

	// A player who CAN see the restricted event sees an ordinary mark: no
	// diamond, no hint that anyone else cannot. Permission is absence.
	player := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: BlockViewer{UserID: "u-player", Role: permissions.RolePlayer}, MonthIndex: 0, Year: 1523})
	for _, row := range player.Month.Rows {
		for _, cell := range row.Cells {
			for _, m := range cell.Marks {
				if m.Audience != nil {
					t.Fatalf("a player received an audience mark on %q", m.Title)
				}
			}
		}
	}
}

// TestBlockRealTimeCalendarPausesTheSyncPill pins that a calendar computing its
// date from the wall clock never claims a date push.
func TestBlockRealTimeCalendarPausesTheSyncPill(t *testing.T) {
	cal := blockRealTimeCal()
	in := calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4,
		Full: "In sync · Foundry · pushed 2m ago · 1 of 4 linked", Compact: "In sync · 1 of 4"}
	got := projectBlock(BlockProjectionInput{Calendar: cal, Events: nil,
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 1, Year: 2028, Sync: in})
	if got.Sync.State != blockSyncStatePause {
		t.Fatalf("real-time calendar sync state = %q, want pause", got.Sync.State)
	}
	if got.Sync.Full != "Paused · date push paused — tracks real time" {
		t.Fatalf("pause Full = %q", got.Sync.Full)
	}
	if got.Sync.Compact != "Paused · tracks real time" {
		t.Fatalf("pause Compact = %q", got.Sync.Compact)
	}
	// A fantasy calendar leaves the campaign-level pill untouched.
	fantasy := projectBlock(BlockProjectionInput{Calendar: blockTenDayCal(), Events: nil,
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523, Sync: in})
	if fantasy.Sync.Full != in.Full {
		t.Fatalf("fantasy calendar rewrote the pill: %q", fantasy.Sync.Full)
	}
}

// TestBlockLayersAreDefOnlyInWave1 pins ruling on LayerState: the default
// surface is a month with its moon phases and nothing else, and — from wave 3 —
// the viewer can leave it.
//
// THE HasSwitchboard HALF IS INVERTED HERE, DELIBERATELY, AND NOT DELETED
// (C-CALV4-LAYERS-P9, W-F). It is a wave-3 EDIT rather than a break, and it
// follows the precedent this same file already set twice for the two
// NeedsBackend halves below: the slice that turns an honesty flag over names
// itself and states the new claim.
//
// The old claim was "HasSwitchboard must be false in wave 1 — the per-viewer
// store is W-F's", and it was worth having: a producer that raised the flag
// with no store would have rendered a switchboard whose every toggle vanished
// on the next page load. W-F built the store (migration 014, POST
// /campaigns/:id/calendar/prefs), so the flag now tracks whether THIS VIEWER
// has somewhere to persist to — which is a fact about the request, not about
// the wave.
//
// THE NEW CLAIM, in three parts, and the inverted statement is worth exactly as
// much as the original:
//
//  1. DEF IS STILL ["moons"] for a viewer with no stored row. Wave 3 shipped
//     the mechanism to LEAVE the default, which is the opposite of changing it.
//  2. No store reachable (the zero blockLayerPrefs — an anonymous viewer, or a
//     Block composed outside a campaign) → HasSwitchboard false and PersistURL
//     empty, exactly the wave-1/2 surface.
//  3. A store reachable → HasSwitchboard TRUE, always together with PersistURL,
//     because r54 pins them as one fact.
func TestBlockLayersAreDefOnlyInWave1(t *testing.T) {
	got := projectBlock(BlockProjectionInput{Calendar: blockTenDayCal(),
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})
	if len(got.Layers.Enabled) != 1 || got.Layers.Enabled[0] != "moons" {
		t.Fatalf("layers = %v, want DEF = [moons]", got.Layers.Enabled)
	}
	if got.Layers.HasSwitchboard || got.Layers.PersistURL != "" {
		t.Fatalf("with nowhere to persist the switchboard must stay off; got "+
			"HasSwitchboard=%v PersistURL=%q", got.Layers.HasSwitchboard, got.Layers.PersistURL)
	}

	// THE INVERSION. Same projection, same DEF, and now a live switchboard.
	live := projectBlock(BlockProjectionInput{Calendar: blockTenDayCal(),
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523,
		LayerPrefs: blockLayerPrefs{PersistURL: blockPrefsPath("camp-1")}})
	if len(live.Layers.Enabled) != 1 || live.Layers.Enabled[0] != "moons" {
		t.Fatalf("DEF changed under a live store: %v — wave 3 ships the way OUT of the "+
			"default, not a different default", live.Layers.Enabled)
	}
	if !live.Layers.HasSwitchboard {
		t.Fatal("HasSwitchboard must be TRUE once the viewer has somewhere to persist to — " +
			"W-F built the store this flag was waiting for")
	}
	if live.Layers.PersistURL != "/campaigns/camp-1/calendar/prefs" {
		t.Fatalf("PersistURL = %q; the producer builds it because the widget has no router",
			live.Layers.PersistURL)
	}

	// THE VIEWER'S OWN SET WINS OVER DEF, and an EMPTY stored set is a real
	// choice — a bare month — not a fall back to the seed.
	chosen := projectBlock(BlockProjectionInput{Calendar: blockTenDayCal(),
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523,
		LayerPrefs: blockLayerPrefs{Stored: []string{}, PersistURL: blockPrefsPath("camp-1")}})
	if len(chosen.Layers.Enabled) != 0 {
		t.Errorf("a viewer who turned everything off got %v — NULL means 'never chosen' and "+
			"'' means 'chose nothing'; collapsing them makes the bare month unreachable",
			chosen.Layers.Enabled)
	}
	// INVERTED IN TWO STEPS, NEVER DELETED, and both halves are wave-2 EDITS
	// rather than breaks. Each half asserted the wave-1 honesty state — the
	// zone docks at its real size and says it has no backend — and each
	// filling slice turned its own half over deliberately:
	//
	//   · the Ledger half, by C-CALV4-LEDGER-P6 (W-B), which filled zone C;
	//   · the Shelf half, by C-CALV4-SHELF-P7 (W-E), which filled zone D with
	//     Upcoming, Filters and the Almanac.
	//
	// The inverted statements are worth exactly as much as the originals: a
	// producer that left either flag up would put "needs backend" beside a
	// list of real events, which is the same lie one direction over.
	//
	// THE FLAGS THEMSELVES ARE NOT RETIRED. They are the host's honesty
	// switch, so a host that docks a zone it cannot fill still gets the chip.
	if got.Ledger.NeedsBackend {
		t.Fatal("the Ledger is FILLED from wave 2 (W-B); its `needs backend` flag must be down")
	}
	if got.Shelf.NeedsBackend {
		t.Fatal("the Shelf is FILLED from wave 2 (W-E); its `needs backend` flag must be down")
	}
}

// --- the per-cell ceiling ---------------------------------------------------

// TestBlockMoreCountFollowsTheSignedCap pins `cap = inline.length > 3 ? 2 : 3`.
// Three chips, OR two chips plus "+n more" — never both, so a named cell is
// always exactly one height.
func TestBlockMoreCountFollowsTheSignedCap(t *testing.T) {
	cases := []struct{ marks, wantMore int }{
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 2}, {5, 3}, {9, 7},
	}
	for _, c := range cases {
		in := make([]calblock.Mark, c.marks)
		got, more := blockCapMarks(in)
		if more != c.wantMore {
			t.Fatalf("%d marks → MoreCount %d, want %d", c.marks, more, c.wantMore)
		}
		if len(got) != c.marks {
			t.Fatalf("%d marks → %d returned; the FULL list is kept so the \"+n more\" "+
				"popover has something to list", c.marks, len(got))
		}
		shown := len(got) - more
		if c.marks > 3 && shown != blockCellMarkCapOverflow {
			t.Fatalf("%d marks → %d shown, want %d", c.marks, shown, blockCellMarkCapOverflow)
		}
		if c.marks <= 3 && shown != c.marks {
			t.Fatalf("%d marks → %d shown, want all of them", c.marks, shown)
		}
	}
}

// --- identity ---------------------------------------------------------------

// TestBlockIdentityIsStableAndGreyscale pins that the identity channels do not
// shuffle between renders and that the pattern (the GREYSCALE channel) is
// always populated — colour is never load-bearing alone.
func TestBlockIdentityIsStableAndGreyscale(t *testing.T) {
	def := blockTenDayCal() // IsDefault
	real := blockRealTimeCal()
	other := blockTenDayCal()
	other.ID = "cal-elven"
	other.Name = "Elven Reckoning"
	other.IsDefault = false

	if blockCalHue(def) != "harptos" || blockCalPattern(def) != "p1" {
		t.Fatalf("default calendar identity = %s/%s, want the signed harptos/p1",
			blockCalHue(def), blockCalPattern(def))
	}
	if blockCalHue(real) != "real" || blockCalPattern(real) != "p2" {
		t.Fatalf("real-world identity = %s/%s, want the signed real/p2",
			blockCalHue(real), blockCalPattern(real))
	}
	if blockCalLetter(def) != "H" || blockCalLetter(real) != "R" || blockCalLetter(other) != "E" {
		t.Fatalf("letters = %s/%s/%s, want H/R/E",
			blockCalLetter(def), blockCalLetter(real), blockCalLetter(other))
	}
	// Stability: the same calendar always resolves to the same channels. A
	// re-derived struct stands in for "a second render on a second server" —
	// the hash is over the UUID, so nothing else about the value can matter.
	hue, pattern := blockCalHue(other), blockCalPattern(other)
	for i := 0; i < 5; i++ {
		again := &Calendar{ID: other.ID, Name: "renamed since", Mode: ModeFantasy}
		if blockCalHue(again) != hue || blockCalPattern(again) != pattern {
			t.Fatalf("identity channels shifted on re-derivation: %s/%s vs %s/%s",
				blockCalHue(again), blockCalPattern(again), hue, pattern)
		}
	}
	if hue != "harptos" && hue != "elven" && hue != "dwarven" {
		t.Fatalf("hue token %q is outside the closed set the signed stylesheet defines", hue)
	}
	if !strings.HasPrefix(pattern, "p") || pattern == "p1" || pattern == "p2" {
		t.Fatalf("non-default pattern %q must sit in p3..p8", pattern)
	}
	// A mark's pattern is locked to its hue key, so a viewer who cannot separate
	// the hues can still separate the marks.
	social := blockPatternFor("social")
	if social == "" {
		t.Fatal("every mark must carry a greyscale pattern")
	}
	if blockPatternFor("soc"+"ial") != social {
		t.Fatal("a mark's pattern is not locked to its key")
	}
}

// TestBlockIdentityFieldsCarryRealIDs is the INVERSION of the old
// TestBlockIdentityIntFieldsAreZeroed tripwire.
//
// The four identity fields were once typed int64 while every one of those
// identities is a VARCHAR(36) UUID, so the producer zeroed them and smuggled the
// calendar's id through CalendarSlug. The old test pinned that workaround so the
// struct amendment could not land silently — it did its job, the pin was amended
// (r50), and this now pins the opposite: the real ids are carried, and nothing
// is zeroed or hashed.
//
// Deleting it rather than inverting it would have removed the only guard on
// these four fields.
func TestBlockIdentityFieldsCarryRealIDs(t *testing.T) {
	cal, events, tied := blockProjectionFixture()
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner, HostEntity: "ent-1"}
	got := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: viewer, MonthIndex: 0, Year: 1523, TiedEventIDs: tied})

	if got.CalendarID != cal.ID {
		t.Errorf("CalendarID = %q, want the calendar's UUID %q", got.CalendarID, cal.ID)
	}
	if got.Viewer.UserID != viewer.UserID {
		t.Errorf("Viewer.UserID = %q, want %q", got.Viewer.UserID, viewer.UserID)
	}
	if got.Viewer.HostEntity != viewer.HostEntity {
		t.Errorf("Viewer.HostEntity = %q, want %q — the tie toggle is gated on this being "+
			"non-empty, so zeroing it is what kept the control off the page",
			got.Viewer.HostEntity, viewer.HostEntity)
	}
	// CalendarSlug goes back to meaning the slug now that CalendarID exists.
	if got.CalendarSlug == "" {
		t.Error("CalendarSlug is empty")
	}

	marks := 0
	for _, row := range got.Month.Rows {
		for _, cell := range row.Cells {
			for _, m := range cell.Marks {
				marks++
				if m.EventID == "" {
					t.Fatalf("a Mark on day %d has no EventID; the renderer keys "+
						"data-event-id off it", cell.Day)
				}
			}
		}
	}
	if marks == 0 {
		t.Fatal("fixture produced no marks — this test would pass vacuously")
	}
}

// --- the date line ----------------------------------------------------------

func TestBlockDateLabelForms(t *testing.T) {
	// In-world, with an epoch: the signed "Sithrel 9, Cycle 218" shape.
	cal := blockTenDayCal()
	cal.Months[0].Name = "Sithrel"
	cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 218, 1, 9
	cal.EpochName = blockStrPtr("Cycle")
	if got, fault := blockDateLine(cal); got != "Sithrel 9, Cycle 218" || fault != "" {
		t.Fatalf("date line = %q fault %q, want \"Sithrel 9, Cycle 218\"", got, fault)
	}

	// No epoch, but a declared yearly cycle names the reckoning.
	cal.EpochName = nil
	cal.Cycles = []Cycle{{ID: 1, Name: "Zodiac", CycleLength: 2, Entries: []CycleEntry{
		{Name: "Wolf", YearOffset: 0}, {Name: "Stag", YearOffset: 1},
	}}}
	if got, _ := blockDateLine(cal); got != "Sithrel 9, Wolf 218" {
		t.Fatalf("cycle-named date line = %q", got)
	}

	// Neither: just the year.
	cal.Cycles = nil
	if got, _ := blockDateLine(cal); got != "Sithrel 9, 218" {
		t.Fatalf("bare date line = %q", got)
	}

	// Real world: the signed "Sat 25 Jul 2026" shape, built from the calendar's
	// OWN month and weekday names so a renamed month still reads correctly.
	rt := blockRealTimeCal()
	rt.CurrentYear, rt.CurrentMonth, rt.CurrentDay = 2026, 7, 25
	got, fault := blockDateLine(rt)
	if fault != "" {
		t.Fatalf("real-world date faulted: %q", fault)
	}
	if !strings.HasSuffix(got, "25 Jul 2026") {
		t.Fatalf("real-world date line = %q, want it to end \"25 Jul 2026\"", got)
	}
}

// TestBlockFaultPrintsWhereTheDateWouldGo pins the honesty state: when the date
// cannot resolve the Block emits a FAULT and NO date — not a zero, not a
// placeholder, not an em dash.
func TestBlockFaultPrintsWhereTheDateWouldGo(t *testing.T) {
	monthless := &Calendar{ID: "c", Name: "Half-built", CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1}
	label, fault := blockDateLine(monthless)
	if label != "" {
		t.Fatalf("a faulted calendar emitted a date label %q", label)
	}
	if !strings.Contains(fault, "0 months defined") {
		t.Fatalf("fault = %q, want it to name the missing months", fault)
	}

	badMonth := blockTenDayCal()
	badMonth.CurrentMonth = 99
	if _, f := blockDateLine(badMonth); !strings.Contains(f, "month 99 of 12") {
		t.Fatalf("out-of-range month fault = %q", f)
	}

	badDay := blockTenDayCal()
	badDay.CurrentDay = 44
	if _, f := blockDateLine(badDay); !strings.Contains(f, "day 44 of 30") {
		t.Fatalf("out-of-range day fault = %q", f)
	}

	// And the fault reaches BlockData rather than being swallowed.
	got := projectBlock(BlockProjectionInput{Calendar: monthless,
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1})
	if got.Fault == "" || got.DateLabel != "" {
		t.Fatalf("BlockData carried fault %q with date %q", got.Fault, got.DateLabel)
	}
}

func TestBlockSeasonAndEraLabelsAreAbsentNotFabricated(t *testing.T) {
	cal := blockTenDayCal()
	if s, e := blockSeasonEraLabels(cal); s != "" || e != "" {
		t.Fatalf("a calendar with no seasons or eras produced %q / %q", s, e)
	}
	cal.Seasons = []Season{{Name: "Long Night", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 28}}
	cal.Eras = []Era{{Name: "Reckoning of Wards", StartYear: 1, EndYear: nil}}
	s, e := blockSeasonEraLabels(cal)
	if s != "Long Night" || e != "Reckoning of Wards" {
		t.Fatalf("labels = %q / %q", s, e)
	}
}

func TestBlockViewerZoneFallsBackToTheCalendarAnchor(t *testing.T) {
	rt := blockRealTimeCal()
	if got := blockViewerZone(rt, BlockViewer{Zone: "Europe/Berlin"}); got != "Europe/Berlin" {
		t.Fatalf("viewer zone = %q", got)
	}
	if got := blockViewerZone(rt, BlockViewer{}); got != "UTC" {
		t.Fatalf("zone fallback = %q, want the calendar's anchor zone", got)
	}
	if got := blockViewerZone(blockTenDayCal(), BlockViewer{}); got != "" {
		t.Fatalf("a fantasy calendar with no viewer zone produced %q", got)
	}
}

// --- intercalary marks ------------------------------------------------------

// TestBlockIntercalaryRowsCarryMarks pins that events on an intercalary day are
// drawn: the day belongs to a real month, it just has no cell in the tenday
// grid. The candidate set is concatenated BEFORE the viewer filter, so those
// marks come from the same single pass as the grid's.
func TestBlockIntercalaryRowsCarryMarks(t *testing.T) {
	cal := blockTenDayCal()
	cal.Months = append(cal.Months[:1], append([]Month{{
		ID: 99, CalendarID: cal.ID, Name: "Midwinter", Days: 1, SortOrder: 1, IsIntercalary: true,
	}}, cal.Months[1:]...)...)

	vigil := Event{ID: "vigil", CalendarID: cal.ID, Name: "Midwinter Vigil",
		Year: 1523, Month: 2, Day: 1, Visibility: "everyone"} // month 2 = Midwinter
	hidden := Event{ID: "hidden", CalendarID: cal.ID, Name: "Secret rite",
		Year: 1523, Month: 2, Day: 1, Visibility: "dm_only"}

	player := projectBlock(BlockProjectionInput{Calendar: cal,
		Events:     []Event{blockEvent("grid", 3, "everyone"), vigil, hidden},
		Viewer:     BlockViewer{UserID: "u", Role: permissions.RolePlayer},
		MonthIndex: 0, Year: 1523})

	if len(player.Month.Intercalary) != 1 {
		t.Fatalf("intercalary rows = %d, want 1", len(player.Month.Intercalary))
	}
	marks := player.Month.Intercalary[0].Marks
	if len(marks) != 1 || marks[0].Title != "Midwinter Vigil" {
		t.Fatalf("intercalary marks = %+v, want just the visible vigil", marks)
	}
	// Both the grid event and the intercalary event count, and the dm_only one
	// counts for neither.
	if player.Viewer.WholeCount != 2 {
		t.Fatalf("WholeCount = %d, want 2 (grid + intercalary, hidden excluded)",
			player.Viewer.WholeCount)
	}
}

// TestBlockRecurringEventsAreExpandedOnce pins that a weekly event marks every
// day it lands on but counts ONCE — the counts are of distinct events, which is
// what the signed toggle labels ("Tied 4 / Whole calendar 9") mean.
func TestBlockRecurringEventsAreExpandedOnce(t *testing.T) {
	cal := blockTenDayCal()
	rt := RecurrenceWeekly
	weekly := blockEvent("weekly", 2, "everyone")
	weekly.IsRecurring = true
	weekly.RecurrenceType = &rt

	got := projectBlock(BlockProjectionInput{Calendar: cal, Events: []Event{weekly},
		Viewer: BlockViewer{Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})

	days := map[int]bool{}
	for _, row := range got.Month.Rows {
		for _, cell := range row.Cells {
			if len(cell.Marks) > 0 {
				days[cell.Day] = true
			}
		}
	}
	// A ten-day week: days 2, 12, 22 in a 30-day month.
	if len(days) != 3 || !days[2] || !days[12] || !days[22] {
		t.Fatalf("weekly event rendered on %v, want days 2/12/22 of a ten-day week", days)
	}
	if got.Viewer.WholeCount != 1 {
		t.Fatalf("WholeCount = %d; three occurrences of one event are one event",
			got.Viewer.WholeCount)
	}
}

// --- r52: the three fields the Ledger row cannot derive ---------------------

// TestBlockR52FieldsAreProducedNotDerived pins the producer half of pin
// amendment r52 (decisions/2026-07-28-calv4-ledger-p6-pin-amendment.md).
//
// All three fields exist because the WIDGET cannot compute them: it has no
// clock, no calendar hour/minute geometry, no zone rules, and no hue→type-name
// mapping. So the only place their absence can be caught is here, on the
// producer's own output — a widget-side test would assert against a BlockData
// the test author wrote, which is the seam blindness block_seam_test.go's
// header describes.
//
// The three acceptance lines of the amendment's §8, in order.
func TestBlockR52FieldsAreProducedNotDerived(t *testing.T) {
	cal := blockTenDayCal()
	cal.EventCategories = []EventCategory{
		{ID: 1, CalendarID: cal.ID, Slug: "quest", Name: "Quest", Icon: "▲", Color: "#a855f7"},
	}
	quest := "quest"
	orphan := "no-such-category"

	timed := blockEvent("timed", 3, "everyone")
	timed.Category = &quest
	timed.StartHour, timed.StartMinute = blockIntPtr(18), blockIntPtr(30)

	untimed := blockEvent("untimed", 4, "everyone")
	untimed.Category = &quest

	typeless := blockEvent("typeless", 5, "everyone")

	// A category slug the calendar does not declare. The widget must not invent
	// a display name out of an identifier — the same refusal block.templ makes
	// for the legend layer.
	unknownType := blockEvent("unknown-type", 6, "everyone")
	unknownType.Category = &orphan

	find := func(d calblock.BlockData, id string) calblock.Mark {
		t.Helper()
		for _, row := range d.Month.Rows {
			for _, c := range row.Cells {
				for _, m := range c.Marks {
					if m.EventID == id {
						return m
					}
				}
			}
		}
		t.Fatalf("no mark for event %q reached the projection", id)
		return calblock.Mark{}
	}

	// ── in-world calendar: a bare time, NEVER a zone label (L15) ─────────────
	inWorld := projectBlock(BlockProjectionInput{
		Calendar: cal,
		Events:   []Event{timed, untimed, typeless, unknownType},
		Viewer:   BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner, Zone: "America/Chicago"},
		MonthIndex: 0, Year: 1523,
	})
	if got := find(inWorld, "timed").Time; got != "18:30" {
		t.Errorf("in-world Mark.Time = %q, want %q — an in-world calendar has no zone to label", got, "18:30")
	}
	if got := find(inWorld, "untimed").Time; got != "" {
		t.Errorf("an untimed event must carry Time %q so the row DROPS the .tm segment; got %q", "", got)
	}
	if got := find(inWorld, "timed").AxisLabel; got != "Quest" {
		t.Errorf("Mark.AxisLabel = %q, want the declared category's display NAME %q", got, "Quest")
	}
	if got := find(inWorld, "typeless").AxisLabel; got != "" {
		t.Errorf("a typeless event must carry AxisLabel %q so the meta line drops the segment; got %q", "", got)
	}
	if got := find(inWorld, "unknown-type").AxisLabel; got != "" {
		t.Errorf("an undeclared category slug must yield %q, not the slug itself; got %q", "", got)
	}

	// ── real-world calendar: the zone label is folded INTO the string ────────
	real := blockRealTimeCal()
	realTimed := Event{ID: "rt", CalendarID: real.ID, Name: "Session 41",
		Year: 2028, Month: 2, Day: 29, Visibility: "everyone",
		StartHour: blockIntPtr(19), StartMinute: blockIntPtr(0)}
	rw := projectBlock(BlockProjectionInput{
		Calendar: real,
		Events:   []Event{realTimed},
		Viewer:   BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner, Zone: "America/Chicago"},
		MonthIndex: 1, Year: 2028,
	})
	got := find(rw, "rt").Time
	if !strings.HasPrefix(got, "19:00 ") || len(got) <= len("19:00 ") {
		t.Errorf("real-world Mark.Time = %q, want %q plus the zone abbreviation L15 requires", got, "19:00")
	}

	// ── HiddenCount: GM-only BY CONSTRUCTION (r52 §3.3 rules 1 and 3) ────────
	hiddenFx := []Event{
		blockEvent("open-1", 3, "everyone"),
		blockEvent("dm-1", 5, "dm_only"),
		blockEvent("dm-2", 7, "dm_only"),
	}
	gm := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(hiddenFx),
		Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})
	if gm.Viewer.HiddenCount != 2 {
		t.Errorf("GM HiddenCount = %d, want 2 — the two dm_only events on this month", gm.Viewer.HiddenCount)
	}
	player := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(hiddenFx),
		Viewer: BlockViewer{UserID: "u-player", Role: permissions.RolePlayer}, MonthIndex: 0, Year: 1523})
	if player.Viewer.HiddenCount != 0 {
		t.Errorf("player HiddenCount = %d, want 0 AT THE PRODUCER — permission is absence, and "+
			"a renderer-side gate is a rule one template edit can lose", player.Viewer.HiddenCount)
	}

	// A recurring dm_only event is ONE hidden event however many days it marks.
	rt := RecurrenceWeekly
	rec := blockEvent("dm-weekly", 2, "dm_only")
	rec.IsRecurring, rec.RecurrenceType = true, &rt
	recurring := projectBlock(BlockProjectionInput{Calendar: cal, Events: []Event{rec},
		Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})
	if recurring.Viewer.HiddenCount != 1 {
		t.Errorf("HiddenCount = %d for one recurring dm_only event on three days; the chip counts "+
			"DISTINCT events, not marks", recurring.Viewer.HiddenCount)
	}
}

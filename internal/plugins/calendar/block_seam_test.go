package calendar

// block_seam_test.go — the cross-layer seam test (C-CALV4-SEAM-P5 §1).
//
// WHY THIS FILE EXISTS, AND WHY IT LIVES HERE. Phase A shipped a producer
// (block_projection.go) and a renderer (internal/widgets/calendar_block) that
// were each correct in isolation, each fully tested, each green — and composed,
// they rendered a Block that was visibly wrong five different ways. Neither
// suite could see it: the widget is plugin-agnostic by construction, so a
// widget-side test can only assert "given a BlockData I wrote, the renderer
// emits Y" — and the test author supplies the BlockData. For every defect that
// lives in the PRODUCER'S CHOICE of input, a widget-side test is not merely
// weak, it is actively misleading: green while the composed Block is broken.
//
// So the discipline (dispatch §1, preflight §F): assertions about what the
// producer chose to emit live HERE, in package calendar, where the test can
// call projectBlock on real fixture data, push the result through
// calblock.Block, and read the HTML the operator would actually see.
// Widget-side tests assert renderer behaviour on hand-written BlockData and
// nothing else.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// seamRender is the seam itself: real fixture data → projectBlock →
// calblock.Block → HTML. Everything in this file asserts on its output.
func seamRender(t *testing.T, in BlockProjectionInput) string {
	t.Helper()
	return seamRenderBlockData(t, projectBlock(in))
}

// seamRenderBlockData is seamRender's second half, split out so a caller that
// must stand in for a store the producer cannot supply — today only the
// per-viewer LAYER store, which is W-F's and does not exist — can override that
// one field and push the producer's REAL output through the same renderer.
// TestSeam_EnabledLayerSetMatchesWhatRenders already needed it inline; the
// count-oracle suite needs it too, and two hand-copied render loops is how a
// seam suite quietly forks.
func seamRenderBlockData(t *testing.T, d calblock.BlockData) string {
	t.Helper()
	var sb strings.Builder
	if err := calblock.Block(d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render composed Block: %v", err)
	}
	return sb.String()
}

// seamWS collapses whitespace so a substring pin survives a templ reformat —
// the same convention as the widget suite's flatten (COMMON §3).
var seamWS = regexp.MustCompile(`\s+`)

func seamContain(t *testing.T, body, want, why string) {
	t.Helper()
	if !strings.Contains(seamWS.ReplaceAllString(body, " "), want) {
		t.Errorf("composed HTML is missing %q — %s", want, why)
	}
}

func seamNotContain(t *testing.T, body, bad, why string) {
	t.Helper()
	if strings.Contains(seamWS.ReplaceAllString(body, " "), bad) {
		t.Errorf("composed HTML must not contain %q — %s", bad, why)
	}
}

// ── regressions: the seams stages 1–3 closed ────────────────────────────────

// TestSeam_TieToggleReachesTheDOM pins §3.1 from the composed side: with a real
// host-entity id, the toggle renders with BOTH counts and the data-tie-mode
// attribute phase B's CSS-only toggle keys off. This is the seam that was
// broken for the whole of wave 1 — the producer zeroed HostEntity (the int64
// pin could not carry a UUID), the renderer gated on it, and the count pair the
// whole oracle discipline protects never reached the DOM.
func TestSeam_TieToggleReachesTheDOM(t *testing.T) {
	cal, events, tied := blockProjectionFixture()
	body := seamRender(t, BlockProjectionInput{
		Calendar:     cal,
		Events:       blockCopyEvents(events),
		Viewer:       BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner, HostEntity: "ent-1", TieMode: "tied"},
		MonthIndex:   0,
		Year:         1523,
		TiedEventIDs: tied,
	})

	seamContain(t, body, `data-tie-mode="tied"`,
		"phase B's CSS-only toggle has nothing to flip without it")
	seamContain(t, body, `class="seg tie"`,
		"the tie toggle control is absent from an entity-hosted Block")
	// Both counts, always — the GM sees 3 tied of 4 (TestBlockCountsAreNotAnOracle
	// pins the numbers; this pins that they reach the operator's screen).
	seamContain(t, body, "Tied 3", "the tied count is computed and then dropped on the floor")
	seamContain(t, body, "Whole calendar 4", "the whole-calendar count never reaches the DOM")

	// C-CALV4-HOST-P3 §3: the toggle is now LIVE, and pure CSS. From the composed
	// side that means the radio pair the stylesheet's :has() rule reads is
	// actually in the HTML a viewer receives, with exactly one option selected —
	// and that the Block still ships no script to operate it.
	seamContain(t, body, `data-tie-pick="tied"`, "the CSS-only toggle's tied radio is absent")
	seamContain(t, body, `data-tie-pick="whole"`, "the CSS-only toggle's whole radio is absent")
	if n := strings.Count(body, " checked"); n != 1 {
		t.Errorf("exactly one tie option may be selected in the composed HTML; got %d", n)
	}
	seamNotContain(t, body, "<script",
		"the Block must ship no script — a <script> in an HTMX-swapped fragment never executes (boot.js:163)")
}

// TestSeam_CalendarIdentityTripleReachesTheDOM pins §3.2 from the composed
// side: the producer emits a hue TOKEN, and the renderer must map it through
// the allowlist to var(--cal-<token>) — plus render the pattern and the letter,
// because colour is never the only identity channel. The widget's own suite
// could not catch the original defect: its fixtures sent colour VALUES, so it
// rendered a correctly-coloured dot while every real calendar rendered grey.
func TestSeam_CalendarIdentityTripleReachesTheDOM(t *testing.T) {
	cal, events, _ := blockProjectionFixture()
	body := seamRender(t, BlockProjectionInput{
		Calendar:   cal,
		Events:     blockCopyEvents(events),
		Viewer:     BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: 0,
		Year:       1523,
	})

	seamContain(t, body, "--cal:var(--cal-harptos)",
		"the default calendar's hue token must resolve through the allowlist, not grey out")
	seamNotContain(t, body, "--cal:var(--rule-structural-strong)",
		"the identity hue fell through to the neutral structural fallback")
	seamContain(t, body, `class="dot p1"`,
		"the greyscale pattern channel — the identity a colour-blind viewer keeps")
	seamContain(t, body, `class="callet" aria-hidden="true">H<`,
		"the calendar letter — the third identity channel")
}

// ── §3.3: dm_only is the DOGEAR, a visibility_rules restriction the DIAMOND ──

// TestSeam_GMMarksDiscriminateDogearFromDiamond pins the AudienceMark
// discriminator ruling (data.go: Restricted == false → dogear, Restricted ==
// true → diamond) at the only level that can see both halves at once. The
// renderer split on !Restricted correctly from day one; the producer set
// Restricted true on BOTH branches, so every dm_only day drew the
// restricted-audience diamond and the gold notch rendered nowhere in the
// product — while both suites stayed green, because the widget's fixtures
// hand-wrote the flag correctly.
//
// Each condition is projected alone so the mark cannot be attributed to the
// wrong event.
// EXTENDED BY C-CALV4-LEDGER-P6 §7. The docked Ledger draws the SAME
// discrimination one layer down — a gold rail plus a `GM` badge for dm_only, an
// audience chip alone for a visibility_rules restriction — so a Ledger that
// drew the rail on both would be the identical defect, and it would be invisible
// to any test that only reads the grid. seamLedger renders the same projection
// with the Ledger layer on, because the producer has no layer input to vary
// (the per-viewer layer store is W-F's).
func TestSeam_GMMarksDiscriminateDogearFromDiamond(t *testing.T) {
	gm := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	// A dm_only event, viewed as GM: the gold notch, and ONLY the notch.
	dmOnly := seamRender(t, BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Events:     []Event{blockEvent("gmonly", 4, "dm_only")},
		Viewer:     gm,
		MonthIndex: 0,
		Year:       1523,
	})
	seamContain(t, dmOnly, `class="dogear" title="GM only"`,
		"a dm_only day must draw the hidden-event notch (Restricted == false)")
	seamNotContain(t, dmOnly, `class="audmark"`,
		"a dm_only day is not a restricted-audience day; the diamond is the wrong mark")

	// A visibility_rules restriction, viewed as GM: the diamond, and ONLY it.
	restricted := blockEvent("restricted", 6, "everyone")
	restricted.VisibilityRules = blockStrPtr(`{"allowed_users":["u-gm","u-player"]}`)
	rules := seamRender(t, BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Events:     []Event{restricted},
		Viewer:     gm,
		MonthIndex: 0,
		Year:       1523,
	})
	seamContain(t, rules, `class="audmark" title="Restricted"`,
		"a visibility_rules day must draw the restricted-audience diamond (Restricted == true)")
	seamNotContain(t, rules, `class="dogear"`,
		"a visibility_rules day is not a dm_only day; the notch is the wrong mark")

	// A player sees neither — permission is ABSENCE. The player CAN see the
	// restricted event (they are in allowed_users), and it renders as an
	// ordinary mark with no hint that anyone else cannot see it.
	restrictedAgain := restricted // OccursOn is pure; the struct is reusable
	player := seamRender(t, BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Events:     []Event{blockEvent("gmonly", 4, "dm_only"), restrictedAgain},
		Viewer:     BlockViewer{UserID: "u-player", Role: permissions.RolePlayer},
		MonthIndex: 0,
		Year:       1523,
	})
	seamNotContain(t, player, `class="dogear"`, "a player must see no trace of a hidden event")
	seamNotContain(t, player, `class="audmark"`, "a player must see no audience marks at all")
	seamContain(t, player, `data-event-id="restricted"`,
		"the player is allowed this event; it must render as an ordinary mark")

	// ── the SAME split, in the docked Ledger (§7) ───────────────────────────
	dmLedger := seamLedger(t, BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Events:     []Event{blockEvent("gmonly", 4, "dm_only")},
		Viewer:     gm,
		MonthIndex: 0,
		Year:       1523,
	})
	seamContain(t, dmLedger, `<i class="gr" title="hidden from players"`,
		"a dm_only event's Ledger row must draw the gold GM rail (Restricted == false)")
	seamContain(t, dmLedger, `class="badge gm">GM<`,
		"the rail and the `GM` badge are one condition, not two")
	seamContain(t, dmLedger, `class="audchip">GM only<`,
		"the audience chip says WHICH permission it is")

	restrictedLedger := restricted // OccursOn is pure; the struct is reusable
	rulesLedger := seamLedger(t, BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Events:     []Event{restrictedLedger},
		Viewer:     gm,
		MonthIndex: 0,
		Year:       1523,
	})
	seamNotContain(t, rulesLedger, `class="gr"`,
		"a visibility_rules row is not a dm_only row: the gold rail is the wrong mark, and "+
			"drawing it on both is the SEAM-P5 stage-4 defect one layer down")
	seamNotContain(t, rulesLedger, `class="badge gm">GM<`,
		"the `GM` badge follows the rail, so it is wrong here too")
	seamContain(t, rulesLedger, `class="audchip">Restricted<`,
		"a restricted audience is stated by the chip alone")

	// And a player receives none of it, in the Ledger as in the grid.
	playerLedger := seamLedger(t, BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Events:     []Event{blockEvent("gmonly", 4, "dm_only"), restrictedAgain},
		Viewer:     BlockViewer{UserID: "u-player", Role: permissions.RolePlayer},
		MonthIndex: 0,
		Year:       1523,
	})
	for _, mark := range []string{`class="gr"`, `class="badge gm"`, `class="audchip"`} {
		seamNotContain(t, playerLedger, mark, "a permission marker reached a player's Ledger")
	}
}

// seamLedger is seamRender with the Ledger layer switched on. The producer has
// no layer input to vary — the per-viewer layer store is W-F's (data.go) — so
// overriding Layers.Enabled on the projected BlockData stands in for exactly
// that store and nothing else; every other field is the producer's real output.
// This is the same stand-in TestSeam_EnabledLayerSetMatchesWhatRenders makes,
// and it goes through the same seamRenderBlockData rather than a second loop.
func seamLedger(t *testing.T, in BlockProjectionInput) string {
	t.Helper()
	d := projectBlock(in)
	d.Layers.Enabled = []string{"moons", "ledger"}
	return seamRenderBlockData(t, d)
}

// TestSeam_LedgerTimesFollowTheCalendarNotTheEvent is r52's own composed-HTML
// acceptance line (§8): "a real-world calendar's Ledger times carry the zone
// abbreviation and an in-world calendar's carry .tm.mono and NO zone".
//
// It can only be asserted here. The renderer picks .tm vs .tm.mono from
// BlockData.IsRealWorld, which a widget-side test sets itself; what this proves
// is that the PRODUCER folds a zone label into Mark.Time on a real-world
// calendar and never on an in-world one — the two halves of one fact, which r52
// refused to let become two fields precisely so they could not disagree. A
// zone-labelled real-world time printed on an in-world calendar is L15's
// forbidden case.
func TestSeam_LedgerTimesFollowTheCalendarNotTheEvent(t *testing.T) {
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner, Zone: "America/Chicago"}

	inWorldEv := blockEvent("timed", 4, "everyone")
	inWorldEv.StartHour, inWorldEv.StartMinute = blockIntPtr(18), blockIntPtr(30)
	inWorld := seamLedger(t, BlockProjectionInput{
		Calendar: blockTenDayCal(), Events: []Event{inWorldEv},
		Viewer: viewer, MonthIndex: 0, Year: 1523,
	})
	seamContain(t, inWorld, `class="tm mono">18:30<`,
		"an in-world calendar's Ledger time is .tm.mono and carries NO zone label — a "+
			"zone-labelled time there would be a claim about a clock that does not exist")

	real := blockRealTimeCal()
	realEv := Event{ID: "rt", CalendarID: real.ID, Name: "Session 41",
		Year: 2028, Month: 2, Day: 29, Visibility: "everyone",
		StartHour: blockIntPtr(19), StartMinute: blockIntPtr(0)}
	rw := seamLedger(t, BlockProjectionInput{
		Calendar: real, Events: []Event{realEv},
		Viewer: viewer, MonthIndex: 1, Year: 2028,
	})
	seamNotContain(t, rw, `class="tm mono"`,
		"a real-world calendar's Ledger time is plain .tm")
	if !regexp.MustCompile(`class="tm">19:00 [A-Z]{2,5}<`).MatchString(seamWS.ReplaceAllString(rw, " ")) {
		t.Error("a real-world Ledger time must carry the zone abbreviation L15 requires, folded " +
			"into the producer's formatted string (r52 §3.1)")
	}

	// An UNTIMED event drops the segment rather than printing an empty one.
	untimed := seamLedger(t, BlockProjectionInput{
		Calendar: blockTenDayCal(), Events: []Event{blockEvent("untimed", 4, "everyone")},
		Viewer: viewer, MonthIndex: 0, Year: 1523,
	})
	seamNotContain(t, untimed, `class="tm`,
		"an untimed event must emit NO .tm element, not an empty one")
}

// ── §3.4: the foot line states the REAL event total ─────────────────────────

// TestSeam_FootTotalEqualsTheDayCellTotal pins the MoreCount ruling (data.go:
// OVERLAPPING, not additive — Marks holds the FULL viewer-visible list and
// MoreCount is how many of those are not drawn as chips) at the only level that
// can see both halves at once. The producer keeps every mark AND declares the
// chip fold, so a renderer that adds the two counts the folded marks twice: a
// month with 7 events prints "10 events" in its foot. Neither suite alone could
// see it — the producer's numbers were internally consistent, and the widget's
// fixture left MoreCount 0 everywhere, the one shape on which the additive and
// overlapping readings agree.
func TestSeam_FootTotalEqualsTheDayCellTotal(t *testing.T) {
	// Five events on one day: dense enough that blockCapMarks declares an
	// overflow (MoreCount 3 beside 5 kept marks), so the additive bug actually
	// fires. Two sparse days prove the total is a sum, not one cell's count.
	events := []Event{
		blockEvent("dense-1", 4, "everyone"),
		blockEvent("dense-2", 4, "everyone"),
		blockEvent("dense-3", 4, "everyone"),
		blockEvent("dense-4", 4, "everyone"),
		blockEvent("dense-5", 4, "everyone"),
		blockEvent("sparse-1", 9, "everyone"),
		blockEvent("sparse-2", 12, "everyone"),
	}
	in := BlockProjectionInput{
		Calendar:   blockTenDayCal(),
		Viewer:     BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: 0,
		Year:       1523,
	}

	// The true total, computed the way the ruling defines it: sum of len(Marks)
	// over every cell, intercalary rows included, MoreCount never added.
	in.Events = blockCopyEvents(events)
	d := projectBlock(in)
	total, folded := 0, 0
	for _, r := range d.Month.Rows {
		for _, c := range r.Cells {
			total += len(c.Marks)
			folded += c.MoreCount
		}
	}
	for _, ic := range d.Month.Intercalary {
		total += len(ic.Marks)
	}
	if total != len(events) {
		t.Fatalf("fixture drift: projected %d marks for %d events", total, len(events))
	}
	if folded == 0 {
		t.Fatal("fixture drift: no cell overflows the chip cap, so an additive foot would not over-count")
	}

	in.Events = blockCopyEvents(events)
	body := seamRender(t, in)
	seamContain(t, body, fmt.Sprintf(">%d events<", total),
		"the foot line must state the true day-cell total")
	seamNotContain(t, body, fmt.Sprintf(">%d events<", total+folded),
		"len(Marks)+MoreCount counts the folded marks twice — MoreCount is OVERLAPPING")

	// ── EXTENDED BY C-CALV4-LEDGER-P6 §6: a THIRD reading of the same number.
	//
	// The docked Ledger's head prints the month total too. Three surfaces now
	// state one number, and the point of extending this test rather than
	// writing a parallel one is that they are three READINGS, not three
	// computations: the Ledger is reassembled from the very cells the foot
	// totals, so a disagreement here means someone introduced a second pass.
	in.Events = blockCopyEvents(events)
	led := seamLedger(t, in)
	seamContain(t, led, fmt.Sprintf(">Deepwinter · %d events<", total),
		"the Ledger head, the foot line and the day-cell total are ONE number read three ways")
	seamNotContain(t, led, fmt.Sprintf(">Deepwinter · %d events<", total+folded),
		"the Ledger head added the chip fold to the day's list — MoreCount is OVERLAPPING")
	if n := strings.Count(led, `class="lrow `); n != total {
		t.Errorf("the Ledger listed %d rows against a day-cell total of %d — a second pass", n, total)
	}
	// The dense day's own head line agrees with its own cell.
	seamContain(t, led, `data-lday="4">4 Deepwinter · 5 events<`,
		"the per-day head line is the day cell's own count, not a share of the month's")
}

// ── §1 acceptance: the enabled-layer set matches what renders ───────────────

// seamLayerSurfaces maps each of the eight LayerState keys (data.go: moons ·
// eras · weeknums · ledger · moongraph · legend · horizon · shelf) to the HTML
// marker its surface carries. A key with no marker here would be a layer that
// gates nothing — the set-and-ignored defect class this file exists to kill.
//
// The halfrule ruler and the .band.half class are NOT in this table: both are
// month GEOMETRY (the producer's Weekday/DayCell/EraBand Half flags), not a
// layer, and gating geometry on a layer key would let a viewer preference
// break the five-column counting aid.
var seamLayerSurfaces = []struct {
	key    string
	marker string
}{
	// THE MARKER IS OPEN-ENDED SINCE C-CALV4-MOONS, and deliberately so: the
	// cluster's class is `phrow` when it is decoration and `phrow phctl` when
	// it is the control that opens the moon panel, so a closing quote here
	// would assert which of the two shipped rather than that the LAYER
	// rendered. Both halves of this table's contract stay exactly as strong —
	// any cluster at all fails the absence check.
	{"moons", `class="phrow`},        // the per-cell moon discs (the moongraph is the CURVE, a separate key)
	{"eras", `class="bands"`},        // the era band row
	{"weeknums", `data-weeknums`},    // the grid states the layer; the gutter labels it
	{"ledger", `data-zone="ledger"`}, // zone C
	{"moongraph", `data-layer="moongraph"`},
	{"legend", `data-layer="legend"`},
	{"horizon", `data-layer="horizon"`},
	{"shelf", `data-zone="shelf"`}, // zone D
	// THE SWITCHBOARD'S OWN ROW (C-CALV4-LAYERS-P9). Every key has a surface
	// in the sheet as well as in the month, and the sheet's row is the ONE
	// marker that must be present whatever the on-set is — a switchboard that
	// only listed the layers already on would be a control you cannot use to
	// turn anything on. It is therefore checked separately, below, rather than
	// being a ninth row here: this table's contract is "enabled ⇒ present,
	// absent ⇒ absent", and the row obeys the opposite rule on purpose.
}

// seamSwitchboardRowMarker is the sheet row for one key. Deliberately NOT in
// seamLayerSurfaces: see the note there.
func seamSwitchboardRowMarker(key string) string { return `data-layer-pick="` + key + `"` }

// TestSeam_EnabledLayerSetMatchesWhatRenders pins the dispatch §1 acceptance
// line at the only level that can see both halves at once: the producer's
// layer choice AND the renderer's obedience to it.
//
// Wave 1's producer emits DEF = ["moons"] (blockDefaultLayers, a signed
// ruling): the default surface is a month with its moon phases and NOTHING
// else. The first half renders that real projection and asserts exactly that —
// discs present, all seven other surfaces absent. Before this stage the
// Ledger and Shelf zones rendered anyway, so the composed DEF surface never
// matched the registry that claims to govern it.
//
// The second half flips the registry to every key EXCEPT moons and re-renders
// the SAME projection.
//
// THE STAND-IN IS GONE (C-CALV4-LAYERS-P9). This comment used to say the
// producer had no layer input to vary and that overriding Layers.Enabled was
// "standing in for exactly that store". W-F built the store, so the second half
// now drives the REAL resolution: a viewer whose stored set is those seven keys,
// handed to the producer as BlockProjectionInput.LayerPrefs, exactly the way the
// route and the repository hand it over in production. Every other field is
// still the producer's own output. Each surface must appear, and the moon discs
// must leave.
//
// A THIRD HALF NOW EXISTS, and it is the one the switchboard made necessary:
// the SHEET must list all eight keys whatever the on-set is, because a
// switchboard that only listed the layers already on could never turn one on.
// That is the opposite rule from the one seamLayerSurfaces encodes, which is
// why it is asserted here rather than as a ninth table row.
func TestSeam_EnabledLayerSetMatchesWhatRenders(t *testing.T) {
	// The fixture must CARRY every layer-owned surface's data, or an absence
	// is unfalsifiable: a missing era band could mean "gated off" or "this
	// calendar has no eras", and only the former is this test's claim.
	cal := blockTenDayCal()
	cal.Eras = []Era{{ID: 1, CalendarID: cal.ID, Name: "Reckoning of Wards", StartYear: 1, Color: "#8b5cf6"}}
	cal.Moons = []Moon{{ID: 1, CalendarID: cal.ID, Name: "Selune", CycleDays: 30.4}}
	// The event carries a DECLARED CATEGORY as of C-CALV4-LAYERS-P9, and the
	// fixture note above is why: the legend is built from Mark.AxisLabel, which
	// resolves only through the calendar's declared categories. An uncategorised
	// event yields no label, the legend renders nothing, and "the legend layer
	// is off" would be indistinguishable from "this month has no types" — the
	// unfalsifiable absence this whole test exists to refuse.
	cal.EventCategories = []EventCategory{
		{ID: 1, CalendarID: cal.ID, Slug: "quest", Name: "Quest", Icon: "▲", Color: "#ef4444"},
	}
	quest := "quest"
	layered := blockEvent("layered-1", 4, "everyone")
	layered.Category = &quest
	events := []Event{layered}

	in := BlockProjectionInput{
		Calendar:   cal,
		Events:     blockCopyEvents(events),
		Viewer:     BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: 0,
		Year:       1523,
	}

	// DEF, exactly as the producer emits it.
	def := seamRender(t, in)
	for _, s := range seamLayerSurfaces {
		if s.key == "moons" {
			seamContain(t, def, s.marker,
				"DEF's month must draw its moon phases")
			continue
		}
		seamNotContain(t, def, s.marker,
			"DEF is a month with its moon phases and NOTHING else; the "+s.key+" layer is off")
	}

	// The complement: every key except moons — through the REAL store, not an
	// override. This is the resolution production runs.
	in.Events = blockCopyEvents(events)
	in.LayerPrefs = blockLayerPrefs{
		Stored:     []string{"eras", "weeknums", "ledger", "moongraph", "legend", "horizon", "shelf"},
		PersistURL: blockPrefsPath("camp-1"),
	}
	d := projectBlock(in)
	if d.Layers.Enabled == nil || len(d.Layers.Enabled) != 7 {
		t.Fatalf("the producer did not honour the viewer's stored set: %v", d.Layers.Enabled)
	}
	inverse := seamRenderBlockData(t, d)
	for _, s := range seamLayerSurfaces {
		if s.key == "moons" {
			seamNotContain(t, inverse, s.marker,
				"the moons layer is off; the discs must leave the DOM")
			continue
		}
		seamContain(t, inverse, s.marker,
			"the "+s.key+" layer is on; its surface must render")
	}
	// The weeknums gutter label, beyond the grid's data attribute.
	seamContain(t, inverse, `class="wknum"`,
		"the weeknums layer labels the gutter, not just the grid")

	// MN-G14 (C-CALV4-MOONS): MOONS OFF MEANS NOTHING AT ALL — no discs, no
	// chip, no panel and NO HIT TARGET. `v4-bare-no-moons.png` draws the layer
	// off as an empty month, because absence is this product's vocabulary for
	// "there is nothing here" and the surface does not draw a frame saying so.
	// The discs are covered by the row above; these three are the control the
	// cluster became, and each is a separate way to leave a dead affordance
	// behind.
	for _, marker := range []string{`data-cal-moonpanel`, `class="moonpick`, `data-moon-pick`} {
		seamNotContain(t, inverse, marker,
			"the moons layer is off; "+marker+" must leave the DOM with the discs — a "+
				"panel, a radio or a hit target that outlives the layer that owns it is a "+
				"control the viewer switched off and still has (MN-G14)")
	}

	// And ON, the control really is there. An absence guard with no presence
	// twin is how the sky band's placeholder could have been deleted with every
	// suite green ([SKY-11]'s own lesson, applied here).
	for _, marker := range []string{`class="phrow phctl"`, `data-cal-moonpanel`, `data-moon-pick="none"`} {
		seamContain(t, def, marker,
			"DEF has the moons layer on, so "+marker+" must render: the cluster is a "+
				"control and the panel it opens is the whole of C-CALV4-MOONS [MN-3]")
	}

	// THE SHEET LISTS EVERY KEY IN BOTH HALVES. `moons` is OFF in this render
	// and its row must still be there, or the viewer could never turn it back
	// on; the seven that are on must be there too, or they could never be
	// turned off.
	for _, s := range seamLayerSurfaces {
		seamContain(t, inverse, seamSwitchboardRowMarker(s.key),
			"the switchboard must list "+s.key+" whether it is on or off — a sheet that "+
				"listed only the enabled layers would be a control you cannot use")
	}

	// And in the DEF half, where seven of the eight are off, through a
	// projection whose store is live.
	in.Events = blockCopyEvents(events)
	in.LayerPrefs = blockLayerPrefs{PersistURL: blockPrefsPath("camp-1")}
	defLive := seamRenderBlockData(t, projectBlock(in))
	for _, s := range seamLayerSurfaces {
		seamContain(t, defLive, seamSwitchboardRowMarker(s.key),
			"DEF's sheet must still list "+s.key+" — the whole point of the default is "+
				"that a viewer can leave it")
	}
	seamContain(t, defLive, "Layers · 1 of 8 on",
		"the denominator is the registry's length for every role, and the numerator is "+
			"the viewer's own on-set")
}

// ── r51 acceptance: the declared-moon total reaches the Nameplate ───────────

// TestSeam_DeclaredMoonTotalReachesTheNameplate pins the last r51 acceptance
// line (decisions/2026-07-27-calv4-tie-mark-emission.md §7): "a calendar
// declaring more moons than the grid draws states the total, and a calendar
// declaring three or fewer states nothing extra."
//
// It drives the REAL producer path — BlockService.Block hydrates
// Calendar.Moons through the repo's MoonsForCalendars batch read before
// buildMonthGeometry runs — because MoonsDeclared cannot be derived from the
// per-cell discs (those are already capped, data.go), so a producer that
// never sets the field leaves a fourth moon silently drawn nowhere while
// every hand-written widget fixture stays green. MoonCap 3 in the request
// makes the disc cap and the stated total coexist in one render: three discs
// drawn, four declared.
func TestSeam_DeclaredMoonTotalReachesTheNameplate(t *testing.T) {
	render := func(moons []Moon) string {
		t.Helper()
		cal := blockTenDayCal()
		cal.Moons = moons
		svc := NewBlockService(newBlockFakeRepo(cal))
		d, err := svc.Block(context.Background(), BlockRequest{
			CalendarID: cal.ID, CampaignID: "camp-1",
			Viewer:  BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
			MoonCap: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		if err := calblock.Block(d).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render composed Block: %v", err)
		}
		return sb.String()
	}

	four := []Moon{
		{ID: 1, CalendarID: "cal-harptos", Name: "Alder", CycleDays: 31.4},
		{ID: 2, CalendarID: "cal-harptos", Name: "Umber", CycleDays: 46.5},
		{ID: 3, CalendarID: "cal-harptos", Name: "Flint", CycleDays: 11.3},
		{ID: 4, CalendarID: "cal-harptos", Name: "Sable", CycleDays: 88.2},
	}
	over := render(four)
	seamContain(t, over, ">3 of 4 moons<",
		"a fourth declared moon is drawn nowhere; without the stated total the omission is silent")

	under := render(four[:3])
	seamNotContain(t, under, "moons</span>",
		"a calendar declaring three or fewer states nothing extra (r51 acceptance)")
}

// ── L21's other half: the declared body the grid does NOT draw ─────────────

// TestSeam_TheFourthDeclaredMoonReachesTheAlmanac is the seam L21 actually
// depends on, and the one C-CALV4-SHELF-P7 §11 asks for by name.
//
// TestSeam_DeclaredMoonTotalReachesTheNameplate above proves only that the
// badge STATES a total. That is the ceiling being announced. It says nothing
// about where the announced-but-undrawn body went — and "the grid can never
// grow with the fiction" is legitimate ONLY because "the Almanac carries every
// declared body at full width" (design notes :667). A producer that filled
// MoonsDeclared and left MonthGeometry.Almanac empty would keep every
// assertion above green while the fourth moon vanished from the product.
//
// IT RUNS THE REAL PRODUCER PATH, exactly as its sibling does — BlockService
// .Block hydrates Calendar.Moons through the repo's batch read before
// buildMonthGeometry runs — because the register cannot be derived from
// anything a hand-written widget fixture could supply.
func TestSeam_TheFourthDeclaredMoonReachesTheAlmanac(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()
	svc := NewBlockService(newBlockFakeRepo(cal))
	d, err := svc.Block(context.Background(), BlockRequest{
		CalendarID: cal.ID, CampaignID: "camp-1",
		Viewer:  BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MoonCap: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The producer's own half: four lanes from a MoonCap-3 request, and the
	// grid still drawing three discs per day. Both facts in ONE render is what
	// makes the ceiling and the register coexist rather than contradict.
	if n := len(d.Month.Almanac); n != 4 {
		t.Fatalf("the register carries %d lanes for four declared moons; the fourth body has "+
			"nowhere else in BlockData to be (DayCell.Moons is capped)", n)
	}
	drawn := 0
	for _, m := range d.Month.Almanac {
		if m.Drawn {
			drawn++
		}
	}
	if drawn != 3 {
		t.Errorf("%d lanes are marked Drawn under MoonCap 3; the badge's '3 of 4' and the "+
			"register's own flags must state one ceiling, not two", drawn)
	}

	// The renderer's half: the Shelf docks, and the undrawn body is IN THE DOM.
	// Overriding Layers stands in for W-F's per-viewer store and nothing else;
	// every other field is the producer's real output.
	d.Layers.Enabled = []string{"moons", "eras", "weeknums", "ledger", "shelf"}
	body := seamRenderBlockData(t, d)

	seamContain(t, body, `data-spane="almanac"`,
		"a calendar with declared moons must reach the Almanac panel")
	seamContain(t, body, ">Sable<",
		"Sable is the fourth declared body and the grid does not draw it; if it is not here "+
			"it is nowhere, and the ceiling stops being legitimate")
	seamContain(t, body, "past the ceiling the grid draws",
		"the Moons panel must SAY which bodies the ceiling excluded — Drawn is the only "+
			"place the renderer learns it")
	seamContain(t, body, ">3 of 4 moons<",
		"the badge and the register agree in one render: three drawn, four declared")
	seamContain(t, body, "all of them are in the Almanac",
		"and the badge's restored hover tail is TRUE in this render (SEAM-P5 §4.8)")
}

// TestSeam_TheAlmanacRegisterIgnoresMoonCap is [S5], SIGNED, at the seam.
//
// MoonCap is a HOST-passed parameter — the entity host passes 3
// (entity_calendar_block.go) — and this is the one sanctioned place it is
// deliberately non-authoritative. It governs the GRID; it does not govern the
// register the grid's ceiling points at. That is a new shape, which is why it
// was signed once here rather than discovered later.
//
// The producer-side unit assertion lives in block_almanac_test.go. This is the
// COMPOSED one: it drives BlockService.Block, which is the path the entity host
// and the Bench actually take, so a MoonCap that leaked into the register
// anywhere between the request and the geometry is caught.
func TestSeam_TheAlmanacRegisterIgnoresMoonCap(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()

	for _, mc := range []int{1, 2, 3, 4, 0} {
		svc := NewBlockService(newBlockFakeRepo(cal))
		d, err := svc.Block(context.Background(), BlockRequest{
			CalendarID: cal.ID, CampaignID: "camp-1",
			Viewer:  BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
			MoonCap: mc,
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(d.Month.Almanac); n != len(cal.Moons) {
			t.Errorf("MoonCap %d: the register carries %d lanes for %d declared moons — the "+
				"Almanac is UNCAPPED ([S5]); MoonCap decides Drawn and nothing else",
				mc, n, len(cal.Moons))
		}
		// Every lane carries its WHOLE month: "never partially filled — a moon
		// that appears here appears with its whole month, because the Month
		// lane, the Tonight readout and the Moons arithmetic are three views of
		// one pass".
		for _, m := range d.Month.Almanac {
			if len(m.Days) != d.Month.Days {
				t.Errorf("MoonCap %d: %s carries %d of %d days", mc, m.Name, len(m.Days), d.Month.Days)
			}
		}
		// …and the GRID still honours it, or the two would be agreeing by both
		// being wrong.
		for _, r := range d.Month.Rows {
			for _, c := range r.Cells {
				if c.Day > 0 && mc > 0 && len(c.Moons) > mc {
					t.Fatalf("MoonCap %d: day %d drew %d discs — the register being uncapped "+
						"must not uncap the grid", mc, c.Day, len(c.Moons))
				}
			}
		}
	}
}

// ── §5: a recurring event marks each day ONCE across intercalary months ─────

// TestSeam_RecurringEventMarksOnceAcrossIntercalaryMonths pins dispatch §5 at
// the only level that can see it: the SERVICE. candidateEvents reads the
// rendered month PLUS every intercalary month hanging off it, and
// ListEventsForMonth's recurring-candidate widening (repository.go, C-CAL-
// EDITOR-EXPANSION PR2) returns every recurring row regardless of the month
// asked for — so the concatenated candidate slice carries one copy of a
// recurring event PER QUERIED MONTH. blockCountEvents dedupes on event id;
// blockMarksForDate does not, and emits one mark per duplicate row, so the
// grid and the totals disagree. TestBlockRecurringEventsAreExpandedOnce could
// never catch this: it hand-feeds projectBlock a single copy, which is exactly
// the input the service fails to produce. The dedupe belongs at the SOURCE
// (candidateEvents), never in a downstream marks filter — the projection's
// one-pass rule assumes `visible` holds each event once.
func TestSeam_RecurringEventMarksOnceAcrossIntercalaryMonths(t *testing.T) {
	// Deepwinter with Midwinter hanging off it, and a weekly event based on
	// day 1: a ten-day week lands it on grid days 1/11/21 AND on the
	// intercalary day itself (absolute day 30), so both surfaces are probed.
	cal := blockTenDayCal()
	cal.Months = append(cal.Months[:1], append([]Month{{
		ID: 99, CalendarID: cal.ID, Name: "Midwinter", Days: 1, SortOrder: 1, IsIntercalary: true,
	}}, cal.Months[1:]...)...)

	rt := RecurrenceWeekly
	repo := newBlockFakeRepo(cal)
	repo.events[cal.ID] = []Event{{
		ID: "weekly", CalendarID: cal.ID, Name: "Tenday market",
		Year: 1523, Month: 1, Day: 1, Visibility: "everyone",
		IsRecurring: true, RecurrenceType: &rt,
	}}
	svc := NewBlockService(repo)

	d, err := svc.Block(context.Background(), BlockRequest{
		CalendarID: cal.ID, CampaignID: "camp-1",
		Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
	})
	if err != nil {
		t.Fatal(err)
	}

	// The producer half: each occurrence day carries the event ONCE. A map of
	// day → mark count makes the failure state legible (one mark per queried
	// month on every occurrence day).
	perDay := map[int]int{}
	for _, row := range d.Month.Rows {
		for _, c := range row.Cells {
			for _, m := range c.Marks {
				if m.EventID == "weekly" {
					perDay[c.Day]++
				}
			}
		}
	}
	if len(perDay) != 3 || perDay[1] != 1 || perDay[11] != 1 || perDay[21] != 1 {
		t.Errorf("weekly marks per grid day = %v, want days 1/11/21 exactly once each — the candidate slice holds duplicates", perDay)
	}
	if len(d.Month.Intercalary) != 1 {
		t.Fatalf("intercalary rows = %d, want 1", len(d.Month.Intercalary))
	}
	if n := len(d.Month.Intercalary[0].Marks); n != 1 {
		t.Errorf("Midwinter carries %d marks for one event, want 1", n)
	}
	// The totals agree with the grid: one DISTINCT event (the counts already
	// dedupe — that green half is what let the marks half hide), four marks.
	if d.Viewer.WholeCount != 1 {
		t.Errorf("WholeCount = %d; four occurrences of one event are one event", d.Viewer.WholeCount)
	}

	// The composed half: the operator's screen. Four chips, and a foot line
	// totalling the marks the grid actually draws.
	var sb strings.Builder
	if err := calblock.Block(d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render composed Block: %v", err)
	}
	body := sb.String()
	if n := strings.Count(body, `data-event-id="weekly"`); n != 4 {
		t.Errorf("composed HTML draws %d chips for the 4 occurrences of one event", n)
	}
	seamContain(t, body, ">4 events<",
		"the foot line must total the deduped mark set, one per occurrence day")
	seamNotContain(t, body, ">8 events<",
		"a doubled foot total is the duplicate candidate slice reaching the operator")
}

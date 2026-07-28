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
	d := projectBlock(in)
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
	{"moons", `class="phrow"`},           // the per-cell moon discs (the moongraph is the CURVE, a separate key)
	{"eras", `class="bands"`},            // the era band row
	{"weeknums", `data-weeknums`},        // the grid states the layer; the gutter labels it
	{"ledger", `data-zone="ledger"`},     // zone C
	{"moongraph", `data-layer="moongraph"`},
	{"legend", `data-layer="legend"`},
	{"horizon", `data-layer="horizon"`},
	{"shelf", `data-zone="shelf"`},       // zone D
}

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
// the SAME projection. The producer has no layer input to vary — the
// per-viewer layer store is W-F (data.go) — so overriding Layers.Enabled on
// the projected BlockData is standing in for exactly that store and nothing
// else; every other field is the producer's real output. Each surface must
// appear, and the moon discs must leave.
func TestSeam_EnabledLayerSetMatchesWhatRenders(t *testing.T) {
	// The fixture must CARRY every layer-owned surface's data, or an absence
	// is unfalsifiable: a missing era band could mean "gated off" or "this
	// calendar has no eras", and only the former is this test's claim.
	cal := blockTenDayCal()
	cal.Eras = []Era{{ID: 1, CalendarID: cal.ID, Name: "Reckoning of Wards", StartYear: 1, Color: "#8b5cf6"}}
	cal.Moons = []Moon{{ID: 1, CalendarID: cal.ID, Name: "Selune", CycleDays: 30.4}}
	events := []Event{blockEvent("layered-1", 4, "everyone")}

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

	// The complement: every key except moons.
	in.Events = blockCopyEvents(events)
	d := projectBlock(in)
	d.Layers.Enabled = []string{"eras", "weeknums", "ledger", "moongraph", "legend", "horizon", "shelf"}
	var sb strings.Builder
	if err := calblock.Block(d).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render composed Block: %v", err)
	}
	inverse := sb.String()
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
}

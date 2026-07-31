package calendar

// block_count_oracle_test.go — C-CALV4-LEDGER-P6's GATE (§6).
//
// WHY A COUNT TEST IS THE GATE AND A FIDELITY SCREENSHOT IS NOT. The docked
// Ledger multiplies the number of counts on screen by roughly five: the head's
// month total, a head total PER SELECTABLE DAY, the nameplate's hidden chip,
// the tie pair, the "+n more" folds and the foot. cv4:1382-1385 is the rule all
// of them answer to, verbatim:
//
//	PERMISSION IS ABSENCE. Players never receive dm rows — no placeholder, no
//	ghost, no "+1". Every count below is computed from the viewer's own set,
//	and every set is scoped to ONE calendar.
//
// THE ASSERTION IS NOT "THE NUMBERS ARE RIGHT". That is the discipline
// TestBlockCountsAreNotAnOracle established and this file extends to every
// number the Ledger added: each one must be INDEPENDENTLY REPRODUCIBLE from
// that viewer's own visible set, recomputed here from filterEventsByUser's
// output. A count that happens to be right because it was computed pre-filter
// passes a value assertion and fails this one.
//
// IT LIVES IN PACKAGE calendar, and it reuses the seam suite's render helper
// (seamRender) rather than authoring a second one — block_seam_test.go's header
// explains at length why a widget-side test cannot make any of these claims:
// its author writes the BlockData, so every "the producer chose correctly"
// assertion is vacuous there. Re-authoring a parallel fixture or a parallel
// render helper is what forking the suite means, and it is forbidden.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// ── the signed fixture: GM 14 · Nissa 11 · Bryn 9 ──────────────────────────

// The oracle numbers are not invented for this test. Design notes L23 (:697-700)
// fixes them and v4-block-full-player.png is `?as=bryn` reading "Deepwinter · 9
// events". 14 − 9 = 3 dm_only + 2 audience-restricted, and Nissa (a Warden) is
// inside the restriction while Bryn (front line) is not — which is what makes
// the middle number a real third reading rather than a rounding of the other
// two.
const (
	oracleGMTotal    = 14
	oracleNissaTotal = 11
	oracleBrynTotal  = 9
)

// oracleLedgerFixture is the signed month, built from the SAME primitives the
// rest of the suite uses (blockTenDayCal, blockEvent) rather than a new
// calendar of its own. Days, types and permissions mirror the mockup's EV list.
func oracleLedgerFixture() (*Calendar, []Event) {
	cal := blockTenDayCal()
	cal.EventCategories = []EventCategory{
		{ID: 1, CalendarID: cal.ID, Slug: "social", Name: "Social", Icon: "◆", Color: "#3b82f6"},
		{ID: 2, CalendarID: cal.ID, Slug: "quest", Name: "Quest", Icon: "▲", Color: "#ef4444"},
		{ID: 3, CalendarID: cal.ID, Slug: "festival", Name: "Festival", Icon: "✦", Color: "#f59e0b"},
		{ID: 4, CalendarID: cal.ID, Slug: "downtime", Name: "Downtime", Icon: "●", Color: "#22c55e"},
	}
	// The restriction Nissa is inside and Bryn is not. AllowedUsers is a
	// whitelist in canUserView, so it is the only primitive on main that can
	// express "these people and no others".
	const wardens = `{"allowed_users":["u-gm","u-nissa"]}`

	spec := []struct {
		id         string
		day        int
		title      string
		kind       string
		dmOnly     bool
		restricted bool
	}{
		{"ev-1", 3, "Council of Wards", "social", false, false},
		{"ev-2", 5, "Barrow scouting", "quest", true, false},
		{"ev-3", 5, "Caravan due", "downtime", false, false},
		{"ev-4", 5, "Ward levy", "social", false, false},
		{"ev-5", 5, "Smith's deadline", "downtime", false, false},
		{"ev-6", 5, "Rumour: the pale", "social", false, false},
		{"ev-7", 5, "Tithe collected", "downtime", false, false},
		{"ev-8", 8, "Supply run", "downtime", false, false},
		{"ev-9", 8, "Into the Barrow", "quest", true, false},
		{"ev-10", 12, "Nissa's recital", "social", false, true},
		{"ev-11", 14, "Emberfall Vigil", "festival", false, false},
		{"ev-12", 17, "Ward-court summons", "social", false, true},
		{"ev-13", 21, "Frost fair", "festival", false, false},
		{"ev-14", 26, "Warden's writ due", "quest", true, false},
	}
	events := make([]Event, 0, len(spec))
	for _, s := range spec {
		vis := "everyone"
		if s.dmOnly {
			vis = "dm_only"
		}
		e := blockEvent(s.id, s.day, vis)
		e.Name = s.title
		kind := s.kind
		e.Category = &kind
		if s.restricted {
			e.VisibilityRules = blockStrPtr(wardens)
		}
		events = append(events, e)
	}
	return cal, events
}

// oracleViewers are the three the signed renders name.
func oracleViewers() []struct {
	name  string
	v     BlockViewer
	total int
} {
	return []struct {
		name  string
		v     BlockViewer
		total int
	}{
		{"GM", BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, oracleGMTotal},
		{"Nissa (Warden)", BlockViewer{UserID: "u-nissa", Role: permissions.RolePlayer}, oracleNissaTotal},
		{"Bryn (front line)", BlockViewer{UserID: "u-bryn", Role: permissions.RolePlayer}, oracleBrynTotal},
	}
}

// oracleVisible recomputes a viewer's own visible set, independently of the
// projection, so every number below can be checked against it rather than
// against another number the projection produced.
func oracleVisible(events []Event, v BlockViewer) []Event {
	return filterEventsByUser(blockCopyEvents(events), v.Role, v.UserID)
}

// TestOracle_TheFixtureReproducesTheSignedNumbers. If this fails, every
// assertion below it is measuring the wrong month and the failures would be
// misleading — so it fails FIRST and loudly.
func TestOracle_TheFixtureReproducesTheSignedNumbers(t *testing.T) {
	_, events := oracleLedgerFixture()
	if len(events) != oracleGMTotal {
		t.Fatalf("the fixture carries %d events; the signed month has %d", len(events), oracleGMTotal)
	}
	for _, w := range oracleViewers() {
		if n := len(oracleVisible(events, w.v)); n != w.total {
			t.Errorf("%s sees %d of %d events; design notes L23 fixes it at %d",
				w.name, n, len(events), w.total)
		}
	}
}

// TestOracle_EveryLedgerCountComesFromTheViewersOwnSet is the gate proper.
//
// Six numbers, three viewers, one rule: each number must be reproducible from
// that viewer's own filterEventsByUser output and from nothing else.
func TestOracle_EveryLedgerCountComesFromTheViewersOwnSet(t *testing.T) {
	cal, events := oracleLedgerFixture()

	for _, w := range oracleViewers() {
		t.Run(w.name, func(t *testing.T) {
			visible := oracleVisible(events, w.v)

			// The independent oracle: this viewer's own per-day tallies.
			perDay := map[int]int{}
			for i := range visible {
				perDay[visible[i].Day]++
			}
			want := len(visible)

			in := BlockProjectionInput{
				Calendar: cal, Events: blockCopyEvents(events),
				Viewer: w.v, MonthIndex: 0, Year: 1523,
			}
			d := projectBlock(in)
			d.Layers.Enabled = []string{"moons", "eras", "weeknums", "ledger", "shelf"}
			body := seamRenderBlockData(t, d)

			// 1. THE LEDGER HEAD, unselected.
			head := ">Deepwinter · " + blockPlural(want) + "<"
			seamContain(t, body, head,
				"the Ledger head must state the total from this viewer's own set")

			// 2. THE FOOT — the same number, from the same pass. The head, the
			//    foot and the day-cell total are three readings of one number.
			seamContain(t, body, ">"+blockPlural(want)+"<",
				"the foot line must agree with the Ledger head")

			// 3. THE DAY-CELL TOTAL, recomputed off the projection itself.
			total := 0
			for _, r := range d.Month.Rows {
				for _, c := range r.Cells {
					total += len(c.Marks)
					if c.Day > 0 && len(c.Marks) != perDay[c.Day] {
						t.Errorf("day %d carries %d marks; this viewer can see %d events there",
							c.Day, len(c.Marks), perDay[c.Day])
					}
				}
			}
			for _, ic := range d.Month.Intercalary {
				total += len(ic.Marks)
			}
			if total != want {
				t.Errorf("the grid draws %d marks for a viewer who can see %d events", total, want)
			}

			// 4. THE PER-DAY HEAD LINES. One per selectable day, each carrying
			//    THAT DAY's own count from the same pass. These are the numbers
			//    the Ledger multiplied by thirty, and they are exactly where a
			//    pre-filter count would hide.
			for day := 1; day <= d.Month.Days; day++ {
				line := `data-lday="` + strconv.Itoa(day) + `">` + strconv.Itoa(day) +
					" Deepwinter · " + blockPlural(perDay[day]) + "<"
				seamContain(t, body, line,
					"day "+strconv.Itoa(day)+"'s head line must state this viewer's own count")
			}

			// 5. "+N MORE" is a fold within the day's own list, never a hint at
			//    a longer one. Day 5 is the dense day: 6 events for the GM, 5
			//    for both players — so the GM prints "+4 more" and a player
			//    "+3 more". A type with count zero LEAVES; it does not render
			//    as a zero and it does not inflate the fold.
			wantMore := perDay[5] - 2 // three chips, or two plus the fold
			seamContain(t, body, "+"+strconv.Itoa(wantMore)+" more",
				"the fold on day 5 must count only what this viewer received")

			// 6. THE LEDGER LISTS EXACTLY THE VIEWER'S OWN SET, once each.
			//
			//    SCOPED TO ZONE C BY C-CALV4-SHELF-P7. Before W-E filled the
			//    Shelf, `class="lrow ` counted the whole Block because only one
			//    zone emitted rows. The Shelf's Upcoming panel now reuses the
			//    SAME row primitive — deliberately, so the two zones cannot
			//    drift — which means an unscoped count reads the Ledger's rows
			//    plus a subset of them again. The intent is unchanged (the
			//    Ledger lists this viewer's own set, once each); what changed
			//    is that the count now names the zone it is counting.
			ledgerZone := oracleZone(t, body, "ledger")
			if n := strings.Count(ledgerZone, `class="lrow `); n != want {
				t.Errorf("the Ledger listed %d rows for a viewer who can see %d events", n, want)
			}
			for i := range visible {
				if !strings.Contains(ledgerZone, `data-event-id="`+visible[i].ID+`"`) {
					t.Errorf("event %s is visible to this viewer and is not in the Ledger", visible[i].ID)
				}
			}

			// 7. THE SHELF'S UPCOMING PANEL COMES FROM THE SAME PASS
			//    (C-CALV4-SHELF-P7 §11). It is a THIRD reading of the same
			//    filtered set — Today's rows plus up to four later ones — and
			//    it is the reading a second walk would break first, because a
			//    forward list is the natural place to reach for the raw event
			//    slice instead of the cells the grid already drew.
			shelfZone := oracleZone(t, body, "shelf")
			today, later := 0, 0
			for i := range visible {
				switch {
				case visible[i].Day == oracleTodayDay:
					today++
				case visible[i].Day > oracleTodayDay:
					later++
				}
			}
			if later > shelfUpcomingCapForTest {
				later = shelfUpcomingCapForTest
			}
			if n := strings.Count(shelfZone, `class="lrow `); n != today+later {
				t.Errorf("the Shelf's Upcoming panel listed %d rows; this viewer has %d today "+
					"and %d later (capped at %d) in their own set",
					n, today, later, shelfUpcomingCapForTest)
			}
			// The cap is NORMATIVE ([S1]) — .sp2 is a fixed 132px and the cap
			// is what keeps its scroller from becoming the panel — so the
			// Shelf's row count is deliberately NOT the month total. Asserting
			// that too is what stops a later hand "fixing" the difference.
			if today+later > want {
				t.Errorf("the Upcoming panel claims %d rows out of a %d-event set", today+later, want)
			}
		})
	}
}

// oracleTodayDay is the fixture calendar's current day, and the anchor the
// Shelf's Upcoming panel splits on. Named rather than inlined because three
// assertions read it and a silent change to blockTenDayCal would otherwise
// move the split without moving the expectation.
const oracleTodayDay = 14

// shelfUpcomingCapForTest mirrors the widget package's unexported
// shelfUpcomingCap. It cannot be imported, and hardcoding 4 in three places is
// how a normative cap becomes a number nobody can find.
const shelfUpcomingCapForTest = 4

// oracleZone slices the rendered Block to ONE zone's markup, from its
// data-zone marker to the next one (or the end).
//
// It exists because W-E gave the Block a second surface that emits the Ledger's
// row primitive, so a whole-body substring count can no longer say which zone
// it counted. It fails cleanly rather than slicing on a negative index —
// COMMON §3 names a bare strings.Index used as a slice bound as the pin shape
// that PANICS on a rename instead of failing.
func oracleZone(t *testing.T, body, zone string) string {
	t.Helper()
	marker := `data-zone="` + zone + `"`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("the rendered Block has no %s zone", zone)
	}
	rest := body[i:]
	if j := strings.Index(rest[len(marker):], `data-zone="`); j >= 0 {
		return rest[:len(marker)+j]
	}
	return rest
}

// TestOracle_NoHiddenEventReachesAPlayerInAnyForm is the negative half, and it
// is the half that matters: permission is ABSENCE, so the test is not "the
// player's numbers are smaller" but "nothing about the hidden events is
// recoverable from anything the player received".
func TestOracle_NoHiddenEventReachesAPlayerInAnyForm(t *testing.T) {
	cal, events := oracleLedgerFixture()

	gmData := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
		Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, MonthIndex: 0, Year: 1523})
	gmData.Layers.Enabled = []string{"moons", "eras", "weeknums", "ledger", "shelf"}
	gm := seamRenderBlockData(t, gmData)

	// The GM DOES receive the chip, and it states the three dm_only events.
	seamContain(t, gm, ">3 hidden<",
		"the signed `<N> hidden` chip states the GM's own hidden total (r52 §3.3)")
	if gmData.Viewer.HiddenCount != 3 {
		t.Errorf("GM HiddenCount = %d, want 3", gmData.Viewer.HiddenCount)
	}

	hiddenTitles := []string{"Barrow scouting", "Into the Barrow", "Warden's writ due"}
	hiddenIDs := []string{"ev-2", "ev-9", "ev-14"}
	restrictedTitles := []string{"Nissa's recital", "Ward-court summons"}

	for _, w := range []struct {
		name    string
		v       BlockViewer
		alsoHid []string
	}{
		{"Nissa (Warden)", BlockViewer{UserID: "u-nissa", Role: permissions.RolePlayer}, nil},
		{"Bryn (front line)", BlockViewer{UserID: "u-bryn", Role: permissions.RolePlayer}, restrictedTitles},
	} {
		t.Run(w.name, func(t *testing.T) {
			data := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
				Viewer: w.v, MonthIndex: 0, Year: 1523})
			data.Layers.Enabled = []string{"moons", "eras", "weeknums", "ledger", "shelf"}
			body := seamRenderBlockData(t, data)

			for _, title := range append(append([]string{}, hiddenTitles...), w.alsoHid...) {
				seamNotContain(t, body, title,
					"a title this viewer may not see reached their HTML")
			}
			for _, id := range hiddenIDs {
				seamNotContain(t, body, `"`+id+`"`,
					"a hidden event's ID reached this viewer's HTML")
			}

			// r52 rule 3: NOT EVEN A ZERO. The producer leaves HiddenCount at
			// zero for a non-GM and the renderer refuses to draw the chip —
			// two locks, because the whole safety argument is that a player has
			// no number to difference anything against.
			if data.Viewer.HiddenCount != 0 {
				t.Errorf("HiddenCount = %d for a player; it must be zero AT THE PRODUCER",
					data.Viewer.HiddenCount)
			}
			seamNotContain(t, body, "hidden<",
				"no hidden-count chip may render to a player in any form, including a zero")

			// The GM marks are absent too, not greyed: no gold rail, no GM
			// badge, no dogear, no diamond.
			for _, mark := range []string{`class="gr"`, `class="badge gm`, `class="dogear"`, `class="audmark"`} {
				seamNotContain(t, body, mark, "a permission marker reached a player")
			}

			// A TYPE WITH COUNT ZERO LEAVES. Every `quest` in this month is
			// dm_only, so a player's Ledger carries no quest meta line at all —
			// it does not render one saying zero.
			seamNotContain(t, body, `class="mt">quest<`,
				"every quest on this month is dm_only; a type with count zero LEAVES")
		})
	}
}

// TestOracle_TiePairStaysNonDifferenceableWithTheLedgerFilled. The Ledger added
// five kinds of number to the same screen as the tie pair. If any of them were
// computed off a different pass, the pair the projection protects could be
// differenced against them instead.
func TestOracle_TiePairStaysNonDifferenceableWithTheLedgerFilled(t *testing.T) {
	cal, events := oracleLedgerFixture()
	// Tie two events the GM can see and one they can: one of the tied events is
	// dm_only, so a naive tie count would leak it to a player.
	tied := map[string]bool{"ev-1": true, "ev-2": true, "ev-13": true}

	for _, w := range oracleViewers() {
		t.Run(w.name, func(t *testing.T) {
			visible := oracleVisible(events, w.v)
			wantTied := 0
			for i := range visible {
				if tied[visible[i].ID] {
					wantTied++
				}
			}
			viewer := w.v
			viewer.HostEntity = "ent-1"
			viewer.TieMode = "tied"

			d := projectBlock(BlockProjectionInput{Calendar: cal, Events: blockCopyEvents(events),
				Viewer: viewer, MonthIndex: 0, Year: 1523, TiedEventIDs: tied})
			if d.Viewer.TiedCount != wantTied || d.Viewer.WholeCount != len(visible) {
				t.Fatalf("tie pair (%d,%d) is not derivable from this viewer's own visible set "+
					"(%d,%d) — one of the counts saw an unfiltered slice",
					d.Viewer.TiedCount, d.Viewer.WholeCount, wantTied, len(visible))
			}
			d.Layers.Enabled = []string{"moons", "eras", "weeknums", "ledger", "shelf"}
			body := seamRenderBlockData(t, d)

			// Both numbers reach the screen, and the Ledger head agrees with
			// the whole-calendar one. Three surfaces, one pass.
			seamContain(t, body, "Tied "+strconv.Itoa(wantTied), "the tied count must reach the DOM")
			seamContain(t, body, "Whole calendar "+strconv.Itoa(len(visible)),
				"the whole count must reach the DOM")
			seamContain(t, body, ">Deepwinter · "+blockPlural(len(visible))+"<",
				"the Ledger head and the whole-calendar count are the same number")
		})
	}
}

// blockPlural mirrors the widget's shipped eventCountLabel. Wave 1 shipped
// "1 event" where the signed still prints "1 events" (contract defect D2); the
// divergence is already made and already consistent, so the ORACLE follows the
// shipped helper rather than re-introducing the still's ungrammatical form.
func blockPlural(n int) string {
	if n == 1 {
		return "1 event"
	}
	return strconv.Itoa(n) + " events"
}

// ── the legend JOINS the oracle (C-CALV4-LAYERS-P9 §8.1) ───────────────────

// TestOracle_LegendCountsSumToTheViewersOwnTotal is the legend's admission to
// this file, and the reason it is admitted rather than tested in the widget
// package: an assertion that "the producer counted correctly" is vacuous where
// the test author writes the BlockData.
//
// THE CLAIM. The legend prints one count per event TYPE. Those counts are not
// checked for being "right" — that is the discipline this file exists to refuse.
// They must be INDEPENDENTLY REPRODUCIBLE from this viewer's own visible set,
// recomputed here from filterEventsByUser's output, for all three signed
// readings: GM 14, Nissa 11, Bryn 9.
//
// WHY IT MATTERS MORE THAN THE OTHER COUNTS ON SCREEN. Every other number here
// is a total; the legend is a BREAKDOWN, and a breakdown is the easiest place
// in the whole Block to leak a hidden event. A legend counted before the viewer
// filter would put "Quest 5" under a grid drawing four quests, and the missing
// one is a dm_only event the viewer just learned exists — differenceable from
// two numbers on the same screen. This asserts the sum AND every per-type
// figure, because the sum alone can be right while two types are swapped.
func TestOracle_LegendCountsSumToTheViewersOwnTotal(t *testing.T) {
	cal, events := oracleLedgerFixture()

	for _, w := range oracleViewers() {
		t.Run(w.name, func(t *testing.T) {
			visible := oracleVisible(events, w.v)

			// The independent oracle: this viewer's own per-TYPE tallies,
			// keyed on the display name the legend prints.
			perType := map[string]int{}
			for i := range visible {
				if label := blockEventAxisLabel(cal, &visible[i]); label != "" {
					perType[label]++
				}
			}

			d := projectBlock(BlockProjectionInput{
				Calendar: cal, Events: blockCopyEvents(events),
				Viewer: w.v, MonthIndex: 0, Year: 1523,
			})
			d.Layers.Enabled = []string{"moons", "legend"}
			body := seamRenderBlockData(t, d)

			sum := 0
			for label, want := range perType {
				sum += want
				// The legend's own markup: the label, then its count in .n.
				marker := label + ` <span class="n">` + strconv.Itoa(want) + `</span>`
				seamContain(t, body, marker,
					"the legend's "+label+" count must be reproducible from this viewer's "+
						"own visible set — a breakdown counted pre-filter is differenceable "+
						"against the grid beside it")
			}
			if sum != len(visible) {
				t.Fatalf("the oracle's own per-type tallies sum to %d for a viewer who can "+
					"see %d events — the fixture, not the legend, is wrong", sum, len(visible))
			}

		})
	}
}

// TestOracle_LegendOmitsATypeTheViewerCannotSeeAtAll is the legend's half of
// r52 rule 3 — NOT EVEN A ZERO — and it needs its own fixture because the
// signed month gives every viewer at least one event of every type, which makes
// the claim unfalsifiable there.
//
// So this hides a WHOLE TYPE: every Festival in the month becomes dm_only. The
// GM's legend still lists Festivals; a player's must not list them at all —
// not "Festival 0", not a greyed swatch, not a tooltip. "Festival 0" beside a
// grid with no festival marks tells a player that festivals exist this month
// and none of them are theirs to know, which is the single most differenceable
// shape a breakdown can take.
func TestOracle_LegendOmitsATypeTheViewerCannotSeeAtAll(t *testing.T) {
	cal, events := oracleLedgerFixture()
	for i := range events {
		if events[i].Category != nil && *events[i].Category == "festival" {
			events[i].Visibility = "dm_only"
		}
	}

	renderFor := func(v BlockViewer) string {
		d := projectBlock(BlockProjectionInput{
			Calendar: cal, Events: blockCopyEvents(events),
			Viewer: v, MonthIndex: 0, Year: 1523,
		})
		d.Layers.Enabled = []string{"moons", "legend"}
		return seamRenderBlockData(t, d)
	}

	gm := renderFor(BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner})
	seamContain(t, gm, "Festival <span",
		"the GM can see every festival, so their legend must list the type")

	for _, v := range []BlockViewer{
		{UserID: "u-nissa", Role: permissions.RolePlayer},
		{UserID: "u-bryn", Role: permissions.RolePlayer},
	} {
		body := renderFor(v)
		if strings.Contains(body, "Festival") {
			t.Errorf("%s's legend names a type whose every event is hidden from them — "+
				"permission is ABSENCE, and not even a zero (r52 rule 3)", v.UserID)
		}
	}
}

// ── the DAY CARD joins the oracle (C-CALV4-DAYCARD, R2-2a, [DC-2] SIGNED) ───

// TestOracle_TheDayCardListsExactlyWhatTheLadderLeavesVisible is THE AGREEMENT
// LAW, and it is here rather than in a suite of its own because [DC-2] says so
// in as many words: joined to the count oracle, never forked.
//
// THE LAW, verbatim: *for any day, the set of events the card lists is EXACTLY
// the set of .lrow elements the ladder leaves visible for that day. Not a
// superset, not a subset, not "close enough."*
//
// WHY IT IS A GO ASSERTION AND NOT A JS ONE. The card's payload and the
// Ledger's rows are two serialisations of ONE viewer-filtered pass, and the
// only place both exist at once is here, at the producer. Asserted in JS it
// would be a claim about DOM that, for a viewer with the `ledger` layer off, is
// not there at all — and that is precisely the viewer the card exists for.
//
// WHAT A FAILURE WOULD MEAN. A card listing one MORE event than the Ledger is a
// permission leak wearing a UI change's clothes: the extra row is, by
// construction, an event this viewer's filter removed. A card listing one FEWER
// is the operator's complaint returning in a new place. Neither is cosmetic.
//
// THE LADDER'S OWN MECHANISM IS WHAT "VISIBLE FOR THAT DAY" MEANS. Choosing day
// N takes every `.lrows .lrow` that is not day N out of the flow (helpers.go's
// answerLadderCSS rule 1), so the surviving set is exactly the rows carrying
// data-lday="N" inside the Ledger zone. That is what is compared, per day, per
// viewer, against the payload the card would open with.
func TestOracle_TheDayCardListsExactlyWhatTheLadderLeavesVisible(t *testing.T) {
	cal, events := oracleLedgerFixture()

	for _, w := range oracleViewers() {
		t.Run(w.name, func(t *testing.T) {
			d := projectBlock(BlockProjectionInput{
				Calendar: cal, Events: blockCopyEvents(events),
				Viewer: w.v, MonthIndex: 0, Year: 1523,
			})
			d.Layers = benchBlockLayers(blockLayerPrefs{})
			body := seamRenderBlockData(t, d)

			// The LADDER's answer: rows inside the Ledger zone, by day. Scoped
			// to the zone because the Shelf's Upcoming panel reuses the SAME
			// row primitive, and an unscoped read would count a subset of these
			// rows twice (the C-CALV4-SHELF-P7 lesson, one layer over).
			ladder := map[string]map[string]bool{}
			for _, m := range oracleLedgerRowRe.FindAllStringSubmatch(oracleZone(t, body, "ledger"), -1) {
				day, id := m[1], m[2]
				if ladder[day] == nil {
					ladder[day] = map[string]bool{}
				}
				ladder[day][id] = true
			}
			if len(ladder) == 0 {
				t.Fatal("the Ledger listed no rows at all; the comparison below is vacuous")
			}

			// The CARD's answer, from the payload the Bench would emit.
			card := buildDayCardCalendar(d)
			seen := map[string]bool{}
			for _, day := range card.Days {
				ids := map[string]bool{}
				for _, ev := range day.Events {
					ids[ev.ID] = true
				}
				seen[day.Ord] = true
				want := ladder[day.Ord]
				if want == nil {
					want = map[string]bool{}
				}
				for id := range ids {
					if !want[id] {
						t.Errorf("day %s: the card lists event %s and the ladder does not — "+
							"a card that shows one more event than the Ledger is a permission "+
							"leak wearing a UI change's clothes", day.Ord, id)
					}
				}
				for id := range want {
					if !ids[id] {
						t.Errorf("day %s: the ladder leaves event %s visible and the card "+
							"omits it — the two answers disagree", day.Ord, id)
					}
				}
			}
			// …and no day the ladder answers is missing from the card entirely.
			for day := range ladder {
				if !seen[day] {
					t.Errorf("day %s has Ledger rows and no card entry at all", day)
				}
			}
		})
	}
}

// oracleLedgerRowRe reads (data-lday, data-event-id) off one rendered Ledger
// row. The two attributes are adjacent in ledgerRow's own emission order
// (ledger.templ:157-165) and the pattern tolerates the attributes between them
// so a templ reformat cannot quietly stop matching — a regex that stopped
// matching would make the assertion above pass by finding nothing, which is why
// the caller fails loudly on an empty result.
var oracleLedgerRowRe = regexp.MustCompile(`data-lday="([^"]+)"[^>]*?data-event-id="([^"]+)"`)

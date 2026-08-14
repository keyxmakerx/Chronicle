// sky_test.go — the sky header's RENDER contract, and the placeholder's
// retirement. C-CALV4-SKY (R2-5): [SKY-1], [SKY-6], [SKY-9], [SKY-11], [SKY-16].
//
// THE FIRST GUARD IN THIS FILE IS TWO-DIRECTIONAL AND THAT IS THE WHOLE POINT
// ([SKY-11] SIGNED). Before this slice, NOTHING in the entire battery named the
// dashed `.skyband` placeholder — a repo-wide grep for its copy and its
// function name across every *_test.go returned zero — so the band could have
// been deleted, no sky shipped in its place, and every suite would have stayed
// green. A guard that only proved the ABSENCE would reproduce that hole exactly.
// So: no template emits `class="skyband"` AND calendar-block.css declares no
// `.skyband` rule AND the seated Block's rendered output carries the sky's own
// summary element.
package calendar_block

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fxSky is the Almanac fixture, seated — i.e. what the producer hands the
// Bench's PRIMARY Block. The two sky fields are the seat gate, so setting them
// is what makes this a Block with a sky rather than a Block without one.
func fxSky(t *testing.T, gm bool) BlockData {
	t.Helper()
	d := fxAlmanac(t, gm)
	d.SkyGradient = "linear-gradient(180deg, oklch(0.62 0.16 60) 0%, oklch(0.38 0.12 350) 100%)"
	d.SkyClock = "19:42"
	return d
}

// --- [SKY-11]: the two-directional retirement guard --------------------------

func TestSky_ThePlaceholderIsGoneAndTheSkyArrived(t *testing.T) {
	// (a) ABSENCE — no template in this package emits the retired class, and
	// the sheet declares no rule for it. Both halves are needed: a rule with no
	// markup is dead CSS, and markup with no rule is the #568 gap.
	//
	// The schedule page's own unrelated `.skyband` (schedule_views.templ /
	// calendar-schedule.css) and bench_section_prefs_test.go's canonical
	// RETIRED key are deliberately out of this walk's scope — this guard is
	// about the Block's dashed idiom, and widening it to the repository would
	// red two files that were never part of it.
	for _, dir := range []string{".", filepath.Join("..", "..", "..", "static", "css")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || (!strings.HasSuffix(name, ".templ") && name != "calendar-block.css") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // test-local walk
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			body := string(src)
			// The class as MARKUP and as a RULE. Prose that explains the
			// retirement is not the thing being banned — a comment naming what
			// was deleted is how the next hand learns it was deliberate.
			for _, bad := range []string{`class="skyband"`, ".skyband {", ".skyband,", ".skyband ."} {
				if strings.Contains(body, bad) {
					t.Errorf("%s still carries %q — the ONE dashed idiom in the product "+
						"retires WITH the thing it reserved a seat for ([SKY-11]); it is "+
						"deleted, not relocated and not hidden behind a flag", name, bad)
				}
			}
		}
	}

	// (b) PRESENCE — the seated Block really does render a sky. This is the
	// half that would have been missing, and it is the half that fails on a
	// slice which deleted the band and shipped nothing.
	body := flatten(render(t, fxSky(t, true)))
	for _, want := range []string{
		`<details class="skygrow" data-cal-sky`,
		`<summary class="skyhdr"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Primary Block's output does not contain %q — the placeholder "+
				"was retired and nothing arrived in its place", want)
		}
	}
}

// --- [SKY-1]: one sky per surface -------------------------------------------

// TestSky_SeatsOnlyWhereTheProducerPutIt is the renderer's half of [SKY-1]. The
// gate is the producer's answer, so a Block with no sky fields renders no band
// AND SAYS NOTHING ABOUT ONE — no reserved strip, no "no sky here" note, no
// empty box. Absence is the honesty mechanism, on the Almanac tab's precedent.
func TestSky_SeatsOnlyWhereTheProducerPutIt(t *testing.T) {
	d := fxAlmanac(t, true) // no SkyGradient, no SkyClock: the neighbour Block
	body := flatten(render(t, d))
	for _, gone := range []string{"skygrow", "data-cal-sky", "skyhdr", "Sky and almanac"} {
		if strings.Contains(body, gone) {
			t.Errorf("a Block the producer gave no sky rendered %q — [SKY-1] is one sky "+
				"per surface, and the real-world Block, the subordinate rows and the "+
				"Ribbon carry no sky and no reserved band", gone)
		}
	}
}

// --- [SKY-6]: three facts, no fourth, and none invented ---------------------

// TestSky_TheClosedStripStatesThreeFacts walks the SUMMARY's own subtree, which
// is the closed strip. Three facts and no fourth, each dropped entirely rather
// than blanked when its source is absent.
//
// AMENDED — C-CALV4-SKY-CHEST stage 2. Fact 1 is still HERE and is still the
// summary's own subtree: the moons cannot leave the <summary> (a non-summary
// child of <details> is hidden while closed). What changed is that they are no
// longer a SLOT in the lid's flex row — `.skscene` is out-of-flow scenery
// painted onto the sky. The class name moved with the meaning, and this guard
// moved with it rather than being relaxed to "some moon markup, anywhere".
func TestSky_TheClosedStripStatesThreeFacts(t *testing.T) {
	band := skyTestSummary(t, render(t, fxSky(t, true)))

	for _, want := range []string{
		`class="skscene"`,  // fact 1 · the phases, painted into the sky
		`class="sktime"`,   // fact 2 · the in-world time
		`19:42`,            //          …as of render, real text
		`class="skseason"`, // fact 3 · the season word
		`Long Night`,
	} {
		if !strings.Contains(band, want) {
			t.Errorf("the closed strip is missing %q", want)
		}
	}
	// The caret is the affordance mark, NOT a fourth fact — and it is
	// aria-hidden because it says nothing a reader needs said twice.
	if !strings.Contains(band, `class="skcaret" aria-hidden="true"`) {
		t.Error("the closed strip carries no affordance mark; a summary with no visible " +
			"\"there is more here\" cue is the register's clause-5 failure")
	}
	// NO SUNRISE AND NO SUNSET, IN ANY FORM ([SKY-6]) — and this is asserted on
	// the WHOLE render, not only the band, because the mock's open pane leads
	// with "Sunset 19:58" and that is the one still this build cannot reproduce
	// under the guards.
	whole := strings.ToLower(render(t, fxSky(t, true)))
	for _, bad := range []string{"sunrise", "sunset"} {
		if strings.Contains(whole, bad) {
			t.Errorf("the sky renders %q — Chronicle does not persist a daylight "+
				"boundary and inventing one is the defect WorldStateSun.Tint already "+
				"refuses by shipping null", bad)
		}
	}
}

// TestSky_AbsentFactsAreDroppedRatherThanBlanked is the other half of [SKY-6]'s
// "dropped entirely" clause: a calendar with no moons keeps its gradient, its
// clock and its season word, renders NO discs, and its expansion offers the
// current sky only — no Tonight/Month/Moons trio, because a control whose panel
// has nothing to draw is inert and absence is the honest answer.
func TestSky_AbsentFactsAreDroppedRatherThanBlanked(t *testing.T) {
	d := fxSky(t, true)
	d.Month.Almanac = nil
	d.SeasonLabel = ""
	body := flatten(render(t, d))

	if !strings.Contains(body, `class="skyhdr"`) || !strings.Contains(body, "19:42") {
		t.Fatal("a moonless, seasonless calendar lost its sky entirely")
	}
	for _, gone := range []string{`class="skscene"`, `class="skseason"`, `class="sktabs"`} {
		if strings.Contains(body, gone) {
			t.Errorf("the strip rendered %q with nothing behind it — an absent fact is "+
				"DROPPED, never blanked, on the Nameplate's own blockSeasonEraLabels idiom",
				gone)
		}
	}
}

// --- [SKY-9]: count 3, taken from the rendered markup ------------------------

// TestSky_TheOpenPaneCarriesAtMostThreeControls counts INTERACTIVE ELEMENTS in
// the rendered output rather than arguing about them. The disclosure affordance
// is the band itself and does not count against the three, which is exactly why
// the band IS the summary.
//
// A count that is argued rather than measured is the count failing ([SKY-15]).
func TestSky_TheOpenPaneCarriesAtMostThreeControls(t *testing.T) {
	sky := skyTestDetails(t, render(t, fxSky(t, true)))

	controls := regexp.MustCompile(`<(input|button|a|select|textarea)[ >]`).FindAllString(sky, -1)
	if len(controls) > 3 {
		t.Errorf("the sky carries %d interactive controls (%v), want at most 3 — the "+
			"Tonight/Month/Moons trio and nothing else. No Sync now, no moon-style "+
			"picker, no weather, no mood tint, no time verbs", len(controls), controls)
	}
	if len(controls) != 3 {
		t.Errorf("the sky carries %d controls, want exactly the trio — a count that "+
			"drops is the trio quietly disappearing", len(controls))
	}
	for _, want := range []string{">Tonight<", ">Month<", ">Moons<"} {
		if !strings.Contains(flatten(sky), want) {
			t.Errorf("the trio is missing %q", want)
		}
	}
	// The Nameplate already states sync; a second sync control on one Block is
	// count 3 repeating itself.
	for _, bad := range []string{"Sync now", "moonstyle", "mood", "weather"} {
		if strings.Contains(strings.ToLower(sky), strings.ToLower(bad)) {
			t.Errorf("the sky carries %q — the switchboard already owns preferences", bad)
		}
	}
	// The radio group is DISTINCT from the Shelf's Almanac trio on the same
	// Block. One shared name and the two panels fight over one selection.
	if strings.Contains(sky, `name="alm-`) {
		t.Error("the sky's tabs share the Almanac panel's radio group name — the Shelf " +
			"renders the same three keys on this Block and the two would collide")
	}
}

// --- [SKY-16]: the accessible surface ---------------------------------------

func TestSky_IsARealDisclosureWithAnAccessibleName(t *testing.T) {
	sky := skyTestDetails(t, render(t, fxSky(t, true)))

	if strings.Contains(sky, `aria-hidden="true"><summary`) || strings.Contains(sky, `<details class="skygrow" aria-hidden`) {
		t.Error("the sky is aria-hidden — that was correct for a dashed placeholder and " +
			"is wrong for a header carrying the in-world time, the season and the phases")
	}
	for _, bad := range []string{`role="button"`, "tabindex"} {
		if strings.Contains(sky, bad) {
			t.Errorf("the sky hand-rolls %q — <details>/<summary> gives native disclosure "+
				"semantics, native keyboard operation and aria-expanded for free, and "+
				"this slice must not add a second pointer-only Block surface", bad)
		}
	}
	label := skyTestAttr(t, sky, `<summary class="skyhdr" aria-label="`)
	if !strings.HasPrefix(label, "Sky and almanac") {
		t.Errorf("the header's accessible name is %q, want it to begin \"Sky and almanac\"", label)
	}
	// EVERY FACT IS IN THE NAME, not only in pixels. An aria-label REPLACES the
	// name computed from the subtree, so a bare four-word label would announce
	// the header and silently drop the clock, the season and every phase — the
	// mock's own construction, and the reason it is not ported verbatim.
	for _, fact := range []string{"19:42", "Long Night", "Alder", "waxing gibbous"} {
		if !strings.Contains(label, fact) {
			t.Errorf("the accessible name %q drops the fact %q — nothing on this surface "+
				"may be conveyed by colour or gradient alone", label, fact)
		}
	}
	// The discs carry their phase word as a label too, so hovering one says
	// what it is rather than requiring the reader to decode a shape.
	if !strings.Contains(sky, `title="Alder 68% waxing gibbous"`) {
		t.Error("a phase disc carries no phase word — identity is shape and fill and a " +
			"stated word, never hue and never shape alone")
	}
}

// TestSky_ThePhaseWordSurvivesTheMoveIntoTheSky is C-CALV4-SKY-CHEST stage 2's
// own guard, and it is written because the move had one way to go quietly
// wrong: the moons stopped being a seated fact in the lid's row, and a slice
// that also dropped their `title` would have left the phase stated NOWHERE a
// reader could reach without opening the chest.
//
// THREE INDEPENDENT STATEMENTS, ASSERTED AS THREE, because any one of them
// alone is one refactor from silence:
//
//	1. the summary's ACCESSIBLE NAME (skyLabel) names every declared body and
//	   its phase — this is the one a screen reader gets, closed;
//	2. the scenery's per-body `title` — the sighted reader's, and it is the
//	   one the move could most easily have dropped;
//	3. the open pane's Tonight row — running text, four aligned columns.
//
// It is deliberately NOT satisfied by the shape of the disc. The house rule is
// "shape and fill AND a stated word, never shape alone".
func TestSky_ThePhaseWordSurvivesTheMoveIntoTheSky(t *testing.T) {
	d := fxSky(t, true)
	body := render(t, d)
	sky := skyTestDetails(t, body)
	band := skyTestSummary(t, body)

	const phase = "waxing gibbous"

	// (1) the accessible name, read off the CLOSED band.
	label := skyTestAttr(t, sky, `<summary class="skyhdr" aria-label="`)
	if !strings.Contains(label, phase) {
		t.Errorf("the closed lid's accessible name %q no longer states a phase — the "+
			"moons left the lid's content row and the name is what carries them now", label)
	}

	// (2) the scenery's own per-body title, INSIDE the summary (i.e. reachable
	// while the chest is closed), not merely somewhere in the pane.
	if !strings.Contains(band, `title="Alder 68% `+phase+`"`) {
		t.Error("the sky's scenery carries no per-body `title` — moving the moons out " +
			"of the lid's row must not also move their only sighted statement out of " +
			"the closed state")
	}

	// (3) the pane's running text, which is where an operator who wants the
	// detail goes ("we have the like menu for the moons and such anyways").
	if !strings.Contains(skyTightened(sky), `<span class="ph">`+phase+`</span>`) {
		t.Errorf("the open pane's Tonight row no longer prints %q as running text", phase)
	}
}

// --- [SKY-10]: it remembers nothing -----------------------------------------

// TestSky_DefaultsClosedAndRemembersNothing. "Compact is the resting state" is
// the directive's own sentence, and clause 5's persistence requirement is
// scoped to REMEMBERED disclosures, which this is not.
func TestSky_DefaultsClosedAndRemembersNothing(t *testing.T) {
	sky := skyTestDetails(t, render(t, fxSky(t, true)))
	if strings.Contains(sky, "<details class=\"skygrow\" data-cal-sky open") ||
		strings.Contains(sky, " open>") || strings.Contains(sky, " open ") {
		t.Error("the sky renders OPEN — it defaults closed at every width for every " +
			"viewer, on every load")
	}
	// Never the retired noun, as a class, a key or a data attribute.
	if strings.Contains(sky, "skyband") {
		t.Error("the sky names \"skyband\" — that literal is the canonical RETIRED " +
			"Bench-section key and re-minting it as a live noun would invert two " +
			"shipped tests for a cosmetic reason ([SKY-10])")
	}
}

// --- [SKY-5]: the two divergences from the signed stills, closed ------------

// TestSky_TheOpenPaneCarriesTheSignedSubHead is the FIRST of the two fidelity
// gaps the adversarial verifier found, and it is written two-directionally
// because a presence-only assertion would have gone green against the shipped
// build that had the line in the WRONG PLACE saying a DIFFERENT THING.
//
// The signed still (mockups/stills/sky-header/sky-open-1440.png) renders a
// muted line DIRECTLY UNDER the bold head and ABOVE the trio. The first pass
// emitted no such line and instead closed the Tonight panel with
// `almanacLitLine`'s >25%-lit summary — a different sentence in a different
// position. So this guard asserts the position (between `.skhead` and
// `.sktabs`), the wording, AND that the lit-line no longer appears in the sky
// while it still appears in the Shelf's own Almanac panel, which is r53's and
// was never in this slice's bounds.
func TestSky_TheOpenPaneCarriesTheSignedSubHead(t *testing.T) {
	body := render(t, fxSky(t, true))
	sky := skyTestDetails(t, body)

	// (a) THE SENTENCE. On the signed fixture, anchor 14: Alder's next turn at
	// or after the anchor is its full on day 19, and it is the soonest of the
	// four declared bodies — every other moon's only turn this month is already
	// behind day 14 and does NOT get restated as a negative distance.
	const want = `<div class="skynote">Alder turns full in 5 days</div>`
	if !strings.Contains(sky, want) {
		t.Errorf("the open pane is missing the stills' muted sub-head %q", want)
	}

	// (b) THE POSITION. Under the head, above the trio — a muted line below the
	// tabs is a footnote, which is what shipped and what this replaces.
	head := strings.Index(sky, `class="skhead"`)
	sub := strings.Index(sky, want)
	tabs := strings.Index(sky, `class="sktabs"`)
	if head < 0 || tabs < 0 {
		t.Fatal("the open pane lost its head or its trio")
	}
	if head >= sub || sub >= tabs {
		t.Errorf("the sub-head sits at %d; it belongs between the head (%d) and the "+
			"trio (%d) — the stills put it in the header stack, not in a footnote",
			sub, head, tabs)
	}

	// (c) THE HALF THAT CANNOT SHIP, ASSERTED AS ABSENT. `calendar.Moon` has no
	// eclipse-node column, so the stills' "Umber in shadow, days 17-21" segment
	// has no source. It is DROPPED, not approximated — and an "in shadow"
	// appearing here later would mean someone derived a shadow window from
	// cycle length, which is the sunrise defect wearing different words.
	if strings.Contains(strings.ToLower(sky), "in shadow") {
		t.Error("the sky states a shadow window — Moon carries no node, no inclination " +
			"and no ascending-node column, so any such window was invented ([SKY-6]'s " +
			"reasoning, applied to the segment [SKY-5] cannot reproduce)")
	}

	// (d) THE LIT-LINE MOVED OUT OF THE SKY AND STAYED IN THE REGISTER. Both
	// halves: absent here, present there. Absence alone would go green if the
	// Shelf's Almanac panel were deleted too.
	lit := almanacLitLine(fxSky(t, true), 14)
	if lit == "" {
		t.Fatal("the fixture produces no lit-line; this guard would be vacuous")
	}
	if strings.Contains(sky, lit) {
		t.Errorf("the sky's Tonight panel still closes with %q — the stills have no such "+
			"line, and the four columns now print the percentages the threshold used "+
			"to summarise", lit)
	}
	if !strings.Contains(almZone(t, body), lit) {
		t.Errorf("the Shelf's Almanac panel lost %q — r53's register was never in this "+
			"slice's bounds and moving the sky's copy must not have touched it", lit)
	}
}

// TestSky_TheTonightRowIsFourAlignedColumns is the SECOND fidelity gap. The
// stills show `ALDER | waxing crescent | 33% | full in 10 days` — four columns,
// the percentage right-aligned, the turn muted at the far right. The first pass
// shipped a two-item flex carrying r53's merged register sentence
// ("33% waxing crescent · next full 10"): different order, different wording,
// no unit on the turn, no column alignment.
func TestSky_TheTonightRowIsFourAlignedColumns(t *testing.T) {
	body := render(t, fxSky(t, true))
	i := strings.Index(body, `data-cal-sky-panel="tonight"`)
	if i < 0 {
		t.Fatal("no Tonight panel in the render")
	}
	pane := body[i:]
	if j := strings.Index(pane, `data-cal-sky-panel="month"`); j > 0 {
		pane = pane[:j]
	}

	// (a) THE COLUMNS, IN ORDER, WITH THE SIGNED FIXTURE'S OWN FIGURES.
	const alder = `<span class="nm">Alder</span>` +
		`<span class="ph">waxing gibbous</span>` +
		`<span class="il">68%</span>` +
		`<span class="nx">full in 5 days</span>`
	if !strings.Contains(skyTightened(pane), alder) {
		t.Errorf("the Tonight row is not the stills' four columns; wanted %q", alder)
	}

	// (b) THE MERGED SENTENCE IS GONE FROM THIS PANEL. `almanacMoonLine` is
	// still correct for the Shelf's register and is asserted present there by
	// almanac_test.go — here it is the shape the stills reject.
	if merged := almanacMoonLine(fxSky(t, true).Month.Almanac[0], 14); strings.Contains(pane, merged) {
		t.Errorf("the Tonight row still carries the register's merged sentence %q", merged)
	}

	// (c) A WRAPPED TURN DROPS ITS COLUMN RATHER THAN PRINTING A NEGATIVE
	// DISTANCE. Umber's only turn this month is its full on day 11, behind the
	// anchor at 14; almanacNextTurn wraps to it and "in -3 days" is not a fact.
	if !strings.Contains(skyTightened(pane), `<span class="nm">Umber</span>`+
		`<span class="ph">waning gibbous</span>`+
		`<span class="il">68%</span>`+
		`<span class="nx"></span>`) {
		t.Error("Umber's row does not drop its turn column — its only turn this month " +
			"is already behind the anchor and a wrapped day cannot be stated as a " +
			"distance")
	}
	if strings.Contains(pane, "in -") {
		t.Error("a turn column printed a negative distance")
	}
}

// TestSky_TheHeadAndSubHeadAreQuotable exists because THREE separate hands have
// now quoted the open pane's head line from memory and TWO got it wrong — once
// naming the season word as the month ("day 14 of Emberfall"; Emberfall is the
// SEASON on the closed strip, the month is Hearthwane in the still), and once
// substituting the MOONS BADGE's sentence ("N moons declared; the grid draws M",
// `helpers.go:843`) for the head line's.
//
// Both substrings were already pinned, in two other files, on two other
// surfaces. What was missing was ONE named place that prints the WHOLE line, so
// a report can quote a test instead of a recollection. That is all this is.
func TestSky_TheHeadAndSubHeadAreQuotable(t *testing.T) {
	d := fxSky(t, true)
	anchor := almanacAnchorDay(d)

	const wantHead = "Day 14 of Deepwinter · 4 moons declared · Flint keeps the month"
	const wantSub = "Alder turns full in 5 days"

	if got := skyHeadLine(d, anchor); got != wantHead {
		t.Errorf("skyHeadLine = %q, want %q", got, wantHead)
	}
	if got := skySubHead(d, anchor); got != wantSub {
		t.Errorf("skySubHead = %q, want %q", got, wantSub)
	}
	// Composition, stated so a change to either half fails HERE rather than
	// silently re-shaping a sentence three books quote.
	if !strings.HasPrefix(wantHead, "Day "+strconv.Itoa(anchor)+" of "+d.Month.Name) {
		t.Error("the head line no longer opens with the day and the MONTH")
	}
	if !strings.HasSuffix(wantHead, almanacSkyLine(d)) {
		t.Error("the head line no longer closes with almanacSkyLine — if the moons " +
			"badge's wording has been substituted here, that is the exact confusion " +
			"this guard is named for")
	}
}

// TestSky_InDaysStatesTheDistanceCorrectly pins the three branches the fixture
// cannot all reach, including the singular the mock's own head line gets wrong.
func TestSky_InDaysStatesTheDistanceCorrectly(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{
		{-2, "tonight"}, {0, "tonight"}, {1, "in 1 day"}, {2, "in 2 days"}, {10, "in 10 days"},
	} {
		if got := almanacInDays(tc.n); got != tc.want {
			t.Errorf("almanacInDays(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// --- small readers ----------------------------------------------------------

// skyTightened collapses the inter-tag whitespace templ emits from the
// template's own line breaks, so a column-order assertion reads as the markup
// rather than as a spacing accident. It touches only whitespace BETWEEN tags —
// text content is left alone, so a lost word still reds the guard.
var skyTagGapRe = regexp.MustCompile(`>\s+<`)

func skyTightened(s string) string { return skyTagGapRe.ReplaceAllString(s, "><") }

// skyTestDetails returns the sky's <details> subtree, or fails. It slices on
// LOCATED bounds rather than on a bare strings.Index result, which would panic
// on a rename instead of failing cleanly (COMMON §3).
func skyTestDetails(t *testing.T, body string) string {
	t.Helper()
	const open = `<details class="skygrow"`
	i := strings.Index(body, open)
	if i < 0 {
		t.Fatal("no sky in the rendered Block")
	}
	rest := body[i:]
	j := strings.Index(rest, "</details>")
	if j < 0 {
		t.Fatal("the sky's <details> is never closed")
	}
	return rest[:j+len("</details>")]
}

// skyTestSummary returns the closed strip — the <summary> subtree.
func skyTestSummary(t *testing.T, body string) string {
	t.Helper()
	sky := skyTestDetails(t, body)
	i := strings.Index(sky, "<summary")
	j := strings.Index(sky, "</summary>")
	if i < 0 || j < i {
		t.Fatal("the sky has no <summary> — the band IS the disclosure")
	}
	return sky[i : j+len("</summary>")]
}

// skyTestAttr reads one attribute value that follows a located prefix.
func skyTestAttr(t *testing.T, body, prefix string) string {
	t.Helper()
	i := strings.Index(body, prefix)
	if i < 0 {
		t.Fatalf("no %q in the rendered sky", prefix)
	}
	rest := body[i+len(prefix):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatalf("unterminated attribute after %q", prefix)
	}
	return rest[:j]
}

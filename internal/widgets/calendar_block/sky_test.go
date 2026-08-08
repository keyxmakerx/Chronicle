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
func TestSky_TheClosedStripStatesThreeFacts(t *testing.T) {
	band := skyTestSummary(t, render(t, fxSky(t, true)))

	for _, want := range []string{
		`class="skdiscs"`,  // fact 1 · the phase discs
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
	for _, gone := range []string{`class="skdiscs"`, `class="skseason"`, `class="sktabs"`} {
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

// --- small readers ----------------------------------------------------------

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

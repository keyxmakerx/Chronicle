// moonpanel_test.go — the render-level guards for C-CALV4-MOONS.
//
// MN-G3 · MN-G4 · MN-G9 · MN-G10 · MN-G11 · MN-G14, plus the accessibility
// claim that is the whole reason the cluster could become a control at all.
// The browser claims (MN-G1/G2 in sky_scenery_probe_test.go; MN-G5/G6/G7/G12 in
// moonpanel_probe_test.go) are elsewhere and are not repeated here.
package calendar_block

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ── MN-G3 · the ceiling is THREE and it is declared ONCE ────────────────────

// TestMoonPanel_TheCeilingIsThreeAndItIsDeclaredOnce is [MN-1], signed by the
// operator on 2026-08-10 in these words: "3 is fine, i'm good with that."
//
// THE NUMBER HAD THREE INDEPENDENT ARTIFACTS BEHIND IT and the operator's
// recollection of four had none: `SKY_MAX = 3` in the signed mock, `moonCap = 3`
// in shipped Go, and the chip printed `3 OF 4 MOONS` in v4-moons-words.png and
// v4-sky-almanac-moons.png. This test pins the shipped one and the "declared
// once" half together, because a second copy of the arithmetic is exactly how
// the Nameplate badge's total went wrong before (block_geometry.go:385).
func TestMoonPanel_TheCeilingIsThreeAndItIsDeclaredOnce(t *testing.T) {
	if moonCap != 3 {
		t.Fatalf("moonCap = %d, want 3. [MN-1] is signed at three by the operator, and it "+
			"is the only number with an artifact behind it — a constant in the signed "+
			"mock, a constant in shipped Go, and the chip in two renders", moonCap)
	}
	// The literal, once. `moonsFor`, `moonsBadgeText` and `moonsBadgeTitle` all
	// read the constant rather than restating it.
	src, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("read helpers.go: %v", err)
	}
	if n := strings.Count(string(src), "moonCap = 3"); n != 1 {
		t.Errorf("`moonCap = 3` appears %d times in helpers.go — L30's whole rule is that "+
			"the ceiling is declared ONCE; a second copy is a second thing to forget", n)
	}
}

// ── MN-G4 · no per-cell "+N", in any state ──────────────────────────────────

// TestMoonPanel_NoCellCarriesAPlusAboutMoons is the ruling three renders, one
// mock comment and one shipped header all make.
//
// `v4-moons-phases-only.png` and `v4-sky-moons-capped.png` DID draw a per-cell
// `+1`, on all thirty cells. They are superseded: the mock's own `skyInCell()`
// note calls it "a marker repeated in every cell was the noisiest thing on the
// surface" and removes it, `cells-zoom.png` and `v4-moons-both.png` draw three
// discs and no `+1`, and moongraph.templ:23 states it as a ruling. The ceiling
// is the Nameplate's chip and the destination is the graph's footnote.
//
// IT WALKS THE CLUSTER'S SUBTREE, NOT THE CELL. A cell legitimately carries
// `+4 more` for its events — that is the chip overflow and it has nothing to do
// with moons. A guard that banned `+` from the cell would be banning the wrong
// thing and would have to be weakened the first time it fired.
func TestMoonPanel_NoCellCarriesAPlusAboutMoons(t *testing.T) {
	body := flatten(render(t, fxAlmanac(t, true)))
	clusters := regexp.MustCompile(`(?s)<label class="phrow phctl".*?</label>`).FindAllString(body, -1)
	if len(clusters) == 0 {
		t.Fatal("no disc cluster rendered as a control — the guard has no subject and " +
			"would pass vacuously")
	}
	for i, c := range clusters {
		if strings.Contains(c, "+") {
			t.Errorf("disc cluster %d carries a `+`:\n%s\nThe ceiling is declared ONCE, in "+
				"the Nameplate — never thirty times in the grid (L30, [MN-1])", i, c)
		}
	}
	// AND THE CHIP STILL SAYS IT, once, where it belongs. An absence guard with
	// no presence twin would stay green on a slice that deleted both.
	if got := moonsBadgeText(fxAlmanac(t, true)); got != "3 of 4 moons" {
		t.Errorf("the Nameplate chip reads %q, want %q — the `+` the operator remembers is "+
			"real and this is where it lives", got, "3 of 4 moons")
	}
}

// ── MN-G9 · no composite, MN-G10 · the two caps, MN-G11 · no epithet ────────

// TestMoonPanel_TheGraphIsCappedAndTheDetailsAreNot is the asymmetry that makes
// the grid's ceiling legitimate rather than a truncation.
//
// The graph draws the `Drawn` bodies and says "+1 more in the almanac" — which
// names a DESTINATION, not a ceiling — and the destination is real because the
// Details tab lists every declared body, Sable included. On the 4-moon fixture
// that is 3 lanes and 4 rows, and neither number may quietly become the other.
func TestMoonPanel_TheGraphIsCappedAndTheDetailsAreNot(t *testing.T) {
	d := fxAlmanac(t, true)
	body := flatten(render(t, d))

	graph := mpPane(t, body, "graph")
	details := mpPane(t, body, "details")

	// MN-G10a — the graph draws exactly the drawn bodies, one lane each.
	if n := strings.Count(graph, `<div class="mprow">`); n != len(graphMoons(d)) {
		t.Errorf("the Graph tab draws %d lanes, want %d (the Drawn bodies)", n, len(graphMoons(d)))
	}
	if strings.Contains(graph, ">Sable<") {
		t.Error("the Graph tab draws Sable — it is past the ceiling and belongs in the " +
			"Details tab, which is where the footnote points")
	}
	if !strings.Contains(graph, "+1 more in the almanac") {
		t.Error("the Graph tab must carry `+1 more in the almanac`. It is permitted where a " +
			"per-cell `+1` is not, because it names a DESTINATION rather than a ceiling — " +
			"and the destination has to be real")
	}

	// MN-G10b — the details are uncapped. This is the whole reason the ceiling
	// is legitimate: a ceiling with nowhere to go is a truncation.
	if n := strings.Count(details, `<div class="mpdrow">`); n != len(d.Month.Almanac) {
		t.Errorf("the Details tab lists %d bodies, want %d — it is UNCAPPED and the fourth "+
			"moon appears here and only here", n, len(d.Month.Almanac))
	}
	if !strings.Contains(details, ">Sable<") {
		t.Error("the Details tab does not list Sable. The overflow body is reachable, " +
			"uncapped, in the register — v4-sky-almanac-moons.png draws exactly four rows")
	}

	// MN-G9 — no composite, in either tab, at any density. Three independent
	// attempts at a summed "how bright is tonight" figure disagreed about the
	// same five nights (L19/L24).
	for _, bad := range []string{"composite", "total illumination", "combined", "sky brightness"} {
		if strings.Contains(strings.ToLower(graph+details), bad) {
			t.Errorf("the panel carries %q — the composite brightness scalar is dead three "+
				"times over (L19/L24) and is not revived in the panel", bad)
		}
	}

	// MN-G11 — no epithet, and no field to hold one. The renders' "the great
	// pale moon" and "the far wanderer" are mock fixture text: calendar.Moon has
	// no such column and AlmanacMoon deliberately carries no field for it.
	for _, bad := range []string{"the great pale moon", "the dark moon", "the far wanderer", "small and fast"} {
		if strings.Contains(body, bad) {
			t.Errorf("the render prints the fixture epithet %q. Print no epithet rather "+
				"than inventing one (MN-G11)", bad)
		}
	}
	src, err := os.ReadFile("data.go")
	if err != nil {
		t.Fatalf("read data.go: %v", err)
	}
	// A DECLARATION, not the word. data.go's own doc comment says "There is
	// deliberately NO Epithet", and a guard that banned the noun would delete
	// the sentence recording the refusal — which is how the next hand learns it
	// was deliberate rather than forgotten.
	if regexp.MustCompile(`(?m)^\s*Epithet\s`).Match(src) {
		t.Error("data.go declares an `Epithet` field. AlmanacMoon carries none on purpose, " +
			"and a migration adding one would ship a dead column with no authoring " +
			"surface to fill it (MN-G11)")
	}
}

// ── MN-G14 · the layer owns everything, including the control ───────────────

// TestMoonPanel_MoonsLayerOffMeansNothingAtAll extends the existing assertion
// rather than replacing it. `v4-bare-no-moons.png` draws the layer off as an
// empty month: no discs, no chip, no graph, no Almanac tab — and now no panel,
// no radio and no hit target either. Absence is this product's vocabulary for
// "there is nothing here"; the surface does not draw a frame saying so.
func TestMoonPanel_MoonsLayerOffMeansNothingAtAll(t *testing.T) {
	d := fxAlmanac(t, true)
	d.Layers.Enabled = nil
	for _, k := range d.Layers.Enabled {
		_ = k
	}
	// Rebuild the on-set without `moons`, leaving every other key as it was.
	on := make([]string, 0, len(fxAlmanac(t, true).Layers.Enabled))
	for _, k := range fxAlmanac(t, true).Layers.Enabled {
		if k != "moons" {
			on = append(on, k)
		}
	}
	d.Layers.Enabled = on
	body := flatten(render(t, d))

	for _, marker := range []string{
		`class="phrow`, `class="moonpick`, `data-cal-moonpanel`, `data-cal-moons`,
		`data-moon-pick`, `data-mp-tab`,
	} {
		if strings.Contains(body, marker) {
			t.Errorf("the moons layer is off and %q is still in the DOM. A panel, a radio "+
				"or a hit target that outlives the layer that owns it is a control the "+
				"viewer switched off and still has (MN-G14)", marker)
		}
	}
	if got := moonsBadgeText(d); got != "" {
		t.Errorf("the Nameplate chip reads %q with the layer off, want empty", got)
	}
}

// ── the accessibility that makes it a control ───────────────────────────────

// TestMoonPanel_TheClusterIsNamedAndReachable is the half of "make it a control"
// that is easiest to skip and least forgivable to skip.
//
// A CONTROL THAT IS INVISIBLE TO ASSISTIVE TECHNOLOGY IS NOT A CONTROL. What
// shipped was `aria-hidden="true"` on the wrapper with a `title` on each disc:
// no name, no role, no tab stop. `aria-hidden` is right for decoration and
// wrong the moment the thing opens a panel, so the wrapper is a <label> for a
// real radio and the discs are the ones that go `aria-hidden` now.
func TestMoonPanel_TheClusterIsNamedAndReachable(t *testing.T) {
	d := fxAlmanac(t, true)
	body := flatten(render(t, d))

	// The wrapper is no longer hidden from the accessibility tree.
	if strings.Contains(body, `<span class="phrow" aria-hidden="true">`) {
		t.Error("the disc cluster still renders as an aria-hidden <span>. It opens a panel " +
			"now, and a control a screen reader is never told about is not a control")
	}
	// It is a label, it points at a radio that exists, and it carries a name.
	if !strings.Contains(body, `<label class="phrow phctl" data-cal-moons`) {
		t.Fatal("the cluster does not render as a labelled control")
	}
	forRe := regexp.MustCompile(`<label class="phrow phctl" data-cal-moons for="([^"]+)"`)
	ms := forRe.FindAllStringSubmatch(body, -1)
	if len(ms) == 0 {
		t.Fatal("no cluster carries a `for` — the label names nothing")
	}
	for _, m := range ms {
		if !strings.Contains(body, `id="`+m[1]+`"`) {
			t.Errorf("the cluster points at %q and no such input was emitted. A label "+
				"addressing a control that does not exist is a dead affordance, which is "+
				"the one outcome the honesty idiom rules out", m[1])
		}
	}
	// The name is the day and the bodies on it, not the word "moons".
	c := DayCell{Day: 14, Moons: []MoonDisc{
		{Name: "Alder", Illum: 0.04, Waxing: true},
		{Name: "Umber", Illum: 0.89, Waxing: true},
	}}
	if got, want := moonClusterLabel(c), "Moons on day 14 — Alder 4%, Umber 89%"; got != want {
		t.Errorf("moonClusterLabel = %q, want %q", got, want)
	}
	// The discs themselves are decoration INSIDE the control and say so.
	cluster := regexp.MustCompile(`(?s)<label class="phrow phctl".*?</label>`).FindString(body)
	discs := regexp.MustCompile(`<i class="ph[^>]*>`).FindAllString(cluster, -1)
	if len(discs) == 0 {
		t.Fatalf("the control holds no discs:\n%s", cluster)
	}
	for _, disc := range discs {
		if !strings.Contains(disc, `aria-hidden="true"`) {
			t.Errorf("a disc inside the control is not aria-hidden: %s\nThe control has ONE "+
				"name; its parts are decoration", disc)
		}
		if strings.Contains(disc, "title=") {
			t.Errorf("a disc inside the control keeps its own title: %s\nThree tooltips on "+
				"the parts of one button are three answers to one question — the cluster "+
				"carries the sentence now", disc)
		}
	}
	// …and the cluster itself carries it, so nothing was simply deleted.
	if !strings.Contains(cluster, `title="Alder`) {
		t.Errorf("the control carries no tooltip of its own:\n%s", cluster)
	}
}

// TestMoonPanel_TheControlIsAbsentWhereThePanelCannotBeBuilt is the honesty
// idiom, stated as a test: past dayPick's ladder bound the Block emits no
// radio, no label and no dead affordance, and the same rule governs here. A
// Block whose host removed the Shelf has no Almanac register, so both tabs
// would be empty — the cluster stays what it always was, decoration with a
// tooltip, rather than becoming a button that opens nothing.
func TestMoonPanel_TheControlIsAbsentWhereThePanelCannotBeBuilt(t *testing.T) {
	d := fxAlmanac(t, true)
	d.Month.Almanac = nil
	body := flatten(render(t, d))

	if strings.Contains(body, "phctl") || strings.Contains(body, "data-cal-moonpanel") {
		t.Error("with no Almanac register the cluster still renders as a control. A button " +
			"that opens two empty tabs is worse than a tooltip")
	}
	if !strings.Contains(body, `<span class="phrow" title=`) {
		t.Error("…and the discs must still DRAW. Losing the control is not losing the sky: " +
			"the moons layer is on and cells-zoom.png's three discs are the resting state")
	}
	if strings.Contains(body, `<span class="phrow" aria-hidden="true">`) {
		t.Error("the non-control cluster is `aria-hidden`, so every disc on this Block is " +
			"outside the accessibility tree and the per-disc `title`s inside it reach " +
			"nobody. That was tolerable while this branch was a corner case; it is now " +
			"the Bench's real-world Block ([SKY-1] renders it noShelf + sky=false, which " +
			"empties the Almanac register) and the real-Moon fallback puts a moon in " +
			"every cell of it. These discs are the ONLY moon information on that Block, " +
			"and a graphic that IS the information cannot be hidden like one that " +
			"merely decorates adjacent text")
	}
}

// TestMoonPanel_TheDecorativeClusterIsStillAnnounced is the other half of the
// honesty idiom, and it is the half that was missing.
//
// "A CONTROL INVISIBLE TO ASSISTIVE TECHNOLOGY IS NOT A CONTROL" is already
// this file's rule (moonClusterLabel's header). The same sentence holds for
// information: on a Block where the panel cannot be built the discs are not an
// affordance, but they are still the phase of every body on every day — and on
// the Bench's real-world Block they are the ONLY moon information present.
//
// SO THIS ASSERTS THE SHAPE, NOT MERELY THE ABSENCE OF `aria-hidden`: one name
// per row naming the day and the bodies, one tooltip per row carrying the same
// sentence, and the discs themselves hidden and title-free — three tooltips on
// the parts of one cluster being three answers to one question, which is the
// argument moonClusterTitle exists for and does not stop being true because
// this cluster opens nothing.
//
// AND IT ASSERTS THAT NO CONTROL APPEARED. The fix above must not have quietly
// bought reachability, because reachability here needs [SKY-7] or [SKY-1]
// amended and neither is this code's to amend.
func TestMoonPanel_TheDecorativeClusterIsStillAnnounced(t *testing.T) {
	d := fxAlmanac(t, true)
	d.Month.Almanac = nil
	body := flatten(render(t, d))

	rows := regexp.MustCompile(`<span class="phrow"[^>]*>`).FindAllString(body, -1)
	if len(rows) == 0 {
		t.Fatal("no decorative cluster rendered at all — the guard has no subject and " +
			"would pass vacuously")
	}
	for i, r := range rows {
		if strings.Contains(r, `aria-hidden`) {
			t.Errorf("decorative cluster %d is hidden from assistive technology: %s", i, r)
		}
		if !strings.Contains(r, `title="`) {
			t.Errorf("decorative cluster %d carries no tooltip: %s", i, r)
		}
	}

	// The accessible name is the control branch's own sentence — day, bodies,
	// illumination — not the word "moons".
	if !strings.Contains(body, `<span class="phrow" title="Alder 4% · Umber 89%`) {
		t.Error("the cluster's tooltip is not moonClusterTitle's sentence. Two surfaces " +
			"stating the same nights differently is how they come to disagree")
	}
	if !strings.Contains(body, `<span class="vh">Moons on day 14 — Alder 4%, Umber 89%`) {
		t.Error("the cluster carries no accessible name. `title` alone does not survive " +
			"a screen reader's preferences and never survived `aria-hidden` at all — the " +
			"name is what a non-sighted reader is actually told")
	}

	// The discs are the parts, not three separate answers.
	discs := regexp.MustCompile(`<i class="ph[^"]*"[^>]*>`).FindAllString(body, -1)
	if len(discs) == 0 {
		t.Fatal("no discs rendered — the row is announced but empty")
	}
	for i, dsc := range discs {
		if strings.Contains(dsc, `title=`) {
			t.Errorf("disc %d keeps a `title` of its own: %s — three tooltips on the parts "+
				"of one cluster is three answers to one question", i, dsc)
		}
		if !strings.Contains(dsc, `aria-hidden="true"`) {
			t.Errorf("disc %d is not hidden: %s — the row carries the name; the discs "+
				"repeating it would announce every day three times", i, dsc)
		}
	}

	// AND NOTHING BECAME A CONTROL.
	for _, marker := range []string{"phctl", "data-cal-moons", "data-cal-moonpanel", "moonpick"} {
		if strings.Contains(body, marker) {
			t.Errorf("the accessibility fix also emitted %q — this Block still cannot build "+
				"an Almanac register, so a control here would open two empty tabs. "+
				"Reachability needs [SKY-7] or [SKY-1] amended, and that is booked, not done",
				marker)
		}
	}
}

// ── the graph tab and the foot zone cannot disagree ─────────────────────────

// TestMoonPanel_TheGraphReusesTheZonesOwnArithmetic. The panel's Graph tab and
// the `moongraph` layer's foot zone are two drawings of the same nights, and two
// surfaces stating the same arithmetic differently is how they come to disagree
// — which is the defect L19/L24 killed the composite over in the first place.
// Both call graphMoons / graphBarStyle / graphCellTitle / graphAxisLabel /
// graphFoot, so this test asserts the OUTPUT matches rather than trusting that.
func TestMoonPanel_TheGraphReusesTheZonesOwnArithmetic(t *testing.T) {
	d := fxAlmanac(t, true)
	d.Layers.Enabled = append(append([]string{}, d.Layers.Enabled...), "moongraph")
	body := flatten(render(t, d))

	graph := mpPane(t, body, "graph")
	zi := strings.Index(body, `data-layer="moongraph"`)
	if zi < 0 {
		t.Fatal("the moongraph zone did not render — the comparison has one side")
	}
	zone := body[zi:]

	// The footnote is the same sentence in both places, letter for letter.
	foot := graphFoot(d)
	if !strings.Contains(graph, foot) || !strings.Contains(zone, foot) {
		t.Errorf("the panel and the foot zone do not print the same footnote %q", foot)
	}
	// And the same number of dated cells: lanes × days, in the dayKey namespace,
	// with the marker guard B4 keys on beside it.
	want := len(graphMoons(d)) * d.Month.Days
	if n := strings.Count(graph, `data-cell="mp"`); n != want {
		t.Errorf("the Graph tab emits %d dated cells, want %d (lanes × days) — guard B4", n, want)
	}
	if n := strings.Count(graph, `data-day="`+d.CalendarSlug+`-14"`); n != len(graphMoons(d)) {
		t.Errorf("day 14 is keyed on %d lanes, want %d. The keys are the dayKey namespace "+
			"moongraphZone already emits, reused rather than reinvented", n, len(graphMoons(d)))
	}
}

// mpPane slices one of the panel's two tabs out of a rendered Block.
func mpPane(t *testing.T, body, tab string) string {
	t.Helper()
	open := `data-cal-moon-tab="` + tab + `"`
	i := strings.Index(body, open)
	if i < 0 {
		t.Fatalf("no %q tab in the render", tab)
	}
	rest := body[i:]
	// Bounded by the NEXT tab marker or the panel's close, whichever comes
	// first — a slice to end-of-document would let the Details tab's content
	// satisfy a claim about the Graph tab.
	end := len(rest)
	for _, stop := range []string{`data-cal-moon-tab=`, `</div></div></div>`} {
		if j := strings.Index(rest[len(open):], stop); j >= 0 && j+len(open) < end {
			end = j + len(open)
		}
	}
	return rest[:end]
}

// TestMoonPanel_DataGoIsUntouched. §8 of the dispatch: `data.go` is not opened
// by this slice. AlmanacMoon's shape, MoonDisc's shape and MonthGeometry's
// register are the producer's contract, and a panel that needed a new field
// would be a panel that had outgrown what the renders drew.
func TestMoonPanel_DataGoIsUntouched(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(".", "data.go"))
	if err != nil {
		t.Fatalf("read data.go: %v", err)
	}
	body := string(src)
	for _, bad := range []string{"moonPick", "moonPanel", "mpan", "MoonPanel"} {
		if strings.Contains(body, bad) {
			t.Errorf("data.go mentions %q — the panel is a RENDER surface over the register "+
				"the producer already ships, and it adds no pin field (§8)", bad)
		}
	}
}

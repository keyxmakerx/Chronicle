// moongraph_test.go — the illumination graph's render contract
// (C-CALV4-LAYERS-P9, W-F).
//
// The claims here are the two bounds the Almanac's own rulings carried into
// this zone (no composite, one declared ceiling), guard B4's dated nodes, and
// the one arithmetic detail that is easy to get subtly wrong and impossible to
// see: a new moon's bar.
package calendar_block

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func graphRender(t *testing.T, gm bool) (BlockData, string) {
	t.Helper()
	d := fxAlmanac(t, gm)
	d.Layers = LayerState{Enabled: []string{"moongraph"}}
	return d, flatten(render(t, d))
}

// L19/L24 — THERE IS NO COMPOSITE, AT ANY DENSITY. One lane per DRAWN body and
// no summed "how bright is tonight" figure anywhere in this zone. Three
// independent attempts at a composite disagreed about the same five nights;
// that is why it is dead and the graph is per-body.
func TestMoongraph_OneLanePerDrawnBodyAndNoComposite(t *testing.T) {
	d, body := graphRender(t, true)

	drawn := 0
	for _, m := range d.Month.Almanac {
		if m.Drawn {
			drawn++
			if !strings.Contains(body, `<span class="nm">`+m.Name+`</span>`) {
				t.Errorf("drawn body %q has no lane", m.Name)
			}
			continue
		}
		if strings.Contains(body, `<span class="nm">`+m.Name+`</span>`) {
			t.Errorf("undrawn body %q got a lane — the graph draws what the grid draws, and "+
				"the rest live in the Almanac", m.Name)
		}
	}
	if drawn == 0 {
		t.Fatal("the fixture draws no bodies; the assertions above are vacuous")
	}
	if n := strings.Count(body, `class="sfrow"`); n != drawn {
		t.Errorf("%d lanes for %d drawn bodies — an extra lane is a composite by another "+
			"name (L19/L24)", n, drawn)
	}
	for _, forbidden := range []string{"combined", "total illumination", "tonight overall"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("the graph names %q — there is no summed brightness figure in this "+
				"zone, ever", forbidden)
		}
	}
}

// L30 — THE CEILING IS DECLARED ONCE, IN THE NAMEPLATE. The graph's foot may
// name a DESTINATION ("+N more in the almanac") because the register really is
// uncapped by MoonCap ([S5]); it may never restate the ceiling, and there is
// never a per-cell "+1".
func TestMoongraph_FootNamesADestinationNotACeiling(t *testing.T) {
	d, body := graphRender(t, true)

	extra := d.Month.MoonsDeclared - len(graphMoons(d))
	if extra <= 0 {
		t.Fatal("the fixture declares no more bodies than it draws; the tail is untestable")
	}
	want := "illumination across " + d.Month.Name + " · filled marks are turns · +" +
		strconv.Itoa(extra) + " more in the almanac"
	if !strings.Contains(body, want) {
		t.Errorf("the footnote must be the signed line plus the destination tail; want %q", want)
	}
	// The Nameplate's "N of M moons" chip is the ONE place the ceiling is
	// stated, and the graph must not repeat it.
	graph := graphSlice(body)
	if strings.Contains(graph, " of "+strconv.Itoa(d.Month.MoonsDeclared)+" moons") {
		t.Error("the graph restates the Nameplate's ceiling — L30 declares it exactly once")
	}
	if strings.Contains(graph, `class="more"`) || regexp.MustCompile(`>\+\d+<`).MatchString(graph) {
		t.Error("a per-cell +N appeared in the graph; the overflow's only mention is the foot")
	}

	// With every declared body drawn, the tail is ABSENT rather than "+0 more".
	full := fxAlmanac(t, true)
	full.Layers = LayerState{Enabled: []string{"moongraph"}}
	full.Month.MoonsDeclared = len(graphMoons(full))
	if strings.Contains(flatten(render(t, full)), "more in the almanac") {
		t.Error("a graph that draws every declared body must not print a destination tail")
	}
}

// Guard B4 — EVERY .sfcell IS A DATED NODE, in the dayKey namespace. It does not
// answer yet (the generated ANSWER ladder is W-B's file and extending it is the
// split-out C-CALV4-ANSWER-EXT), but the key is emitted for the slice that pays
// for the ladder — exactly as wave 1 emitted keys for a consumer that did not
// exist.
func TestMoongraph_EveryCellCarriesItsAnswerKey(t *testing.T) {
	d, body := graphRender(t, true)

	lanes := graphMoons(d)
	if len(lanes) == 0 || d.Month.Days == 0 {
		t.Fatal("the fixture has no lanes or no days")
	}
	for day := 1; day <= d.Month.Days; day++ {
		want := `data-day="` + dayKey(d.CalendarSlug, day) + `"`
		if n := strings.Count(body, want); n < len(lanes) {
			t.Errorf("day %d carries %d data-day keys across %d lanes — every cell is a dated "+
				"node under guard B4", day, n, len(lanes))
		}
	}
	if n := strings.Count(body, `data-cell="sf"`); n != len(lanes)*d.Month.Days {
		t.Errorf("%d graph cells; want %d (%d lanes × %d days)",
			n, len(lanes)*d.Month.Days, len(lanes), d.Month.Days)
	}
}

// THE 1px FLOOR IS LOAD-BEARING. A new moon is 0% lit, and a zero-height bar
// reads as MISSING DATA rather than as darkness — the graph would appear to
// have holes in it on exactly the nights it is most precise about.
func TestMoongraph_ADarkNightStillDrawsABar(t *testing.T) {
	d := fxAlmanac(t, true)
	d.Layers = LayerState{Enabled: []string{"moongraph"}}
	for i := range d.Month.Almanac {
		for j := range d.Month.Almanac[i].Days {
			d.Month.Almanac[i].Days[j].Illum = 0
		}
	}
	body := flatten(render(t, d))
	if strings.Contains(body, "height:0px") {
		t.Error("a 0% night drew a zero-height bar — that reads as missing data, not as darkness")
	}
	if !strings.Contains(body, "height:1px") {
		t.Error("the 1px floor is missing entirely")
	}
	// And a full moon reaches the top of the 14px scale.
	for i := range d.Month.Almanac {
		for j := range d.Month.Almanac[i].Days {
			d.Month.Almanac[i].Days[j].Illum = 1
		}
	}
	if !strings.Contains(flatten(render(t, d)), "height:14px") {
		t.Error("a fully lit night must reach the signed 14px ceiling")
	}
}

// The turn ticks are ACHROMATIC BY LAW — the sky may never borrow the event
// colour axis — and "new" and "full" are two different marks, not one mark with
// two opacities.
func TestMoongraph_TurnTicksAreStructuralAndDistinct(t *testing.T) {
	d, body := graphRender(t, true)
	graph := graphSlice(body)

	turns := map[string]int{}
	for _, m := range graphMoons(d) {
		for _, a := range m.Days {
			if a.Turn != "" {
				turns[a.Turn]++
			}
		}
	}
	if len(turns) == 0 {
		t.Fatal("the fixture has no turns; the assertions below are vacuous")
	}
	for kind, n := range turns {
		if got := strings.Count(graph, `class="tn `+kind+`"`); got != n {
			t.Errorf("%d `%s` ticks rendered; the register carries %d", got, kind, n)
		}
	}
	for _, forbidden := range []string{"var(--axis)", "var(--accent)"} {
		if strings.Contains(graph, forbidden) {
			t.Errorf("the graph references %q — the sky is achromatic and never borrows the "+
				"event colour axis", forbidden)
		}
	}
}

// The graph is the SAME for a player as for a GM. Illumination is calendar
// geometry, not campaign content: there is nothing in it to be entitled to.
func TestMoongraph_IsIdenticalForEveryRole(t *testing.T) {
	_, gm := graphRender(t, true)
	_, player := graphRender(t, false)

	if graphSlice(gm) == "" || graphSlice(player) == "" {
		t.Fatal("one of the roles rendered no graph at all")
	}
	if graphSlice(gm) != graphSlice(player) {
		t.Error("the illumination graph differs by role — it is calendar geometry, and a " +
			"role-varying sky would imply the moons keep secrets")
	}
}


// graphSlice isolates the graph's own markup — from its zone marker to the end
// of its footnote — so a role comparison judges the GRAPH and not the Block's
// mark counts, which legitimately differ by viewer. Never slices on a bare
// strings.Index result, which PANICS on a rename instead of failing cleanly.
func graphSlice(body string) string {
	i := strings.Index(body, `data-layer="moongraph"`)
	if i < 0 {
		return ""
	}
	rest := body[i:]
	j := strings.Index(rest, `class="sffoot"`)
	if j < 0 {
		return ""
	}
	k := strings.Index(rest[j:], "</div>")
	if k < 0 {
		return ""
	}
	return rest[:j+k]
}

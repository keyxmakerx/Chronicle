// map_links_test.go pins a pin that opens another map: which maps a link may
// name, the partial-update contract on linked_map_id, the trail built from
// ?from=, and the tree of linked maps (bounded on cycles, built only from the
// pins this viewer can see).
package maps

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// linkRepo is a mock repo holding three maps of camp-1 and one of camp-2.
func linkRepo() *mockMapRepo {
	maps := map[string]*Map{
		"map-1":   {ID: "map-1", CampaignID: "camp-1", Name: "World"},
		"map-2":   {ID: "map-2", CampaignID: "camp-1", Name: "Isle"},
		"map-3":   {ID: "map-3", CampaignID: "camp-1", Name: "Port"},
		"foreign": {ID: "foreign", CampaignID: "camp-2", Name: "Elsewhere"},
	}
	return &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			if m, ok := maps[id]; ok {
				c := *m
				return &c, nil
			}
			return nil, nil
		},
		createMarkerFn: func(context.Context, *Marker) error { return nil },
		getMarkerFn:    func(context.Context, string) (*Marker, error) { return linkedStored(), nil },
	}
}

// linkedStored is a pin on map-1 that opens map-2.
func linkedStored() *Marker {
	mk := storedMarker()
	mk.LinkedMapID = strPtrMK("map-2")
	mk.LinkedMapName = "Isle"
	return mk
}

func TestCreateMarker_LinkedMapValidation(t *testing.T) {
	cases := []struct {
		name     string
		link     *string
		wantCode int // 0: accepted
		wantID   *string
		wantName string
	}{
		{"no link is a plain pin", nil, 0, nil, ""},
		{"empty string is a plain pin", strPtrMK(""), 0, nil, ""},
		{"another map of the campaign", strPtrMK("map-2"), 0, strPtrMK("map-2"), "Isle"},
		{"the pin's own map", strPtrMK("map-1"), 422, nil, ""},
		{"a map that does not exist", strPtrMK("missing"), 422, nil, ""},
		{"another campaign's map (IDOR)", strPtrMK("foreign"), 422, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mk, err := newTestMapService(linkRepo()).CreateMarker(context.Background(), CreateMarkerInput{
				MapID: "map-1", Name: "Pin", X: 1, Y: 1, LinkedMapID: tc.link,
			})
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertMarkerPtr(t, "LinkedMapID", mk.LinkedMapID, tc.wantID)
			if mk.LinkedMapName != tc.wantName {
				t.Errorf("LinkedMapName = %q, want %q", mk.LinkedMapName, tc.wantName)
			}
		})
	}
}

// A missing map and another campaign's map must be indistinguishable to the
// caller, or the write becomes a probe for map ids elsewhere.
func TestLinkedMap_MissingAndForeignAnswerTheSame(t *testing.T) {
	svc := newTestMapService(linkRepo())
	_, errMissing := svc.CreateMarker(context.Background(), CreateMarkerInput{MapID: "map-1", Name: "P", LinkedMapID: strPtrMK("missing")})
	_, errForeign := svc.CreateMarker(context.Background(), CreateMarkerInput{MapID: "map-1", Name: "P", LinkedMapID: strPtrMK("foreign")})
	if errMissing == nil || errForeign == nil || errMissing.Error() != errForeign.Error() {
		t.Errorf("missing %v vs foreign %v: the two must answer identically", errMissing, errForeign)
	}
}

func TestUpdateMarker_LinkedMap_ThreeDirections(t *testing.T) {
	cases := []struct {
		name     string
		input    UpdateMarkerInput
		wantCode int
		want     *string
		wantName string
	}{
		{"absent preserves the link (a rename)", UpdateMarkerInput{Name: patch.Of("renamed")}, 0, strPtrMK("map-2"), "Isle"},
		{"absent preserves the link (a drag)", UpdateMarkerInput{X: patch.Of(5.0), Y: patch.Of(6.0)}, 0, strPtrMK("map-2"), "Isle"},
		{"present replaces", UpdateMarkerInput{LinkedMapID: patch.Of("map-3")}, 0, strPtrMK("map-3"), "Port"},
		{"explicit null clears", UpdateMarkerInput{LinkedMapID: patch.Null[string]()}, 0, nil, ""},
		{"empty string clears", UpdateMarkerInput{LinkedMapID: patch.Of("")}, 0, nil, ""},
		{"echo of the stored link is accepted", UpdateMarkerInput{LinkedMapID: patch.Of("map-2")}, 0, strPtrMK("map-2"), "Isle"},
		{"own map refused", UpdateMarkerInput{LinkedMapID: patch.Of("map-1")}, 422, nil, ""},
		{"other campaign refused", UpdateMarkerInput{LinkedMapID: patch.Of("foreign")}, 422, nil, ""},
		{"missing map refused", UpdateMarkerInput{LinkedMapID: patch.Of("gone")}, 422, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var written *Marker
			repo := linkRepo()
			repo.updateMarkerFn = func(_ context.Context, mk *Marker) error { written = mk; return nil }
			err := newTestMapService(repo).UpdateMarker(context.Background(), "mk-1", tc.input, true)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
				if written != nil {
					t.Error("a refused link must write nothing")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertMarkerPtr(t, "LinkedMapID", written.LinkedMapID, tc.want)
			if written.LinkedMapName != tc.wantName {
				t.Errorf("LinkedMapName = %q, want %q", written.LinkedMapName, tc.wantName)
			}
			// Nothing else the input left absent moves.
			if tc.input.VisibilityRules.Present() || written.VisibilityRules == nil {
				t.Error("visibility_rules must be preserved")
			}
		})
	}
}

func TestFoldTrail(t *testing.T) {
	known := map[string]string{"w": "World", "i": "Isle", "p": "Port", "c": "Cellar"}
	ids := func(steps []TrailStep) string {
		var out []string
		for _, s := range steps {
			out = append(out, s.ID)
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		name    string
		from    []string
		current string
		want    string
	}{
		{"no trail", nil, "p", ""},
		{"followed two links", []string{"w", "i"}, "p", "w,i,p"},
		{"unknown and foreign ids are dropped", []string{"w", "nope", "i"}, "p", "w,i,p"},
		{"only unknown ids show no trail", []string{"nope"}, "p", ""},
		{"coming back to the current map cuts the loop", []string{"p", "w", "p", "i"}, "p", "i,p"},
		{"a map twice keeps the path simple", []string{"w", "i", "c", "i"}, "p", "w,i,p"},
		{"an unknown current map shows no trail", []string{"w"}, "zzz", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ids(foldTrail(tc.from, tc.current, known)); got != tc.want {
				t.Errorf("foldTrail = %q, want %q", got, tc.want)
			}
		})
	}

	// A long trail keeps only its last MaxTrailSteps maps plus the current one.
	big := map[string]string{"cur": "Here"}
	var long []string
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("m%02d", i)
		big[id] = id
		long = append(long, id)
	}
	got := foldTrail(long, "cur", big)
	if len(got) != MaxTrailSteps+1 || got[len(got)-1].ID != "cur" {
		t.Errorf("long trail = %d steps ending %q, want %d ending cur", len(got), got[len(got)-1].ID, MaxTrailSteps+1)
	}
}

func TestParseTrailParam(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"a,b", []string{"a", "b"}},
		{" a , ,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		if got := ParseTrailParam(tc.raw); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseTrailParam(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
	if got := ParseTrailParam(strings.Repeat("x,", 500)); len(got) > maxTrailInput {
		t.Errorf("an oversized list kept %d ids, want at most %d", len(got), maxTrailInput)
	}
}

// treeShape renders a tree as "a(b(c),d)" for compact assertions.
func treeShape(nodes []LinkNode) string {
	var parts []string
	for _, n := range nodes {
		s := n.ID
		if len(n.Children) > 0 {
			s += "(" + treeShape(n.Children) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}

func mapsNamed(ids ...string) []Map {
	out := make([]Map, len(ids))
	for i, id := range ids {
		out[i] = Map{ID: id, Name: strings.ToUpper(id)}
	}
	return out
}

func edge(from, to string) linkEdge {
	return linkEdge{From: from, To: to, Pin: &Marker{MapID: from, LinkedMapID: &to, Visibility: "everyone"}}
}

func TestBuildLinkTree(t *testing.T) {
	ms := mapsNamed("w", "i", "p", "c", "v", "x")
	cases := []struct {
		name      string
		edges     []linkEdge
		want      string
		wantCount int
	}{
		{"no links, no tree", nil, "", 0},
		{"a chain", []linkEdge{edge("w", "i"), edge("i", "p"), edge("p", "c")}, "w(i(p(c)))", 4},
		{"two children in pin order", []linkEdge{edge("w", "v"), edge("w", "i")}, "w(v,i)", 3},
		{"a cycle is placed once and stops", []linkEdge{edge("w", "i"), edge("i", "w")}, "w(i)", 2},
		{"a pure cycle still gets a root", []linkEdge{edge("i", "p"), edge("p", "i")}, "i(p)", 2},
		{"a cycle starts at the map that opens the most", []linkEdge{edge("i", "p"), edge("p", "w"), edge("w", "i"), edge("w", "c")}, "w(i(p),c)", 4},
		{"a map reached twice sits under its first parent", []linkEdge{edge("w", "i"), edge("w", "p"), edge("p", "i")}, "w(i,p)", 3},
		{"a self link is dropped", []linkEdge{edge("w", "w")}, "", 0},
		{"a link to an unknown map is dropped", []linkEdge{edge("w", "zz")}, "", 0},
		{"duplicate links count once", []linkEdge{edge("w", "i"), edge("w", "i")}, "w(i)", 2},
		{"separate trees", []linkEdge{edge("w", "i"), edge("c", "v")}, "w(i),c(v)", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := buildLinkTree(ms, tc.edges, false)
			if got := treeShape(tree.Roots); got != tc.want {
				t.Errorf("tree = %q, want %q", got, tc.want)
			}
			if tree.Count != tc.wantCount {
				t.Errorf("count = %d, want %d", tree.Count, tc.wantCount)
			}
			if tree.Truncated {
				t.Error("a small tree must not be truncated")
			}
		})
	}
}

// A chain deeper than the bound, and more maps than the bound, both stop and
// say so instead of running on.
func TestBuildLinkTree_Bounds(t *testing.T) {
	var ids []string
	for i := 0; i < MaxLinkTreeNodes+50; i++ {
		ids = append(ids, fmt.Sprintf("m%04d", i))
	}
	ms := mapsNamed(ids...)

	var chain []linkEdge
	for i := 0; i+1 < 40; i++ {
		chain = append(chain, edge(ids[i], ids[i+1]))
	}
	deep := buildLinkTree(ms, chain, false)
	depth, n := 0, deep.Roots
	for len(n) > 0 {
		depth++
		n = n[0].Children
	}
	if depth > MaxLinkTreeDepth || !deep.Truncated {
		t.Errorf("depth %d (truncated %v), want at most %d and truncated", depth, deep.Truncated, MaxLinkTreeDepth)
	}

	var fan []linkEdge
	for _, id := range ids[1:] {
		fan = append(fan, edge(ids[0], id))
	}
	wide := buildLinkTree(ms, fan, false)
	if wide.Count > MaxLinkTreeNodes || !wide.Truncated {
		t.Errorf("count %d (truncated %v), want at most %d and truncated", wide.Count, wide.Truncated, MaxLinkTreeNodes)
	}
}

// Who can follow a link is told only to viewers allowed to know.
func TestBuildLinkTree_ViaOnlyForDM(t *testing.T) {
	ms := mapsNamed("w", "i")
	e := edge("w", "i")
	e.Pin.VisibilityRules = strPtrMK(`{"allowed_users":["u-mira"]}`)
	dm := buildLinkTree(ms, []linkEdge{e}, true)
	via := dm.Roots[0].Children[0].Via
	if via == nil || via.Visibility != "everyone" || !reflect.DeepEqual(via.AllowedUsers, []string{"u-mira"}) {
		t.Errorf("DM via = %+v, want everyone + [u-mira]", via)
	}
	if dm.Roots[0].Via != nil {
		t.Error("a root has no link leading to it, so no via")
	}
	player := buildLinkTree(ms, []linkEdge{e}, false)
	if player.Roots[0].Children[0].Via != nil {
		t.Error("a player must not be told who else can follow a link")
	}
}

// LinkTree reads every edge through ListMarkers for the viewer, so a pin the
// fog hides contributes nothing, and the DM-only answer stays with the DM.
func TestLinkTree_UsesTheViewersMarkerList(t *testing.T) {
	dx, dy, lx, ly := fogPositions()
	two, three := "map-2", "map-3"
	pins := map[string][]Marker{
		"map-1": {
			{ID: "lit", MapID: "map-1", X: lx, Y: ly, LinkedMapID: &two, Visibility: "everyone"},
			{ID: "dark", MapID: "map-1", X: dx, Y: dy, LinkedMapID: &three, Visibility: "everyone"},
		},
	}
	repo := linkRepo()
	repo.listMapsFn = func(context.Context, string) ([]Map, error) {
		return []Map{{ID: "map-1", CampaignID: "camp-1", Name: "World"}, {ID: "map-2", CampaignID: "camp-1", Name: "Isle"}, {ID: "map-3", CampaignID: "camp-1", Name: "Port"}}, nil
	}
	repo.linkSources = []string{"map-1", "elsewhere"}
	repo.listMarkersFn = func(_ context.Context, mapID string, _ int) ([]Marker, error) {
		return append([]Marker(nil), pins[mapID]...), nil
	}
	svc := NewMapService(repo)
	svc.SetHexFogLookup(fakeFogLookup{mask: fogMaskFixture()})

	player, err := svc.LinkTree(context.Background(), "camp-1", int(permissions.RolePlayer), "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := treeShape(player.Roots); got != "map-1(map-2)" {
		t.Errorf("player tree = %q, want map-1(map-2): the pin under fog must add no link", got)
	}
	owner, err := svc.LinkTree(context.Background(), "camp-1", int(permissions.RoleOwner), "u-owner")
	if err != nil {
		t.Fatal(err)
	}
	if got := treeShape(owner.Roots); got != "map-1(map-2,map-3)" {
		t.Errorf("owner tree = %q, want map-1(map-2,map-3)", got)
	}
	if owner.Roots[0].Children[0].Via == nil {
		t.Error("the owner is told who can follow each link")
	}
}

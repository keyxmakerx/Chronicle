package maps

import (
	"context"
	"errors"
	"math/bits"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

func roadCells(keys ...HexKey) []HexCell {
	out := make([]HexCell, len(keys))
	for i, k := range keys {
		out[i] = HexCell{Col: k.Col, Row: k.Row, Terrain: strp(TerrainRoad), Piece: intp(3)}
	}
	return out
}

func landCell(k HexKey, terrain string) HexCell {
	return HexCell{Col: k.Col, Row: k.Row, Terrain: strp(terrain)}
}

func planMasks(cells []HexCell) map[HexKey]int {
	out := map[HexKey]int{}
	for _, c := range PlanOldRoadConversion(cells) {
		out[HexKey{c.Col, c.Row}] = c.Road
	}
	return out
}

// keptEdges lists each kept join once, derived from the masks, and fails the
// test when a join is not symmetric.
func keptEdges(t *testing.T, masks map[HexKey]int) [][2]HexKey {
	t.Helper()
	var out [][2]HexKey
	for k, m := range masks {
		for i, n := range Neighbors(k.Col, k.Row) {
			if m&(1<<i) == 0 {
				continue
			}
			back := -1
			for j, nn := range Neighbors(n.Col, n.Row) {
				if nn == k {
					back = j
				}
			}
			nm, ok := masks[n]
			if !ok || nm&(1<<back) == 0 {
				t.Fatalf("exit (%d,%d)->(%d,%d) has no matching exit back", k.Col, k.Row, n.Col, n.Row)
			}
			if hexLess(k, n) {
				out = append(out, [2]HexKey{k, n})
			}
		}
	}
	return out
}

func adjacency(edges [][2]HexKey) map[HexKey][]HexKey {
	g := map[HexKey][]HexKey{}
	for _, e := range edges {
		g[e[0]] = append(g[e[0]], e[1])
		g[e[1]] = append(g[e[1]], e[0])
	}
	return g
}

// components labels each key with a component number over graph g.
func components(keys []HexKey, g map[HexKey][]HexKey) map[HexKey]int {
	label := map[HexKey]int{}
	next := 0
	for _, k := range keys {
		if _, ok := label[k]; ok {
			continue
		}
		next++
		stack := []HexKey{k}
		label[k] = next
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, n := range g[c] {
				if _, ok := label[n]; !ok {
					label[n] = next
					stack = append(stack, n)
				}
			}
		}
	}
	return label
}

// checkRoadGraph asserts the kept graph is symmetric, joins exactly what the
// full graph joins (nobody is cut off), and has no loop under six hexes.
func checkRoadGraph(t *testing.T, keys []HexKey) [][2]HexKey {
	t.Helper()
	masks := planMasks(roadCells(keys...))
	edges := keptEdges(t, masks)
	kept := adjacency(edges)

	in := map[HexKey]bool{}
	for _, k := range keys {
		in[k] = true
	}
	var full [][2]HexKey
	for _, k := range keys {
		for _, n := range Neighbors(k.Col, k.Row) {
			if in[n] && hexLess(k, n) {
				full = append(full, [2]HexKey{k, n})
			}
		}
	}
	lf, lk := components(keys, adjacency(full)), components(keys, kept)
	for _, a := range keys {
		for _, b := range keys {
			if (lf[a] == lf[b]) != (lk[a] == lk[b]) {
				t.Fatalf("connectivity changed between (%d,%d) and (%d,%d)", a.Col, a.Row, b.Col, b.Row)
			}
		}
	}
	keptSet := map[[2]HexKey]bool{}
	for _, e := range edges {
		keptSet[e] = true
	}
	for _, e := range full {
		if !keptSet[e] && !reachableWithin(kept, e[0], e[1], minRoadCycle-2) {
			t.Fatalf("edge (%d,%d)-(%d,%d) was dropped but closes no small loop", e[0].Col, e[0].Row, e[1].Col, e[1].Row)
		}
	}
	for _, e := range edges {
		without := map[HexKey][]HexKey{}
		for k, ns := range kept {
			for _, n := range ns {
				if !(k == e[0] && n == e[1]) && !(k == e[1] && n == e[0]) {
					without[k] = append(without[k], n)
				}
			}
		}
		if reachableWithin(without, e[0], e[1], minRoadCycle-2) {
			t.Fatalf("edge (%d,%d)-(%d,%d) closes a loop under %d hexes", e[0].Col, e[0].Row, e[1].Col, e[1].Row, minRoadCycle)
		}
	}
	return edges
}

// ring12 is the ring of hexes at distance two around centre.
func ring12(centre HexKey) []HexKey {
	var out []HexKey
	for row := centre.Row - 3; row <= centre.Row+3; row++ {
		for col := centre.Col - 4; col <= centre.Col+4; col++ {
			if HexDistance(HexKey{col, row}, centre) == 2 {
				out = append(out, HexKey{col, row})
			}
		}
	}
	return out
}

func TestPlanOldRoads_Shapes(t *testing.T) {
	a := HexKey{4, 4}
	nb := Neighbors(a.Col, a.Row)
	// Bend: x - a - b - c - y where a and c also touch.
	b, c := nb[0], nb[1]
	x := nb[3]
	y := Neighbors(c.Col, c.Row)[1]
	// Diamond: a, b, c and d, the hex beside both b and c.
	var d HexKey
	for _, n := range Neighbors(b.Col, b.Row) {
		if n != a && HexDistance(n, c) == 1 {
			d = n
		}
	}
	centre := HexKey{4, 4}
	ring := Neighbors(centre.Col, centre.Row)

	tests := []struct {
		name      string
		keys      []HexKey
		wantEdges int
	}{
		{"straight line", []HexKey{{0, 0}, {1, 0}, {2, 0}}, 2},
		{"bend with a shortcut", []HexKey{x, a, b, c, y}, 4},
		{"first triangle with a tail", []HexKey{{5, 3}, {6, 3}, {7, 3}, {7, 4}}, 3},
		{"second triangle with a tail", []HexKey{{8, 5}, {9, 5}, {10, 5}, {10, 6}}, 3},
		{"diamond of four", []HexKey{a, b, c, d}, 3},
		{"ring of six around a non-road hex", ring[:], 6},
		{"single isolated hex", []HexKey{{9, 9}}, 0},
		{"ring of twelve at distance two", ring12(HexKey{6, 6}), 12},
		{"two separate lines", []HexKey{{0, 0}, {1, 0}, {5, 5}, {6, 5}}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkRoadGraph(t, tc.keys); len(got) != tc.wantEdges {
				t.Errorf("kept %d edges, want %d", len(got), tc.wantEdges)
			}
		})
	}

	t.Run("bend: shortcut dropped, middle hex keeps two exits", func(t *testing.T) {
		masks := planMasks(roadCells(x, a, b, c, y))
		if bits.OnesCount(uint(masks[b])) != 2 {
			t.Errorf("middle hex mask %b, want two exits", masks[b])
		}
		for i, n := range Neighbors(a.Col, a.Row) {
			if n == c && masks[a]&(1<<i) != 0 {
				t.Error("the shortcut a-c was kept")
			}
		}
	})

	t.Run("filled patch of seven keeps no small loop", func(t *testing.T) {
		keys := append([]HexKey{centre}, ring[:]...)
		checkRoadGraph(t, keys)
	})

	t.Run("an isolated hex is a dot", func(t *testing.T) {
		got := PlanOldRoadConversion(roadCells(HexKey{9, 9}))
		if len(got) != 1 || got[0].Road != 0 || got[0].Terrain != nil {
			t.Errorf("got %+v, want one conversion with road 0 (a dot, never NULL) and no land", got)
		}
	})
}

func TestPlanOldRoads_ExistingMaskIsKept(t *testing.T) {
	cells := roadCells(HexKey{0, 0}, HexKey{1, 0})
	cells[0].Road = intp(1 << 4)
	cells[1].Road = intp(1 << 5)
	got := map[HexKey]int{}
	for _, c := range PlanOldRoadConversion(cells) {
		got[HexKey{c.Col, c.Row}] = c.Road
	}
	if got[HexKey{0, 0}] != 1|1<<4 || got[HexKey{1, 0}] != 1<<3|1<<5 {
		t.Errorf("masks %v: the new exit must be OR-ed with the stored one", got)
	}
	lone := roadCells(HexKey{9, 9})
	lone[0].Road = intp(1 << 2)
	if r := PlanOldRoadConversion(lone)[0].Road; r != 1<<2 {
		t.Errorf("isolated hex with a mask = %d, want it kept", r)
	}
}

func TestPlanOldRoads_Land(t *testing.T) {
	h := HexKey{4, 4}
	nb := Neighbors(h.Col, h.Row)
	around := func(terrains ...string) []HexCell {
		cells := roadCells(h)
		for i, t := range terrains {
			if t != "" {
				cells = append(cells, landCell(nb[i], t))
			}
		}
		return cells
	}
	tests := []struct {
		name  string
		cells []HexCell
		want  string // "" is no land
	}{
		{"majority wins", around("forest", "forest", "hills"), TerrainForest},
		{"tie goes to the earlier kind", around("hills", "forest"), TerrainForest},
		{"tie between later kinds", around("water", "swamp"), TerrainWater},
		{"town is ignored", around("town", "town", "town", "hills"), TerrainHills},
		{"only a town gives nothing", around("town"), ""},
		{"water neighbours give water", around("water", "water"), TerrainWater},
		{"nothing around gives none", around(), ""},
		{"a neighbour with no terrain does not count", append(around(), HexCell{Col: nb[0].Col, Row: nb[0].Row, Name: "x"}), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanOldRoadConversion(tc.cells)
			if len(got) != 1 {
				t.Fatalf("got %d conversions", len(got))
			}
			if tc.want == "" {
				if got[0].Terrain != nil {
					t.Errorf("terrain %q, want none", *got[0].Terrain)
				}
			} else if got[0].Terrain == nil || *got[0].Terrain != tc.want {
				t.Errorf("terrain %v, want %q", got[0].Terrain, tc.want)
			}
		})
	}

	t.Run("land spreads into the middle of a patch in later passes", func(t *testing.T) {
		ring := Neighbors(h.Col, h.Row)
		keys := append([]HexKey{h}, ring[:]...)
		cells := roadCells(keys...)
		out := Neighbors(ring[0].Col, ring[0].Row)
		for _, n := range out {
			if HexDistance(n, h) == 2 {
				cells = append(cells, landCell(n, TerrainForest))
				break
			}
		}
		got := PlanOldRoadConversion(cells)
		if len(got) != 7 {
			t.Fatalf("got %d conversions, want 7", len(got))
		}
		for _, c := range got {
			if c.Terrain == nil || *c.Terrain != TerrainForest {
				t.Errorf("(%d,%d) terrain %v, want forest", c.Col, c.Row, c.Terrain)
			}
		}
	})

	t.Run("a map with no old road plans nothing", func(t *testing.T) {
		if got := PlanOldRoadConversion([]HexCell{landCell(h, TerrainForest)}); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}

// An explored road hex must not learn land from the fog, however it is spread.
func TestPlanOldRoads_ExploredRoadReadsOnlyExploredLand(t *testing.T) {
	h := HexKey{4, 4}
	nb := Neighbors(h.Col, h.Row)
	land := func(i int, terrain string, explored bool) HexCell {
		c := landCell(nb[i], terrain)
		c.Explored = explored
		return c
	}
	road := func(explored bool) HexCell {
		c := roadCells(h)[0]
		c.Explored = explored
		return c
	}
	tests := []struct {
		name  string
		cells []HexCell
		want  string
	}{
		{"explored road beside only unexplored land gets none",
			[]HexCell{road(true), land(0, "mountain", false), land(1, "mountain", false)}, ""},
		{"explored road prefers the explored land over more unexplored land",
			[]HexCell{road(true), land(0, "plains", true), land(1, "mountain", false), land(2, "mountain", false)}, TerrainPlains},
		{"unexplored road reads every neighbour",
			[]HexCell{road(false), land(0, "plains", true), land(1, "mountain", false), land(2, "mountain", false)}, TerrainMountain},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanOldRoadConversion(tc.cells)
			if len(got) != 1 {
				t.Fatalf("got %d conversions", len(got))
			}
			if tc.want == "" && got[0].Terrain != nil || tc.want != "" && (got[0].Terrain == nil || *got[0].Terrain != tc.want) {
				t.Errorf("terrain %v, want %q", got[0].Terrain, tc.want)
			}
		})
	}

	t.Run("the spread does not carry land from an unexplored road into an explored one", func(t *testing.T) {
		far := nb[0] // an unexplored road beside h, with unexplored forest next to it
		cells := roadCells(h, far)
		cells[0].Explored = true
		for _, n := range Neighbors(far.Col, far.Row) {
			if HexDistance(n, h) == 2 {
				c := landCell(n, TerrainForest)
				cells = append(cells, c)
				break
			}
		}
		got := map[HexKey]*string{}
		for _, c := range PlanOldRoadConversion(cells) {
			got[HexKey{c.Col, c.Row}] = c.Terrain
		}
		if got[far] == nil || *got[far] != TerrainForest {
			t.Fatalf("the unexplored road should read the forest, got %v", got[far])
		}
		if got[h] != nil {
			t.Errorf("explored road took %q from an unexplored road", *got[h])
		}
	})
}

func TestPlanOldRoads_Deterministic(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	var cells []HexCell
	for col := 0; col < 5; col++ {
		for row := 0; row < 5; row++ {
			k := HexKey{col, row}
			switch r.Intn(3) {
			case 0:
				cells = append(cells, roadCells(k)...)
			case 1:
				cells = append(cells, landCell(k, hexTerrains[r.Intn(len(hexTerrains)-1)]))
			}
		}
	}
	want := PlanOldRoadConversion(cells)
	for i := 0; i < 20; i++ {
		shuffled := append([]HexCell(nil), cells...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := PlanOldRoadConversion(shuffled); !reflect.DeepEqual(got, want) {
			t.Fatalf("order changed the result:\n got %v\nwant %v", got, want)
		}
	}
}

func TestPlanOldRoads_RandomShapesKeepTheRule(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 300; i++ {
		var keys []HexKey
		for col := 0; col < 5; col++ {
			for row := 0; row < 5; row++ {
				if r.Intn(100) < 45 {
					keys = append(keys, HexKey{col, row})
				}
			}
		}
		if len(keys) == 0 {
			continue
		}
		sort.Slice(keys, func(a, b int) bool { return hexLess(keys[a], keys[b]) })
		checkRoadGraph(t, keys)
	}
}

type fakeRoadStore struct {
	ids     []string
	failOn  string
	listErr error
	cells   map[string][]HexCell
	done    []string
}

func (f *fakeRoadStore) ListMapIDsWithOldRoads(context.Context) ([]string, error) {
	return f.ids, f.listErr
}

func (f *fakeRoadStore) ConvertMap(_ context.Context, id string, plan func([]HexCell) []OldRoadConversion) (int, error) {
	if id == f.failOn {
		return 0, errors.New("boom")
	}
	f.done = append(f.done, id)
	return len(plan(f.cells[id])), nil
}

func TestReconcileOldRoadHexes(t *testing.T) {
	two := roadCells(HexKey{0, 0}, HexKey{1, 0})
	tests := []struct {
		name       string
		store      *fakeRoadStore
		wantMaps   int
		wantHexes  int
		wantErr    bool
		wantMapsDo []string
	}{
		{"nothing to do", &fakeRoadStore{}, 0, 0, false, nil},
		{"converts every map", &fakeRoadStore{ids: []string{"a", "b"}, cells: map[string][]HexCell{"a": two, "b": two}}, 2, 4, false, []string{"a", "b"}},
		{"a failed map is skipped and reported", &fakeRoadStore{ids: []string{"a", "b"}, failOn: "a", cells: map[string][]HexCell{"b": two}}, 1, 2, true, []string{"b"}},
		{"a listing failure is reported", &fakeRoadStore{listErr: errors.New("db down")}, 0, 0, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, h, err := ReconcileOldRoadHexes(context.Background(), tc.store)
			if m != tc.wantMaps || h != tc.wantHexes || (err != nil) != tc.wantErr {
				t.Errorf("got %d maps %d hexes err %v; want %d, %d, err %v", m, h, err, tc.wantMaps, tc.wantHexes, tc.wantErr)
			}
			if !reflect.DeepEqual(tc.store.done, tc.wantMapsDo) {
				t.Errorf("converted %v, want %v", tc.store.done, tc.wantMapsDo)
			}
		})
	}
}

package maps

import (
	"math"
	"strconv"
	"testing"
)

func TestHexLine(t *testing.T) {
	tests := []struct {
		name string
		a, b HexKey
	}{
		{"same hex", HexKey{3, 3}, HexKey{3, 3}},
		{"neighbour east", HexKey{3, 3}, HexKey{4, 3}},
		{"along a row", HexKey{0, 2}, HexKey{7, 2}},
		{"down a column", HexKey{2, 0}, HexKey{2, 9}},
		{"diagonal", HexKey{0, 0}, HexKey{6, 9}},
		{"backwards", HexKey{8, 7}, HexKey{1, 1}},
		{"odd rows", HexKey{4, 1}, HexKey{9, 6}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := HexLine(tc.a, tc.b)
			if want := HexDistance(tc.a, tc.b) + 1; len(line) != want {
				t.Fatalf("len = %d, want distance+1 = %d", len(line), want)
			}
			if line[0] != tc.a || line[len(line)-1] != tc.b {
				t.Errorf("ends = %v..%v, want %v..%v", line[0], line[len(line)-1], tc.a, tc.b)
			}
			for i := 1; i < len(line); i++ {
				if d := HexDistance(line[i-1], line[i]); d != 1 {
					t.Errorf("step %d is %d hexes long, want 1 (%v -> %v)", i, d, line[i-1], line[i])
				}
			}
		})
	}
}

// The party's walk is drawn client-side from the same line; a golden vector
// keeps both ends on one rounding rule.
func TestHexLine_Golden(t *testing.T) {
	got := HexLine(HexKey{0, 0}, HexKey{3, 0})
	want := []HexKey{{0, 0}, {1, 0}, {2, 0}, {3, 0}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line = %v, want %v", got, want)
		}
	}
}

func TestRevealZone(t *testing.T) {
	tests := []struct {
		name       string
		path       []HexKey
		cols, rows int
		wantLen    int // -1: only check the invariants
	}{
		{"one hex reveals itself and six neighbours", []HexKey{{5, 5}}, 20, 20, 7},
		{"corner is clipped to the field", []HexKey{{0, 0}}, 20, 20, 3},
		{"a long path", HexLine(HexKey{2, 2}, HexKey{9, 6}), 20, 20, -1},
		{"nothing outside the field", []HexKey{{19, 19}}, 20, 20, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RevealZone(tc.path, tc.cols, tc.rows)
			if tc.wantLen >= 0 && len(got) != tc.wantLen {
				t.Errorf("len = %d, want %d: %v", len(got), tc.wantLen, got)
			}
			seen := map[HexKey]bool{}
			for _, k := range got {
				if seen[k] {
					t.Errorf("%v revealed twice", k)
				}
				seen[k] = true
				if k.Col < 0 || k.Row < 0 || k.Col >= tc.cols || k.Row >= tc.rows {
					t.Errorf("%v is outside the %dx%d field", k, tc.cols, tc.rows)
				}
				near := false
				for _, p := range tc.path {
					if HexDistance(p, k) <= 1 {
						near = true
					}
				}
				if !near {
					t.Errorf("%v is more than one step from the path", k)
				}
			}
			// Every on-field hex within one step of the path must be there.
			for _, p := range tc.path {
				for _, n := range append([]HexKey{p}, neighborKeys(p)...) {
					onField := n.Col >= 0 && n.Row >= 0 && n.Col < tc.cols && n.Row < tc.rows
					if onField && !seen[n] {
						t.Errorf("%v next to %v was not revealed", n, p)
					}
				}
			}
		})
	}
}

func TestOnExploredFrontier(t *testing.T) {
	explored := map[HexKey]bool{{5, 5}: true}
	fn := func(k HexKey) bool { return explored[k] }
	tests := []struct {
		name string
		k    HexKey
		want bool
	}{
		{"an explored hex", HexKey{5, 5}, true},
		{"a neighbour of one", Neighbors(5, 5)[2], true},
		{"two steps away", HexKey{5, 7}, false},
		{"far away", HexKey{0, 0}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := OnExploredFrontier(fn, tc.k); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// fogMaskFixture is a 1000x1000 map with hexes 100 wide (radius 50): hex (c,r)
// is centred at (c*86.6 (+43.3 on odd rows), 75*r). Only hexes (1,1) and (2,1)
// are explored.
func fogMaskFixture() *FogMask {
	return &FogMask{
		Version:  7,
		Geo:      NewHexGeometry(100, 1000, 1000),
		MapW:     1000,
		MapH:     1000,
		Explored: map[HexKey]bool{{1, 1}: true, {2, 1}: true},
	}
}

func pctOf(g HexGeometry, k HexKey, mapSide float64) (x, y float64) {
	cx, cy := g.Center(k.Col, k.Row)
	return cx / mapSide * 100, cy / mapSide * 100
}

func TestFogMask_HidesPoint(t *testing.T) {
	f := fogMaskFixture()
	ex, ey := pctOf(f.Geo, HexKey{1, 1}, 1000)
	ux, uy := pctOf(f.Geo, HexKey{4, 4}, 1000)
	tests := []struct {
		name string
		x, y float64
		want bool
	}{
		{"centre of an explored hex", ex, ey, false},
		{"centre of an unexplored hex", ux, uy, true},
		{"top-left of the map is unexplored land", 0.5, 0.5, true},
		{"beyond the field is not hidden", 250, 250, false},
		{"NaN fails closed", math.NaN(), 10, true},
		{"infinity fails closed", math.Inf(1), 10, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.HidesPoint(tc.x, tc.y); got != tc.want {
				t.Errorf("HidesPoint(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.want)
			}
		})
	}
	var nilMask *FogMask
	if nilMask.HidesPoint(5, 5) {
		t.Error("a nil mask hides nothing")
	}
}

func TestFogMask_HidesMarkerAndDrawing(t *testing.T) {
	f := fogMaskFixture()
	ex, ey := pctOf(f.Geo, HexKey{1, 1}, 1000)
	ux, uy := pctOf(f.Geo, HexKey{4, 4}, 1000)
	vx, vy := pctOf(f.Geo, HexKey{5, 6}, 1000)
	pts := func(s string) []byte { return []byte(s) }
	f2 := *f
	f2.AnchorID = "pic"
	tests := []struct {
		name string
		mask *FogMask
		d    Drawing
		want bool
	}{
		{"wholly in the dark", f, Drawing{ID: "a", DrawingType: "freehand", Points: pts(`[{"x":` + fl(ux) + `,"y":` + fl(uy) + `},{"x":` + fl(vx) + `,"y":` + fl(vy) + `}]`)}, true},
		{"one point on explored land stays", f, Drawing{ID: "b", DrawingType: "freehand", Points: pts(`[{"x":` + fl(ux) + `,"y":` + fl(uy) + `},{"x":` + fl(ex) + `,"y":` + fl(ey) + `}]`)}, false},
		{"no points", f, Drawing{ID: "c", DrawingType: "text", Points: pts(`[]`)}, false},
		{"unparseable fails closed", f, Drawing{ID: "d", DrawingType: "freehand", Points: pts(`nope`)}, true},
		{"a shadow is never hidden", f, Drawing{ID: "e", DrawingType: DrawingTypeShadow, Points: pts(`[{"x":` + fl(ux) + `,"y":` + fl(uy) + `},{"x":` + fl(vx) + `,"y":` + fl(vy) + `}]`)}, false},
		{"the anchor picture is kept", &f2, Drawing{ID: "pic", DrawingType: DrawingTypeImage, Points: pts(`[{"x":` + fl(ux) + `,"y":` + fl(uy) + `},{"x":` + fl(vx) + `,"y":` + fl(vy) + `}]`)}, false},
		{"another picture in the dark is hidden", &f2, Drawing{ID: "other", DrawingType: DrawingTypeImage, Points: pts(`[{"x":` + fl(ux) + `,"y":` + fl(uy) + `},{"x":` + fl(vx) + `,"y":` + fl(vy) + `}]`)}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.d
			if got := tc.mask.HidesDrawing(&d); got != tc.want {
				t.Errorf("HidesDrawing = %v, want %v", got, tc.want)
			}
		})
	}

	mk := &Marker{X: ux, Y: uy}
	if !f.HidesMarker(mk) {
		t.Error("a pin in an unexplored hex must be hidden")
	}
	if f.HidesMarker(&Marker{X: ex, Y: ey}) {
		t.Error("a pin in an explored hex must stay")
	}
	if f.HidesMarker(nil) {
		t.Error("a nil marker is not hidden")
	}
}

func fl(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

func TestFogMask_AnchoredGeometryUsesThePictureBox(t *testing.T) {
	// A picture covering the right half of a 1000x1000 map; hex (0,0) sits at
	// its top-left, so map point (500+w/2, r) is hex (0,0) and the left half of
	// the map is outside the field.
	g := NewAnchoredHexGeometry(100, 500, 0, 500, 1000)
	f := &FogMask{Geo: g, MapW: 1000, MapH: 1000, Explored: map[HexKey]bool{}, AnchorID: "pic"}
	cx, cy := g.Center(0, 0)
	if !f.HidesPoint(cx/10, cy/10) {
		t.Error("an unexplored hex inside the picture must hide a pin")
	}
	if f.HidesPoint(10, 50) {
		t.Error("land outside the picture is not covered by the layer")
	}
	f.Explored[HexKey{0, 0}] = true
	if f.HidesPoint(cx/10, cy/10) {
		t.Error("an explored hex must not hide a pin")
	}
	if f.CoversMapPicture() {
		t.Error("a layer pinned to a picture does not cover the map's own picture")
	}
	if !fogMaskFixture().CoversMapPicture() {
		t.Error("a whole-map layer covers the map's picture")
	}
}

func TestFogMask_WithholdsImageOf(t *testing.T) {
	f := fogMaskFixture()
	if f.WithholdsImageOf(&Drawing{ID: "pic"}) {
		t.Error("a whole-map layer has no anchor picture to withhold")
	}
	f.AnchorID = "pic"
	if !f.WithholdsImageOf(&Drawing{ID: "pic"}) || f.WithholdsImageOf(&Drawing{ID: "other"}) {
		t.Error("only the anchor picture's file is withheld")
	}
	var nilMask *FogMask
	if nilMask.WithholdsImageOf(&Drawing{ID: "pic"}) {
		t.Error("no fog withholds nothing")
	}
}

func TestVisiblePartyLayer(t *testing.T) {
	col, row := 2, 3
	cells := []HexCell{{Col: 2, Row: 3, Explored: true}, {Col: 5, Row: 5}}
	onHidden := HexLayer{FogEnabled: true, PartyCol: &[]int{5}[0], PartyRow: &[]int{5}[0]}
	onExplored := HexLayer{FogEnabled: true, PartyCol: &col, PartyRow: &row}
	tests := []struct {
		name  string
		layer HexLayer
		role  int
		shown bool
	}{
		{"player, party on an explored hex", onExplored, 1, true},
		{"player, party on an unexplored hex", onHidden, 1, false},
		{"public, party on an unexplored hex", onHidden, 0, false},
		{"owner sees it anywhere", onHidden, 3, true},
		{"fog off shows everyone", func() HexLayer { l := onHidden; l.FogEnabled = false; return l }(), 1, true},
		{"never placed", HexLayer{FogEnabled: true}, 1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := VisiblePartyLayer(tc.layer, cells, tc.role)
			if shown := got.PartyCol != nil; shown != tc.shown {
				t.Errorf("party shown = %v, want %v", shown, tc.shown)
			}
		})
	}
}

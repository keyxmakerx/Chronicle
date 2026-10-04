package maps

import (
	"math"
	"sort"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The golden vectors below were produced by running the approved mockup's own
// functions (geo, ctrOf, hexAtPt, cube, hdist, hline) with gridSize 60 over a
// 1000 x 700 field, so the Go maths and the viewer agree with the design.

func TestOffsetToCube(t *testing.T) {
	tests := []struct {
		col, row int
		want     Cube
	}{
		{0, 0, Cube{0, 0, 0}},
		{1, 0, Cube{1, -1, 0}},
		{0, 1, Cube{0, -1, 1}},
		{1, 1, Cube{1, -2, 1}},
		{5, 4, Cube{3, -7, 4}},
		{7, 9, Cube{3, -12, 9}},
		{3, 2, Cube{2, -4, 2}},
		{0, 5, Cube{-2, -3, 5}},
		{10, 10, Cube{5, -15, 10}},
	}
	for _, tc := range tests {
		got := OffsetToCube(tc.col, tc.row)
		if got != tc.want {
			t.Errorf("OffsetToCube(%d,%d) = %v, want %v", tc.col, tc.row, got, tc.want)
		}
		if got.X+got.Y+got.Z != 0 {
			t.Errorf("OffsetToCube(%d,%d) = %v does not sum to zero", tc.col, tc.row, got)
		}
		c, r := CubeToOffset(got)
		if c != tc.col || r != tc.row {
			t.Errorf("CubeToOffset(OffsetToCube(%d,%d)) = %d,%d", tc.col, tc.row, c, r)
		}
	}
}

func TestOffsetCubeRoundTrip_Negative(t *testing.T) {
	for row := -5; row <= 5; row++ {
		for col := -5; col <= 5; col++ {
			c, r := CubeToOffset(OffsetToCube(col, row))
			if c != col || r != row {
				t.Errorf("round trip (%d,%d) -> (%d,%d)", col, row, c, r)
			}
		}
	}
}

func TestNeighbors(t *testing.T) {
	tests := []struct {
		col, row int
		want     []HexKey
	}{
		{4, 3, []HexKey{{4, 2}, {5, 2}, {3, 3}, {5, 3}, {4, 4}, {5, 4}}},
		{5, 4, []HexKey{{4, 3}, {5, 3}, {4, 4}, {6, 4}, {4, 5}, {5, 5}}},
		{1, 1, []HexKey{{1, 0}, {2, 0}, {0, 1}, {2, 1}, {1, 2}, {2, 2}}},
		{2, 5, []HexKey{{2, 4}, {3, 4}, {1, 5}, {3, 5}, {2, 6}, {3, 6}}},
	}
	for _, tc := range tests {
		got := Neighbors(tc.col, tc.row)
		keys := got[:]
		sortKeys := func(k []HexKey) {
			sort.Slice(k, func(i, j int) bool {
				if k[i].Row != k[j].Row {
					return k[i].Row < k[j].Row
				}
				return k[i].Col < k[j].Col
			})
		}
		want := append([]HexKey(nil), tc.want...)
		sortKeys(keys)
		sortKeys(want)
		for i := range want {
			if keys[i] != want[i] {
				t.Errorf("Neighbors(%d,%d) = %v, want %v", tc.col, tc.row, keys, want)
				break
			}
		}
		for _, n := range got {
			if d := HexDistance(HexKey{tc.col, tc.row}, n); d != 1 {
				t.Errorf("neighbour %v of (%d,%d) is %d away", n, tc.col, tc.row, d)
			}
		}
	}
}

func TestHexDistance(t *testing.T) {
	tests := []struct {
		a, b HexKey
		want int
	}{
		{HexKey{0, 0}, HexKey{3, 0}, 3},
		{HexKey{0, 0}, HexKey{0, 3}, 3},
		{HexKey{2, 2}, HexKey{5, 6}, 5},
		{HexKey{1, 1}, HexKey{4, 1}, 3},
		{HexKey{4, 3}, HexKey{4, 3}, 0},
		{HexKey{0, 0}, HexKey{1, 1}, 2},
		{HexKey{0, 1}, HexKey{0, 0}, 1},
	}
	for _, tc := range tests {
		if got := HexDistance(tc.a, tc.b); got != tc.want {
			t.Errorf("HexDistance(%v,%v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := HexDistance(tc.b, tc.a); got != tc.want {
			t.Errorf("HexDistance is not symmetric for %v,%v", tc.a, tc.b)
		}
	}
}

func TestHexGeometry(t *testing.T) {
	g := NewHexGeometry(60, 1000, 700)
	if g.R != 30 || g.Cols != 21 || g.Rows != 17 {
		t.Fatalf("geometry = R %v cols %d rows %d, want 30/21/17", g.R, g.Cols, g.Rows)
	}

	centers := []struct {
		col, row int
		x, y     float64
	}{
		{0, 0, 0, 0},
		{1, 0, 51.96152422706631, 0},
		{0, 1, 25.980762113533157, 45},
		{3, 2, 155.88457268119893, 90},
		{6, 7, 337.749907475931, 315},
	}
	for _, tc := range centers {
		x, y := g.Center(tc.col, tc.row)
		if math.Abs(x-tc.x) > 1e-9 || math.Abs(y-tc.y) > 1e-9 {
			t.Errorf("Center(%d,%d) = %v,%v, want %v,%v", tc.col, tc.row, x, y, tc.x, tc.y)
		}
	}

	points := []struct {
		x, y     float64
		col, row int
		ok       bool
	}{
		{0, 0, 0, 0, true},
		{10, 5, 0, 0, true},
		{26, 0, 1, 0, true},
		{27, 10, 1, 0, true},
		{40, 20, 1, 0, true},
		{100, 80, 2, 2, true},
		{250.5, 300.25, 4, 7, true},
		{500, 400, 9, 9, true},
		{999, 699, 19, 16, true},
		{-5, -5, 0, 0, true},
		{1020, 10, 20, 0, true},
		{15, 45, 0, 1, true},
		{60, 60, 1, 1, true},
		{33.3, 77.7, 1, 2, true},
		// Well outside the field on each side.
		{-200, 10, 0, 0, false},
		{10, -200, 0, 0, false},
		{5000, 10, 0, 0, false},
		{10, 5000, 0, 0, false},
	}
	for _, tc := range points {
		col, row, ok := g.HexAtPoint(tc.x, tc.y)
		if ok != tc.ok || (ok && (col != tc.col || row != tc.row)) {
			t.Errorf("HexAtPoint(%v,%v) = %d,%d,%v, want %d,%d,%v", tc.x, tc.y, col, row, ok, tc.col, tc.row, tc.ok)
		}
	}

	// A point at a hex's own centre must land in that hex, wherever it is.
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			x, y := g.Center(col, row)
			c, r, ok := g.HexAtPoint(x, y)
			if !ok || c != col || r != row {
				t.Fatalf("centre of (%d,%d) resolved to (%d,%d,%v)", col, row, c, r, ok)
			}
		}
	}
}

func TestHexGeometry_DegenerateGridSize(t *testing.T) {
	g := NewHexGeometry(0, 1000, 700)
	if _, _, ok := g.HexAtPoint(10, 10); ok {
		t.Error("a zero-size grid must not resolve points")
	}
}

func TestVisibleCells(t *testing.T) {
	cells := []HexCell{
		{Col: 0, Row: 0, Explored: true, Name: "Port"},
		{Col: 1, Row: 0, Explored: false, Name: "Hidden barrow"},
		{Col: 2, Row: 0, Explored: true},
		{Col: 3, Row: 0, Explored: false},
	}
	tests := []struct {
		name string
		fog  bool
		role int
		want int
	}{
		{"fog on: owner sees everything", true, permissions.RoleOwner, 4},
		// A DM grant arrives as the promoted VisibilityRole, so it is the owner level.
		{"fog on: DM grant (promoted) sees everything", true, permissions.RoleOwner, 4},
		{"fog on: scribe gets explored only", true, permissions.RoleScribe, 2},
		{"fog on: player gets explored only", true, permissions.RolePlayer, 2},
		{"fog on: public gets explored only", true, permissions.RoleNone, 2},
		{"fog off: owner sees everything", false, permissions.RoleOwner, 4},
		{"fog off: scribe sees everything", false, permissions.RoleScribe, 4},
		{"fog off: player sees everything", false, permissions.RolePlayer, 4},
		{"fog off: public sees everything", false, permissions.RoleNone, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			layer := HexLayer{FogEnabled: tc.fog}
			got := VisibleCells(layer, cells, tc.role)
			if len(got) != tc.want {
				t.Fatalf("got %d cells, want %d", len(got), tc.want)
			}
			if tc.fog && !permissions.CanSeeDmOnly(tc.role) {
				for _, c := range got {
					if !c.Explored {
						t.Errorf("unexplored hex (%d,%d) reached a non-DM viewer", c.Col, c.Row)
					}
					if c.Name == "Hidden barrow" {
						t.Error("an unexplored hex's name reached a non-DM viewer")
					}
				}
			}
		})
	}
}

func TestVisibleCells_NeverNilAndDoesNotAlias(t *testing.T) {
	if got := VisibleCells(HexLayer{FogEnabled: true}, nil, permissions.RolePlayer); got == nil {
		t.Error("an empty result must be an empty slice so it encodes as [] not null")
	}
	in := []HexCell{{Col: 1, Row: 1, Explored: true}}
	out := VisibleCells(HexLayer{}, in, permissions.RoleOwner)
	out[0].Col = 99
	if in[0].Col != 1 {
		t.Error("VisibleCells must return a copy")
	}
}

func TestIsValidTerrain(t *testing.T) {
	for _, ok := range []string{"plains", "forest", "hills", "mountain", "water", "swamp", "desert", "snow", "town", "road"} {
		if !IsValidTerrain(ok) {
			t.Errorf("%q should be allowed", ok)
		}
	}
	for _, bad := range []string{"", "lava", "Forest", "forest ", "<script>", "mountains"} {
		if IsValidTerrain(bad) {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestUsableAnchor(t *testing.T) {
	tests := []struct {
		name string
		d    *Drawing
		want bool
	}{
		{"a picture on this map", &Drawing{MapID: "m", DrawingType: DrawingTypeImage}, true},
		{"nothing", nil, false},
		{"another map's picture", &Drawing{MapID: "other", DrawingType: DrawingTypeImage}, false},
		{"a rectangle", &Drawing{MapID: "m", DrawingType: "rectangle"}, false},
		{"a shadow", &Drawing{MapID: "m", DrawingType: DrawingTypeShadow}, false},
	}
	for _, tc := range tests {
		if got := UsableAnchor("m", tc.d); got != tc.want {
			t.Errorf("%s: UsableAnchor = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAnchorHidesLayer(t *testing.T) {
	hidden := &Drawing{Visibility: "dm_only"}
	shown := &Drawing{Visibility: "everyone"}
	tests := []struct {
		name   string
		anchor *Drawing
		role   int
		want   bool
	}{
		{"no anchor", nil, permissions.RolePlayer, false},
		{"shown picture, player", shown, permissions.RolePlayer, false},
		{"hidden picture, public", hidden, permissions.RoleNone, true},
		{"hidden picture, player", hidden, permissions.RolePlayer, true},
		{"hidden picture, scribe", hidden, permissions.RoleScribe, true},
		{"hidden picture, owner", hidden, permissions.RoleOwner, false},
	}
	for _, tc := range tests {
		if got := AnchorHidesLayer(tc.anchor, tc.role); got != tc.want {
			t.Errorf("%s: AnchorHidesLayer = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Resizing a picture must scale the hexes with it and keep every key: the same
// columns and rows, and the same hex under the same relative point.
func TestAnchoredGeometry_ScalesWithThePicture(t *testing.T) {
	small := NewAnchoredHexGeometry(80, 100, 50, 400, 300)
	big := NewAnchoredHexGeometry(80, 300, 20, 800, 600)
	if small.Cols != big.Cols || small.Rows != big.Rows {
		t.Fatalf("field changed with size: %dx%d vs %dx%d", small.Cols, small.Rows, big.Cols, big.Rows)
	}
	if math.Abs(big.R-2*small.R) > 1e-9 {
		t.Errorf("R = %v, want twice %v", big.R, small.R)
	}
	inside := 0
	for fx := 0.0123; fx <= 1.0; fx += 0.037 {
		for fy := 0.0117; fy <= 1.0; fy += 0.041 {
			c1, r1, ok1 := small.HexAtPoint(100+fx*400, 50+fy*300)
			c2, r2, ok2 := big.HexAtPoint(300+fx*800, 20+fy*600)
			if ok1 != ok2 || (ok1 && (c1 != c2 || r1 != r2)) {
				t.Fatalf("point (%.3f,%.3f): small = %d,%d,%v big = %d,%d,%v", fx, fy, c1, r1, ok1, c2, r2, ok2)
			}
			if ok1 {
				inside++
			}
		}
	}
	if inside < 100 {
		t.Errorf("only %d sample points landed in the field", inside)
	}
}

// The field stays inside the picture: every hex's outline lies within the box,
// and at least one hex always exists, even for a box too small to hold one.
func TestAnchoredGeometry_StaysInsideTheBox(t *testing.T) {
	tests := []struct {
		name         string
		size, bx, by float64
		bw, bh       float64
	}{
		{"typical picture", 80, 100, 50, 400, 300},
		{"small hexes", 20, 0, 0, 640, 480},
		{"wide picture", 115, 30, 40, 900, 200},
		{"a hex bigger than the picture", 300, 10, 10, 100, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewAnchoredHexGeometry(tc.size, tc.bx, tc.by, tc.bw, tc.bh)
			if g.Cols < 1 || g.Rows < 1 {
				t.Fatalf("field %dx%d has no hex", g.Cols, g.Rows)
			}
			if tc.name == "a hex bigger than the picture" {
				return
			}
			w := g.Width()
			for row := 0; row < g.Rows; row++ {
				for col := 0; col < g.Cols; col++ {
					x, y := g.Center(col, row)
					if x-w/2 < tc.bx-1e-6 || x+w/2 > tc.bx+tc.bw+1e-6 || y-g.R < tc.by-1e-6 || y+g.R > tc.by+tc.bh+1e-6 {
						t.Fatalf("hex (%d,%d) centred %.1f,%.1f spills out of the box", col, row, x, y)
					}
				}
			}
		})
	}
}

func TestAnchoredGeometry_Degenerate(t *testing.T) {
	for _, g := range []HexGeometry{
		NewAnchoredHexGeometry(0, 0, 0, 400, 300),
		NewAnchoredHexGeometry(80, 0, 0, 0, 300),
		NewAnchoredHexGeometry(80, 0, 0, 400, -1),
	} {
		if _, _, ok := g.HexAtPoint(10, 10); ok {
			t.Errorf("a degenerate field %+v resolved a point", g)
		}
	}
}

// Golden vectors shared with test/js/map_hexes.test.mjs: the same field built
// in a box 1000 wide and scaled must give the same keys.
func TestAnchoredGeometry_GoldenVectors(t *testing.T) {
	g := NewAnchoredHexGeometry(80, 100, 50, 400, 300)
	if g.Cols != 13 || g.Rows != 12 || math.Abs(g.R-16) > 1e-9 {
		t.Fatalf("field = R %v %dx%d, want 16 13x12", g.R, g.Cols, g.Rows)
	}
	for _, tc := range []struct {
		x, y     float64
		col, row int
		ok       bool
	}{
		{120, 60, 0, 0, true}, {250, 200, 5, 6, true}, {300, 100, 6, 1, true},
		{100, 50, 0, 0, false}, {499, 349, 0, 0, false}, {90, 50, 0, 0, false},
	} {
		col, row, ok := g.HexAtPoint(tc.x, tc.y)
		if ok != tc.ok || (ok && (col != tc.col || row != tc.row)) {
			t.Errorf("HexAtPoint(%v,%v) = %d,%d,%v, want %d,%d,%v", tc.x, tc.y, col, row, ok, tc.col, tc.row, tc.ok)
		}
	}
}

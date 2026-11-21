package maps

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

func TestHexMapSize(t *testing.T) {
	tests := []struct {
		name                    string
		cols, rows              int
		width, height, gridSize int
		wantCols, wantRows      int
	}{
		{name: "30 by 20", cols: 30, rows: 20, width: 1000, height: 564, gridSize: 41, wantCols: 30, wantRows: 20},
		{name: "smallest", cols: 4, rows: 4, width: 1000, height: 0, gridSize: 300, wantCols: 5, wantRows: 4},
		{name: "widest", cols: 100, rows: 100, width: 1000, height: 0, gridSize: 11, wantCols: 106, wantRows: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, g := HexMapSize(tt.cols, tt.rows)
			if w != tt.width || g != tt.gridSize || (tt.height != 0 && h != tt.height) {
				t.Fatalf("got %d x %d at grid %d, want %d x %d at grid %d", w, h, g, tt.width, tt.height, tt.gridSize)
			}
			geo := NewHexGeometry(float64(g), float64(w), float64(h))
			if geo.Cols != tt.wantCols || geo.Rows != tt.wantRows {
				t.Fatalf("field is %d x %d, want %d x %d", geo.Cols, geo.Rows, tt.wantCols, tt.wantRows)
			}
		})
	}
}

// Every size the format allows gets a field that holds it, with the shortest
// height and the largest grid size that do, inside the settings range.
func TestHexMapSize_EverySize(t *testing.T) {
	for cols := hexsketch.MinSide; cols <= hexsketch.MaxSide; cols++ {
		for rows := hexsketch.MinSide; rows <= hexsketch.MaxSide; rows += 7 {
			w, h, g := HexMapSize(cols, rows)
			if g < gridSizeMin || g > gridSizeMax {
				t.Fatalf("%d x %d: grid size %d is outside %d..%d", cols, rows, g, gridSizeMin, gridSizeMax)
			}
			geo := NewHexGeometry(float64(g), float64(w), float64(h))
			if geo.Cols < cols || geo.Rows < rows {
				t.Fatalf("%d x %d: field is only %d x %d", cols, rows, geo.Cols, geo.Rows)
			}
			if less := NewHexGeometry(float64(g), float64(w), float64(h-1)); h > 1 && less.Rows >= rows {
				t.Fatalf("%d x %d: height %d is not the shortest", cols, rows, h)
			}
			if g < gridSizeMax && NewHexGeometry(float64(g+1), float64(w), float64(h)).Cols >= cols {
				t.Fatalf("%d x %d: grid size %d is not the largest", cols, rows, g)
			}
			if geo.Cols > 400 || geo.Rows > MaxHexCoord+1 {
				t.Fatalf("%d x %d: field %d x %d is past the viewer's limit", cols, rows, geo.Cols, geo.Rows)
			}
		}
	}
}

// hexsketch carries its own copy of the hex maths and limits (it may not
// import this package); this pins the copies to the originals.
func TestHexsketchMatchesMaps(t *testing.T) {
	t.Run("neighbours in exit-bit order", func(t *testing.T) {
		for r := 0; r < 12; r++ {
			for c := 0; c < 12; c++ {
				want := Neighbors(c, r)
				got := hexsketch.Neighbors(hexsketch.Hex{Col: c, Row: r})
				for i := range want {
					if got[i].Col != want[i].Col || got[i].Row != want[i].Row {
						t.Fatalf("neighbour %d of %d,%d = %v, want %v", i, c, r, got[i], want[i])
					}
				}
			}
		}
	})
	t.Run("runs are the viewer's lines", func(t *testing.T) {
		for _, a := range []HexKey{{0, 0}, {3, 5}, {11, 2}, {7, 7}} {
			for _, b := range []HexKey{{0, 9}, {10, 0}, {5, 5}, {2, 11}, {7, 7}} {
				want := HexLine(a, b)
				got := hexsketch.Run(hexsketch.Hex{Col: a.Col, Row: a.Row}, hexsketch.Hex{Col: b.Col, Row: b.Row})
				if len(got) != len(want) {
					t.Fatalf("%v..%v: %d hexes, want %d", a, b, len(got), len(want))
				}
				for i := range want {
					if got[i].Col != want[i].Col || got[i].Row != want[i].Row {
						t.Fatalf("%v..%v step %d = %v, want %v", a, b, i, got[i], want[i])
					}
				}
			}
		}
	})
	t.Run("lands are the terrains but road", func(t *testing.T) {
		if len(hexsketch.Lands) != len(hexTerrains)-1 {
			t.Fatalf("%d lands for %d terrains", len(hexsketch.Lands), len(hexTerrains))
		}
		for _, l := range hexsketch.Lands {
			if !IsValidTerrain(l) || l == TerrainRoad {
				t.Errorf("%q is not a land the maps plugin takes", l)
			}
		}
		for _, w := range []string{"mountains", "scrubland", "wasteland", "deep sea", "caverns", "cave", "sea", "lake", "city", "village", "places", "plain", "hill"} {
			if l, ok := hexsketch.Land(w); !ok || !IsValidTerrain(l) {
				t.Errorf("%q reads as %q, which is not a terrain", w, l)
			}
		}
	})
	t.Run("limits", func(t *testing.T) {
		if hexsketch.MaxNameRunes != MaxHexNameRunes || hexsketch.MaxNotesRunes != MaxHexNotesRunes {
			t.Errorf("text limits differ: %d/%d vs %d/%d", hexsketch.MaxNameRunes, hexsketch.MaxNotesRunes, MaxHexNameRunes, MaxHexNotesRunes)
		}
		if hexsketch.MaxSide*hexsketch.MaxSide > MaxHexCellsPerMap || hexsketch.MaxSide > MaxHexCoord {
			t.Errorf("a %d by %d map would not fit the layer", hexsketch.MaxSide, hexsketch.MaxSide)
		}
		if len(hexsketch.Lines) != 3 {
			t.Errorf("lines = %v", hexsketch.Lines)
		}
	})
}

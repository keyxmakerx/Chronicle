package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

func TestWouldChange(t *testing.T) {
	tests := []struct {
		name string
		old  HexCellState
		cell hexsketch.Cell
		want bool
	}{
		{name: "an unpainted hex gains land", old: HexCellState{}, cell: hexsketch.Cell{Terrain: "forest"}, want: false},
		{name: "the same land again", old: HexCellState{Terrain: "plains"}, cell: hexsketch.Cell{Terrain: "plains"}, want: false},
		{name: "other land", old: HexCellState{Terrain: "plains"}, cell: hexsketch.Cell{Terrain: "forest"}, want: true},
		{name: "land over a name-only hex", old: HexCellState{Name: "Inn"}, cell: hexsketch.Cell{Terrain: "forest"}, want: false},
		{name: "a line on painted land", old: HexCellState{Terrain: "plains"}, cell: hexsketch.Cell{Road: ptr(3)}, want: false},
		{name: "a line over a line", old: HexCellState{Terrain: "plains", Road: ptr(1)}, cell: hexsketch.Cell{Road: ptr(4)}, want: false},
		{name: "a place on an unnamed hex", old: HexCellState{Terrain: "plains"}, cell: hexsketch.Cell{Name: "Inn", Notes: "Warm."}, want: false},
		{name: "a place with the same name and notes", old: HexCellState{Name: "Inn", Notes: "Warm."}, cell: hexsketch.Cell{Name: "Inn", Notes: "Warm."}, want: false},
		{name: "a place with another name", old: HexCellState{Name: "Inn"}, cell: hexsketch.Cell{Name: "Tavern"}, want: true},
		{name: "a place with other notes", old: HexCellState{Name: "Inn", Notes: "Warm."}, cell: hexsketch.Cell{Name: "Inn", Notes: "Cold."}, want: true},
		{name: "a place with no notes keeps the notes", old: HexCellState{Name: "Inn", Notes: "Warm."}, cell: hexsketch.Cell{Name: "Inn"}, want: false},
		{name: "notes onto a hex with none", old: HexCellState{Name: "Inn"}, cell: hexsketch.Cell{Name: "Inn", Notes: "Warm."}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wouldChange(tt.old, tt.cell); got != tt.want {
				t.Errorf("wouldChange = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr(n int) *int { return &n }

func TestMerged(t *testing.T) {
	old := HexCellState{Col: 2, Row: 1, Terrain: "plains", Road: ptr(1), Name: "Inn", Notes: "Warm."}
	got := merged(old, hexsketch.Cell{Col: 2, Row: 1, Terrain: "forest", Road: ptr(4), River: ptr(2), Name: "Tavern"})
	if got.Terrain != "forest" || *got.Road != 5 || *got.River != 2 || got.Trail != nil || got.Name != "Tavern" || got.Notes != "Warm." {
		t.Errorf("merged = %+v", got)
	}
	// A line the block does not draw, and land it does not give, stay as stored.
	got = merged(old, hexsketch.Cell{Col: 2, Row: 1, Trail: ptr(0)})
	if got.Terrain != "plains" || *got.Road != 1 || *got.Trail != 0 || got.Name != "Inn" {
		t.Errorf("merged = %+v", got)
	}
}

// decode reads a plan's picture data back as the widget does.
func decode(t *testing.T, s string) (p struct {
	Cols, Rows int
	Art        string
	Update     bool
	Lands      []string
	Cells      [][]any
	After      [][]any
	Ring       [][2]int
}) {
	t.Helper()
	if s == "" {
		t.Fatal("the plan carries no picture data")
	}
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		t.Fatalf("picture data is not JSON: %v\n%s", err, s)
	}
	return p
}

func TestHexMapKind_PlanPictureCreate(t *testing.T) {
	_, _, k := hexKit()
	p := k.Plan(context.Background(), camp, owner, hexRec(ActionCreate, "Coast", "6 x 5", "water 0,0\nroad 0,1 2,1\nplace \"Inn\" 1,1: Warm."))
	d := decode(t, p.HexMap)
	if d.Cols != 6 || d.Rows != 5 || d.Art != "detailed" || d.Update || len(d.After) != 0 || len(d.Ring) != 0 {
		t.Fatalf("picture = %+v", d)
	}
	if len(d.Cells) != 4 {
		t.Fatalf("cells = %v, want the four hexes the block touches", d.Cells)
	}
	first := d.Cells[0]
	if int(first[2].(float64)) != 4 || d.Lands[4] != "water" {
		t.Errorf("first hex = %v (lands %v), want water", first, d.Lands)
	}
	if strings.Contains(p.HexMap, "Warm.") {
		t.Error("notes are not part of the picture")
	}
	if !strings.Contains(p.HexMap, `"Inn"`) {
		t.Error("a place's name is part of the picture")
	}
}

func TestHexMapKind_PlanPictureUpdate(t *testing.T) {
	stored := []HexCellState{
		{Col: 0, Row: 0, Terrain: "plains"},                              // painted with other land: ringed
		{Col: 1, Row: 0, Terrain: "forest"},                              // same land again: not ringed
		{Col: 2, Row: 0, Terrain: "plains"},                              // only gains a road: not ringed
		{Col: 3, Row: 0, Terrain: "plains", Name: "Inn", Notes: "Warm."}, // a place with other words: ringed
		{Col: 4, Row: 0, Terrain: "hills", Name: "Keep"},                 // a place with the same name: not ringed
		{Col: 5, Row: 0, Terrain: "snow"},                                // not touched
	}
	body := "water 0,0\nforest 1,0\nroad 2,0 2,2\nplace \"Tavern\" 3,0: Cold.\nplace \"Keep\" 4,0\nhills 0,3"
	svc, _, k := hexKit()
	svc.cells, svc.cols, svc.rows, svc.art = stored, 6, 5, "simple"
	p := k.Plan(context.Background(), camp, owner, hexRec(ActionUpdate, "Imix", "6 x 5", body))
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	if !strings.Contains(p.Summary, "paints 8 hexes on the Imix map: 3 new, 5 painted over, of which 2 would change") {
		t.Errorf("summary = %q", p.Summary)
	}
	if !strings.Contains(strings.Join(p.Warnings, "|"), "The 2 hexes that would change are ringed on the map.") {
		t.Errorf("warnings = %q", p.Warnings)
	}
	d := decode(t, p.HexMap)
	if !d.Update || d.Art != "simple" || d.Cols != 6 || d.Rows != 5 {
		t.Fatalf("picture = %+v", d)
	}
	if len(d.Cells) != len(stored) {
		t.Errorf("Before holds %d hexes, want the %d stored", len(d.Cells), len(stored))
	}
	if len(d.After) != 8 {
		t.Errorf("After holds %d touched hexes, want 8", len(d.After))
	}
	if len(d.Ring) != 2 || d.Ring[0] != [2]int{0, 0} || d.Ring[1] != [2]int{3, 0} {
		t.Errorf("ring = %v, want 0,0 and 3,0", d.Ring)
	}
}

func TestHexMapKind_PlanNothingRingedSaysNothing(t *testing.T) {
	svc, _, k := hexKit()
	svc.cells, svc.cols, svc.rows = []HexCellState{{Col: 1, Row: 1, Terrain: "forest"}}, 6, 5
	p := k.Plan(context.Background(), camp, owner, hexRec(ActionUpdate, "Imix", "6 x 5", "forest 1,1\nroad 0,1 3,1"))
	if strings.Contains(p.Summary+strings.Join(p.Warnings, "|"), "ringed") || !strings.Contains(p.Summary, "of which 0 would change") {
		t.Errorf("plan = %+v", p)
	}
	if d := decode(t, p.HexMap); d.Art != "detailed" || len(d.Ring) != 0 {
		t.Errorf("picture = %+v, want the default art and no rings", d)
	}
}

func TestHexMapKind_PlanOutsideTheField(t *testing.T) {
	tests := []struct {
		name       string
		cols, rows int
		body       string
		want       string
	}{
		{name: "none outside", cols: 8, rows: 5, body: "water 7,4"},
		{name: "one outside", cols: 6, rows: 5, body: "water 5,4\nwater 6,0", want: "1 hex in this block lies outside the map's own hex field (6 across by 5 down) and will not show on the map"},
		{name: "several outside, across and down", cols: 5, rows: 4, body: "water 0,0 to 5,4", want: "10 hexes in this block lie outside the map's own hex field (5 across by 4 down)"},
		{name: "field not known", cols: 0, rows: 0, body: "water 7,4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, k := hexKit()
			svc.cols, svc.rows = tt.cols, tt.rows
			p := k.Plan(context.Background(), camp, owner, hexRec(ActionUpdate, "Imix", "8 x 5", tt.body))
			joined := strings.Join(p.Warnings, "|")
			if tt.want == "" && strings.Contains(joined, "outside") {
				t.Errorf("unexpected warning %q", joined)
			}
			if tt.want != "" && !strings.Contains(joined, tt.want) {
				t.Errorf("warnings = %q, want %q", joined, tt.want)
			}
		})
	}
}

func TestHexMapKind_PlanCreateNeverWarnsOutsideTheField(t *testing.T) {
	_, _, k := hexKit()
	p := k.Plan(context.Background(), camp, owner, hexRec(ActionCreate, "Coast", "6 x 5", "water 0,0 to 9,9"))
	if strings.Contains(strings.Join(p.Warnings, "|"), "outside") {
		t.Errorf("warnings = %q", p.Warnings)
	}
}

func TestHexMapKind_PlanPictureOfALargeMap(t *testing.T) {
	_, _, k := hexKit()
	p := k.Plan(context.Background(), camp, owner, hexRec(ActionCreate, "Big", "100 x 100", "fill plains\nforest around 50,50 radius 20\nriver 0,0 99,99"))
	d := decode(t, p.HexMap)
	if d.Cols != 100 || d.Rows != 100 || len(d.Cells) != 10000 {
		t.Fatalf("cols %d rows %d cells %d", d.Cols, d.Rows, len(d.Cells))
	}
	if len(p.HexMap) > 400_000 {
		t.Errorf("a 100 x 100 map's data is %d bytes, which is too much to sit in the row", len(p.HexMap))
	}
}

package records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/hexsketch"
)

type fakeHexMaps struct {
	created []string
	size    [2]int
	notHex  map[string]bool
	cells   []HexCellState
	cols    int
	rows    int
	art     string
	batches [][]HexWrite
	actors  []Actor
	// failBatch is the 1-based batch that fails; 0 never fails.
	failBatch int
}

func (f *fakeHexMaps) CreateHexMap(_ context.Context, _, name string, cols, rows int) (string, error) {
	f.created = append(f.created, name)
	f.size = [2]int{cols, rows}
	return "new-hex", nil
}

func (f *fakeHexMaps) UsesHexGrid(_ context.Context, id string) (bool, error) {
	return !f.notHex[id], nil
}

func (f *fakeHexMaps) Read(_ context.Context, _, _ string, a Actor) (HexMapState, error) {
	f.actors = append(f.actors, a)
	return HexMapState{Cells: f.cells, Cols: f.cols, Rows: f.rows, Art: f.art}, nil
}

func (f *fakeHexMaps) PaintCells(_ context.Context, _, mapID string, a Actor, cells []HexWrite) error {
	if mapID != "new-hex" && mapID != "m1" {
		return errors.New("wrong map")
	}
	f.actors = append(f.actors, a)
	f.batches = append(f.batches, cells)
	if len(f.batches) == f.failBatch {
		return badRequestf("the map refused it")
	}
	return nil
}

func hexKit() (*fakeHexMaps, *fakeMaps, HexMapKind) {
	svc := &fakeHexMaps{notHex: map[string]bool{"m-square": true}}
	maps := &fakeMaps{maps: []MapRef{
		{ID: "m1", CampaignID: camp, Name: "Imix"},
		{ID: "m-square", CampaignID: camp, Name: "Grid"},
		{ID: "m2", CampaignID: "other", Name: "Elsewhere"},
	}}
	return svc, maps, HexMapKind{Svc: svc, Maps: maps}
}

func hexRec(action, name, size, body string) Record {
	return rec("hex-map", action, name, map[string]any{"size": size}, body)
}

const coastBody = "fill plains\nforest 1,1 to 2,2\nriver 0,0 3,0\nroad 0,1 3,1\nplace \"A\" 1,1\nplace \"B\" 2,2"

func TestHexMapKind_Plan(t *testing.T) {
	tests := []struct {
		name        string
		r           Record
		cells       []HexCellState
		wantErr     string
		wantSummary string
		wantWarn    string
	}{
		{name: "create", r: hexRec(ActionCreate, "Coast", "6 x 5", coastBody),
			wantSummary: "makes a hex map 6 by 5 with 30 hexes of land (plains 26, forest 4), 1 river, 1 road and 2 named places"},
		{name: "create with only land", r: hexRec(ActionCreate, "Coast", "6 x 5", "water 1,1"),
			wantSummary: "makes a hex map 6 by 5 with 1 hex of land (water 1)"},
		{name: "create with only a line", r: hexRec(ActionCreate, "Coast", "6 x 5", "trail 1,1 3,1"),
			wantSummary: "makes a hex map 6 by 5 with 1 trail"},
		{name: "many kinds are cut short", r: hexRec(ActionCreate, "Coast", "6 x 5",
			"plains 0,0\nforest 1,0\nhills 2,0\nwater 3,0\nsnow 4,0\nice 5,0"),
			wantSummary: "6 hexes of land (forest 1, hills 1, ice 1, plains 1, snow 1, …)"},
		{name: "same name warns", r: hexRec(ActionCreate, "Imix", "6 x 5", "water 1,1"),
			wantSummary: "makes a hex map", wantWarn: "already exists; this makes another"},
		{name: "bad line warns with its number", r: hexRec(ActionCreate, "Coast", "6 x 5", "water 1,1\ntower 2,2"),
			wantSummary: "1 hex of land", wantWarn: `Line 2: "tower" isn't a kind of land`},
		{name: "road as land is refused", r: hexRec(ActionCreate, "Coast", "6 x 5", "fill road"),
			wantErr: "nothing on this hex map", wantWarn: "Line 1: road is a line, not land: write road c,r c,r"},
		{name: "road as land beside good lines", r: hexRec(ActionCreate, "Coast", "6 x 5", "water 1,1\nroad around 2,2 radius 1"),
			wantSummary: "1 hex of land", wantWarn: "road is a line, not land"},
		{name: "delete refused", r: hexRec(ActionDelete, "Imix", "6 x 5", "water 1,1"), wantErr: "delete a map on its map page"},
		{name: "no name", r: hexRec(ActionCreate, "", "6 x 5", "water 1,1"), wantErr: "needs `name`"},
		{name: "no size", r: rec("hex-map", ActionCreate, "M", nil, "water 1,1"), wantErr: "needs `size`"},
		{name: "size too small", r: hexRec(ActionCreate, "M", "3 x 5", "water 1,1"), wantErr: "4 to 100"},
		{name: "size not numbers", r: hexRec(ActionCreate, "M", "big", "water 1,1"), wantErr: "like 30 x 20"},
		{name: "nothing drawable", r: hexRec(ActionCreate, "M", "6 x 5", "# only a note"), wantErr: "nothing on this hex map"},

		{name: "update new and painted over",
			r:           hexRec(ActionUpdate, "imix", "6 x 5", "forest 1,1 to 3,1\nriver 0,0 4,0\nplace \"X\" 1,1\nplace \"Y\" 2,0"),
			cells:       []HexCellState{{Col: 1, Row: 1}, {Col: 0, Row: 0}, {Col: 5, Row: 4}},
			wantSummary: "paints 8 hexes on the Imix map: 6 new, 2 painted over, of which 0 would change; adds 1 river; names 2 places"},
		{name: "update of one hex says hex",
			r:           hexRec(ActionUpdate, "Imix", "6 x 5", "water 1,1"),
			wantSummary: "paints 1 hex on the Imix map: 1 new, 0 painted over"},
		{name: "update names the map with map:",
			r:           rec("hex-map", ActionUpdate, "Whatever", map[string]any{"size": "6 x 5", "map": "Imix"}, "water 1,1"),
			wantSummary: "on the Imix map"},
		{name: "update refuses a square map",
			r: hexRec(ActionUpdate, "Grid", "6 x 5", "water 1,1"), wantErr: "does not use a hex grid; switch the map's grid to hexes on its map page"},
		{name: "update of a missing map", r: hexRec(ActionUpdate, "Nowhere", "6 x 5", "water 1,1"), wantErr: `no map called "Nowhere" to paint onto`},
		{name: "update cannot reach another campaign's map", r: hexRec(ActionUpdate, "Elsewhere", "6 x 5", "water 1,1"), wantErr: "no map called"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, maps, k := hexKit()
			svc.cells = tt.cells
			p := k.Plan(context.Background(), camp, owner, tt.r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("Error = %q, want %q", p.Error, tt.wantErr)
				}
			} else if p.Error != "" || !strings.Contains(p.Summary, tt.wantSummary) {
				t.Fatalf("plan = %+v, want summary with %q", p, tt.wantSummary)
			}
			joined := strings.Join(p.Warnings, "|")
			if tt.wantWarn != "" && !strings.Contains(joined, tt.wantWarn) {
				t.Errorf("warnings = %q, want %q", p.Warnings, tt.wantWarn)
			}
			if tt.wantWarn == "" && len(p.Warnings) > 0 {
				t.Errorf("unexpected warnings %q", p.Warnings)
			}
			if len(svc.created)+len(svc.batches) != 0 || len(maps.created) != 0 {
				t.Errorf("a plan wrote something: %v %v", svc.created, svc.batches)
			}
		})
	}
}

func TestHexMapKind_ApplyCreate(t *testing.T) {
	svc, _, k := hexKit()
	r := hexRec(ActionCreate, "Coast", "6 x 5", coastBody)
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if len(svc.created) != 1 || svc.created[0] != "Coast" || svc.size != [2]int{6, 5} {
		t.Fatalf("created %v at %v, want one Coast of 6 x 5", svc.created, svc.size)
	}
	if len(svc.batches) != 1 || len(svc.batches[0]) != 30 {
		t.Fatalf("batches = %d", len(svc.batches))
	}
	byHex := map[[2]int]HexWrite{}
	for _, w := range svc.batches[0] {
		byHex[[2]int{w.Col, w.Row}] = w
	}
	// The river runs along row 0, so 1,0 has exits east and west.
	if w := byHex[[2]int{1, 0}]; w.Terrain != "plains" || w.River == nil || *w.River != 1|8 || w.Road != nil || w.Place {
		t.Errorf("1,0 = %+v", w)
	}
	if w := byHex[[2]int{1, 1}]; w.Terrain != "forest" || !w.Place || w.Name != "A" || w.Notes != "" || *w.Road != 1|8 {
		t.Errorf("1,1 = %+v", w)
	}
	if svc.actors[0] != owner {
		t.Errorf("painted as %+v, want the owner", svc.actors[0])
	}
}

func TestHexMapKind_ApplyBatches(t *testing.T) {
	tests := []struct {
		name      string
		size      string
		body      string
		failBatch int
		want      []int
		wantErr   string
	}{
		{name: "one batch exactly full", size: "25 x 20", body: "fill plains", want: []int{500}},
		{name: "over 500 splits", size: "30 x 20", body: "fill plains", want: []int{500, 100}},
		{name: "three batches", size: "50 x 30", body: "fill plains", want: []int{500, 500, 500}},
		{name: "second batch fails", size: "30 x 20", body: "fill plains", failBatch: 2, want: []int{500, 100},
			wantErr: "made the Coast map, but painted only 500 of its 600 hexes: the other 100 were not painted (the map refused it)"},
		{name: "first batch fails", size: "30 x 20", body: "fill plains", failBatch: 1, want: []int{500},
			wantErr: "made the Coast map, but painted only 0 of its 600 hexes: the other 600 were not painted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, k := hexKit()
			svc.failBatch = tt.failBatch
			err := k.Apply(context.Background(), camp, owner, hexRec(ActionCreate, "Coast", tt.size, tt.body))
			var got []int
			for _, b := range svc.batches {
				got = append(got, len(b))
			}
			if len(got) != len(tt.want) {
				t.Fatalf("batches = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("batches = %v, want %v", got, tt.want)
				}
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(planError(err), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestHexMapKind_ApplyUpdate(t *testing.T) {
	t.Run("lines are OR-ed onto what is stored, land replaces land, a place replaces name and notes", func(t *testing.T) {
		svc, _, k := hexKit()
		two, sixteen := 2, 16
		svc.cells = []HexCellState{{Col: 3, Row: 2, Road: &two, River: &sixteen}, {Col: 9, Row: 9, Road: &two}}
		body := "road 2,2 to 4,2\nforest 3,2\nplace \"Old Shrine\" 4,2\nplace \"Inn\" 2,2: warm beds"
		if err := k.Apply(context.Background(), camp, owner, hexRec(ActionUpdate, "Imix", "6 x 5", body)); err != nil {
			t.Fatal(err)
		}
		if len(svc.created) != 0 {
			t.Fatalf("an update made a map: %v", svc.created)
		}
		if len(svc.batches) != 1 || len(svc.batches[0]) != 3 {
			t.Fatalf("batches = %+v, want one of the 3 hexes the block names (9,9 is not touched)", svc.batches)
		}
		by := map[[2]int]HexWrite{}
		for _, w := range svc.batches[0] {
			by[[2]int{w.Col, w.Row}] = w
		}
		mid := by[[2]int{3, 2}]
		if mid.Road == nil || *mid.Road != 1|8|2 {
			t.Errorf("3,2 road = %v, want stored exit 2 kept with the new 1 and 8", mid.Road)
		}
		if mid.River != nil {
			t.Errorf("3,2 river = %d: the block drew none, so it must be left as stored", *mid.River)
		}
		if mid.Terrain != "forest" || mid.Place {
			t.Errorf("3,2 = %+v", mid)
		}
		if w := by[[2]int{4, 2}]; !w.Place || w.Name != "Old Shrine" || w.Notes != "" || w.Terrain != "" {
			t.Errorf("4,2 = %+v", w)
		}
		if w := by[[2]int{2, 2}]; w.Name != "Inn" || w.Notes != "warm beds" {
			t.Errorf("2,2 = %+v", w)
		}
	})
	t.Run("a partial failure says how many were painted", func(t *testing.T) {
		svc, _, k := hexKit()
		svc.failBatch = 2
		err := k.Apply(context.Background(), camp, owner, hexRec(ActionUpdate, "Imix", "30 x 20", "fill plains"))
		want := "painted only 500 of the 600 hexes onto the Imix map: the other 100 were not painted (the map refused it)"
		if err == nil || !strings.Contains(planError(err), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
	})
	t.Run("a square map writes nothing", func(t *testing.T) {
		svc, _, k := hexKit()
		err := k.Apply(context.Background(), camp, owner, hexRec(ActionUpdate, "Grid", "6 x 5", "water 1,1"))
		if err == nil || !strings.Contains(planError(err), "does not use a hex grid") {
			t.Fatalf("err = %v", err)
		}
		if len(svc.batches) != 0 {
			t.Errorf("wrote despite refusal: %v", svc.batches)
		}
	})
	t.Run("a missing map writes nothing", func(t *testing.T) {
		svc, _, k := hexKit()
		err := k.Apply(context.Background(), camp, owner, hexRec(ActionUpdate, "Nowhere", "6 x 5", "water 1,1"))
		if err == nil || !strings.Contains(planError(err), `no map called "Nowhere"`) {
			t.Fatalf("err = %v", err)
		}
		if len(svc.batches)+len(svc.created) != 0 {
			t.Errorf("wrote despite refusal")
		}
	})
	t.Run("a delete writes nothing", func(t *testing.T) {
		svc, _, k := hexKit()
		if err := k.Apply(context.Background(), camp, owner, hexRec(ActionDelete, "Imix", "6 x 5", "water 1,1")); err == nil {
			t.Fatal("delete was applied")
		}
		if len(svc.batches)+len(svc.created) != 0 {
			t.Errorf("wrote despite refusal")
		}
	})
}

// A pin may name a hex map an earlier row of the same paste makes.
func TestPlanAll_PinOnHexMapMadeInPaste(t *testing.T) {
	svc, maps, _ := hexKit()
	r := NewRegistry(HexMapKind{Svc: svc, Maps: maps}, PinKind{Svc: maps, Entities: fakeEntities{}})
	recs := []Record{
		hexRec(ActionCreate, "The Coast", "6 x 5", "water 1,1"),
		rec("pin", ActionCreate, "Gate", map[string]any{"map": "the coast", "x": 10, "y": 20}, ""),
	}
	_, plans := r.PlanAll(context.Background(), camp, owner, recs)
	if plans[0].Error != "" || plans[1].Error != "" || !strings.Contains(plans[1].Summary, "made earlier in this paste") {
		t.Errorf("plans = %+v", plans)
	}
}

func TestHexMapKind_PromptText(t *testing.T) {
	svc, maps, k := hexKit()
	reg := NewRegistry(k)
	if got := reg.Capabilities(); !strings.Contains(got, "- `kind: hex-map`: hex maps: land painted hex by hex, with roads, rivers and trails on top and named places") {
		t.Errorf("capabilities = %q", got)
	}
	if !strings.Contains(reg.Docs(), "### kind: hex-map (Hex map)") {
		t.Errorf("docs lack the kind")
	}
	if got, err := k.Export(context.Background(), camp, owner); got != "" || err != nil {
		t.Errorf("Export = %q, %v", got, err)
	}

	// Both worked examples in the Doc must be accepted by the real parser.
	doc := k.Doc()
	blocks := strings.Split(doc, "```")
	var examples int
	for i := 1; i < len(blocks); i += 2 {
		parts := strings.SplitN(strings.TrimSpace(blocks[i]), "---", 3)
		if len(parts) != 3 {
			t.Fatalf("example %d has no front matter: %q", i, blocks[i])
		}
		var size string
		for _, l := range strings.Split(parts[1], "\n") {
			if v, ok := strings.CutPrefix(l, "size: "); ok {
				size = v
			}
		}
		cols, rows, err := hexsketch.ParseSize(size)
		if err != nil {
			t.Fatalf("example %d: %v", i, err)
		}
		if res := hexsketch.Parse(parts[2], cols, rows); len(res.Problems) != 0 || len(res.Parts) == 0 {
			t.Errorf("example %d: parts %d, problems %v", i, len(res.Parts), res.Problems)
		}
		examples++
	}
	if examples != 2 {
		t.Errorf("%d examples in the Doc, want a create and an update", examples)
	}
	_, _ = svc, maps
}

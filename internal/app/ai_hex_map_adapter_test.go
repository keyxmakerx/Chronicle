package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// The records package cannot import maps, so its batch size is a copy of the
// hex layer's cap; they must stay equal.
func TestAIHexBatchMatchesTheHexLayerCap(t *testing.T) {
	if got := records.HexBatch; got != maps.MaxHexBatch {
		t.Fatalf("records.HexBatch = %d, maps.MaxHexBatch = %d", got, maps.MaxHexBatch)
	}
}

type hexMapsStub struct {
	maps.MapService
	created *maps.CreateMapInput
	updated *maps.UpdateMapInput
	stored  *maps.Map
}

func (s *hexMapsStub) CreateMap(_ context.Context, in maps.CreateMapInput) (*maps.Map, error) {
	s.created = &in
	return &maps.Map{ID: "m-new", Name: in.Name}, nil
}

func (s *hexMapsStub) UpdateMap(_ context.Context, _ string, in maps.UpdateMapInput) error {
	s.updated = &in
	return nil
}

func (s *hexMapsStub) GetMap(context.Context, string) (*maps.Map, error) { return s.stored, nil }

func TestAIHexMapAdapter_CreateMakesABlankHexMap(t *testing.T) {
	ms := &hexMapsStub{}
	id, err := aiHexMapAdapter{maps: ms}.CreateHexMap(context.Background(), "camp", "Coast", 30, 20)
	if err != nil || id != "m-new" {
		t.Fatalf("id %q, err %v", id, err)
	}
	w, h, g := maps.HexMapSize(30, 20)
	if ms.created.CampaignID != "camp" || ms.created.ImageWidth != w || ms.created.ImageHeight != h || ms.created.ImageID != nil {
		t.Errorf("created %+v, want a pictureless %d x %d map", ms.created, w, h)
	}
	raw, _ := ms.updated.DisplaySettings.Get()
	next, err := maps.MergeDisplaySettings(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	probe := maps.ResolveDisplay(&maps.Map{Display: next}, "")
	if probe.GridType != maps.GridHex || probe.GridSize != g || !probe.KeepBlank {
		t.Errorf("display = %+v, want a hex grid of %d on a map kept blank", probe, g)
	}
}

func TestAIHexMapAdapter_UsesHexGrid(t *testing.T) {
	tests := []struct {
		name string
		m    *maps.Map
		want bool
	}{
		{"no settings", &maps.Map{}, false},
		{"square", &maps.Map{Display: &maps.DisplaySettings{Grid: &maps.GridDisplay{Type: maps.GridSquare}}}, false},
		{"hex", &maps.Map{Display: &maps.DisplaySettings{Grid: &maps.GridDisplay{Type: maps.GridHex}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := aiHexMapAdapter{maps: &hexMapsStub{stored: tt.m}}.UsesHexGrid(context.Background(), "m")
			if err != nil || got != tt.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

type hexLayerStub struct {
	maps.HexService
	role    int
	cells   []maps.HexCell
	anchor  *string
	art     string
	actor   maps.HexActor
	entries []maps.UpdateHexCellInput
}

func (s *hexLayerStub) GetLayer(_ context.Context, _, _ string, role int) (*maps.HexLayerView, error) {
	s.role = role
	return &maps.HexLayerView{Layer: maps.HexLayer{AnchorDrawingID: s.anchor}, Cells: s.cells, Art: s.art}, nil
}

func (s *hexLayerStub) PatchCells(_ context.Context, _, _ string, a maps.HexActor, e []maps.UpdateHexCellInput) (*maps.HexWriteResult, error) {
	s.actor, s.entries = a, e
	return &maps.HexWriteResult{Updated: len(e)}, nil
}

func TestAIHexMapAdapter_PaintCellsKeepsPartialUpdateSemantics(t *testing.T) {
	hs := &hexLayerStub{}
	zero, five := 0, 5
	cells := []records.HexWrite{
		{Col: 1, Row: 2, Terrain: "forest"},
		{Col: 3, Row: 4, Road: &five, River: &zero},
		{Col: 5, Row: 6, Place: true, Name: "Inn", Notes: "warm"},
		{Col: 7, Row: 8, Place: true, Name: "Hut"},
	}
	a := records.Actor{UserID: "u1", Role: 3}
	if err := (aiHexMapAdapter{hexes: hs}).PaintCells(context.Background(), "camp", "m", a, cells); err != nil {
		t.Fatal(err)
	}
	if hs.actor != (maps.HexActor{UserID: "u1", Role: 3, IsDM: true}) {
		t.Errorf("actor = %+v", hs.actor)
	}
	e := hs.entries
	if len(e) != 4 {
		t.Fatalf("entries = %d", len(e))
	}
	// Absent fields must stay absent: an echo of untouched fields would clear them.
	if !e[0].Terrain.Present() || e[0].Road.Present() || e[0].River.Present() || e[0].Trail.Present() || e[0].Name.Present() || e[0].Notes.Present() || e[0].Piece.Present() {
		t.Errorf("terrain-only entry sends more: %+v", e[0])
	}
	if v, _ := e[1].Road.Get(); !e[1].Road.Present() || v != 5 {
		t.Errorf("road = %+v", e[1].Road)
	}
	if v, _ := e[1].River.Get(); !e[1].River.Present() || e[1].River.IsNull() || v != 0 {
		t.Errorf("a no-exit river must be written as 0, not cleared: %+v", e[1].River)
	}
	if e[1].Terrain.Present() || e[1].Trail.Present() || e[1].Name.Present() {
		t.Errorf("line-only entry sends more: %+v", e[1])
	}
	if n, _ := e[2].Name.Get(); n != "Inn" || e[2].Notes.IsNull() {
		t.Errorf("place = %+v", e[2])
	}
	if e[3].Notes.Present() {
		t.Errorf("a place without notes must leave the stored notes alone: %+v", e[3].Notes)
	}
}

func TestAIHexMapAdapter_ReadAsTheActor(t *testing.T) {
	two := 2
	land, notes := "forest", "Candles."
	w, h, g := maps.HexMapSize(30, 20)
	display, err := json.Marshal(map[string]any{"grid": map[string]any{"type": maps.GridHex, "size": g}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := maps.MergeDisplaySettings(nil, display)
	if err != nil {
		t.Fatal(err)
	}
	stored := &maps.Map{ImageWidth: w, ImageHeight: h, Display: next}
	cells := []maps.HexCell{{Col: 1, Row: 2, Road: &two, Terrain: &land, Name: "Shrine", Notes: &notes}}
	anchor := "pic"

	tests := []struct {
		name      string
		anchor    *string
		wantCols  bool
		wantArt   string
		storedArt string
	}{
		{name: "whole map has its field", wantCols: true, wantArt: "simple", storedArt: "simple"},
		{name: "a layer pinned to a picture has no known field", anchor: &anchor, wantArt: "real", storedArt: "real"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := &hexLayerStub{cells: cells, anchor: tt.anchor, art: tt.storedArt}
			got, err := aiHexMapAdapter{hexes: hs, maps: &hexMapsStub{stored: stored}}.Read(context.Background(), "camp", "m", records.Actor{Role: 3})
			if err != nil || hs.role != 3 || len(got.Cells) != 1 {
				t.Fatalf("got %+v, role %d, err %v", got, hs.role, err)
			}
			c := got.Cells[0]
			if c.Col != 1 || c.Row != 2 || c.Road != &two || c.Terrain != "forest" || c.Name != "Shrine" || c.Notes != "Candles." {
				t.Errorf("cell = %+v", c)
			}
			if got.Art != tt.wantArt {
				t.Errorf("art = %q, want %q", got.Art, tt.wantArt)
			}
			geo := maps.NewHexGeometry(float64(g), float64(w), float64(h))
			if tt.wantCols && (got.Cols != geo.Cols || got.Rows != geo.Rows || got.Cols < 30 || got.Rows < 20) {
				t.Errorf("field = %d x %d, want %d x %d (at least 30 x 20)", got.Cols, got.Rows, geo.Cols, geo.Rows)
			}
			if !tt.wantCols && (got.Cols != 0 || got.Rows != 0) {
				t.Errorf("a pinned layer reported a field %d x %d", got.Cols, got.Rows)
			}
		})
	}
}

// foggedLayerStub answers GetLayer the way the hex service does: through
// maps.VisibleCells, with fog on.
type foggedLayerStub struct {
	maps.HexService
	cells []maps.HexCell
}

func (s foggedLayerStub) GetLayer(_ context.Context, _, mapID string, role int) (*maps.HexLayerView, error) {
	layer := maps.DefaultHexLayer(mapID)
	layer.FogEnabled = true
	return &maps.HexLayerView{Layer: layer, Cells: maps.VisibleCells(layer, s.cells, role)}, nil
}

type oneHexMap struct{ records.MapsAPI }

func (oneHexMap) ListMaps(context.Context, string) ([]records.MapRef, error) {
	return []records.MapRef{{ID: "m", CampaignID: "camp", Name: "Imix"}}, nil
}

func (oneHexMap) ListPins(context.Context, string, string, int, string) ([]records.PinRef, error) {
	return nil, nil
}

func TestAIHexRead_APlayerUnderFogReadsOnlyExploredHexes(t *testing.T) {
	forest, swamp := "forest", "swamp"
	east, both, west := 1, 1|8, 8
	secret := "The lich sleeps here."
	// A river runs 1,2 to 2,2 to 3,2. Only the first two hexes are explored.
	cells := []maps.HexCell{
		{Col: 1, Row: 2, Terrain: &forest, River: &east, Explored: true},
		{Col: 2, Row: 2, Terrain: &forest, River: &both, Explored: true},
		{Col: 3, Row: 2, Terrain: &swamp, River: &west, Name: "Lich Fen", Notes: &secret},
	}
	display, err := json.Marshal(map[string]any{"grid": map[string]any{"type": maps.GridHex, "size": 40}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := maps.MergeDisplaySettings(nil, display)
	if err != nil {
		t.Fatal(err)
	}
	l := &records.Lookups{Maps: oneHexMap{}, Hexes: aiHexMapAdapter{
		hexes: foggedLayerStub{cells: cells},
		maps:  &hexMapsStub{stored: &maps.Map{ImageWidth: 1000, ImageHeight: 600, Display: next}},
	}}
	ask := []records.Record{{Kind: records.KindLookup, Fields: map[string]any{"what": "pins", "map": "Imix"}}}

	for _, role := range []int{1, 2} {
		got := l.Answer(context.Background(), "camp", records.Actor{UserID: "u", Role: role}, ask)[0].Text
		if !strings.Contains(got, "forest 1,2 to 2,2") || !strings.Contains(got, "river 1,2 2,2\n") {
			t.Fatalf("role %d: the explored hexes are missing:\n%s", role, got)
		}
		for _, hidden := range []string{"3,2", "swamp", "Lich", "lich"} {
			if strings.Contains(got, hidden) {
				t.Fatalf("role %d: the answer gives away %q from an unexplored hex:\n%s", role, hidden, got)
			}
		}
	}
	got := l.Answer(context.Background(), "camp", records.Actor{UserID: "o", Role: 3}, ask)[0].Text
	if !strings.Contains(got, "river 1,2 2,2 3,2\n") || !strings.Contains(got, `place "Lich Fen" 3,2: The lich sleeps here.`) {
		t.Fatalf("the owner does not read the whole map:\n%s", got)
	}
}

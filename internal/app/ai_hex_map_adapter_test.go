package app

import (
	"context"
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
	actor   maps.HexActor
	entries []maps.UpdateHexCellInput
}

func (s *hexLayerStub) GetLayer(_ context.Context, _, _ string, role int) (*maps.HexLayerView, error) {
	s.role = role
	return &maps.HexLayerView{Cells: s.cells}, nil
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

func TestAIHexMapAdapter_CellsReadAsTheActor(t *testing.T) {
	two := 2
	hs := &hexLayerStub{cells: []maps.HexCell{{Col: 1, Row: 2, Road: &two}}}
	got, err := aiHexMapAdapter{hexes: hs}.Cells(context.Background(), "camp", "m", records.Actor{Role: 3})
	if err != nil || hs.role != 3 || len(got) != 1 || got[0].Col != 1 || got[0].Row != 2 || got[0].Road != &two {
		t.Fatalf("got %+v, role %d, err %v", got, hs.role, err)
	}
}

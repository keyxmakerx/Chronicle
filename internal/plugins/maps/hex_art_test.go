package maps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The pieces and terrain art of the hex layer: which looks a hex may carry,
// and who may change the map's art and how it reaches open viewers.

func TestPieceCount(t *testing.T) {
	tests := []struct {
		terrain string
		want    int
	}{
		{TerrainForest, HexPiecesPerTerrain},
		{TerrainTown, HexPiecesPerTerrain},
		{TerrainWater, HexPiecesPerTerrain},
		{TerrainRoad, 0},
		{"lava", 0},
		{"", 0},
	}
	for _, tc := range tests {
		if got := PieceCount(tc.terrain); got != tc.want {
			t.Errorf("PieceCount(%q) = %d, want %d", tc.terrain, got, tc.want)
		}
	}
}

func pieceOf(v int) patch.Field[int] { return patch.Of(v) }

func TestPatchCells_Pieces(t *testing.T) {
	forest := HexCell{Col: 1, Row: 1, Terrain: strp(TerrainForest), Piece: intp(5)}
	bare := HexCell{Col: 1, Row: 1, Name: "Named only"}
	tests := []struct {
		name      string
		stored    *HexCell
		entry     UpdateHexCellInput
		ok        bool
		wantSet   bool
		wantPiece *int
	}{
		{"a piece with its terrain", nil, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("forest"), Piece: pieceOf(0)}, true, true, intp(0)},
		{"the last piece", nil, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("swamp"), Piece: pieceOf(HexPiecesPerTerrain - 1)}, true, true, intp(HexPiecesPerTerrain - 1)},
		{"one past the last piece", nil, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("swamp"), Piece: pieceOf(HexPiecesPerTerrain)}, false, false, nil},
		{"a negative piece", nil, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("forest"), Piece: pieceOf(-1)}, false, false, nil},
		{"a road has no pieces", nil, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("road"), Piece: pieceOf(0)}, false, false, nil},
		{"Mix with its terrain", nil, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("forest"), Piece: patch.Null[int]()}, true, true, nil},
		{"a piece alone uses the stored terrain", &forest, UpdateHexCellInput{Col: 1, Row: 1, Piece: pieceOf(3)}, true, true, intp(3)},
		{"a piece alone on an unpainted hex", nil, UpdateHexCellInput{Col: 1, Row: 1, Piece: pieceOf(3)}, false, false, nil},
		{"a piece alone on a hex with only a name", &bare, UpdateHexCellInput{Col: 1, Row: 1, Piece: pieceOf(3)}, false, false, nil},
		{"a piece with the terrain cleared", &forest, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchNull(), Piece: pieceOf(3)}, false, false, nil},
		{"Mix alone is accepted", &forest, UpdateHexCellInput{Col: 1, Row: 1, Piece: patch.Null[int]()}, true, true, nil},
		{"repainting with another terrain keeps a piece that still fits", &forest, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("hills")}, true, false, nil},
		{"painting a road puts the hex back to Mix", &forest, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchOf("road")}, true, true, nil},
		{"clearing the terrain puts the hex back to Mix", &forest, UpdateHexCellInput{Col: 1, Row: 1, Terrain: patchNull()}, true, true, nil},
		{"a rename leaves the piece alone", &forest, UpdateHexCellInput{Col: 1, Row: 1, Name: patchOf("Greywood")}, true, false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeHexRepo()
			if tc.stored != nil {
				repo.cells[HexKey{tc.stored.Col, tc.stored.Row}] = *tc.stored
			}
			_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner,
				[]UpdateHexCellInput{tc.entry})
			if !tc.ok {
				if !isBadRequest(err) || len(repo.applied) != 0 {
					t.Fatalf("want 400 and no write, got %v (%d writes)", err, len(repo.applied))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			w := repo.applied[0][0]
			if w.PieceSet != tc.wantSet || !intPtrEq(w.Piece, tc.wantPiece) {
				t.Errorf("piece write = set %v, %v; want set %v, %v", w.PieceSet, ptrStr(w.Piece), tc.wantSet, ptrStr(tc.wantPiece))
			}
		})
	}
}

func TestPatchCells_PieceFoldsWithLaterEntries(t *testing.T) {
	repo := newFakeHexRepo()
	_, err := hexSvc(repo, DrawWhoScribes).PatchCells(context.Background(), "camp-1", "map-1", actorOwner, []UpdateHexCellInput{
		{Col: 2, Row: 2, Terrain: patchOf("forest"), Piece: pieceOf(4)},
		{Col: 2, Row: 2, Piece: pieceOf(7)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.applied[0]) != 1 || !repo.applied[0][0].PieceSet || *repo.applied[0][0].Piece != 7 {
		t.Errorf("writes = %+v, want one write with piece 7", repo.applied[0])
	}
}

func TestHexHandler_PatchBindsPiece(t *testing.T) {
	repo := newFakeHexRepo()
	h := NewHexHandler(hexSvc(repo, DrawWhoScribes))
	rec := hexRequest(t, h, http.MethodPatch,
		`{"cells":[{"col":1,"row":1,"terrain":"hills","piece":2},{"col":2,"row":2,"terrain":"forest","piece":null},{"col":3,"row":3,"terrain":"water"}]}`,
		campaigns.RoleOwner, false, h.PatchHexCells)
	if rec.status != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.status, rec.Body.String())
	}
	w := repo.applied[0]
	if !w[0].PieceSet || w[0].Piece == nil || *w[0].Piece != 2 {
		t.Errorf("entry 0 = %+v: piece 2 was sent", w[0])
	}
	if !w[1].PieceSet || w[1].Piece != nil {
		t.Errorf("entry 1 = %+v: a null piece is Mix", w[1])
	}
	if w[2].PieceSet {
		t.Errorf("entry 2 = %+v: no piece was sent, so none may be written", w[2])
	}
}

// artFixture is fogFixture with a recording art writer.
func artFixture(stored string) (HexService, *fakeHexRepo, *fakeHexEvents, *[]string) {
	svc, repo, events, _ := fogFixture(PartyWhoScribes)
	m := &Map{ID: "map-1", CampaignID: "camp-1", ImageWidth: 1000, ImageHeight: 1000,
		Display: &DisplaySettings{Grid: &GridDisplay{Type: GridHex, Size: 100}}}
	if stored != "" {
		m.Display.Hexes = &HexesDisplay{Art: stored}
	}
	svc.SetMapLoader(func(context.Context, string) (*Map, error) { return m, nil })
	writes := []string{}
	svc.SetArtWriter(func(_ context.Context, mapID, art string) error {
		writes = append(writes, art)
		return nil
	})
	return svc, repo, events, &writes
}

func TestUpdateLayer_Art(t *testing.T) {
	tests := []struct {
		name    string
		actor   HexActor
		in      UpdateHexLayerInput
		wantErr bool
		want    string
	}{
		{"owner picks Realistic", actorOwner, UpdateHexLayerInput{Art: patch.Of(HexArtRealistic)}, false, HexArtRealistic},
		{"a DM grant picks Simple", actorDM, UpdateHexLayerInput{Art: patch.Of(HexArtSimple)}, false, HexArtSimple},
		{"null restores Detailed", actorOwner, UpdateHexLayerInput{Art: patch.Null[string]()}, false, HexArtDetailed},
		{"an unknown style is refused", actorOwner, UpdateHexLayerInput{Art: patch.Of("photo")}, true, ""},
		{"an empty style is refused", actorOwner, UpdateHexLayerInput{Art: patch.Of("")}, true, ""},
		{"a bad style refuses the whole body", actorOwner, UpdateHexLayerInput{FogEnabled: patch.Of(true), Art: patch.Of("photo")}, true, ""},
		{"a scribe is refused", actorScribe, UpdateHexLayerInput{Art: patch.Of(HexArtRealistic)}, true, ""},
		{"a player is refused", actorPlayer, UpdateHexLayerInput{Art: patch.Of(HexArtRealistic)}, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, writes := artFixture("")
			res, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", tc.actor, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				if len(*writes) != 0 || repo.bumps != 0 || len(events.calls) != 0 || repo.layer != nil {
					t.Errorf("a refused change must not write or publish: art=%v bumps=%d events=%d", *writes, repo.bumps, len(events.calls))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(*writes) != 1 || (*writes)[0] != tc.want || res.Art != tc.want {
				t.Errorf("art writes = %v, result %q; want one write of %q", *writes, res.Art, tc.want)
			}
			// The version moves and is announced, so open viewers read the new art.
			if repo.bumps != 1 || len(events.calls) != 1 || events.calls[0].version != res.Version || res.Version == 0 {
				t.Errorf("bumps=%d events=%+v version=%d: the change must be announced once", repo.bumps, events.calls, res.Version)
			}
		})
	}
}

// A body naming only the art changes nothing else on the layer.
func TestUpdateLayer_ArtLeavesTheLayerAlone(t *testing.T) {
	svc, repo, _, _ := artFixture("")
	repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, MilesPerHex: 9, MilesPerDay: 30, Version: 4}
	if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{Art: patch.Of(HexArtSimple)}); err != nil {
		t.Fatal(err)
	}
	if !repo.layer.FogEnabled || repo.layer.MilesPerHex != 9 || repo.layer.MilesPerDay != 30 || repo.anchorSets != 0 || repo.travelSets != 0 {
		t.Errorf("layer = %+v: an art change must not touch fog, travel or the anchor", repo.layer)
	}
}

func TestUpdateLayer_ArtFailsClosedWithoutWriter(t *testing.T) {
	svc, repo, events, _ := fogFixture(PartyWhoScribes)
	_, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{Art: patch.Of(HexArtRealistic)})
	if err == nil || repo.bumps != 0 || len(events.calls) != 0 {
		t.Errorf("err=%v bumps=%d: an unwired art write must fail without writing", err, repo.bumps)
	}
}

func TestUpdateLayer_ArtWriterErrorStopsTheBump(t *testing.T) {
	svc, repo, events, _ := fogFixture(PartyWhoScribes)
	svc.SetArtWriter(func(context.Context, string, string) error { return errors.New("db down") })
	if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{Art: patch.Of(HexArtSimple)}); err == nil {
		t.Fatal("want the writer's error")
	}
	if repo.bumps != 0 || len(events.calls) != 0 {
		t.Errorf("bumps=%d events=%d: nothing may be announced when the art was not saved", repo.bumps, len(events.calls))
	}
}

func TestGetLayer_SendsArt(t *testing.T) {
	tests := []struct {
		name   string
		stored string
		want   string
	}{
		{"default", "", HexArtDetailed},
		{"realistic", HexArtRealistic, HexArtRealistic},
		{"simple", HexArtSimple, HexArtSimple},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, _ := artFixture(tc.stored)
			view, err := svc.GetLayer(context.Background(), "camp-1", "map-1", 0)
			if err != nil {
				t.Fatal(err)
			}
			if view.Art != tc.want {
				t.Errorf("art = %q, want %q", view.Art, tc.want)
			}
		})
	}
}

func TestHexHandler_PutLayerBindsArt(t *testing.T) {
	tests := []struct {
		body string
		want []string
	}{
		{`{"art":"real"}`, []string{HexArtRealistic}},
		{`{"art":null}`, []string{HexArtDetailed}},
		{`{"fog_enabled":true}`, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.body, func(t *testing.T) {
			svc, _, _, writes := artFixture("")
			h := NewHexHandler(svc)
			rec := hexRequest(t, h, http.MethodPut, tc.body, campaigns.RoleOwner, false, h.PutHexLayer)
			if rec.status != http.StatusOK {
				t.Fatalf("status = %d (%s)", rec.status, rec.Body.String())
			}
			got, _ := json.Marshal(*writes)
			want, _ := json.Marshal(tc.want)
			if string(got) != string(want) {
				t.Errorf("art writes = %s, want %s", got, want)
			}
		})
	}
}

// SetHexArt writes the art through the settings merge, so the party rule and
// the other groups survive it.
func TestMapService_SetHexArt(t *testing.T) {
	tests := []struct {
		name    string
		current *DisplaySettings
		art     string
		wantErr bool
		want    *DisplaySettings
	}{
		{"onto no settings", nil, HexArtRealistic, false, &DisplaySettings{Hexes: &HexesDisplay{Art: HexArtRealistic}}},
		{"keeps the party rule and the grid",
			&DisplaySettings{Grid: &GridDisplay{Type: GridHex}, Hexes: &HexesDisplay{PartyWho: PartyWhoOwners}}, HexArtSimple, false,
			&DisplaySettings{Grid: &GridDisplay{Type: GridHex}, Hexes: &HexesDisplay{PartyWho: PartyWhoOwners, Art: HexArtSimple}}},
		{"the default is not stored",
			&DisplaySettings{Grid: &GridDisplay{Type: GridHex}, Hexes: &HexesDisplay{Art: HexArtRealistic}}, HexArtDetailed, false,
			&DisplaySettings{Grid: &GridDisplay{Type: GridHex}}},
		{"an unknown style is refused", nil, "photo", true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saved *Map
			repo := &mockMapRepo{
				getMapFn: func(context.Context, string) (*Map, error) {
					return &Map{ID: "map-1", Name: "Isle", Display: tc.current}, nil
				},
				updateMapFn: func(_ context.Context, m *Map) error { saved = m; return nil },
			}
			err := NewMapService(repo).(*mapService).SetHexArt(context.Background(), "map-1", tc.art)
			if tc.wantErr {
				if err == nil || saved != nil {
					t.Fatalf("want an error and no save, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(saved.Display)
			want, _ := json.Marshal(tc.want)
			if string(got) != string(want) || saved.Name != "Isle" {
				t.Errorf("saved %s (name %q), want %s", got, saved.Name, want)
			}
		})
	}
}

func intPtrEq(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ptrStr(p *int) string {
	if p == nil {
		return "nil"
	}
	b, _ := json.Marshal(*p)
	return string(b)
}

// hex_fog_repository_integration_test.go proves the fog and party SQL against
// a real MariaDB: explored writes, the hide that frees a row, the reset, the
// stale-position conflict and that each write bumps the version once.
package maps

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestHexFogRepository_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewHexRepository(db)

	userID := newMapsDBID(t)
	campaignID := newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Hex Fog Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Hex Fog Test", campaignID, userID)
	newMap := func() string {
		id := newMapsDBID(t)
		mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`, id, campaignID, "Fog Map")
		return id
	}
	isConflict := func(err error) bool {
		var ae *apperror.AppError
		return errors.As(err, &ae) && ae.Code == http.StatusConflict
	}

	t.Run("fog toggles without touching anything else", func(t *testing.T) {
		mapID := newMap()
		anchor := newMapsDBID(t)
		if _, err := repo.SetAnchor(ctx, mapID, &anchor); err != nil {
			t.Fatal(err)
		}
		v, err := repo.SetFog(ctx, mapID, true)
		if err != nil || v != 2 {
			t.Fatalf("SetFog = %d, %v; want version 2", v, err)
		}
		l, _ := repo.GetLayer(ctx, mapID)
		if !l.FogEnabled || l.AnchorDrawingID == nil || *l.AnchorDrawingID != anchor || l.PartyCol != nil {
			t.Errorf("layer = %+v; want fog on, anchor kept, no party", l)
		}
		if _, err := repo.SetFog(ctx, mapID, false); err != nil {
			t.Fatal(err)
		}
		l, _ = repo.GetLayer(ctx, mapID)
		if l.FogEnabled || l.AnchorDrawingID == nil {
			t.Errorf("layer = %+v; want fog off, anchor kept", l)
		}
	})

	t.Run("fog on a map with no layer row creates it", func(t *testing.T) {
		mapID := newMap()
		v, err := repo.SetFog(ctx, mapID, true)
		if err != nil || v != 1 {
			t.Fatalf("SetFog = %d, %v; want version 1", v, err)
		}
	})

	t.Run("travel writes only the figure named", func(t *testing.T) {
		mapID := newMap()
		nine, thirty := 9, 30
		v, err := repo.SetTravel(ctx, mapID, &nine, nil)
		if err != nil || v != 1 {
			t.Fatalf("SetTravel = %d, %v; want version 1 on a new row", v, err)
		}
		l, _ := repo.GetLayer(ctx, mapID)
		if l.MilesPerHex != 9 || l.MilesPerDay != DefaultMilesPerDay {
			t.Errorf("layer = %+v; want 9 per hex and the default per day", l)
		}
		if _, err := repo.SetFog(ctx, mapID, true); err != nil {
			t.Fatal(err)
		}
		v, err = repo.SetTravel(ctx, mapID, nil, &thirty)
		if err != nil || v != 3 {
			t.Fatalf("SetTravel = %d, %v; want version 3", v, err)
		}
		l, _ = repo.GetLayer(ctx, mapID)
		if l.MilesPerHex != 9 || l.MilesPerDay != 30 || !l.FogEnabled {
			t.Errorf("layer = %+v; want 9 / 30 with fog kept", l)
		}
	})

	t.Run("reveal, hide and list explored", func(t *testing.T) {
		mapID := newMap()
		// A painted hex keeps its row when hidden; a bare explored one loses it.
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{Col: 1, Row: 1, TerrainSet: true, Terrain: hexStrp("forest")}}); err != nil {
			t.Fatal(err)
		}
		keys := []HexKey{{1, 1}, {2, 1}, {3, 1}}
		v, err := repo.SetExplored(ctx, mapID, userID, keys, true)
		if err != nil || v != 2 {
			t.Fatalf("reveal = %d, %v; want version 2", v, err)
		}
		got, err := repo.ListExplored(ctx, mapID)
		if err != nil || len(got) != 3 {
			t.Fatalf("explored = %v, %v; want 3", got, err)
		}
		cells, _ := repo.GetCells(ctx, mapID, []HexKey{{1, 1}})
		if c := cells[HexKey{1, 1}]; c.Terrain == nil || *c.Terrain != "forest" || !c.Explored {
			t.Errorf("revealing must keep the painted terrain: %+v", c)
		}
		if _, err := repo.SetExplored(ctx, mapID, userID, keys, false); err != nil {
			t.Fatal(err)
		}
		if got, _ := repo.ListExplored(ctx, mapID); len(got) != 0 {
			t.Errorf("explored after hide = %v, want none", got)
		}
		all, _ := repo.ListCells(ctx, mapID)
		if len(all) != 1 || all[0].Col != 1 || all[0].Terrain == nil {
			t.Errorf("cells after hide = %+v; want only the painted hex to keep its row", all)
		}
	})

	t.Run("hiding a hex that was never stored is harmless", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.SetExplored(ctx, mapID, userID, []HexKey{{9, 9}}, false); err != nil {
			t.Fatal(err)
		}
		if n, _ := repo.CountCells(ctx, mapID); n != 0 {
			t.Errorf("stored %d cells, want 0", n)
		}
	})

	t.Run("reset hides everything and frees bare rows", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{Col: 0, Row: 0, NameSet: true, Name: "Keep"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.SetExplored(ctx, mapID, userID, []HexKey{{0, 0}, {5, 5}, {6, 6}}, true); err != nil {
			t.Fatal(err)
		}
		v, err := repo.ResetExplored(ctx, mapID, userID)
		if err != nil || v != 3 {
			t.Fatalf("reset = %d, %v; want version 3", v, err)
		}
		if got, _ := repo.ListExplored(ctx, mapID); len(got) != 0 {
			t.Errorf("explored after reset = %v", got)
		}
		cells, _ := repo.ListCells(ctx, mapID)
		if len(cells) != 1 || cells[0].Name != "Keep" {
			t.Errorf("cells after reset = %+v; want only the named hex", cells)
		}
	})

	t.Run("reset only touches its own map", func(t *testing.T) {
		a, b := newMap(), newMap()
		for _, id := range []string{a, b} {
			if _, err := repo.SetExplored(ctx, id, userID, []HexKey{{1, 1}}, true); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := repo.ResetExplored(ctx, a, userID); err != nil {
			t.Fatal(err)
		}
		if got, _ := repo.ListExplored(ctx, b); len(got) != 1 {
			t.Errorf("another map lost its explored hexes: %v", got)
		}
	})

	t.Run("party move places, reveals and bumps once", func(t *testing.T) {
		mapID := newMap()
		reveal := []HexKey{{5, 5}, {6, 5}, {4, 5}}
		v, err := repo.ApplyParty(ctx, mapID, userID, nil, HexKey{5, 5}, reveal)
		if err != nil || v != 1 {
			t.Fatalf("first placement = %d, %v; want version 1", v, err)
		}
		l, _ := repo.GetLayer(ctx, mapID)
		if l.PartyCol == nil || *l.PartyCol != 5 || *l.PartyRow != 5 {
			t.Fatalf("party = %v,%v; want 5,5", l.PartyCol, l.PartyRow)
		}
		if got, _ := repo.ListExplored(ctx, mapID); len(got) != 3 {
			t.Errorf("explored = %v; want the 3 revealed hexes", got)
		}
		from := HexKey{5, 5}
		v, err = repo.ApplyParty(ctx, mapID, userID, &from, HexKey{8, 5}, []HexKey{{8, 5}})
		if err != nil || v != 2 {
			t.Fatalf("second move = %d, %v; want version 2", v, err)
		}
		l, _ = repo.GetLayer(ctx, mapID)
		if *l.PartyCol != 8 || l.FogEnabled {
			t.Errorf("layer = %+v; want party at 8,5 and fog untouched", l)
		}
	})

	t.Run("a stale position is a conflict and changes nothing", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.ApplyParty(ctx, mapID, userID, nil, HexKey{1, 1}, []HexKey{{1, 1}}); err != nil {
			t.Fatal(err)
		}
		wrong := HexKey{9, 9}
		for name, from := range map[string]*HexKey{"wrong position": &wrong, "never placed": nil} {
			if _, err := repo.ApplyParty(ctx, mapID, userID, from, HexKey{2, 2}, []HexKey{{2, 2}}); !isConflict(err) {
				t.Errorf("%s: err = %v, want a conflict", name, err)
			}
		}
		l, _ := repo.GetLayer(ctx, mapID)
		if *l.PartyCol != 1 || l.Version != 1 {
			t.Errorf("layer = %+v; a refused move must roll back (party 1,1, version 1)", l)
		}
		if got, _ := repo.ListExplored(ctx, mapID); len(got) != 1 {
			t.Errorf("explored = %v; a refused move must not reveal", got)
		}
		unplaced := newMap()
		if _, err := repo.ApplyParty(ctx, unplaced, userID, &wrong, HexKey{2, 2}, nil); !isConflict(err) {
			t.Errorf("expecting a position the party never had: err = %v, want a conflict", err)
		}
	})

	t.Run("an empty-name explored hex never trips the empty-row sweep of paints", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.SetExplored(ctx, mapID, userID, []HexKey{{3, 3}}, true); err != nil {
			t.Fatal(err)
		}
		// Clearing terrain on an explored hex must keep its row: explored is data.
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{Col: 3, Row: 3, TerrainSet: true, Terrain: nil}}); err != nil {
			t.Fatal(err)
		}
		if got, _ := repo.ListExplored(ctx, mapID); len(got) != 1 {
			t.Errorf("explored = %v; want the hex to survive a cleared paint", got)
		}
	})
}

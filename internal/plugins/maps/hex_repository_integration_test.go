// hex_repository_integration_test.go proves the hex repository's SQL against a
// real MariaDB: the upsert's per-field merge, version bumping, FK cascade and
// the explored flag the service filters on. Uses the scratch-schema helper from
// repository_integration_test.go and skips when no server is reachable.
package maps

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func hexStrp(s string) *string { return &s }

func TestHexMigration_Reapply_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	// Tables exist after the helper's migration run.
	for _, tbl := range []string{"map_hex_layers", "map_hex_cells"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name = ?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s missing (n=%d, err=%v)", tbl, n, err)
		}
	}
	// Re-running the DDL must be harmless (CREATE TABLE IF NOT EXISTS).
	sqlBytes, err := MigrationsFS.ReadFile("migrations/009_hex_layers.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for _, stmt := range splitSQLStatements(string(sqlBytes)) {
		mustExecMaps(t, db, stmt)
	}
}

// splitSQLStatements drops comment lines and splits on ';' — enough for this
// migration, which has no semicolons inside literals.
func splitSQLStatements(src string) []string {
	var out []string
	cur := ""
	for _, line := range splitLines(src) {
		trimmed := line
		for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
			trimmed = trimmed[1:]
		}
		if len(trimmed) >= 2 && trimmed[:2] == "--" {
			continue
		}
		cur += line + "\n"
		if len(trimmed) > 0 && trimmed[len(trimmed)-1] == ';' {
			out = append(out, cur)
			cur = ""
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func TestHexRepository_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewHexRepository(db)

	userID := newMapsDBID(t)
	campaignID := newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Hex Int Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Hex Int Test", campaignID, userID)
	newMap := func() string {
		id := newMapsDBID(t)
		mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`, id, campaignID, "Hex Map")
		return id
	}

	t.Run("first write creates layer, version bumps per batch", func(t *testing.T) {
		mapID := newMap()
		layer, err := repo.GetLayer(ctx, mapID)
		if err != nil || layer != nil {
			t.Fatalf("GetLayer before write = %v, %v; want nil, nil", layer, err)
		}
		for i, want := range []uint64{1, 2, 3} {
			v, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{
				{Col: i, Row: 0, TerrainSet: true, Terrain: hexStrp("forest")},
			})
			if err != nil || v != want {
				t.Fatalf("batch %d: version=%d err=%v; want %d", i, v, err, want)
			}
		}
		layer, err = repo.GetLayer(ctx, mapID)
		if err != nil || layer == nil {
			t.Fatalf("GetLayer after write = %v, %v", layer, err)
		}
		if layer.Version != 3 || layer.MilesPerHex != 6 || layer.MilesPerDay != 24 || layer.FogEnabled {
			t.Errorf("layer = %+v; want version 3 with column defaults 6/24, fog off", layer)
		}
	})

	t.Run("a version bump creates the layer and touches nothing else", func(t *testing.T) {
		mapID := newMap()
		for _, want := range []uint64{1, 2} {
			v, err := repo.BumpVersion(ctx, mapID)
			if err != nil || v != want {
				t.Fatalf("BumpVersion = %d, %v; want %d", v, err, want)
			}
		}
		layer, err := repo.GetLayer(ctx, mapID)
		if err != nil || layer == nil || layer.Version != 2 || layer.FogEnabled || layer.AnchorDrawingID != nil || layer.MilesPerHex != 6 {
			t.Fatalf("layer = %+v, %v; want version 2 and every column at its default", layer, err)
		}
	})

	t.Run("partial update", func(t *testing.T) {
		tests := []struct {
			name        string
			second      HexCellWrite
			wantTerrain *string
			wantName    string
			wantNotes   *string
			wantPiece   *int
		}{
			{"rename keeps terrain, piece and notes",
				HexCellWrite{NameSet: true, Name: "Ashford"},
				hexStrp("hills"), "Ashford", hexStrp("old road"), intp(4)},
			{"null terrain clears only terrain",
				HexCellWrite{TerrainSet: true, Terrain: nil},
				nil, "Old", hexStrp("old road"), intp(4)},
			{"null notes clears only notes",
				HexCellWrite{NotesSet: true, Notes: nil},
				hexStrp("hills"), "Old", nil, intp(4)},
			{"empty name clears only name",
				HexCellWrite{NameSet: true, Name: ""},
				hexStrp("hills"), "", hexStrp("old road"), intp(4)},
			{"a piece replaces only the piece",
				HexCellWrite{PieceSet: true, Piece: intp(9)},
				hexStrp("hills"), "Old", hexStrp("old road"), intp(9)},
			{"a null piece is Mix and clears only the piece",
				HexCellWrite{PieceSet: true, Piece: nil},
				hexStrp("hills"), "Old", hexStrp("old road"), nil},
			{"repainting the terrain keeps the piece",
				HexCellWrite{TerrainSet: true, Terrain: hexStrp("forest")},
				hexStrp("forest"), "Old", hexStrp("old road"), intp(4)},
			{"empty write changes nothing",
				HexCellWrite{},
				hexStrp("hills"), "Old", hexStrp("old road"), intp(4)},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mapID := newMap()
				if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{
					Col: 2, Row: 3,
					TerrainSet: true, Terrain: hexStrp("hills"),
					PieceSet: true, Piece: intp(4),
					NameSet: true, Name: "Old",
					NotesSet: true, Notes: hexStrp("old road"),
				}}); err != nil {
					t.Fatalf("seed: %v", err)
				}
				w := tc.second
				w.Col, w.Row = 2, 3
				if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{w}); err != nil {
					t.Fatalf("second write: %v", err)
				}
				got, err := repo.GetCells(ctx, mapID, []HexKey{{2, 3}})
				if err != nil {
					t.Fatal(err)
				}
				c, ok := got[HexKey{2, 3}]
				if !ok {
					t.Fatal("cell missing")
				}
				if !eqStrPtr(c.Terrain, tc.wantTerrain) || c.Name != tc.wantName || !eqStrPtr(c.Notes, tc.wantNotes) || !intPtrEq(c.Piece, tc.wantPiece) {
					t.Errorf("cell = terrain %v name %q notes %v piece %s; want %v %q %v %s",
						deref(c.Terrain), c.Name, deref(c.Notes), ptrStr(c.Piece), deref(tc.wantTerrain), tc.wantName, deref(tc.wantNotes), ptrStr(tc.wantPiece))
				}
			})
		}
	})

	t.Run("unnamed fields on a fresh cell take column defaults", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.ApplyCells(ctx, mapID, "", []HexCellWrite{{Col: 0, Row: 0, NameSet: true, Name: "Only"}}); err != nil {
			t.Fatal(err)
		}
		cells, _ := repo.ListCells(ctx, mapID)
		if len(cells) != 1 || cells[0].Terrain != nil || cells[0].Notes != nil || cells[0].Explored || cells[0].UpdatedBy != nil {
			t.Errorf("cells = %+v; want one cell with NULL terrain/notes/updated_by, unexplored", cells)
		}
	})

	t.Run("read back exactly the stored cells, ordered row then col", func(t *testing.T) {
		mapID := newMap()
		other := newMap()
		writes := []HexCellWrite{
			{Col: 5, Row: 1, TerrainSet: true, Terrain: hexStrp("water")},
			{Col: -2, Row: 0, TerrainSet: true, Terrain: hexStrp("forest")},
			{Col: 0, Row: 0, NameSet: true, Name: "Origin"},
		}
		if _, err := repo.ApplyCells(ctx, mapID, userID, writes); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ApplyCells(ctx, other, userID, []HexCellWrite{{Col: 9, Row: 9, NameSet: true, Name: "Elsewhere"}}); err != nil {
			t.Fatal(err)
		}
		cells, err := repo.ListCells(ctx, mapID)
		if err != nil {
			t.Fatal(err)
		}
		want := []HexKey{{-2, 0}, {0, 0}, {5, 1}}
		if len(cells) != len(want) {
			t.Fatalf("got %d cells, want %d", len(cells), len(want))
		}
		for i, k := range want {
			if cells[i].Col != k.Col || cells[i].Row != k.Row {
				t.Errorf("cell %d = (%d,%d); want (%d,%d)", i, cells[i].Col, cells[i].Row, k.Col, k.Row)
			}
		}
		got, err := repo.GetCells(ctx, mapID, []HexKey{{5, 1}, {7, 7}, {9, 9}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[HexKey{5, 1}].Terrain == nil || *got[HexKey{5, 1}].Terrain != "water" {
			t.Errorf("GetCells = %+v; want only (5,1) water (no leak from other map, no phantom cells)", got)
		}
		empty, err := repo.GetCells(ctx, mapID, nil)
		if err != nil || len(empty) != 0 {
			t.Errorf("GetCells(nil) = %v, %v; want empty", empty, err)
		}
	})

	t.Run("explored flag round-trips for VisibleCells", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{
			{Col: 0, Row: 0, NameSet: true, Name: "seen"},
			{Col: 1, Row: 0, NameSet: true, Name: "hidden"},
		}); err != nil {
			t.Fatal(err)
		}
		mustExecMaps(t, db, "UPDATE map_hex_cells SET explored = 1 WHERE map_id = ? AND col = 0 AND `row` = 0", mapID)
		cells, err := repo.ListCells(ctx, mapID)
		if err != nil {
			t.Fatal(err)
		}
		if len(cells) != 2 || !cells[0].Explored || cells[1].Explored {
			t.Fatalf("explored flags = %+v; want [true false]", cells)
		}
		layer := DefaultHexLayer(mapID)
		layer.FogEnabled = true
		if got := VisibleCells(layer, cells, int(permissions.RolePlayer)); len(got) != 1 || got[0].Name != "seen" {
			t.Errorf("player view under fog = %+v; want only the explored cell", got)
		}
		if got := VisibleCells(layer, cells, int(permissions.RoleOwner)); len(got) != 2 {
			t.Errorf("owner view under fog = %d cells; want 2", len(got))
		}
	})

	t.Run("CountCells", func(t *testing.T) {
		mapID := newMap()
		if n, err := repo.CountCells(ctx, mapID); err != nil || n != 0 {
			t.Fatalf("empty count = %d, %v", n, err)
		}
		var writes []HexCellWrite
		for i := 0; i < 7; i++ {
			writes = append(writes, HexCellWrite{Col: i, Row: i, TerrainSet: true, Terrain: hexStrp("plains")})
		}
		// Re-writing an existing hex must not inflate the count.
		writes = append(writes, HexCellWrite{Col: 0, Row: 0, NameSet: true, Name: "dup"})
		if _, err := repo.ApplyCells(ctx, mapID, userID, writes); err != nil {
			t.Fatal(err)
		}
		if n, err := repo.CountCells(ctx, mapID); err != nil || n != 7 {
			t.Errorf("count = %d, %v; want 7", n, err)
		}
	})

	t.Run("a cleared hex is deleted, one with anything left stays", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{
			{Col: 0, Row: 0, TerrainSet: true, Terrain: hexStrp("forest")},
			{Col: 1, Row: 0, TerrainSet: true, Terrain: hexStrp("forest"), NameSet: true, Name: "Keep"},
			{Col: 2, Row: 0, NotesSet: true, Notes: hexStrp("note")},
			{Col: 3, Row: 0, TerrainSet: true, Terrain: hexStrp("hills")},
		}); err != nil {
			t.Fatal(err)
		}
		// An explored hex is held open by a later slice, so it must survive.
		mustExecMaps(t, db, "UPDATE map_hex_cells SET explored = 1 WHERE map_id = ? AND col = 3 AND `row` = 0", mapID)
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{
			{Col: 0, Row: 0, TerrainSet: true},
			{Col: 1, Row: 0, TerrainSet: true},
			{Col: 2, Row: 0, NotesSet: true},
			{Col: 3, Row: 0, TerrainSet: true},
		}); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetCells(ctx, mapID, []HexKey{{0, 0}, {1, 0}, {2, 0}, {3, 0}})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got[HexKey{0, 0}]; ok {
			t.Error("a hex with every field cleared must be deleted")
		}
		if c, ok := got[HexKey{1, 0}]; !ok || c.Name != "Keep" || c.Terrain != nil {
			t.Errorf("a hex that still has a name must stay: %+v", c)
		}
		if _, ok := got[HexKey{2, 0}]; ok {
			t.Error("a hex whose only field was cleared must be deleted")
		}
		if _, ok := got[HexKey{3, 0}]; !ok {
			t.Error("an explored hex must stay")
		}
		if n, err := repo.CountCells(ctx, mapID); err != nil || n != 2 {
			t.Errorf("count = %d, %v; want 2", n, err)
		}
	})

	t.Run("an untouched empty row elsewhere is not swept", func(t *testing.T) {
		mapID := newMap()
		mustExecMaps(t, db, "INSERT INTO map_hex_cells (map_id, col, `row`) VALUES (?, 9, 9)", mapID)
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{Col: 0, Row: 0, TerrainSet: true, Terrain: hexStrp("snow")}}); err != nil {
			t.Fatal(err)
		}
		if n, err := repo.CountCells(ctx, mapID); err != nil || n != 2 {
			t.Errorf("count = %d, %v; want 2 (only touched rows are checked)", n, err)
		}
	})

	t.Run("SetAnchor creates the layer, bumps the version and touches only the anchor", func(t *testing.T) {
		mapID := newMap()
		pic := newMapsDBID(t)
		v, err := repo.SetAnchor(ctx, mapID, &pic)
		if err != nil || v != 1 {
			t.Fatalf("first SetAnchor: version=%d err=%v; want 1", v, err)
		}
		mustExecMaps(t, db, `UPDATE map_hex_layers SET fog_enabled = 1, miles_per_hex = 9 WHERE map_id = ?`, mapID)
		v, err = repo.SetAnchor(ctx, mapID, &pic)
		if err != nil || v != 2 {
			t.Fatalf("second SetAnchor: version=%d err=%v; want 2", v, err)
		}
		layer, _ := repo.GetLayer(ctx, mapID)
		if layer == nil || layer.AnchorDrawingID == nil || *layer.AnchorDrawingID != pic || !layer.FogEnabled || layer.MilesPerHex != 9 {
			t.Fatalf("layer = %+v; want anchor %s with fog and miles untouched", layer, pic)
		}
		v, err = repo.SetAnchor(ctx, mapID, nil)
		if err != nil || v != 3 {
			t.Fatalf("clearing SetAnchor: version=%d err=%v; want 3", v, err)
		}
		layer, _ = repo.GetLayer(ctx, mapID)
		if layer.AnchorDrawingID != nil {
			t.Errorf("anchor after null = %v; want nil", *layer.AnchorDrawingID)
		}
		// A painted cell must survive an anchor change untouched.
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{Col: 2, Row: 2, TerrainSet: true, Terrain: hexStrp("water")}}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.SetAnchor(ctx, mapID, &pic); err != nil {
			t.Fatal(err)
		}
		if n, _ := repo.CountCells(ctx, mapID); n != 1 {
			t.Errorf("cells after SetAnchor = %d; want 1", n)
		}
	})

	t.Run("deleting the map cascades to both hex tables", func(t *testing.T) {
		mapID := newMap()
		if _, err := repo.ApplyCells(ctx, mapID, userID, []HexCellWrite{{Col: 1, Row: 1, NameSet: true, Name: "x"}}); err != nil {
			t.Fatal(err)
		}
		mustExecMaps(t, db, `DELETE FROM maps WHERE id = ?`, mapID)
		layer, err := repo.GetLayer(ctx, mapID)
		if err != nil || layer != nil {
			t.Errorf("layer after map delete = %v, %v; want nil", layer, err)
		}
		if n, err := repo.CountCells(ctx, mapID); err != nil || n != 0 {
			t.Errorf("cells after map delete = %d, %v; want 0", n, err)
		}
	})
}

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

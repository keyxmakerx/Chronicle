// repository_integration_test.go is a real-MariaDB proof that ListMarkers'
// and ListDrawings' SQL predicates agree with VisibilityRules.Allows on the
// anonymous-deny rule (ADR-049): a marker/drawing carrying a non-empty
// denied_users list must not be served to a request with no user id, the
// same way an authenticated-but-unrelated user still sees it.
//
// Each test gets its own scratch schema, fully migrated, and drops it on
// cleanup — deliberately NOT the caller's DSN database, since
// `make test-int-local` hands a DSN with no schema name selected and other
// agents may share the same server. Skips (never fails) when no server is
// reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/maps/... -run TestList
package maps

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// newMapsScratchDB creates and migrates a throwaway schema (core + maps
// plugin), returning a handle to it. Mirrors
// internal/plugins/sessions/dbtest_support_test.go's newScratchDB.
func newMapsScratchDB(t *testing.T) *sql.DB {
	t.Helper()

	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the row-level tests")
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
	}
	cfg.ParseTime = true

	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s: %v — run `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_maps_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	mapsSub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: "maps", MigrationsFS: mapsSub},
	}) {
		if !res.Healthy {
			t.Skipf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}
	return db
}

func newMapsDBID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func mustExecMaps(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestListMarkers_DeniedUsersExcludesAnonymous_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewMapRepository(db)

	userID := newMapsDBID(t)
	campaignID := newMapsDBID(t)
	mapID := newMapsDBID(t)
	deniedMarkerID := newMapsDBID(t)

	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Maps Vis Int Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Maps Vis Int Test", campaignID, userID)
	mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`,
		mapID, campaignID, "Test Map")
	mustExecMaps(t, db,
		`INSERT INTO map_markers (id, map_id, name, visibility, visibility_rules) VALUES (?, ?, ?, 'everyone', ?)`,
		deniedMarkerID, mapID, "Denied Marker", `{"denied_users":["u-bryn"]}`)

	rolePlayer := int(permissions.RolePlayer)

	anon, err := repo.ListMarkers(ctx, mapID, rolePlayer, "")
	if err != nil {
		t.Fatalf("ListMarkers(anonymous): %v", err)
	}
	if len(anon) != 0 {
		t.Errorf("anonymous saw %d markers; want 0 — a non-empty deny list must exclude an anonymous viewer (ADR-049)", len(anon))
	}

	other, err := repo.ListMarkers(ctx, mapID, rolePlayer, "u-someone-else")
	if err != nil {
		t.Fatalf("ListMarkers(authenticated non-member): %v", err)
	}
	if len(other) != 1 {
		t.Errorf("authenticated non-member saw %d markers; want 1 — only the named player is denied", len(other))
	}

	denied, err := repo.ListMarkers(ctx, mapID, rolePlayer, "u-bryn")
	if err != nil {
		t.Fatalf("ListMarkers(denied player): %v", err)
	}
	if len(denied) != 0 {
		t.Errorf("denied player saw %d markers; want 0", len(denied))
	}
}

func TestListDrawings_DeniedUsersExcludesAnonymous_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewDrawingRepository(db)

	userID := newMapsDBID(t)
	campaignID := newMapsDBID(t)
	mapID := newMapsDBID(t)
	deniedDrawingID := newMapsDBID(t)

	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Maps Vis Int Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Maps Vis Int Test", campaignID, userID)
	mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`,
		mapID, campaignID, "Test Map")
	mustExecMaps(t, db,
		`INSERT INTO map_drawings (id, map_id, drawing_type, points, visibility, visibility_rules) VALUES (?, ?, 'rectangle', ?, 'everyone', ?)`,
		deniedDrawingID, mapID, `[]`, `{"denied_users":["u-bryn"]}`)

	rolePlayer := int(permissions.RolePlayer)

	anon, err := repo.ListDrawings(ctx, mapID, rolePlayer, "")
	if err != nil {
		t.Fatalf("ListDrawings(anonymous): %v", err)
	}
	if len(anon) != 0 {
		t.Errorf("anonymous saw %d drawings; want 0 — a non-empty deny list must exclude an anonymous viewer (ADR-049)", len(anon))
	}

	other, err := repo.ListDrawings(ctx, mapID, rolePlayer, "u-someone-else")
	if err != nil {
		t.Fatalf("ListDrawings(authenticated non-member): %v", err)
	}
	if len(other) != 1 {
		t.Errorf("authenticated non-member saw %d drawings; want 1 — only the named player is denied", len(other))
	}
}

// A picture round-trips its media id, crop and stacking position, and a
// picture hidden from players (dm_only) is absent from a player's list while
// staff still get it.
func TestPictureDrawings_RoundTripAndHiddenFromPlayers_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	repo := NewDrawingRepository(db)

	userID := newMapsDBID(t)
	campaignID := newMapsDBID(t)
	mapID := newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Maps Picture Int Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Maps Picture Int Test", campaignID, userID)
	mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`,
		mapID, campaignID, "Test Map")

	img := "media-1"
	shown := &Drawing{
		ID: newMapsDBID(t), MapID: mapID, DrawingType: "image", Points: []byte(`[{"x":1,"y":1},{"x":9,"y":9}]`),
		StrokeColor: "#000000", StrokeWidth: 2, FillAlpha: 0.6, Visibility: "everyone",
		ImageID: &img, Crop: []byte(`{"t":5,"r":0,"b":0,"l":0}`), SortOrder: 2,
	}
	hidden := &Drawing{
		ID: newMapsDBID(t), MapID: mapID, DrawingType: "image", Points: []byte(`[{"x":1,"y":1},{"x":9,"y":9}]`),
		StrokeColor: "#000000", StrokeWidth: 2, FillAlpha: 1, Visibility: "dm_only", ImageID: &img,
	}
	for _, d := range []*Drawing{shown, hidden} {
		if err := repo.CreateDrawing(ctx, d); err != nil {
			t.Fatalf("CreateDrawing: %v", err)
		}
	}

	got, err := repo.GetDrawing(ctx, shown.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageID == nil || *got.ImageID != img || got.SortOrder != 2 || string(got.Crop) == "" {
		t.Errorf("picture did not round-trip: %+v", got)
	}
	plain, _ := repo.GetDrawing(ctx, hidden.ID)
	if len(plain.Crop) != 0 {
		t.Errorf("an unset crop must read back empty, got %s", plain.Crop)
	}

	players, err := repo.ListDrawings(ctx, mapID, int(permissions.RolePlayer), "u-player")
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].ID != shown.ID {
		t.Errorf("player sees %d drawings, want only the visible picture", len(players))
	}
	staff, _ := repo.ListDrawings(ctx, mapID, int(permissions.RoleOwner), "u-owner")
	if len(staff) != 2 {
		t.Errorf("owner sees %d drawings, want both pictures", len(staff))
	}
}

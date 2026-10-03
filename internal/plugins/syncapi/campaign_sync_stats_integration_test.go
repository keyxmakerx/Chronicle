package syncapi

// The admin dashboard's per-campaign sync table is one hand-written query
// across four tables. A mock cannot show that every table it names exists,
// so this runs it against a migrated scratch schema. Skips (never fails)
// when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/syncapi/ -run TestCampaignSyncStatsIntegration

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

func newSyncStatsScratchDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (tools/start-test-db.sh) to run the row-level tests")
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("invalid CHRONICLE_TEST_DB_DSN: %v", err)
	}
	cfg.ParseTime = true
	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB reachable: %v", err)
	}
	name := fmt.Sprintf("chronicle_syncstats_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	if err := database.RunMigrations(db, scratch.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: "syncapi", MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("syncapi migrations did not apply: %v", res.Error)
		}
	}
	return db
}

func TestCampaignSyncStatsIntegration_CountsKeysMappingsAndErrors(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid, idle := "u-stats-0000000000000000000000000001", "c-stats-0000000000000000000000000001", "c-stats-0000000000000000000000000002"
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, "stats@example.test", "Ana", "x")
	mustExec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Synced", "synced", uid)
	mustExec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, idle, "Idle", "idle", uid)
	mustExec(`INSERT INTO api_keys (key_hash, key_prefix, user_id, campaign_id, name, permissions) VALUES (?,?,?,?,?,?)`,
		"h1", "chr_aaaa", uid, cid, "Foundry", `["read","write","sync"]`)
	var keyID int
	if err := db.QueryRow(`SELECT id FROM api_keys WHERE key_prefix = 'chr_aaaa'`).Scan(&keyID); err != nil {
		t.Fatal(err)
	}
	mustExec(`INSERT INTO sync_mappings (id, campaign_id, chronicle_type, chronicle_id, external_system, external_id) VALUES (?,?,?,?,?,?)`,
		"m-stats-0000000000000000000000000001", cid, "entity", "e-1", "foundry", "JournalEntry.abc")
	for _, status := range []int{200, 409, 500} {
		mustExec(`INSERT INTO api_request_log (api_key_id, campaign_id, user_id, method, path, status_code, ip_address) VALUES (?,?,?,?,?,?,?)`,
			keyID, cid, uid, "PUT", "/x", status, "127.0.0.1")
	}

	stats, err := NewSyncMappingRepository(db).ListCampaignSyncStats(ctx)
	if err != nil {
		t.Fatalf("ListCampaignSyncStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("want 1 campaign (the idle one has no keys or mappings), got %d: %+v", len(stats), stats)
	}
	s := stats[0]
	if s.CampaignID != cid || s.ActiveKeys != 1 || s.TotalMappings != 1 || s.RecentErrors != 2 || s.LastActivity == nil {
		t.Fatalf("unexpected stats: %+v", s)
	}
}

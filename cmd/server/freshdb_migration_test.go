package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// TestFreshDatabase_EveryPluginSchemaApplies replays the server's real schema
// bootstrap — core migrations, then foundry_vtt.PreMigrationCheck, then
// database.RunPluginMigrations over the real registeredPlugins() list — against
// a database that has NEVER been migrated, and requires every plugin to come up
// healthy.
//
// Why this test exists (C-SWEEP-R4 / data/fvtt-fresh-db-rename): CI had no
// fresh-DB replay anywhere. tools/restore-drill.sh loads a dump of an
// already-migrated database, and every other integration test assumes
// `make migrate-up` already ran. So `foundry_vtt`'s migration 001 — a
// consolidation migration that RENAMEs a table the deleted foundry_modules
// plugin used to create and DROPs a catalog table that only ever existed on an
// upgrade path — failed at its FIRST statement on every new self-hosted
// install, permanently disabling the Foundry integration, and nothing in the
// suite noticed. The runner returns on the first failed migration, so no later
// migration could ever repair it either.
//
// The test asserts the whole set, not just foundry_vtt: any plugin whose
// migrations assume a predecessor's schema fails here on the day it lands.
//
// Discovery + skip rules follow the house integration-test convention
// (internal/plugins/entities/repository_integration_test.go):
//   - Skipped under `-short`, so `make test-unit` / `make verify` never need a DB.
//   - Server DSN from CHRONICLE_TEST_DB_DSN, else the DB_* env vars, else the
//     dev default that matches the Makefile's DATABASE_URL.
//   - If no server answers, SKIP rather than fail.
//
// Unlike the other integration tests it does NOT use the configured database:
// it creates a uniquely-named scratch schema, migrates that from zero, and
// drops it again, so running it never touches dev data.
//
// Run with: `make docker-up && make test-freshdb`.
func TestFreshDatabase_EveryPluginSchemaApplies(t *testing.T) {
	if testing.Short() {
		t.Skip("fresh-database migration replay requires a database; skipped under -short")
	}

	db, freshDSN := newScratchSchema(t)

	// --- 1. Core migrations, exactly as cmd/server/main.go runs them. ---
	if err := database.RunMigrations(db, freshDSN, coreMigrationsDir(t)); err != nil {
		t.Fatalf("core migrations failed on a fresh database: %v", err)
	}

	// --- 2. The foundry_vtt pre-check + reconciler, in main.go's position. ---
	if err := foundry_vtt.PreMigrationCheck(context.Background(), db); err != nil {
		t.Fatalf("foundry_vtt.PreMigrationCheck failed on a fresh database: %v", err)
	}
	if err := foundry_vtt.ReconcileConsolidationState(context.Background(), db); err != nil {
		t.Fatalf("foundry_vtt.ReconcileConsolidationState failed on a fresh database: %v", err)
	}

	// --- 3. Every registered plugin's own migrations. ---
	results := database.RunPluginMigrations(db, registeredPlugins())
	if len(results) == 0 {
		t.Fatal("RunPluginMigrations returned no results — registeredPlugins() is empty?")
	}
	for _, r := range results {
		if !r.Healthy {
			t.Errorf("plugin %q is DEGRADED on a fresh database (version %d of %d): %v",
				r.Slug, r.Version, r.LatestVersion, r.Error)
			continue
		}
		if r.Version != r.LatestVersion {
			t.Errorf("plugin %q reports healthy at version %d but ships %d migrations",
				r.Slug, r.Version, r.LatestVersion)
		}
	}

	// --- 4. The specific table the fresh-install crash left missing. ---
	// foundry_vtt's repository reads and writes foundry_vtt_campaign_tokens on
	// every manifest URL issue and every token rotation; without it the plugin
	// is dead even when the migration runner reports success.
	var tokenTables int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = DATABASE() AND table_name = 'foundry_vtt_campaign_tokens'
	`).Scan(&tokenTables); err != nil {
		t.Fatalf("querying information_schema for foundry_vtt_campaign_tokens: %v", err)
	}
	if tokenTables != 1 {
		t.Errorf("foundry_vtt_campaign_tokens missing after a fresh-database bootstrap")
	}

	// The predecessor plugin's tables must NOT be resurrected by the repair:
	// the whole point of migration 001 is that they are gone after it.
	for _, dead := range []string{"foundry_module_campaign_tokens", "foundry_module_versions"} {
		var n int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.tables
			 WHERE table_schema = DATABASE() AND table_name = ?
		`, dead).Scan(&n); err != nil {
			t.Fatalf("querying information_schema for %s: %v", dead, err)
		}
		if n != 0 {
			t.Errorf("table %s exists after a fresh-database bootstrap; the post-consolidation "+
				"shape must not contain the deleted foundry_modules plugin's tables", dead)
		}
	}
}

// TestUpgradeDatabase_FoundryConsolidationStillRuns is the other half of the
// fresh-DB fix, and the one that keeps it from being a data-loss bug.
//
// The repair for data/fvtt-fresh-db-rename teaches the bootstrap to SKIP
// foundry_vtt's migration 001 when its RENAME has no source table. A skip that
// fired one state too wide would be far worse than the crash it replaces: on a
// real pre-consolidation database, 001's RENAME is what carries live
// per-campaign token rows into foundry_vtt_campaign_tokens. Skip it there and
// every campaign's signed manifest URL silently stops resolving, because the
// repository queries a table that was never populated.
//
// So this test builds the PRE-consolidation state by hand — the predecessor
// token table with a real row in it, plus the (empty) versions catalog — runs
// the identical bootstrap, and requires that the row arrived under the new
// name. The row is the proof: only 001's RENAME can put it there, since
// migration 002 only ever CREATEs the table empty.
func TestUpgradeDatabase_FoundryConsolidationStillRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("migration replay requires a database; skipped under -short")
	}

	db, dsn := newScratchSchema(t)

	if err := database.RunMigrations(db, dsn, coreMigrationsDir(t)); err != nil {
		t.Fatalf("core migrations failed: %v", err)
	}

	// --- Rebuild the pre-consolidation shape the deleted foundry_modules
	// plugin left behind, exactly as its migration 001 created it. ---
	mustExecFresh(t, db, `
		CREATE TABLE foundry_module_campaign_tokens (
			campaign_id    CHAR(36)  NOT NULL,
			token_version  INT       NOT NULL DEFAULT 1,
			rotated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (campaign_id),
			CONSTRAINT fk_fmct_campaign FOREIGN KEY (campaign_id)
				REFERENCES campaigns(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	// An EMPTY foundry_module_versions, which is the state PreMigrationCheck
	// allows and 001's second statement drops.
	mustExecFresh(t, db, `
		CREATE TABLE foundry_module_versions (
			id      CHAR(36)    NOT NULL,
			version VARCHAR(50) NOT NULL,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)

	// A campaign to hang the token row off (the FK is ON DELETE CASCADE).
	const userID = "11111111-1111-1111-1111-111111111111"
	const campaignID = "22222222-2222-2222-2222-222222222222"
	mustExecFresh(t, db,
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "fvtt-upgrade@example.test", "FVTT Upgrade", "x")
	mustExecFresh(t, db,
		`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "FVTT Upgrade", "fvtt-upgrade", userID)
	// token_version 7: a value no default could produce, so finding it on the
	// far side proves the ROW travelled rather than being re-created.
	mustExecFresh(t, db,
		`INSERT INTO foundry_module_campaign_tokens (campaign_id, token_version) VALUES (?, ?)`,
		campaignID, 7)

	// --- The identical bootstrap. ---
	if err := foundry_vtt.PreMigrationCheck(context.Background(), db); err != nil {
		t.Fatalf("PreMigrationCheck failed on a pre-consolidation database: %v", err)
	}
	if err := foundry_vtt.ReconcileConsolidationState(context.Background(), db); err != nil {
		t.Fatalf("ReconcileConsolidationState failed on a pre-consolidation database: %v", err)
	}
	for _, r := range database.RunPluginMigrations(db, registeredPlugins()) {
		if !r.Healthy {
			t.Fatalf("plugin %q DEGRADED on the upgrade path (version %d of %d): %v",
				r.Slug, r.Version, r.LatestVersion, r.Error)
		}
	}

	// --- The row is the proof that 001 actually ran. ---
	var version int
	err := db.QueryRow(
		`SELECT token_version FROM foundry_vtt_campaign_tokens WHERE campaign_id = ?`,
		campaignID).Scan(&version)
	if err != nil {
		t.Fatalf("the pre-consolidation token row did not survive the upgrade: %v. "+
			"Migration 001 was skipped on a database where its RENAME had real work to do — "+
			"every campaign's signed manifest URL would silently stop resolving", err)
	}
	if version != 7 {
		t.Errorf("token_version = %d, want 7 — the row was re-created, not carried across", version)
	}

	// And 001's second half still happened.
	if freshDBTableExists(t, db, "foundry_module_versions") {
		t.Error("foundry_module_versions still exists after the upgrade path")
	}
	if freshDBTableExists(t, db, "foundry_module_campaign_tokens") {
		t.Error("foundry_module_campaign_tokens still exists after the upgrade path")
	}
}

// newScratchSchema creates a uniquely-named, NEVER-migrated schema on the test
// server and returns a handle to it plus its DSN. The schema is dropped when
// the test finishes, so running these tests never touches dev data.
//
// Skips (rather than fails) when no server answers, per the house integration
// convention — see internal/plugins/entities/repository_integration_test.go.
func newScratchSchema(t *testing.T) (*sql.DB, string) {
	t.Helper()

	serverDSN, baseCfg := freshDBServerDSN(t)
	admin, err := sql.Open("mysql", serverDSN)
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s (ping: %v) — run `make docker-up`",
			baseCfg.Addr, err)
	}

	// The whole point is that NOTHING has been applied to it. Randomised so
	// parallel runs cannot collide.
	schema := fmt.Sprintf("chronicle_freshdb_%06d", rand.Intn(1000000))
	if _, err := admin.Exec("CREATE DATABASE `" + schema +
		"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema %s (needs CREATE privilege): %v", schema, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS `" + schema + "`"); err != nil {
			t.Logf("warning: could not drop scratch schema %s: %v", schema, err)
		}
	})

	cfg := *baseCfg
	cfg.DBName = schema
	dsn := cfg.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db, dsn
}

// mustExecFresh runs a fixture statement, failing the test on error.
func mustExecFresh(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("fixture failed: %v\nSQL: %.200s", err, query)
	}
}

// freshDBTableExists reports whether a table is present in the scratch schema.
func freshDBTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = DATABASE() AND table_name = ?
	`, table).Scan(&n); err != nil {
		t.Fatalf("querying information_schema for %s: %v", table, err)
	}
	return n > 0
}

// freshDBServerDSN returns a DSN pointing at the SERVER (no schema selected)
// plus the parsed config it was built from, so the caller can re-point it at a
// scratch schema.
func freshDBServerDSN(t *testing.T) (string, *mysql.Config) {
	t.Helper()

	var cfg *mysql.Config
	if raw := os.Getenv("CHRONICLE_TEST_DB_DSN"); raw != "" {
		parsed, err := mysql.ParseDSN(raw)
		if err != nil {
			t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
		}
		cfg = parsed
	} else {
		cfg = mysql.NewConfig()
		cfg.User = freshDBEnv("DB_USER", "chronicle")
		cfg.Passwd = freshDBEnv("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = freshDBEnv("DB_HOST", "127.0.0.1:3306")
	}
	cfg.ParseTime = true
	cfg.MultiStatements = false

	serverCfg := *cfg
	serverCfg.DBName = ""
	return serverCfg.FormatDSN(), cfg
}

func freshDBEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// coreMigrationsDir resolves db/migrations from this test file's own location,
// so the test does not depend on the working directory `go test` chose.
func coreMigrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this test file's path")
	}
	// cmd/server/freshdb_migration_test.go → repo root is two levels up.
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	dir := filepath.Join(root, "db", "migrations")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("core migrations dir %s not found: %v", dir, err)
	}
	// golang-migrate's file:// source wants forward slashes.
	return strings.ReplaceAll(dir, string(filepath.Separator), "/")
}

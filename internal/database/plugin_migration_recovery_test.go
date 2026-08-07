package database

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-sql-driver/mysql"
)

// plugin_migration_recovery_test.go replays the whole failure of
// C-SWEEP-R4 / data/plugin-migration-no-transaction against a real MariaDB,
// because it is a claim about what the SERVER does with a half-applied
// migration and no string assertion over the runner can answer it.
//
// Discovery + skip rules follow the house integration convention: skipped
// under -short, DSN from CHRONICLE_TEST_DB_DSN else the DB_* env vars else the
// Makefile's dev default, and SKIP (not fail) when no server answers. Each
// test migrates its own throwaway schema, so dev data is never touched.

// TestPluginMigration_ResumesInsteadOfCrashLooping is the reproduction and the
// fix in one run.
//
// The migration's third statement adds a foreign key to a table that does not
// exist yet — a failure the table-granularity pre-flight deliberately does not
// model, so it is exactly the case the resume offset has to carry.
//
// Attempt 1: statements 1 and 2 apply, statement 3 fails.
// Attempt 2: the environment is still broken, so it must fail AT STATEMENT 3
// again — with the original error. Before this fix it failed at statement 2
// with "Duplicate column name", because it replayed a non-idempotent ALTER
// that had already applied. That is the crash-loop: the real error was
// replaced by an artefact of the retry, and no amount of fixing the real
// problem could ever get past it.
// Attempt 3: the missing table is created — the operator repair the finding
// says must be possible — and the migration completes and records its version.
func TestPluginMigration_ResumesInsteadOfCrashLooping(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin migration recovery test requires a database; skipped under -short")
	}
	db := newMigrationScratchSchema(t)

	const slug = "sweeptest"
	up := strings.Join([]string{
		"CREATE TABLE IF NOT EXISTS sweep_recover (id INT NOT NULL PRIMARY KEY);",
		"ALTER TABLE sweep_recover ADD COLUMN note VARCHAR(50);",
		"ALTER TABLE sweep_recover ADD CONSTRAINT fk_sweep_recover " +
			"FOREIGN KEY (id) REFERENCES sweep_parent(id);",
	}, "\n")
	schema := PluginSchema{Slug: slug, MigrationsFS: fstest.MapFS{
		"001_recover.up.sql":   {Data: []byte(up)},
		"001_recover.down.sql": {Data: []byte("DROP TABLE IF EXISTS sweep_recover;")},
	}}

	// --- Attempt 1: fails on the third statement. ---
	res := runOnePlugin(t, db, schema)
	if res.Healthy {
		t.Fatal("attempt 1 unexpectedly succeeded; the fixture no longer reproduces the failure")
	}
	if !strings.Contains(res.Error.Error(), "statement 3 of 3") {
		t.Fatalf("attempt 1 failed somewhere other than the third statement: %v", res.Error)
	}
	// The first two statements really did apply and really were not rolled
	// back — this is the non-atomicity the fix cannot remove, only survive.
	if !migrationColumnExists(t, db, "sweep_recover", "note") {
		t.Fatal("statement 2 did not apply; the fixture no longer reproduces a PARTIAL failure")
	}
	if got := recordedProgress(t, db, slug, 1); got != 2 {
		t.Fatalf("recorded progress = %d statements, want 2 — without it the next attempt "+
			"replays the ALTER that already applied and can never get past it", got)
	}

	// --- Attempt 2: same broken environment, so the SAME honest error. ---
	res = runOnePlugin(t, db, schema)
	if res.Healthy {
		t.Fatal("attempt 2 unexpectedly succeeded")
	}
	if strings.Contains(res.Error.Error(), "Duplicate column name") {
		t.Fatalf("attempt 2 replayed an already-applied statement: %v\n"+
			"This is the crash-loop: the real failure is masked by an artefact of the "+
			"retry, so repairing the real problem can never help", res.Error)
	}
	if !strings.Contains(res.Error.Error(), "statement 3 of 3") {
		t.Fatalf("attempt 2 did not resume at the failing statement: %v", res.Error)
	}

	// --- Attempt 3: the operator repairs the cause; the boot must recover. ---
	mustExecMigration(t, db,
		`CREATE TABLE sweep_parent (id INT NOT NULL PRIMARY KEY) ENGINE=InnoDB`)
	res = runOnePlugin(t, db, schema)
	if !res.Healthy {
		t.Fatalf("attempt 3 still failed after the cause was repaired: %v\n"+
			"A plugin migration that fails part-way must be recoverable without "+
			"hand-editing the database", res.Error)
	}
	if res.Version != 1 {
		t.Errorf("version = %d, want 1", res.Version)
	}
	if got := recordedProgress(t, db, slug, 1); got != -1 {
		t.Errorf("the resume offset outlived the migration (statements_applied = %d); a stale "+
			"row would let a later run of this version skip statements", got)
	}
}

// TestPluginMigration_PreflightRefusesBeforeApplyingAnything is the other
// half: when a migration CANNOT complete, it must not apply its first half.
//
// Statement 1 is a perfectly good CREATE; statement 2 renames a table that
// does not exist — the shape that broke every fresh install. Without the
// pre-flight the CREATE lands and the schema is left in a state no migration
// version describes. With it, the database is untouched.
func TestPluginMigration_PreflightRefusesBeforeApplyingAnything(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin migration pre-flight test requires a database; skipped under -short")
	}
	db := newMigrationScratchSchema(t)

	schema := PluginSchema{Slug: "sweeppreflight", MigrationsFS: fstest.MapFS{
		"001_halfway.up.sql": {Data: []byte(
			"CREATE TABLE IF NOT EXISTS sweep_preflight_a (id INT NOT NULL PRIMARY KEY);\n" +
				"RENAME TABLE sweep_preflight_absent TO sweep_preflight_b;")},
		"001_halfway.down.sql": {Data: []byte("DROP TABLE IF EXISTS sweep_preflight_a;")},
	}}

	res := runOnePlugin(t, db, schema)
	if res.Healthy {
		t.Fatal("a migration whose RENAME has no source table reported success")
	}
	if !strings.Contains(res.Error.Error(), "NOTHING was executed") {
		t.Errorf("the error does not tell the operator the database is untouched: %v", res.Error)
	}
	if migrationTableExists(t, db, "sweep_preflight_a") {
		t.Error("sweep_preflight_a was created by a migration that could never complete; " +
			"the schema is now in a state no migration version describes, and MariaDB " +
			"cannot roll it back")
	}
}

// TestPluginMigration_IgnoresProgressRecordedForDifferentSQL pins the one
// thing that makes resuming safe at all.
//
// A resume offset is a promise that the first N statements of THIS migration
// already ran. If the offset were keyed on (plugin, version) alone, a version
// whose SQL is not the text that produced the offset would silently skip its
// own first N statements — a resume that skips the WRONG statements is far
// worse than the crash-loop it replaces, because it leaves the schema short of
// objects while reporting the migration applied.
//
// Migrations are append-only and immutable (ADR-044/045) so this should never
// happen in a released build; it is checked rather than assumed precisely
// because the failure would be silent. Nothing else in the suite would notice
// the hash comparison being deleted.
func TestPluginMigration_IgnoresProgressRecordedForDifferentSQL(t *testing.T) {
	if testing.Short() {
		t.Skip("plugin migration hash-guard test requires a database; skipped under -short")
	}
	db := newMigrationScratchSchema(t)

	const slug = "sweephash"

	// Text A fails on its second statement, so an offset of 1 is recorded.
	textA := strings.Join([]string{
		"CREATE TABLE IF NOT EXISTS sweep_hash_a (id INT NOT NULL PRIMARY KEY);",
		"ALTER TABLE sweep_hash_a ADD CONSTRAINT fk_sweep_hash " +
			"FOREIGN KEY (id) REFERENCES sweep_hash_absent(id);",
	}, "\n")
	res := runOnePlugin(t, db, PluginSchema{Slug: slug, MigrationsFS: fstest.MapFS{
		"001_a.up.sql": {Data: []byte(textA)},
	}})
	if res.Healthy {
		t.Fatal("the fixture no longer fails; there is no recorded offset to ignore")
	}
	if got := recordedProgress(t, db, slug, 1); got != 1 {
		t.Fatalf("recorded progress = %d, want 1", got)
	}

	// Text B is a DIFFERENT migration under the same version. Both of its
	// statements must run. Honouring the stale offset would skip the first.
	textB := strings.Join([]string{
		"CREATE TABLE IF NOT EXISTS sweep_hash_b (id INT NOT NULL PRIMARY KEY);",
		"CREATE TABLE IF NOT EXISTS sweep_hash_c (id INT NOT NULL PRIMARY KEY);",
	}, "\n")
	res = runOnePlugin(t, db, PluginSchema{Slug: slug, MigrationsFS: fstest.MapFS{
		"001_b.up.sql": {Data: []byte(textB)},
	}})
	if !res.Healthy {
		t.Fatalf("the replacement migration failed: %v", res.Error)
	}
	if !migrationTableExists(t, db, "sweep_hash_b") {
		t.Error("sweep_hash_b is missing: an offset recorded for DIFFERENT SQL was honoured, " +
			"so this migration skipped its own first statement and still reported success")
	}
	if !migrationTableExists(t, db, "sweep_hash_c") {
		t.Error("sweep_hash_c is missing; the second statement did not run")
	}
}

// --- Harness -------------------------------------------------------------

// runOnePlugin runs RunPluginMigrations for a single plugin and returns its
// result, so each attempt goes through the real runner rather than a
// test-only shortcut.
func runOnePlugin(t *testing.T, db *sql.DB, schema PluginSchema) PluginMigrationResult {
	t.Helper()
	results := RunPluginMigrations(db, []PluginSchema{schema})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	return results[0]
}

// recordedProgress returns statements_applied for a migration, or -1 when no
// row exists.
func recordedProgress(t *testing.T, db *sql.DB, slug string, version int) int {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT statements_applied FROM plugin_migration_progress WHERE plugin_slug = ? AND version = ?`,
		slug, version).Scan(&n)
	if err == sql.ErrNoRows {
		return -1
	}
	if err != nil {
		t.Fatalf("reading plugin_migration_progress: %v", err)
	}
	return n
}

func migrationTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&n); err != nil {
		t.Fatalf("querying information_schema.tables: %v", err)
	}
	return n > 0
}

func migrationColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		table, column).Scan(&n); err != nil {
		t.Fatalf("querying information_schema.columns: %v", err)
	}
	return n > 0
}

func mustExecMigration(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("fixture failed: %v\nSQL: %.200s", err, query)
	}
}

// newMigrationScratchSchema creates and returns a handle to an empty,
// throwaway schema, dropped when the test ends.
func newMigrationScratchSchema(t *testing.T) *sql.DB {
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
		cfg.User = migrationTestEnv("DB_USER", "chronicle")
		cfg.Passwd = migrationTestEnv("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = migrationTestEnv("DB_HOST", "127.0.0.1:3306")
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
		t.Skipf("no test DB server reachable at %s (ping: %v) — run `make docker-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_migsafety_%06d", rand.Intn(1000000))
	if _, err := admin.Exec("CREATE DATABASE `" + name +
		"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema %s (needs CREATE privilege): %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
			t.Logf("warning: could not drop scratch schema %s: %v", name, err)
		}
	})

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func migrationTestEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

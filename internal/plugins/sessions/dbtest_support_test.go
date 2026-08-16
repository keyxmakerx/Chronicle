package sessions

// Real-MariaDB test support for the sessions plugin.
//
// Several of this plugin's invariants are about ROWS — what
// availability_exceptions holds after a write, whether a single-use token row
// can be spent twice, whether an UPDATE that matches a row but changes nothing
// is distinguishable from one that matched nothing. A mock repository cannot
// answer any of those: MySQL/MariaDB's RowsAffected counts CHANGED rows, not
// matched rows, and that distinction is itself a defect source here.
//
// Each test gets its OWN scratch schema, fully migrated, and drops it on
// cleanup — deliberately NOT the caller's DSN database. The repo has a standing
// problem where integration tests take CHRONICLE_TEST_DB_DSN verbatim and die
// on "Error 1046 (3D000): No database selected" because `make test-int-local`
// hands them a DSN with no schema name; building our own schema sidesteps it.
//
// Skips (never fails) when no server is reachable, per the house convention.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/sessions/ -run TestDB

import (
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
)

// newScratchDB creates and migrates a throwaway schema, returning a handle to it.
func newScratchDB(t *testing.T) *sql.DB {
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

	name := fmt.Sprintf("chronicle_sess_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
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
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	// sessions/migrations/001 carries `FOREIGN KEY (calendar_id) REFERENCES
	// calendars(id)`, so the CALENDAR plugin's schema has to exist first. It is
	// loaded off disk rather than by importing the package, because
	// internal/wire/plugin_import_guard_test.go forbids a sessions→calendar
	// edge outright.
	calDir := os.DirFS(filepath.Join(root, "internal", "plugins", "calendar", "migrations"))
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: "calendar", MigrationsFS: calDir},
		{Slug: "sessions", MigrationsFS: sub},
	}) {
		if !res.Healthy {
			t.Skipf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}
	return db
}

// newDBID returns a UUID-shaped identifier for seeded rows.
func newDBID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// seedCampaign inserts one user + one campaign, returning (campaignID, userID).
func seedCampaign(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	userID, campID := newDBID(t), newDBID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
		userID, userID+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`,
		campID, "Table", campID, userID); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return campID, userID
}

// seedUser inserts one additional user and returns their ID.
func seedUser(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	id := newDBID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
		id, id+"@example.test", name, "x"); err != nil {
		t.Fatalf("seed user %s: %v", name, err)
	}
	return id
}

// seedSession inserts one planned session for a campaign and returns its ID.
func seedSession(t *testing.T, db *sql.DB, campaignID, createdBy, name string) string {
	t.Helper()
	id := newDBID(t)
	if _, err := db.Exec(
		`INSERT INTO sessions (id, campaign_id, name, status, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,NOW(),NOW())`, id, campaignID, name, StatusPlanned, createdBy); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return id
}

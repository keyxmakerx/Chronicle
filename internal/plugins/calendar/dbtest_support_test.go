// dbtest_support_test.go: DB test helpers shared by every *_integration_test.go
// in this package. openTestDB mirrors internal/plugins/timeline/repository_test.go's
// helper of the same name: this repo's convention is one small copy per
// package rather than a shared test-only import across plugin boundaries.
package calendar

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	mrand "math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// openTestDB connects to the integration test database SERVER (CHRONICLE_TEST_DB_DSN,
// else the DB_* env vars, else the dev default matching the Makefile's
// DATABASE_URL — none of which name a database), creates a scratch schema on
// it, applies core migrations then this plugin's own, and drops the schema
// on cleanup. A server-only DSN (e.g. `make test-int-local`'s
// 'root@tcp(127.0.0.1:13306)/') used to fail every query in this package's
// integration tests with "No database selected", because the old version of
// this helper connected with whatever database name the DSN carried (often
// none) instead of ever selecting one itself.
//
// Skips only when no DB SERVER answers at all — an environment problem, not
// this plugin's. Once the server has answered, a migration failure is
// FATAL: it means this plugin's own migrations are broken, and hiding that
// behind a skip is exactly how "No database selected" went unnoticed here.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		cfg := mysql.NewConfig()
		cfg.User = getenvDefault("DB_USER", "chronicle")
		cfg.Passwd = getenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = getenvDefault("DB_HOST", "127.0.0.1:3306")
		raw = cfg.FormatDSN()
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
		t.Skipf("no test DB server reachable at %s: %v — run `make docker-up` or `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_cal_%06d", mrand.Intn(1000000)) //nolint:gosec // test schema name
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
		t.Fatalf("core migrations did not apply: %v", err)
	}
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: PluginSlug, MigrationsFS: sub},
	}) {
		if !res.Healthy {
			t.Fatalf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}
	return db
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func testUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// testFixture is one (user, campaign) pair with its own teardown, for tests
// that need tenant isolation between two campaigns. CASCADE on campaigns
// clears everything below it; users has no cascade, so both are torn down
// explicitly.
type testFixture struct {
	UserID     string
	CampaignID string
}

// newTestCampaign inserts a user + campaign fixture and registers its
// teardown. label distinguishes fixtures in test failure output and keeps
// email/slug unique across parallel fixtures in the same test.
func newTestCampaign(t *testing.T, db *sql.DB, label string) testFixture {
	t.Helper()
	userID := testUUID(t)
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "calv5-"+label+"-"+userID+"@example.test", "CalV5 "+label, "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "CalV5 "+label, "calv5-"+label+"-"+campaignID[:8], userID)
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	})
	return testFixture{UserID: userID, CampaignID: campaignID}
}

// newTestEntity inserts a minimal entity (+ its entity_type, if not already
// created for this campaign) for entity-tie tests. Returns the entity id.
func newTestEntity(t *testing.T, db *sql.DB, campaignID, userID, name string) string {
	t.Helper()
	typeID := testUUID(t)[:8] // unique slug suffix
	var entityTypeID int64
	res, err := db.Exec(
		`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, ?, ?, ?)`,
		campaignID, "calv5-type-"+typeID, "CalV5 Type", "CalV5 Types")
	if err != nil {
		t.Fatalf("insert entity_type: %v", err)
	}
	entityTypeID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("entity_type last insert id: %v", err)
	}

	entityID := testUUID(t)
	mustExec(t, db,
		`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		entityID, campaignID, entityTypeID, name, "calv5-entity-"+entityID[:8], userID)
	return entityID
}

// dbtest_support_test.go: DB test helpers shared by every *_integration_test.go
// in this package. Mirrors internal/plugins/entities/repository_integration_test.go
// and internal/plugins/timeline/repository_test.go verbatim: this repo's
// convention is one small copy per package rather than a shared test-only
// import across plugin boundaries.
package calendar

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// openTestDB connects to the integration test database, or skips the test
// if none is reachable. DSN comes from CHRONICLE_TEST_DB_DSN, else the DB_*
// env vars, else the dev default matching the Makefile's DATABASE_URL.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if dsn == "" {
		cfg := mysql.NewConfig()
		cfg.User = getenvDefault("DB_USER", "chronicle")
		cfg.Passwd = getenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = getenvDefault("DB_HOST", "127.0.0.1:3306")
		cfg.DBName = getenvDefault("DB_NAME", "chronicle")
		cfg.ParseTime = true
		dsn = cfg.FormatDSN()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("no test DB reachable at %s (ping: %v); run `make test-db-up` or `make docker-up && make migrate-up`", maskDSN(dsn), err)
	}
	return db
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// maskDSN hides the password when reporting a skipped/failed connection.
func maskDSN(dsn string) string {
	if cfg, err := mysql.ParseDSN(dsn); err == nil {
		return cfg.Addr + "/" + cfg.DBName
	}
	return "configured DSN"
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

// dbtest_support_test.go: DB test helpers shared by every *_integration_test.go
// in this package; the scratch schema itself comes from internal/database/dbtest.
package calendar

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
)

// openTestDB returns a scratch schema with core migrations then this plugin's
// own applied, dropped on cleanup. A server-only DSN (e.g. `make
// test-int-local`'s 'root@tcp(127.0.0.1:13306)/') works because the shared
// helper never uses the DSN's own database.
//
// Skips only when no DB SERVER answers at all. Once the server has answered, a
// migration failure is FATAL: it means this plugin's own migrations are
// broken, and a skip would hide it.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_cal_",
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: PluginSlug, FS: sub}},
	})
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

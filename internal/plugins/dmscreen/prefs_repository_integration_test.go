package dmscreen

// Real-MariaDB tests for the screen prefs store. What the SQL does is the
// point: a row is keyed by campaign, person and screen, a save merges over
// what is stored, the migration applies twice and rolls back, and the rows
// go with their person or campaign.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/dmscreen/ -run TestDBPrefs

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func prefsScratch(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_dms_",
		RequireDSN:       true,
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: PluginSlug, FS: sub}},
	})
}

func prefsID() string {
	var b [16]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func prefsSeedUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := prefsID()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, id, id+"@example.test", "U", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func prefsSeedCampaign(t *testing.T, db *sql.DB, owner string) string {
	t.Helper()
	id := prefsID()
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, id, "Table", id, owner); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return id
}

func TestDBPrefsOwnRowMergeAndCascade(t *testing.T) {
	db := prefsScratch(t)
	ctx := context.Background()
	dm := prefsSeedUser(t, db)
	other := prefsSeedUser(t, db)
	camp := prefsSeedCampaign(t, db, dm)
	svc := NewPrefsService(NewPrefsRepository(db), nil)
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: camp}, MemberRole: campaigns.RoleOwner, IsMember: true}

	if v, err := svc.Get(ctx, cc, dm, ScreenDM); err != nil || v.Stored {
		t.Fatalf("nothing stored yet: %+v, %v", v, err)
	}
	if _, err := svc.Update(ctx, cc, dm, ScreenDM, decodeInput(t, `{"layouts":[{"name":"Fight night","open":["fight"]}],"current":"Fight night"}`)); err != nil {
		t.Fatal(err)
	}
	// A second save of another key merges over the first.
	v, err := svc.Update(ctx, cc, dm, ScreenDM, decodeInput(t, `{"style":{"surface":"glass"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !v.Stored || v.Prefs.Current != "Fight night" || len(v.Prefs.Layouts) != 1 || v.Prefs.Style.Surface != "glass" {
		t.Fatalf("merged row: %+v", v.Prefs)
	}
	// Another person, and another screen, see nothing of it.
	if v, err := svc.Get(ctx, cc, other, ScreenDM); err != nil || v.Stored {
		t.Fatalf("another person's read: %+v, %v", v, err)
	}
	if doc, found, err := NewPrefsRepository(db).Get(ctx, camp, dm, ScreenPlayer); err != nil || found || doc != nil {
		t.Fatalf("another screen's row: %s, %v, %v", doc, found, err)
	}

	// The up migration is idempotent: replaying it leaves the rows alone.
	up, err := fs.ReadFile(MigrationsFS, "migrations/002_dm_screen_prefs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatalf("replaying the migration: %v", err)
	}
	if v, err := svc.Get(ctx, cc, dm, ScreenDM); err != nil || !v.Stored {
		t.Fatalf("replay changed the rows: %+v, %v", v, err)
	}

	count := func(where string, arg string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM dm_screen_prefs WHERE `+where+` = ?`, arg).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := svc.Update(ctx, cc, other, ScreenDM, decodeInput(t, `{"style":{"density":"compact"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = ?`, other); err != nil {
		t.Fatal(err)
	}
	if n := count("user_id", other); n != 0 {
		t.Fatalf("rows left after their person was deleted: %d", n)
	}
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, camp); err != nil {
		t.Fatal(err)
	}
	if n := count("campaign_id", camp); n != 0 {
		t.Fatalf("rows left after their campaign was deleted: %d", n)
	}

	// The down migration drops the table, and the up builds it again.
	down, err := fs.ReadFile(MigrationsFS, "migrations/002_dm_screen_prefs.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{string(down), string(down), string(up)} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("down then up: %v", err)
		}
	}
	if n := count("campaign_id", camp); n != 0 {
		t.Fatalf("a rebuilt table holds rows: %d", n)
	}
}

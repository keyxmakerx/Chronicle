package rumours

// Real-MariaDB tests for the repository. What the SQL does is the point: a
// read made for someone who may not have the truth never selects it, a write
// can reach only its own page and campaign, the cap holds, and the table's own
// migration applies twice and cleans up after a page.
//
//	make test-db-up
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/rumours/ -run TestDB

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
)

func scratch(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	return dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_rum_",
		RequireDSN:       true,
		StrictMigrations: true,
		Plugins:          []dbtest.PluginMigrations{{Slug: PluginSlug, FS: sub}},
	})
}

func seedUser(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	id := newIDStr()
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, id, id+"@example.test", name, "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedPage makes a campaign with one page and returns (campaignID, pageID).
func seedPage(t *testing.T, db *sql.DB, owner string) (string, string) {
	t.Helper()
	camp, page := newIDStr(), newIDStr()
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, camp, "Table", camp, owner); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	res, err := db.Exec(`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'npc', 'NPC', 'NPCs')`, camp)
	if err != nil {
		t.Fatalf("seed entity type: %v", err)
	}
	typeID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
		VALUES (?, ?, ?, 'Dessa', 'dessa', false, 'default', ?, NOW(), NOW())`, page, camp, typeID, owner); err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	return camp, page
}

func rumour(camp, page, by string, byDM bool, quote string) Rumour {
	return Rumour{ID: newIDStr(), CampaignID: camp, EntityID: page, Quote: quote, Source: "Bram", Sure: 2, ByDM: byDM, CreatedBy: by}
}

func TestDB_TruthIsOnlyReadWhenAsked(t *testing.T) {
	db := scratch(t)
	ctx := context.Background()
	dm := seedUser(t, db, "Keyx")
	mira := seedUser(t, db, "Mira")
	camp, page := seedPage(t, db, dm)
	repo := NewRepository(db)

	a := rumour(camp, page, mira, false, "She took the treasure.")
	if err := repo.Insert(ctx, a, MaxRumours); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.SetTruth(ctx, camp, page, a.ID, TruthFalse); err != nil || !ok {
		t.Fatalf("SetTruth = %v, %v", ok, err)
	}
	if ok, err := repo.SetBelief(ctx, camp, page, a.ID, BeliefTrue, mira, false); err != nil || !ok {
		t.Fatalf("SetBelief = %v, %v", ok, err)
	}

	cases := []struct {
		name      string
		withTruth bool
		want      string
	}{
		{"the Director's read has the verdict", true, TruthFalse},
		{"anyone else's read has an empty string", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := repo.List(ctx, camp, page, tc.withTruth, 10)
			if err != nil || total != 1 || len(rows) != 1 {
				t.Fatalf("List = %v rows, total %d, %v", len(rows), total, err)
			}
			if rows[0].Truth != tc.want {
				t.Errorf("List truth = %q, want %q", rows[0].Truth, tc.want)
			}
			got, err := repo.Get(ctx, camp, page, a.ID, tc.withTruth)
			if err != nil || got == nil {
				t.Fatalf("Get = %v, %v", got, err)
			}
			if got.Truth != tc.want {
				t.Errorf("Get truth = %q, want %q", got.Truth, tc.want)
			}
			if got.Belief != BeliefTrue || got.BeliefName != "Mira" || got.AuthorName != "Mira" {
				t.Errorf("belief/name columns = %q/%q/%q", got.Belief, got.BeliefName, got.AuthorName)
			}
		})
	}
}

func TestDB_ScopeAndWrites(t *testing.T) {
	db := scratch(t)
	ctx := context.Background()
	dm := seedUser(t, db, "Keyx")
	camp, page := seedPage(t, db, dm)
	otherCamp, otherPage := seedPage(t, db, dm)
	repo := NewRepository(db)

	a := rumour(camp, page, dm, true, "In the first campaign.")
	if err := repo.Insert(ctx, a, MaxRumours); err != nil {
		t.Fatal(err)
	}

	// A rumour is reachable only through its own page and campaign.
	for _, tc := range []struct{ name, c, p string }{
		{"another campaign", otherCamp, page},
		{"another page", camp, otherPage},
		{"both wrong", otherCamp, otherPage},
	} {
		t.Run("cannot reach it through "+tc.name, func(t *testing.T) {
			if got, err := repo.Get(ctx, tc.c, tc.p, a.ID, true); err != nil || got != nil {
				t.Errorf("Get = %v, %v", got, err)
			}
			if ok, _ := repo.UpdateText(ctx, tc.c, tc.p, a.ID, "x", "y", 1); ok {
				t.Error("UpdateText reached it")
			}
			if ok, _ := repo.SetTruth(ctx, tc.c, tc.p, a.ID, TruthTrue); ok {
				t.Error("SetTruth reached it")
			}
			if ok, _ := repo.SetBelief(ctx, tc.c, tc.p, a.ID, BeliefTrue, dm, true); ok {
				t.Error("SetBelief reached it")
			}
			if ok, _ := repo.Delete(ctx, tc.c, tc.p, a.ID); ok {
				t.Error("Delete reached it")
			}
		})
	}
	got, _ := repo.Get(ctx, camp, page, a.ID, true)
	if got == nil || got.Quote != "In the first campaign." || got.Truth != TruthOpen || got.Belief != BeliefNone {
		t.Fatalf("a refused write changed the row: %+v", got)
	}

	// A page that is not in the campaign takes no rumour at all.
	if err := repo.Insert(ctx, rumour(otherCamp, page, dm, true, "Wrong campaign."), MaxRumours); err != errNoPage {
		t.Errorf("Insert into a page of another campaign = %v, want errNoPage", err)
	}

	// Writing the value already stored is still "found".
	if ok, err := repo.SetTruth(ctx, camp, page, a.ID, TruthOpen); err != nil || !ok {
		t.Errorf("rewriting the same truth = %v, %v", ok, err)
	}
	if ok, err := repo.UpdateText(ctx, camp, page, a.ID, "In the first campaign.", "Bram", 2); err != nil || !ok {
		t.Errorf("rewriting the same words = %v, %v", ok, err)
	}
	if ok, err := repo.SetBelief(ctx, camp, page, a.ID, BeliefNone, "", true); err != nil || !ok {
		t.Errorf("clearing an unset belief = %v, %v", ok, err)
	}
	if ok, err := repo.Delete(ctx, camp, page, a.ID); err != nil || !ok {
		t.Errorf("Delete = %v, %v", ok, err)
	}
	if ok, _ := repo.Delete(ctx, camp, page, a.ID); ok {
		t.Error("deleting twice should report not found")
	}
}

func TestDB_CapAndOrder(t *testing.T) {
	db := scratch(t)
	ctx := context.Background()
	dm := seedUser(t, db, "Keyx")
	camp, page := seedPage(t, db, dm)
	repo := NewRepository(db)

	const cap = 3
	for i := 0; i < cap; i++ {
		x := rumour(camp, page, dm, true, fmt.Sprintf("Rumour %d.", i))
		if err := repo.Insert(ctx, x, cap); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if err := repo.Insert(ctx, rumour(camp, page, dm, true, "One too many."), cap); err != errPageFull {
		t.Fatalf("insert past the cap = %v, want errPageFull", err)
	}

	// A limit below the count returns that many and still reports them all.
	rows, total, err := repo.List(ctx, camp, page, true, 2)
	if err != nil || len(rows) != 2 || total != cap {
		t.Fatalf("List(limit 2) = %d rows, total %d, %v", len(rows), total, err)
	}
	if rows[0].CreatedAt.Before(rows[1].CreatedAt) {
		t.Error("newest should come first")
	}
}

func TestDB_PageDeleteCascadesAndMigrationIsRepeatable(t *testing.T) {
	db := scratch(t)
	ctx := context.Background()
	dm := seedUser(t, db, "Keyx")
	camp, page := seedPage(t, db, dm)
	repo := NewRepository(db)
	if err := repo.Insert(ctx, rumour(camp, page, dm, true, "Soon gone."), MaxRumours); err != nil {
		t.Fatal(err)
	}

	// The up migration is idempotent: replaying it leaves the rows alone.
	up, err := fs.ReadFile(MigrationsFS, "migrations/001_page_rumours.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatalf("replaying the migration: %v", err)
	}
	if _, total, _ := repo.List(ctx, camp, page, true, 10); total != 1 {
		t.Fatalf("replay changed the rows: %d", total)
	}

	if _, err := db.Exec(`DELETE FROM entities WHERE id = ?`, page); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM page_rumours WHERE entity_id = ?`, page).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rumours left after their page was deleted: %d, %v", n, err)
	}
}

// newIDStr is newID for a struct literal, where there is no *testing.T to hand.
func newIDStr() string {
	var b [16]byte
	_, _ = crand.Read(b[:])
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

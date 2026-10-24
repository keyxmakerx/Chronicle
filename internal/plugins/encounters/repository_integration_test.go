package encounters

// The repository is hand-written SQL, so the fake the service tests use proves
// nothing about it; this runs it against a migrated scratch schema. Skips when
// CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/encounters/ -run Integration

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

type encFixture struct {
	db                       *sql.DB
	user, campaign, other    string
	page, page2, foreignPage string
}

func newEncFixture(t *testing.T) *encFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the encounters repository tests")
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
	name := fmt.Sprintf("chronicle_encounters_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: PluginSlug, MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("encounters migrations did not apply: %v", res.Error)
		}
	}

	fx := &encFixture{db: db,
		user:        "00000000-0000-0000-0000-000000000001",
		campaign:    "00000000-0000-0000-0000-000000000002",
		other:       "00000000-0000-0000-0000-000000000003",
		page:        "00000000-0000-0000-0000-000000000004",
		page2:       "00000000-0000-0000-0000-000000000005",
		foreignPage: "00000000-0000-0000-0000-000000000006",
	}
	fx.exec(t, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, fx.user, "p@example.test", "P", "x")
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, fx.campaign, "One", "one", fx.user)
	fx.exec(t, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, fx.other, "Two", "two", fx.user)
	for _, c := range []string{fx.campaign, fx.other} {
		fx.exec(t, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?,?,?,?)`, c, "place", "Place", "Places")
	}
	for _, p := range []struct{ id, campaign string }{{fx.page, fx.campaign}, {fx.page2, fx.campaign}, {fx.foreignPage, fx.other}} {
		fx.exec(t, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, visibility, created_by, created_at, updated_at)
			VALUES (?, ?, (SELECT id FROM entity_types WHERE campaign_id = ? AND slug = 'place'), ?, ?, false, 'default', ?, NOW(), NOW())`,
			p.id, p.campaign, p.campaign, "Page "+p.id[len(p.id)-1:], "p"+p.id[len(p.id)-1:], fx.user)
	}
	return fx
}

func (fx *encFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestRepositoryIntegration_CRUDAndScope(t *testing.T) {
	fx := newEncFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)

	r := Row{ID: "10000000-0000-0000-0000-000000000001", CampaignID: fx.campaign, EntityID: fx.page, Name: "Ramparts", Data: []byte(`{"heroes":[],"creatures":[]}`)}
	if err := repo.Create(ctx, r, fx.user); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, found, err := repo.Get(ctx, fx.campaign, r.ID)
	if err != nil || !found || got.Version != 1 || got.Name != "Ramparts" || got.Ready {
		t.Fatalf("get: %+v found=%v err=%v", got, found, err)
	}
	if _, found, _ := repo.Get(ctx, fx.other, r.ID); found {
		t.Fatal("another campaign must not read the row")
	}
	if ok, err := repo.Update(ctx, fx.campaign, r.ID, "Ramparts fight", true, []byte(`{}`), 2, fx.user); err != nil || ok {
		t.Fatalf("stale version must lose: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Update(ctx, fx.other, r.ID, "x", true, []byte(`{}`), 1, fx.user); err != nil || ok {
		t.Fatalf("another campaign must not update: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Update(ctx, fx.campaign, r.ID, "Ramparts fight", true, []byte(`{"heroes":[]}`), 1, fx.user); err != nil || !ok {
		t.Fatalf("update: ok=%v err=%v", ok, err)
	}
	got, _, _ = repo.Get(ctx, fx.campaign, r.ID)
	if got.Version != 2 || !got.Ready || got.Name != "Ramparts fight" {
		t.Fatalf("after update: %+v", got)
	}
	list, err := repo.ListForPage(ctx, fx.campaign, fx.page)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if ok, _ := repo.Delete(ctx, fx.other, r.ID); ok {
		t.Fatal("another campaign must not delete")
	}
	if ok, err := repo.Delete(ctx, fx.campaign, r.ID); err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
}

func TestRepositoryIntegration_Search(t *testing.T) {
	fx := newEncFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)
	mk := func(id, page, name string, ready bool) {
		t.Helper()
		if err := repo.Create(ctx, Row{ID: id, CampaignID: fx.campaign, EntityID: page, Name: name, Ready: ready, Data: []byte(`{}`)}, ""); err != nil {
			t.Fatal(err)
		}
	}
	mk("20000000-0000-0000-0000-000000000001", fx.page, "Bandits at the ford", false)
	mk("20000000-0000-0000-0000-000000000002", fx.page2, "100% ambush", true)
	if err := repo.Create(ctx, Row{ID: "20000000-0000-0000-0000-000000000003", CampaignID: fx.other, EntityID: fx.foreignPage, Name: "Elsewhere", Data: []byte(`{}`)}, ""); err != nil {
		t.Fatal(err)
	}

	rows, total, err := repo.Search(ctx, fx.campaign, "", 50)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("search: %d %v", total, err)
	}
	if !rows[0].Ready || rows[0].PageName != "Page 5" {
		t.Fatalf("ready first, with its page name: %+v", rows[0])
	}
	// LIKE wildcards in the query match literally.
	if rows, total, _ = repo.Search(ctx, fx.campaign, "100%", 50); total != 1 || rows[0].Name != "100% ambush" {
		t.Fatalf("literal %%: %d", total)
	}
	if _, total, _ = repo.Search(ctx, fx.campaign, "%", 50); total != 1 {
		t.Fatalf("a bare %% matches only names holding one, got %d", total)
	}
	// The page name matches too.
	if _, total, _ = repo.Search(ctx, fx.campaign, "Page 4", 50); total != 1 {
		t.Fatalf("page name: %d", total)
	}
	if rows, total, _ = repo.Search(ctx, fx.campaign, "", 1); total != 2 || len(rows) != 1 {
		t.Fatalf("limit: %d rows, total %d", len(rows), total)
	}
	// A trashed page's encounters are left out.
	fx.exec(t, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, fx.page2)
	if _, total, _ = repo.Search(ctx, fx.campaign, "", 50); total != 1 {
		t.Fatalf("trashed page: %d", total)
	}
	// Deleting the page removes its encounters.
	fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.page)
	if list, _ := repo.ListForPage(ctx, fx.campaign, fx.page); len(list) != 0 {
		t.Fatal("encounters must go with their page")
	}
}

func TestRunRepositoryIntegration(t *testing.T) {
	fx := newEncFixture(t)
	repo := NewRunRepository(fx.db)
	ctx := context.Background()
	if _, found, err := repo.GetRun(ctx, fx.campaign); err != nil || found {
		t.Fatalf("no fight yet: found=%v err=%v", found, err)
	}
	if ok, err := repo.CreateRun(ctx, fx.campaign, []byte(`{"round":1}`), fx.user); err != nil || !ok {
		t.Fatalf("create: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.CreateRun(ctx, fx.campaign, []byte(`{"round":9}`), fx.user); err != nil || ok {
		t.Fatalf("a second start must not replace the fight: ok=%v err=%v", ok, err)
	}
	if _, found, _ := repo.GetRun(ctx, fx.other); found {
		t.Fatal("another campaign must not read the fight")
	}
	if ok, err := repo.UpdateRun(ctx, fx.campaign, []byte(`{"round":2}`), 1, fx.user); err != nil || !ok {
		t.Fatalf("update: ok=%v err=%v", ok, err)
	}
	if ok, _ := repo.UpdateRun(ctx, fx.campaign, []byte(`{"round":3}`), 1, fx.user); ok {
		t.Fatal("a stale version must not update")
	}
	if ok, _ := repo.UpdateRun(ctx, fx.other, []byte(`{"round":3}`), 2, fx.user); ok {
		t.Fatal("another campaign must not update the fight")
	}
	r, found, err := repo.GetRun(ctx, fx.campaign)
	if err != nil || !found || r.Version != 2 || string(r.Data) != `{"round":2}` {
		t.Fatalf("after update: %+v found=%v err=%v", r, found, err)
	}
	if err := repo.DeleteRun(ctx, fx.other); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := repo.GetRun(ctx, fx.campaign); !found {
		t.Fatal("ending another campaign's fight ended this one")
	}
	if err := repo.DeleteRun(ctx, fx.campaign); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := repo.GetRun(ctx, fx.campaign); found {
		t.Fatal("the fight is still on after the end")
	}
	if err := repo.DeleteRun(ctx, fx.campaign); err != nil {
		t.Fatalf("ending no fight: %v", err)
	}
}

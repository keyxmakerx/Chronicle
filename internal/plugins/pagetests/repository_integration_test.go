package pagetests

// The repository is hand-written SQL, so the fake the service tests use proves
// nothing about it; this runs it against a migrated scratch schema. Skips when
// CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/pagetests/ -run Integration

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

type ptFixture struct {
	db                       *sql.DB
	user, campaign, other    string
	page, page2, foreignPage string
}

func newPTFixture(t *testing.T) *ptFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the tests repository tests")
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
	name := fmt.Sprintf("chronicle_pagetests_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("page tests migrations did not apply: %v", res.Error)
		}
	}

	fx := &ptFixture{db: db,
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

func (fx *ptFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestRepositoryIntegration_CRUDAndScope(t *testing.T) {
	fx := newPTFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)

	r := Row{ID: "10000000-0000-0000-0000-000000000001", CampaignID: fx.campaign, EntityID: fx.page, Name: "Hunt", Data: []byte(`{"results":[]}`)}
	if err := repo.Create(ctx, r, fx.user); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, found, err := repo.Get(ctx, fx.campaign, r.ID)
	if err != nil || !found || got.Version != 1 || got.Name != "Hunt" || got.Shown {
		t.Fatalf("get: %+v found=%v err=%v", got, found, err)
	}
	if _, found, _ := repo.Get(ctx, fx.other, r.ID); found {
		t.Fatal("another campaign must not read the row")
	}
	if ok, err := repo.Update(ctx, fx.campaign, r.ID, "Hunt", true, []byte(`{}`), 2, fx.user); err != nil || ok {
		t.Fatalf("stale version must lose: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Update(ctx, fx.other, r.ID, "x", true, []byte(`{}`), 1, fx.user); err != nil || ok {
		t.Fatalf("another campaign must not update: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Update(ctx, fx.campaign, r.ID, "The hunt", true, []byte(`{"results":[]}`), 1, fx.user); err != nil || !ok {
		t.Fatalf("update: ok=%v err=%v", ok, err)
	}
	got, _, _ = repo.Get(ctx, fx.campaign, r.ID)
	if got.Version != 2 || !got.Shown || got.Name != "The hunt" {
		t.Fatalf("after update: %+v", got)
	}
	if list, err := repo.ListForPage(ctx, fx.campaign, fx.page); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if list, _ := repo.ListForPage(ctx, fx.other, fx.page); len(list) != 0 {
		t.Fatal("another campaign must not list the page's tests")
	}
	if many, err := repo.GetMany(ctx, fx.campaign, []string{r.ID, "missing"}); err != nil || len(many) != 1 {
		t.Fatalf("get many: %v %v", many, err)
	}
	if many, _ := repo.GetMany(ctx, fx.other, []string{r.ID}); len(many) != 0 {
		t.Fatal("another campaign must not get the row by id")
	}
	if ok, _ := repo.Delete(ctx, fx.other, r.ID); ok {
		t.Fatal("another campaign must not delete")
	}
	if ok, err := repo.Delete(ctx, fx.campaign, r.ID); err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
}

func TestRepositoryIntegration_CampaignList(t *testing.T) {
	fx := newPTFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)
	mk := func(id, campaign, page, name string) {
		t.Helper()
		if err := repo.Create(ctx, Row{ID: id, CampaignID: campaign, EntityID: page, Name: name, Data: []byte(`{}`)}, ""); err != nil {
			t.Fatal(err)
		}
	}
	mk("20000000-0000-0000-0000-000000000001", fx.campaign, fx.page, "Hunt")
	mk("20000000-0000-0000-0000-000000000002", fx.campaign, fx.page2, "Restore the Lastwell")
	mk("20000000-0000-0000-0000-000000000003", fx.other, fx.foreignPage, "Elsewhere")
	fx.exec(t, `UPDATE page_tests SET updated_at = NOW() + INTERVAL 1 MINUTE WHERE id = ?`, "20000000-0000-0000-0000-000000000002")

	rows, err := repo.ListForCampaign(ctx, fx.campaign, 50)
	if err != nil || len(rows) != 2 {
		t.Fatalf("campaign list: %v %v", rows, err)
	}
	if rows[0].Name != "Restore the Lastwell" || rows[0].PageName != "Page 5" {
		t.Fatalf("newest change first, with its page name: %+v", rows[0])
	}
	if rows, _ = repo.ListForCampaign(ctx, fx.campaign, 1); len(rows) != 1 {
		t.Fatalf("limit: %d", len(rows))
	}
	// A trashed page's tests are left out.
	fx.exec(t, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, fx.page2)
	if rows, _ = repo.ListForCampaign(ctx, fx.campaign, 50); len(rows) != 1 {
		t.Fatalf("trashed page: %d", len(rows))
	}
	// Deleting the page removes its tests.
	fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.page)
	if list, _ := repo.ListForPage(ctx, fx.campaign, fx.page); len(list) != 0 {
		t.Fatal("tests must go with their page")
	}
}

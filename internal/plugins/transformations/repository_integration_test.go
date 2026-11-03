package transformations

// The repository is hand-written SQL, so the fake the service tests use proves
// nothing about it; this runs it against a migrated scratch schema. Skips when
// CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/transformations/ -run Integration

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

type layerFixture struct {
	db                       *sql.DB
	user, campaign, other    string
	page, page2, foreignPage string
}

func newLayerFixture(t *testing.T) *layerFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the layers repository tests")
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
	name := fmt.Sprintf("chronicle_layers_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("layers migrations did not apply: %v", res.Error)
		}
	}

	fx := &layerFixture{db: db,
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

func (fx *layerFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestRepositoryIntegration_LayersScopeAndLimit(t *testing.T) {
	fx := newLayerFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)

	row := func(id, campaign, page string) Row {
		return Row{ID: id, CampaignID: campaign, EntityID: page, Name: "Revenant", Source: "drawsteel:revenant", Data: []byte(`{"note":"","changes":[]}`)}
	}
	a := row("10000000-0000-0000-0000-000000000001", fx.campaign, fx.page)
	b := row("10000000-0000-0000-0000-000000000002", fx.campaign, fx.page)
	if ok, err := repo.Create(ctx, a, fx.user, 2); err != nil || !ok {
		t.Fatalf("create a: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Create(ctx, b, fx.user, 2); err != nil || !ok {
		t.Fatalf("create b: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Create(ctx, row("10000000-0000-0000-0000-000000000003", fx.campaign, fx.page), fx.user, 2); err != nil || ok {
		t.Fatalf("a full page must refuse a third: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Create(ctx, row("10000000-0000-0000-0000-000000000004", fx.campaign, fx.page2), fx.user, 2); err != nil || !ok {
		t.Fatalf("the limit is per page: ok=%v err=%v", ok, err)
	}
	list, err := repo.ListForPage(ctx, fx.campaign, fx.page)
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[0].Source != "drawsteel:revenant" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if list, _ := repo.ListForPage(ctx, fx.other, fx.page); len(list) != 0 {
		t.Fatal("another campaign must not list the page's layers")
	}
	if _, found, _ := repo.Get(ctx, fx.other, a.ID); found {
		t.Fatal("another campaign must not read the row")
	}
	if ok, _ := repo.Delete(ctx, fx.other, a.ID); ok {
		t.Fatal("another campaign must not delete the row")
	}
	if ok, err := repo.Delete(ctx, fx.campaign, a.ID); err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.page)
	if list, _ := repo.ListForPage(ctx, fx.campaign, fx.page); len(list) != 0 {
		t.Fatal("deleting the page must delete its layers")
	}
}

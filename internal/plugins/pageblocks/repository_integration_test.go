package pageblocks

// The repository is hand-written SQL, so the fake the service tests use proves
// nothing about it; this runs it against a migrated scratch schema. Skips when
// CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/pageblocks/ -run Integration

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

type blocksFixture struct {
	db                       *sql.DB
	user, campaign, other    string
	page, page2, foreignPage string
}

func newBlocksFixture(t *testing.T) *blocksFixture {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the pageblocks repository tests")
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
	name := fmt.Sprintf("chronicle_pageblocks_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
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
			t.Fatalf("pageblocks migrations did not apply: %v", res.Error)
		}
	}

	fx := &blocksFixture{db: db,
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

func (fx *blocksFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := fx.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestRepositoryIntegration_SaveGetVersioning(t *testing.T) {
	fx := newBlocksFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)

	if _, found, err := repo.Get(ctx, fx.campaign, fx.page, "b1"); err != nil || found {
		t.Fatalf("empty block: found=%v err=%v", found, err)
	}
	// 0 means "no row yet"; a second first-save loses the race cleanly.
	if ok, err := repo.Save(ctx, fx.campaign, fx.page, "b1", KindSteps, []byte(`{"steps":[]}`), 0, fx.user); err != nil || !ok {
		t.Fatalf("first save: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.Save(ctx, fx.campaign, fx.page, "b1", KindSteps, []byte(`{"steps":[]}`), 0, fx.user); err != nil || ok {
		t.Fatalf("duplicate first save must lose the race: ok=%v err=%v", ok, err)
	}

	steps := []struct {
		name     string
		expected int
		data     string
		userID   string
		wantOK   bool
		wantVer  int
	}{
		{"current version wins and bumps", 1, `{"a":2}`, fx.user, true, 2},
		{"stale version fails and changes nothing", 1, `{"a":3}`, fx.user, false, 2},
		{"empty user id stores NULL author", 2, `{"a":4}`, "", true, 3},
		{"future version fails", 9, `{"a":5}`, fx.user, false, 3},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := repo.Save(ctx, fx.campaign, fx.page, "b1", KindTable, []byte(tc.data), tc.expected, tc.userID)
			if err != nil || ok != tc.wantOK {
				t.Fatalf("ok=%v err=%v, want ok=%v", ok, err, tc.wantOK)
			}
			got, found, err := repo.Get(ctx, fx.campaign, fx.page, "b1")
			if err != nil || !found || got.Version != tc.wantVer {
				t.Fatalf("stored version = %d (found %v, err %v), want %d", got.Version, found, err, tc.wantVer)
			}
			// The kind is fixed by the first save, even if a later one names another.
			if got.Kind != KindSteps {
				t.Fatalf("kind = %q, want it kept as %q", got.Kind, KindSteps)
			}
		})
	}
}

func TestRepositoryIntegration_ScopingAndCascade(t *testing.T) {
	fx := newBlocksFixture(t)
	ctx := context.Background()
	repo := NewRepository(fx.db)

	if ok, err := repo.Save(ctx, fx.campaign, fx.page, "b1", KindReadAloud, []byte(`{"text":"a"}`), 0, fx.user); err != nil || !ok {
		t.Fatalf("save: ok=%v err=%v", ok, err)
	}
	// The same block id on another page is a different block.
	if _, found, _ := repo.Get(ctx, fx.campaign, fx.page2, "b1"); found {
		t.Fatal("block leaked to another page")
	}
	// A page id read through another campaign finds nothing.
	if _, found, _ := repo.Get(ctx, fx.other, fx.page, "b1"); found {
		t.Fatal("block readable through another campaign")
	}
	// Nor can another campaign overwrite it.
	if ok, _ := repo.Save(ctx, fx.other, fx.page, "b1", KindReadAloud, []byte(`{"text":"x"}`), 1, fx.user); ok {
		t.Fatal("block overwritten through another campaign")
	}

	// Deleting the page removes its blocks.
	fx.exec(t, `DELETE FROM entities WHERE id = ?`, fx.page)
	if _, found, _ := repo.Get(ctx, fx.campaign, fx.page, "b1"); found {
		t.Fatal("block survived its page")
	}
}

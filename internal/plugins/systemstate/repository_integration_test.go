package systemstate

// The repository is hand-written SQL, so the fake the service tests use proves
// nothing about it; this runs the campaign-wide document against a migrated
// scratch schema. Skips when CHRONICLE_TEST_DB_DSN is unset or no server answers.
//
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/systemstate/ -run Integration

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

func newStateDB(t *testing.T) (db *sql.DB, user, campaign, other string) {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN to run the system state repository tests")
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
	name := fmt.Sprintf("chronicle_systemstate_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	db, err = sql.Open("mysql", scratch.FormatDSN())
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
			t.Fatalf("system state migrations did not apply: %v", res.Error)
		}
	}

	user = "00000000-0000-0000-0000-000000000001"
	campaign = "00000000-0000-0000-0000-000000000002"
	other = "00000000-0000-0000-0000-000000000003"
	for _, q := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, []any{user, "p@example.test", "P", "x"}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{campaign, "One", "one", user}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, []any{other, "Two", "two", user}},
	} {
		if _, err := db.Exec(q.q, q.args...); err != nil {
			t.Fatalf("exec %q: %v", q.q, err)
		}
	}
	return db, user, campaign, other
}

func TestCampaignStateRepositoryIntegration(t *testing.T) {
	db, user, campaign, other := newStateDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	if st, err := repo.GetCampaign(ctx, campaign, "dnd5e", "party"); err != nil || st != nil {
		t.Fatalf("nothing stored yet: %+v err=%v", st, err)
	}
	if err := repo.UpsertCampaign(ctx, Write{CampaignID: campaign, SystemID: "dnd5e", Key: "party", GM: []byte(`{"handles":1200}`), UpdatedBy: user}); err != nil {
		t.Fatal(err)
	}
	// A public-only write keeps the gm half.
	if err := repo.UpsertCampaign(ctx, Write{CampaignID: campaign, SystemID: "dnd5e", Key: "party", Public: []byte(`{"shown":1}`)}); err != nil {
		t.Fatal(err)
	}
	st, err := repo.GetCampaign(ctx, campaign, "dnd5e", "party")
	if err != nil || st == nil || st.UpdatedAt == nil {
		t.Fatalf("after writes: %+v err=%v", st, err)
	}
	if string(st.GM) != `{"handles":1200}` || string(st.Public) != `{"shown":1}` {
		t.Fatalf("halves = gm %s public %s", st.GM, st.Public)
	}
	if st, _ := repo.GetCampaign(ctx, other, "dnd5e", "party"); st != nil {
		t.Fatalf("another campaign read the document: %+v", st)
	}
	if st, _ := repo.GetCampaign(ctx, campaign, "drawsteel", "party"); st != nil {
		t.Fatalf("another system read the document: %+v", st)
	}
	// Deleting the campaign removes its documents.
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, campaign); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM campaign_system_state`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows after campaign delete = %d err=%v", n, err)
	}
}

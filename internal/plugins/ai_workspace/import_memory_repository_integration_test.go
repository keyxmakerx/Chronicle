package ai_workspace

// Row-level tests for the import memory tables: the tray upserts, marking a
// part applied, emptying the tray, the fingerprint lookup and the campaign scoping can
// only be shown against the database. Each test gets its own migrated scratch schema and skips (never
// fails) when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/ai_workspace/ -run TestImportMemoryRepoIntegration

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

func newPartsScratchDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (tools/start-test-db.sh) to run the row-level tests")
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
	name := fmt.Sprintf("chronicle_parts_%06d", rand.Intn(1000000)) //nolint:gosec // scratch schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	scratch.MultiStatements = true
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("open scratch: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	if err := database.RunMigrations(db, scratch.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	// The plugin's own migrations, in order, run as written.
	ups, err := fs.Glob(MigrationsFS, "migrations/*.up.sql")
	if err != nil || len(ups) == 0 {
		t.Fatalf("plugin migrations: %v", err)
	}
	for _, f := range ups {
		up, err := fs.ReadFile(MigrationsFS, f)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ { // idempotent DDL: a re-run must not fail
			if _, err := db.Exec(string(up)); err != nil {
				t.Fatalf("%s run %d: %v", f, i+1, err)
			}
		}
	}
	return db
}

func partsUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func seedPartsCampaign(t *testing.T, db *sql.DB) (campaignID, userID string) {
	t.Helper()
	uid, cid := partsUUID(t), partsUUID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, uid+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Table", cid, uid); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return cid, uid
}

func TestImportMemoryRepoIntegration_TrayAppliedAndClear(t *testing.T) {
	db := newPartsScratchDB(t)
	repo := NewImportMemoryRepository(db)
	ctx := context.Background()
	camp, user := seedPartsCampaign(t, db)
	other, _ := seedPartsCampaign(t, db)

	three := StoredPart{AnswerKey: "lastwell-fixes", AnswerName: "Lastwell fixes", Number: 3, Of: 3,
		Needs: []int{1, 2}, Markdown: "---\nkind: part\n---", AddedBy: user}
	if err := repo.PutInTray(ctx, camp, three); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutInTray(ctx, other, StoredPart{AnswerKey: "lastwell-fixes", AnswerName: "x", Number: 2, Markdown: "y", AddedBy: user}); err != nil {
		t.Fatal(err)
	}
	// Pasting the same part again replaces it rather than adding a row.
	three.Markdown = "---\nkind: part\nname: again\n---"
	if err := repo.PutInTray(ctx, camp, three); err != nil {
		t.Fatal(err)
	}

	tray, err := repo.Tray(ctx, camp)
	if err != nil || len(tray) != 1 || tray[0].Markdown != three.Markdown ||
		!reflect.DeepEqual(tray[0].Needs, []int{1, 2}) || tray[0].Of != 3 || tray[0].Applied {
		t.Fatalf("tray = %+v, %v; another campaign's part must not count", tray, err)
	}

	// Applying a part takes it out of the tray but remembers it.
	one := StoredPart{AnswerKey: "lastwell-fixes", AnswerName: "Lastwell fixes", Number: 1, Of: 3, AddedBy: user}
	if err := repo.MarkApplied(ctx, camp, one); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkApplied(ctx, camp, three); err != nil {
		t.Fatal(err)
	}
	if tray, err := repo.Tray(ctx, camp); err != nil || len(tray) != 0 {
		t.Fatalf("tray after applying = %+v, %v", tray, err)
	}
	parts, err := repo.Parts(ctx, camp, "lastwell-fixes")
	if err != nil || len(parts) != 2 || !parts[0].Applied || !parts[1].Applied || parts[1].Markdown != "" {
		t.Fatalf("parts = %+v, %v", parts, err)
	}

	// Emptying the tray drops a never-applied part and keeps applied ones.
	two := StoredPart{AnswerKey: "lastwell-fixes", AnswerName: "Lastwell fixes", Number: 2, Of: 3, Markdown: "z", AddedBy: user}
	if err := repo.PutInTray(ctx, camp, two); err != nil {
		t.Fatal(err)
	}
	three.Markdown = "again"
	if err := repo.PutInTray(ctx, camp, three); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClearTray(ctx, camp, "lastwell-fixes"); err != nil {
		t.Fatal(err)
	}
	parts, err = repo.Parts(ctx, camp, "lastwell-fixes")
	if err != nil || len(parts) != 2 || parts[0].Number != 1 || parts[1].Number != 3 || !parts[1].Applied || parts[1].Markdown != "" {
		t.Fatalf("parts after emptying = %+v, %v", parts, err)
	}
	if tray, _ := repo.Tray(ctx, other); len(tray) != 1 {
		t.Errorf("emptying one campaign's tray touched another: %+v", tray)
	}

	// Fingerprints: seen only in their own campaign, and remembering one
	// twice is fine.
	h1, h2 := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if err := repo.Remember(ctx, camp, []string{h1, h1}); err != nil {
		t.Fatal(err)
	}
	seen, err := repo.Seen(ctx, camp, []string{h1, h2})
	if err != nil || len(seen) != 1 || seen[h1].IsZero() {
		t.Fatalf("seen = %v, %v", seen, err)
	}
	if seen, _ := repo.Seen(ctx, other, []string{h1}); len(seen) != 0 {
		t.Errorf("another campaign's block must not count: %v", seen)
	}

	// Removing the campaign removes what it remembered.
	if _, err := db.Exec(`DELETE FROM campaigns WHERE id = ?`, camp); err != nil {
		t.Fatal(err)
	}
	if parts, _ := repo.Parts(ctx, camp, "lastwell-fixes"); len(parts) != 0 {
		t.Errorf("parts outlived their campaign: %v", parts)
	}
	if seen, _ := repo.Seen(ctx, camp, []string{h1}); len(seen) != 0 {
		t.Errorf("blocks outlived their campaign: %v", seen)
	}
}

func TestNoteAccessRepoIntegration(t *testing.T) {
	db := newPartsScratchDB(t)
	repo := NewNoteAccessRepository(db)
	ctx := context.Background()
	camp, user := seedPartsCampaign(t, db)
	other, _ := seedPartsCampaign(t, db)

	if a, err := repo.Get(ctx, camp); err != nil || a.CoDMs || a.Scribes {
		t.Fatalf("never set = %+v, %v; want both off", a, err)
	}
	if err := repo.Set(ctx, camp, user, records.NoteAccess{CoDMs: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, camp, user, records.NoteAccess{CoDMs: true, Scribes: true}); err != nil {
		t.Fatal(err)
	}
	if a, err := repo.Get(ctx, camp); err != nil || !a.CoDMs || !a.Scribes {
		t.Fatalf("after two saves = %+v, %v", a, err)
	}
	if a, err := repo.Get(ctx, other); err != nil || a.CoDMs || a.Scribes {
		t.Fatalf("another campaign = %+v, %v; must stay off", a, err)
	}
}

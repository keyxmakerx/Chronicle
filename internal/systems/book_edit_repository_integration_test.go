package systems

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// TestBookEditRepository_Integration runs the hand-written SQL against a real
// MariaDB: the nullable unique key (many own pages, one copy per package
// page), the upsert, promotion, house chapters and campaign scoping. A fake
// repository can't show any of that.
//
// Skipped under -short. The server comes from CHRONICLE_TEST_DB_DSN, else the
// DB_* env vars; each run gets its own scratch schema, and the test skips when
// no server answers. Run with `make test-int-local`.
func TestBookEditRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openBookTestDB(t)
	ctx := context.Background()
	repo := NewBookEditRepository(db)

	userID, campA, campB := bookTestUUID(t), bookTestUUID(t), bookTestUUID(t)
	mustBookExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "book-"+userID+"@example.test", "Book Test", "x")
	for _, c := range []string{campA, campB} {
		mustBookExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			c, "Book "+c, "book-"+c, userID)
	}

	idx := func(i int) *int { return &i }
	copyPage := func(camp string, i int, body, hash string) StoredBookPage {
		return StoredBookPage{CampaignID: camp, SystemID: "drawsteel", ChapterID: "combat",
			PackageIndex: idx(i), PageJSON: body, BaseHash: hash, UpdatedBy: userID}
	}

	// One copy per package page: saving twice replaces, not duplicates.
	if err := repo.SaveCopy(ctx, copyPage(campA, 0, `{"title":"v1"}`, "h1")); err != nil {
		t.Fatalf("save copy: %v", err)
	}
	if err := repo.SaveCopy(ctx, copyPage(campA, 0, `{"title":"v2"}`, "h2")); err != nil {
		t.Fatalf("resave copy: %v", err)
	}
	// Many own pages share package_index NULL in the unique key.
	own1, err := repo.AddOwn(ctx, StoredBookPage{CampaignID: campA, SystemID: "drawsteel", ChapterID: "combat", PageJSON: `{"title":"own 1"}`, UpdatedBy: userID})
	if err != nil {
		t.Fatalf("add own 1: %v", err)
	}
	own2, err := repo.AddOwn(ctx, StoredBookPage{CampaignID: campA, SystemID: "drawsteel", ChapterID: "combat", PageJSON: `{"title":"own 2"}`, UpdatedBy: userID})
	if err != nil {
		t.Fatalf("add own 2: %v", err)
	}
	if own1 == own2 {
		t.Fatalf("own pages share an id %d", own1)
	}

	edits, err := repo.Load(ctx, campA, "drawsteel")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(edits.Pages) != 3 {
		t.Fatalf("want 3 pages (1 copy, 2 own), got %d", len(edits.Pages))
	}
	for _, p := range edits.Pages {
		if p.PackageIndex != nil && (p.PageJSON != `{"title":"v2"}` || p.BaseHash != "h2") {
			t.Errorf("copy not replaced: %+v", p)
		}
	}

	// Another campaign sees nothing and can't touch A's rows by id.
	if other, err := repo.Load(ctx, campB, "drawsteel"); err != nil || len(other.Pages) != 0 {
		t.Fatalf("campaign B load: %v, %d pages", err, len(other.Pages))
	}
	if ok, err := repo.DeleteOwn(ctx, campB, "drawsteel", "combat", own1); err != nil || ok {
		t.Fatalf("campaign B deleted A's page: ok=%v err=%v", ok, err)
	}

	// Rewriting an own page with identical content is still a success.
	for i := 0; i < 2; i++ {
		if err := repo.UpdateOwn(ctx, campA, "drawsteel", "combat", own1, `{"title":"same"}`, userID); err != nil {
			t.Fatalf("update own (pass %d): %v", i, err)
		}
	}

	// Promotion turns the copy into an own page after the existing ones.
	promoted, err := repo.PromoteCopy(ctx, campA, "drawsteel", "combat", 0)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	edits, _ = repo.Load(ctx, campA, "drawsteel")
	var maxOther int
	for _, p := range edits.Pages {
		if p.PackageIndex != nil {
			t.Errorf("copy still present after promotion: %+v", p)
		}
		if p.ID != promoted && p.SortOrder > maxOther {
			maxOther = p.SortOrder
		}
	}
	for _, p := range edits.Pages {
		if p.ID == promoted && (p.SortOrder <= maxOther || p.BaseHash != "") {
			t.Errorf("promoted page not last or kept its hash: %+v", p)
		}
	}

	// A copy can be dropped; dropping again reports nothing removed.
	if err := repo.SaveCopy(ctx, copyPage(campA, 3, `{"title":"c3"}`, "h3")); err != nil {
		t.Fatalf("save copy 3: %v", err)
	}
	if ok, err := repo.DeleteCopy(ctx, campA, "drawsteel", "combat", 3); err != nil || !ok {
		t.Fatalf("delete copy: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.DeleteCopy(ctx, campA, "drawsteel", "combat", 3); err != nil || ok {
		t.Fatalf("second delete copy: ok=%v err=%v", ok, err)
	}

	// House chapters: added with their first page, updated, removed with pages.
	ch := HouseChapter{CampaignID: campA, SystemID: "drawsteel", ChapterID: "house_our-table", Title: "Our table", CreatedBy: userID}
	if err := repo.AddChapter(ctx, ch, StoredBookPage{PageJSON: `{"blocks":[{}]}`, UpdatedBy: userID}); err != nil {
		t.Fatalf("add chapter: %v", err)
	}
	ch.Title, ch.Director = "Our table rules", true
	for i := 0; i < 2; i++ {
		if err := repo.UpdateChapter(ctx, ch); err != nil {
			t.Fatalf("update chapter (pass %d): %v", i, err)
		}
	}
	edits, _ = repo.Load(ctx, campA, "drawsteel")
	if len(edits.Chapters) != 1 || edits.Chapters[0].Title != "Our table rules" || !edits.Chapters[0].Director {
		t.Fatalf("chapter after update: %+v", edits.Chapters)
	}
	if ok, err := repo.DeleteChapter(ctx, campA, "drawsteel", "house_our-table"); err != nil || !ok {
		t.Fatalf("delete chapter: ok=%v err=%v", ok, err)
	}
	edits, _ = repo.Load(ctx, campA, "drawsteel")
	for _, p := range edits.Pages {
		if p.ChapterID == "house_our-table" {
			t.Errorf("house chapter page survived its chapter: %+v", p)
		}
	}

	// Deleting a campaign takes its book edits with it.
	mustBookExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campA)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM campaign_book_pages WHERE campaign_id = ?`, campA).Scan(&n); err != nil || n != 0 {
		t.Fatalf("pages left after campaign delete: %d (%v)", n, err)
	}
}

func openBookTestDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		cfg := mysql.NewConfig()
		cfg.User = bookGetenvDefault("DB_USER", "chronicle")
		cfg.Passwd = bookGetenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = bookGetenvDefault("DB_HOST", "127.0.0.1:3306")
		raw = cfg.FormatDSN()
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
	}
	cfg.ParseTime = true

	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s: %v — run `make test-db-up`", cfg.Addr, err)
	}
	name := fmt.Sprintf("chronicle_book_%06d", mrand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Fatalf("core migrations did not apply: %v", err)
	}
	return db
}

func bookGetenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustBookExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func bookTestUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

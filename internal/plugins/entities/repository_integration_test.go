package entities

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestEntityTypeRepository_Integration exercises every entity_types read path
// against a real MariaDB so the claimable column added in migration 000029 is
// actually round-tripped through the SELECT column list and scanEntityType.
// A scan/column-order drift panics at runtime (the scan binds by position, not
// name) and never surfaces in `go build` or the mock-based unit tests — only a
// live query catches it, which is the point of this test.
//
// Discovery + skip rules:
//   - Skipped under `-short` (so `make test-unit` never needs a DB).
//   - DSN comes from CHRONICLE_TEST_DB_DSN, else the DB_* env vars, else the
//     dev default that matches the Makefile's DATABASE_URL
//     (chronicle:chronicle@tcp(127.0.0.1:3306)/chronicle). If no DB answers,
//     the test SKIPS rather than fails, so it's safe in CI without a database.
//
// Run with: `make docker-up && make migrate-up && make test-int`.
func TestEntityTypeRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewEntityTypeRepository(db)

	// --- Fixtures: a user → a campaign (entity_types.campaign_id FK chain). ---
	userID := testUUID(t)
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "claimable-int-"+userID+"@example.test", "Claimable Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Claimable Int Test", "claimable-int-"+campaignID[:8], userID)
	// CASCADE on campaigns→entity_types means deleting the campaign clears the
	// types too; users has no cascade, so drop it explicitly.
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	truePtr, falsePtr := boolPtr(true), boolPtr(false)

	// Parent type with claimable unset (NULL → legacy heuristic).
	parent := &EntityType{
		CampaignID: campaignID, Slug: "int-parent", Name: "Int Parent", NamePlural: "Int Parents",
		Icon: "fa-circle", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(),
		SortOrder: 1, Enabled: true, Claimable: nil,
	}
	mustCreate(t, repo, ctx, parent)

	// A claimable=true type carrying a preset category (drives ListByPresetCategory).
	pcCat := PresetCategoryPlayerCharacter
	pc := &EntityType{
		CampaignID: campaignID, Slug: SlugPlayerCharacter, Name: "Player Character", NamePlural: "Player Characters",
		Icon: "fa-user", Color: "#222222", PresetCategory: &pcCat, Fields: []FieldDefinition{},
		Layout: DefaultLayout(), SortOrder: 2, Enabled: true, Claimable: truePtr,
	}
	mustCreate(t, repo, ctx, pc)

	// A claimable=false CHILD of parent (drives ListChildTypes + FindByID's
	// parent-name follow-up query).
	child := &EntityType{
		CampaignID: campaignID, Slug: "int-child", Name: "Int Child", NamePlural: "Int Children",
		Icon: "fa-square", Color: "#333333", ParentTypeID: &parent.ID, Fields: []FieldDefinition{},
		Layout: DefaultLayout(), SortOrder: 3, Enabled: true, Claimable: falsePtr,
	}
	mustCreate(t, repo, ctx, child)

	// --- FindByID: each of the three claimable states + parent-name resolution. ---
	t.Run("FindByID round-trips claimable", func(t *testing.T) {
		gotParent, err := repo.FindByID(ctx, parent.ID)
		if err != nil {
			t.Fatalf("FindByID(parent): %v", err)
		}
		if gotParent.Claimable != nil {
			t.Errorf("parent claimable: want nil, got %v", *gotParent.Claimable)
		}
		gotPC, err := repo.FindByID(ctx, pc.ID)
		if err != nil {
			t.Fatalf("FindByID(pc): %v", err)
		}
		if gotPC.Claimable == nil || !*gotPC.Claimable {
			t.Errorf("pc claimable: want true, got %v", gotPC.Claimable)
		}
		gotChild, err := repo.FindByID(ctx, child.ID)
		if err != nil {
			t.Fatalf("FindByID(child): %v", err)
		}
		if gotChild.Claimable == nil || *gotChild.Claimable {
			t.Errorf("child claimable: want false, got %v", gotChild.Claimable)
		}
		// FindByID's second lookup must populate the parent's name for the child.
		if gotChild.ParentTypeName == nil || *gotChild.ParentTypeName != parent.Name {
			t.Errorf("child parent_type_name: want %q, got %v", parent.Name, gotChild.ParentTypeName)
		}
		// A top-level type has no parent name.
		if gotParent.ParentTypeName != nil {
			t.Errorf("parent parent_type_name: want nil, got %q", *gotParent.ParentTypeName)
		}
	})

	t.Run("FindBySlug round-trips claimable", func(t *testing.T) {
		got, err := repo.FindBySlug(ctx, campaignID, SlugPlayerCharacter)
		if err != nil {
			t.Fatalf("FindBySlug: %v", err)
		}
		if got.Claimable == nil || !*got.Claimable {
			t.Errorf("claimable: want true, got %v", got.Claimable)
		}
	})

	t.Run("ListByCampaign round-trips claimable", func(t *testing.T) {
		got, err := repo.ListByCampaign(ctx, campaignID)
		if err != nil {
			t.Fatalf("ListByCampaign: %v", err)
		}
		bySlug := indexBySlug(got)
		assertClaimable(t, "parent", bySlug["int-parent"], nil)
		assertClaimable(t, "pc", bySlug[SlugPlayerCharacter], truePtr)
		assertClaimable(t, "child", bySlug["int-child"], falsePtr)
	})

	t.Run("ListByPresetCategory round-trips claimable", func(t *testing.T) {
		got, err := repo.ListByPresetCategory(ctx, campaignID, PresetCategoryPlayerCharacter)
		if err != nil {
			t.Fatalf("ListByPresetCategory: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 preset-category row, got %d", len(got))
		}
		assertClaimable(t, "pc", &got[0], truePtr)
	})

	t.Run("ListChildTypes round-trips claimable", func(t *testing.T) {
		got, err := repo.ListChildTypes(ctx, parent.ID)
		if err != nil {
			t.Fatalf("ListChildTypes: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 child row, got %d", len(got))
		}
		assertClaimable(t, "child", &got[0], falsePtr)
	})

	t.Run("ListAll round-trips claimable", func(t *testing.T) {
		got, err := repo.ListAll(ctx)
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		bySlug := indexBySlug(filterByCampaign(got, campaignID))
		assertClaimable(t, "parent", bySlug["int-parent"], nil)
		assertClaimable(t, "pc", bySlug[SlugPlayerCharacter], truePtr)
		assertClaimable(t, "child", bySlug["int-child"], falsePtr)
	})

	t.Run("Update persists claimable change", func(t *testing.T) {
		// nil → true, then true → false, reading back through FindByID each time.
		child.Claimable = truePtr
		if err := repo.Update(ctx, child); err != nil {
			t.Fatalf("Update(child→true): %v", err)
		}
		got, err := repo.FindByID(ctx, child.ID)
		if err != nil {
			t.Fatalf("FindByID after update: %v", err)
		}
		if got.Claimable == nil || !*got.Claimable {
			t.Errorf("after update→true: want true, got %v", got.Claimable)
		}

		child.Claimable = falsePtr
		if err := repo.Update(ctx, child); err != nil {
			t.Fatalf("Update(child→false): %v", err)
		}
		got, err = repo.FindByID(ctx, child.ID)
		if err != nil {
			t.Fatalf("FindByID after second update: %v", err)
		}
		if got.Claimable == nil || *got.Claimable {
			t.Errorf("after update→false: want false, got %v", got.Claimable)
		}
	})
}

// --- helpers ---

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if dsn == "" {
		cfg := mysql.NewConfig()
		cfg.User = getenvDefault("DB_USER", "chronicle")
		cfg.Passwd = getenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = getenvDefault("DB_HOST", "127.0.0.1:3306")
		cfg.DBName = getenvDefault("DB_NAME", "chronicle")
		cfg.ParseTime = true
		dsn = cfg.FormatDSN()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("no test DB reachable at %s (ping: %v) — run `make docker-up && make migrate-up`", maskDSN(dsn), err)
	}
	return db
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// maskDSN hides the password when reporting a skipped/failed connection.
func maskDSN(dsn string) string {
	if cfg, err := mysql.ParseDSN(dsn); err == nil {
		return cfg.Addr + "/" + cfg.DBName
	}
	return "configured DSN"
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func mustCreate(t *testing.T, repo EntityTypeRepository, ctx context.Context, et *EntityType) {
	t.Helper()
	if err := repo.Create(ctx, et); err != nil {
		t.Fatalf("create entity type %q: %v", et.Slug, err)
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

func indexBySlug(types []EntityType) map[string]*EntityType {
	m := make(map[string]*EntityType, len(types))
	for i := range types {
		m[types[i].Slug] = &types[i]
	}
	return m
}

func filterByCampaign(types []EntityType, campaignID string) []EntityType {
	var out []EntityType
	for _, et := range types {
		if et.CampaignID == campaignID {
			out = append(out, et)
		}
	}
	return out
}

// assertClaimable compares an *bool claimable value, tolerating nil on both sides.
func assertClaimable(t *testing.T, label string, got *EntityType, want *bool) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: missing from result set", label)
	}
	switch {
	case want == nil && got.Claimable != nil:
		t.Errorf("%s claimable: want nil, got %v", label, *got.Claimable)
	case want != nil && got.Claimable == nil:
		t.Errorf("%s claimable: want %v, got nil", label, *want)
	case want != nil && got.Claimable != nil && *want != *got.Claimable:
		t.Errorf("%s claimable: want %v, got %v", label, *want, *got.Claimable)
	}
}

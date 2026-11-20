package bestiary

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/database/dbtest"
)

// openBestiaryDB applies the plugin's own migrations to a scratch schema and
// seeds two users, so the SQL these tests cover runs against the real tables.
func openBestiaryDB(t *testing.T) *sql.DB {
	t.Helper()
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	db := dbtest.Open(t, dbtest.Options{
		Prefix:           "chronicle_bst_",
		RequireDSN:       true,
		Plugins:          []dbtest.PluginMigrations{{Slug: "bestiary", FS: sub}},
		StrictMigrations: true,
	})
	for _, id := range []string{"creator-1", "fan-1"} {
		if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			id, id+"@example.test", id, "x"); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	return db
}

func seedPublication(t *testing.T, db *sql.DB, id, visibility string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO bestiary_publications
		(id, creator_id, system_id, name, slug, statblock_json, visibility)
		VALUES (?, 'creator-1', 'drawsteel', ?, ?, '{"name":"x"}', ?)`,
		id, "Name "+id, "slug-"+id, visibility); err != nil {
		t.Fatalf("seed publication: %v", err)
	}
}

// TestFavoritesAndProfileRespectVisibility pins that a favorited publication
// its creator later hides drops out of the favorites list and count (the row
// stays, so it returns when restored), and that profile stats count only
// published creatures.
func TestFavoritesAndProfileRespectVisibility(t *testing.T) {
	db := openBestiaryDB(t)
	repo := NewBestiaryRepository(db)
	ctx := context.Background()

	for id, vis := range map[string]string{
		"p-pub": VisibilityPublished, "p-unl": VisibilityUnlisted,
		"p-draft": VisibilityDraft, "p-arch": VisibilityArchived, "p-flag": VisibilityFlagged,
	} {
		seedPublication(t, db, id, vis)
		if _, err := db.Exec(`INSERT INTO bestiary_favorites (user_id, publication_id) VALUES ('fan-1', ?)`, id); err != nil {
			t.Fatal(err)
		}
	}

	pubs, total, err := repo.ListFavorites(ctx, "fan-1", 1, 20)
	if err != nil {
		t.Fatalf("ListFavorites: %v", err)
	}
	if total != 2 || len(pubs) != 2 {
		t.Fatalf("fan sees %d (total %d) favorites, want the published and unlisted one", len(pubs), total)
	}
	// The creator still sees their own hidden ones among favorites.
	if _, err := db.Exec(`INSERT INTO bestiary_favorites (user_id, publication_id) VALUES ('creator-1', 'p-draft')`); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := repo.ListFavorites(ctx, "creator-1", 1, 20); total != 1 {
		t.Errorf("creator favorites total = %d, want 1", total)
	}

	stats, err := repo.GetCreatorStats(ctx, "creator-1")
	if err != nil {
		t.Fatalf("GetCreatorStats: %v", err)
	}
	if stats.PublicationCount != 1 {
		t.Errorf("profile counts %d publications, want only the published one", stats.PublicationCount)
	}
}

// TestAutoFlagHidesUnlistedAndRemembersState pins that three flags hide an
// unlisted publication too, record what it was, and that ApplyModeration then
// returns it and zeroes the count.
func TestAutoFlagHidesUnlistedAndRemembersState(t *testing.T) {
	db := openBestiaryDB(t)
	repo := NewBestiaryRepository(db)
	ctx := context.Background()
	seedPublication(t, db, "p-unl", VisibilityUnlisted)
	if _, err := db.Exec(`UPDATE bestiary_publications SET flagged_count = 2 WHERE id = 'p-unl'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.FlagPublicationAtomic(ctx, "fan-1", "p-unl", nil, flagThreshold); err != nil {
		t.Fatalf("flag: %v", err)
	}
	p, err := repo.GetByID(ctx, "p-unl")
	if err != nil {
		t.Fatal(err)
	}
	if p.Visibility != VisibilityFlagged || p.PriorVisibility == nil || *p.PriorVisibility != VisibilityUnlisted {
		t.Fatalf("after flags: visibility %q prior %v, want flagged and unlisted", p.Visibility, p.PriorVisibility)
	}
	if err := repo.ApplyModeration(ctx, "p-unl", VisibilityUnlisted, nil, true); err != nil {
		t.Fatal(err)
	}
	p, _ = repo.GetByID(ctx, "p-unl")
	if p.Visibility != VisibilityUnlisted || p.FlaggedCount != 0 || p.PriorVisibility != nil {
		t.Errorf("after approve: %q, count %d, prior %v", p.Visibility, p.FlaggedCount, p.PriorVisibility)
	}
}

// TestReconcileEscapedText pins the one-shot unescape: rows written by the old
// path (marker set) are unescaped once; rows written after (marker clear) keep
// a literal "&amp;" the author typed, and a second run changes nothing.
func TestReconcileEscapedText(t *testing.T) {
	db := openBestiaryDB(t)
	repo := NewBestiaryRepository(db)
	ctx := context.Background()
	seedPublication(t, db, "old", VisibilityPublished)
	seedPublication(t, db, "new", VisibilityPublished)
	if _, err := db.Exec(`UPDATE bestiary_publications SET name = 'Ogre&#39;s Pit &amp; Co', description = '1 &lt; 2', legacy_escaped_text = 1 WHERE id = 'old'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE bestiary_publications SET name = 'Fish &amp; Chips', legacy_escaped_text = 0 WHERE id = 'new'`); err != nil {
		t.Fatal(err)
	}

	for run, want := range []int{1, 0} {
		n, err := repo.ReconcileEscapedText(ctx)
		if err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
		if n != want {
			t.Errorf("run %d changed %d rows, want %d", run+1, n, want)
		}
	}
	old, _ := repo.GetByID(ctx, "old")
	if old.Name != "Ogre's Pit & Co" || old.Description == nil || *old.Description != "1 < 2" {
		t.Errorf("old row = %q / %v", old.Name, old.Description)
	}
	fresh, _ := repo.GetByID(ctx, "new")
	if fresh.Name != "Fish &amp; Chips" {
		t.Errorf("new row name = %q, want it left as typed", fresh.Name)
	}
}

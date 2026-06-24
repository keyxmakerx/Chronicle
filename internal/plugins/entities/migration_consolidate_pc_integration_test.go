package entities

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConsolidatePlayerCharacterDuplicate_Integration exercises migration
// 000030 against a real MariaDB. The migration is a one-time, generic
// reconciliation of a duplicate "Player Characters" sub-category: it moves the
// generic (preset_category 'player_character') type's entities onto the single
// system character type (preset_category 'character', not the default parent),
// then deletes the emptied generic type — but ONLY in campaigns where both
// sides are unambiguous. We replay the migration's SQL (read from the .up.sql
// file so the test tracks the real migration text) over three seeded campaigns:
//
//   - A: the duplicate shape → entities move, generic deleted.
//   - B: system-less (generic only, no target) → left untouched (no-op).
//   - C: ambiguous (generic + TWO system character types) → left untouched.
//
// And it runs the SQL twice to prove idempotency. Skipped under -short / when no
// DB answers, matching the repository integration test.
func TestConsolidatePlayerCharacterDuplicate_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewEntityTypeRepository(db)

	userID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "consolidate-int-"+userID+"@example.test", "Consolidate Int Test", "x")
	defer mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)

	// newCampaign seeds a campaign and registers its teardown (CASCADE clears its
	// entity_types + entities).
	newCampaign := func(t *testing.T, label string) string {
		t.Helper()
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			id, "Consolidate "+label, "consolidate-"+id[:8], userID)
		t.Cleanup(func() { mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, id) })
		return id
	}

	pcCat, charCat := PresetCategoryPlayerCharacter, "character"
	// makeType creates an entity type with the given preset category + slug.
	makeType := func(t *testing.T, campaignID, slug, preset string) int {
		t.Helper()
		et := &EntityType{
			CampaignID: campaignID, Slug: slug, Name: slug, NamePlural: slug + "s",
			Icon: "fa-user", Color: "#444444", PresetCategory: &preset,
			Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true,
		}
		mustCreate(t, repo, ctx, et)
		return et.ID
	}
	// addEntity inserts one entity onto a type (distinct slug per campaign).
	addEntity := func(t *testing.T, campaignID string, typeID int, slug string) string {
		t.Helper()
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`,
			id, campaignID, typeID, "Hero "+slug, slug, userID)
		return id
	}
	countOnType := func(t *testing.T, typeID int) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM entities WHERE entity_type_id = ?`, typeID).Scan(&n); err != nil {
			t.Fatalf("count entities on type %d: %v", typeID, err)
		}
		return n
	}
	typeExists := func(t *testing.T, typeID int) bool {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM entity_types WHERE id = ?`, typeID).Scan(&n); err != nil {
			t.Fatalf("type exists %d: %v", typeID, err)
		}
		return n > 0
	}

	// --- A: the duplicate shape. ---
	campA := newCampaign(t, "A-duplicate")
	aSrc := makeType(t, campA, SlugPlayerCharacter, pcCat)     // generic stray (holds the entities)
	aTgt := makeType(t, campA, "drawsteel-character", charCat) // system character type (empty)
	addEntity(t, campA, aSrc, "a-hero-1")
	addEntity(t, campA, aSrc, "a-hero-2")

	// --- B: system-less (generic only, no system target). ---
	campB := newCampaign(t, "B-systemless")
	bSrc := makeType(t, campB, SlugPlayerCharacter, pcCat)
	addEntity(t, campB, bSrc, "b-hero-1")

	// --- C: ambiguous (generic + TWO system character types). ---
	campC := newCampaign(t, "C-ambiguous")
	cSrc := makeType(t, campC, SlugPlayerCharacter, pcCat)
	cTgt1 := makeType(t, campC, "system-one-character", charCat)
	makeType(t, campC, "system-two-character", charCat)
	addEntity(t, campC, cSrc, "c-hero-1")

	// Replay the real migration SQL (idempotent → run twice).
	migPath := filepath.Join("..", "..", "..", "db", "migrations",
		"000030_consolidate_player_character_duplicate.up.sql")
	execSQLFile(t, db, migPath)
	execSQLFile(t, db, migPath)

	// A: both entities moved onto the system type; generic deleted.
	if got := countOnType(t, aTgt); got != 2 {
		t.Errorf("A: want 2 entities on system type, got %d", got)
	}
	if typeExists(t, aSrc) {
		t.Errorf("A: generic stray should have been deleted")
	}

	// B: untouched — the legitimate generic keeps its entity and itself.
	if !typeExists(t, bSrc) {
		t.Errorf("B: system-less generic should be preserved")
	}
	if got := countOnType(t, bSrc); got != 1 {
		t.Errorf("B: want 1 entity still on generic, got %d", got)
	}

	// C: ambiguous — nothing moved or deleted.
	if !typeExists(t, cSrc) {
		t.Errorf("C: ambiguous generic should be preserved")
	}
	if got := countOnType(t, cSrc); got != 1 {
		t.Errorf("C: want 1 entity still on generic, got %d", got)
	}
	if got := countOnType(t, cTgt1); got != 0 {
		t.Errorf("C: system type should have received nothing, got %d", got)
	}
}

// execSQLFile strips line comments, splits on ';', and execs each statement on a
// connection without multiStatements. Skips (not fails) if the file is missing
// so the test stays robust to where it's invoked from.
func execSQLFile(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read migration file %s: %v", path, err)
	}
	var sb strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	for _, stmt := range strings.Split(sb.String(), ";") {
		s := strings.TrimSpace(stmt)
		if s == "" {
			continue
		}
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec migration statement failed: %v\n--- statement ---\n%s", err, s)
		}
	}
}

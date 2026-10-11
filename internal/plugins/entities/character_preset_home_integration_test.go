package entities

import (
	"context"
	"testing"
)

// TestMovePagesAndAdoptPresetCategory_Integration runs the owner's "move the
// heroes into Characters" transaction against MariaDB: pages (Trash included),
// sidebar folders and saved filters move; Characters takes the preset; the old
// type is disabled, not claimable, and not deleted. Skipped under -short.
func TestMovePagesAndAdoptPresetCategory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewEntityTypeRepository(db)

	userID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "home-int-"+userID+"@example.test", "Home Int Test", "x")
	t.Cleanup(func() { mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID) })
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Home", "home-"+campaignID[:8], userID)
	t.Cleanup(func() { mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID) })

	chars := &EntityType{CampaignID: campaignID, Slug: DefaultCharacterTypeSlug, Name: "Character",
		NamePlural: "Characters", Icon: "fa-user", Color: "#444444", Fields: []FieldDefinition{},
		Layout: DefaultLayout(), Enabled: true}
	mustCreate(t, repo, ctx, chars)
	preset := "character"
	heroes := &EntityType{CampaignID: campaignID, Slug: "drawsteel-character", Name: "Hero",
		NamePlural: "Heroes", Icon: "fa-user", Color: "#444444", PresetCategory: &preset,
		ParentTypeID: &chars.ID, Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true}
	mustCreate(t, repo, ctx, heroes)

	folderID := testUUID(t)
	mustExec(t, db, `INSERT INTO sidebar_nodes (id, campaign_id, entity_type_id, name) VALUES (?, ?, ?, ?)`,
		folderID, campaignID, heroes.ID, "Party")
	mustExec(t, db, `INSERT INTO saved_filters (id, user_id, campaign_id, entity_type_id, name, tag_slugs) VALUES (?, ?, ?, ?, ?, '[]')`,
		testUUID(t), userID, campaignID, heroes.ID, "Front line")
	for i, slug := range []string{"tyne", "ginko", "old-hero"} {
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, parent_node_id, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
			testUUID(t), campaignID, heroes.ID, slug, slug, folderID, userID)
		if i == 2 {
			mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE campaign_id = ? AND slug = ?`, campaignID, slug)
		}
	}

	moved, err := repo.MovePagesAndAdoptPresetCategory(ctx, campaignID, heroes.ID, chars.ID, preset)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved != 3 {
		t.Errorf("moved = %d, want 3 (Trash included)", moved)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM entities WHERE entity_type_id = ?`, heroes.ID); n != 0 {
		t.Errorf("%d pages left on Heroes", n)
	}
	if n := count(`SELECT COUNT(*) FROM sidebar_nodes WHERE id = ? AND entity_type_id = ?`, folderID, chars.ID); n != 1 {
		t.Error("the folder should belong to Characters")
	}
	if n := count(`SELECT COUNT(*) FROM saved_filters WHERE entity_type_id = ?`, chars.ID); n != 1 {
		t.Error("the saved filter should belong to Characters")
	}
	gotChars, _ := repo.FindByID(ctx, chars.ID)
	gotHeroes, err := repo.FindByID(ctx, heroes.ID)
	if err != nil {
		t.Fatalf("Heroes must not be deleted: %v", err)
	}
	if gotChars.PresetCategory == nil || *gotChars.PresetCategory != preset {
		t.Error("Characters should carry the character preset")
	}
	if gotHeroes.Enabled || gotHeroes.PresetCategory != nil || isClaimableType(gotHeroes) {
		t.Errorf("Heroes = enabled %v, preset %v, claimable %v; want all off",
			gotHeroes.Enabled, gotHeroes.PresetCategory, isClaimableType(gotHeroes))
	}

	// Characters now has a preset, so a replay is refused and changes nothing.
	if _, err := repo.MovePagesAndAdoptPresetCategory(ctx, campaignID, heroes.ID, chars.ID, preset); err == nil {
		t.Error("a replay onto a Characters that already has the preset should be refused")
	}
}

package entities

import (
	"context"
	"testing"
)

// TestMoveEntitiesAndDeleteType_Integration runs a category merge's
// transaction against MariaDB: every page (Trash included), the sidebar
// folder and the saved filter move to the target, a page stays in its folder,
// the source row is gone, and another campaign's rows are untouched. Skipped
// under -short.
func TestMoveEntitiesAndDeleteType_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewEntityTypeRepository(db)

	userID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "merge-int-"+userID+"@example.test", "Merge Int Test", "x")
	t.Cleanup(func() { mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID) })

	// seed makes a campaign with a category, a folder in it, a saved filter on
	// it and pages (the last one in the Trash), all filed in the folder.
	type seeded struct {
		campaign, folder, filter string
		typ                      *EntityType
	}
	seed := func(name string, pages ...string) seeded {
		s := seeded{campaign: testUUID(t), folder: testUUID(t), filter: testUUID(t)}
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
			s.campaign, name, "merge-"+s.campaign[:8], userID)
		t.Cleanup(func() { mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, s.campaign) })
		s.typ = &EntityType{CampaignID: s.campaign, Slug: "adventures-2", Name: "Adventure", NamePlural: "Adventures",
			Icon: "fa-box", Color: "#6366f1", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true}
		mustCreate(t, repo, ctx, s.typ)
		mustExec(t, db, `INSERT INTO sidebar_nodes (id, campaign_id, entity_type_id, name) VALUES (?, ?, ?, ?)`,
			s.folder, s.campaign, s.typ.ID, "Act one")
		mustExec(t, db, `INSERT INTO saved_filters (id, user_id, campaign_id, entity_type_id, name, tag_slugs) VALUES (?, ?, ?, ?, ?, '[]')`,
			s.filter, userID, s.campaign, s.typ.ID, "Open threads")
		for i, slug := range pages {
			mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, parent_node_id, created_by, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, NOW() - INTERVAL 1 DAY, NOW() - INTERVAL 1 DAY)`,
				testUUID(t), s.campaign, s.typ.ID, slug, slug, s.folder, userID)
			if i == len(pages)-1 {
				mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE campaign_id = ? AND slug = ?`, s.campaign, slug)
			}
		}
		return s
	}

	src := seed("Merge", "the-heist", "the-chase", "old-draft")
	target := &EntityType{CampaignID: src.campaign, Slug: "lore", Name: "Lore", NamePlural: "Lore",
		Icon: "fa-book", Color: "#10b981", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true}
	mustCreate(t, repo, ctx, target)
	other := seed("Other", "their-page", "their-trash")

	moved, err := repo.MoveEntitiesAndDeleteType(ctx, src.campaign, src.typ.ID, target.ID)
	if err != nil {
		t.Fatalf("merge: %v", err)
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
	checks := []struct {
		what  string
		query string
		args  []any
		want  int
	}{
		{"pages in the target", `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND entity_type_id = ?`, []any{src.campaign, target.ID}, 3},
		{"the trashed page in the target", `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND entity_type_id = ? AND deleted_at IS NOT NULL`, []any{src.campaign, target.ID}, 1},
		{"pages still in their folder", `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND parent_node_id = ?`, []any{src.campaign, src.folder}, 3},
		{"pages marked changed for sync", `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND updated_at > NOW() - INTERVAL 1 HOUR`, []any{src.campaign}, 3},
		{"the folder in the target", `SELECT COUNT(*) FROM sidebar_nodes WHERE id = ? AND entity_type_id = ?`, []any{src.folder, target.ID}, 1},
		{"the filter on the target", `SELECT COUNT(*) FROM saved_filters WHERE id = ? AND entity_type_id = ?`, []any{src.filter, target.ID}, 1},
		{"the source category", `SELECT COUNT(*) FROM entity_types WHERE id = ?`, []any{src.typ.ID}, 0},
		{"the other campaign's category", `SELECT COUNT(*) FROM entity_types WHERE id = ?`, []any{other.typ.ID}, 1},
		{"the other campaign's pages where they were", `SELECT COUNT(*) FROM entities WHERE campaign_id = ? AND entity_type_id = ? AND parent_node_id = ? AND updated_at < NOW() - INTERVAL 1 HOUR`, []any{other.campaign, other.typ.ID, other.folder}, 2},
		{"the other campaign's folder", `SELECT COUNT(*) FROM sidebar_nodes WHERE id = ? AND entity_type_id = ?`, []any{other.folder, other.typ.ID}, 1},
		{"the other campaign's filter", `SELECT COUNT(*) FROM saved_filters WHERE id = ? AND entity_type_id = ?`, []any{other.filter, other.typ.ID}, 1},
	}
	for _, c := range checks {
		if n := count(c.query, c.args...); n != c.want {
			t.Errorf("%s: %d, want %d", c.what, n, c.want)
		}
	}

	// A category of another campaign is never moved or deleted by this one's
	// merge, even when its id is named.
	if moved, err := repo.MoveEntitiesAndDeleteType(ctx, src.campaign, other.typ.ID, target.ID); err != nil || moved != 0 {
		t.Errorf("cross-campaign merge = %d, %v; want 0, nil", moved, err)
	}
	if n := count(`SELECT COUNT(*) FROM entity_types WHERE id = ?`, other.typ.ID); n != 1 {
		t.Error("another campaign's category was deleted")
	}
}

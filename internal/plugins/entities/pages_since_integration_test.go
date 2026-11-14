package entities

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// PagesSince counts and names only the pages the viewer may open that were
// created after the cut-off: a private page, a custom-permission page they
// are not granted, an Adventures page, a template, a trashed page and an old
// page add to nothing, and the owner sees what players cannot.
func TestPagesSince_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, player, camp, other := testUUID(t), testUUID(t), testUUID(t), testUUID(t)
	for _, u := range []string{userID, player} {
		mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, u, "since-"+u+"@example.test", "Since", "x")
	}
	for _, c := range []string{camp, other} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Since", "since-"+c[:8], userID)
	}
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, camp, other)
		mustExec(t, db, `DELETE FROM users WHERE id IN (?, ?)`, userID, player)
	}()
	typeIn := func(c, slug, preset string) int64 {
		var p any
		if preset != "" {
			p = preset
		}
		res, err := db.ExecContext(ctx, `INSERT INTO entity_types (campaign_id, slug, name, name_plural, preset_category) VALUES (?, ?, ?, ?, ?)`, c, slug, slug, slug, p)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	place, adventures, foreignType := typeIn(camp, "place", ""), typeIn(camp, "adventure", PresetCategoryAdventures), typeIn(other, "place", "")

	cutoff := time.Now().UTC().Add(-48 * time.Hour)
	page := func(c string, typ int64, name string, age time.Duration, private, template, trashed bool, visibility string) string {
		id := testUUID(t)
		created := time.Now().UTC().Add(-age)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, is_private, is_template, visibility, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, c, typ, name, "p-"+id[:8], private, template, visibility, userID, created, created)
		if trashed {
			mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, id)
		}
		return id
	}
	hour := time.Hour
	page(camp, place, "Harrowgate", 1*hour, false, false, false, "default")
	page(camp, place, "The Iron Pact", 2*hour, false, false, false, "default")
	page(camp, place, "Old Town", 100*hour, false, false, false, "default")
	page(camp, place, "Secret Cult Vault", 1*hour, true, false, false, "default")
	page(camp, place, "Custom Locked Room", 1*hour, false, false, false, "custom")
	page(camp, adventures, "Adventure Prep", 1*hour, false, false, false, "default")
	page(camp, place, "Template Page", 1*hour, false, true, false, "default")
	page(camp, place, "Trashed Page", 1*hour, false, false, true, "default")
	page(other, foreignType, "Other Campaign Page", 1*hour, false, false, false, "default")

	r := NewPagesSinceReader(db)
	got, err := r.Since(ctx, camp, cutoff, int(permissions.RolePlayer), player, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || len(got.Newest) != 2 || got.Newest[0].Name != "Harrowgate" || got.Newest[1].Name != "The Iron Pact" {
		t.Fatalf("player: %+v", got)
	}
	for _, p := range got.Newest {
		if strings.Contains(p.Name, "Secret") || strings.Contains(p.Name, "Locked") || strings.Contains(p.Name, "Adventure") {
			t.Errorf("player was shown %q", p.Name)
		}
	}
	owner, err := r.Since(ctx, camp, cutoff, int(permissions.RoleOwner), userID, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Owner sees the private, custom and Adventures pages as well; templates
	// and trashed pages never count.
	if owner.Count != 5 {
		t.Errorf("owner count = %d, want 5: %+v", owner.Count, owner.Newest)
	}
	if none, _ := r.Since(ctx, camp, time.Now().UTC(), int(permissions.RoleOwner), userID, 3); none.Count != 0 || len(none.Newest) != 0 {
		t.Errorf("nothing is newer than now: %+v", none)
	}
}

package entities

import (
	"context"
	"testing"
)

// A page made while a trashed page of the same name still holds the plain
// slug gets "-2"; a name lookup finds the live page, never the trashed one,
// never a page whose own name only looks like the suffix, and never a page
// whose slug merely starts with the name's.
func TestGetByNameSlug_PastATrashedPage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, camp := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, userID, "slug-"+userID+"@example.test", "Slug", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, camp, "Slugs", "slugs-"+camp[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, camp)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()
	res, err := db.ExecContext(ctx, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'shop', 'Shop', 'Shops')`, camp)
	if err != nil {
		t.Fatal(err)
	}
	typeID, _ := res.LastInsertId()
	page := func(name, slug string, trashed bool) string {
		id := testUUID(t)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, camp, typeID, name, slug, userID)
		if trashed {
			mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, id)
		}
		return id
	}
	page("Hessa Ord's Workshop Annex", "hessa-ord-s-workshop-annex", false)
	page("Hessa Ord's Workshop", "hessa-ord-s-workshop", true)
	live := page("Hessa Ord's Workshop", "hessa-ord-s-workshop-2", false)
	page("Lamp 2", "lamp-2", false)

	svc := NewEntityService(NewEntityRepository(db), NewEntityTypeRepository(db), NewEntityPermissionRepository(db))
	e, err := svc.GetByNameSlug(ctx, camp, Slugify("Hessa Ord's Workshop"))
	if err != nil || e.ID != live {
		t.Fatalf("got %+v, %v; want the live page %s", e, err, live)
	}
	if e, err := svc.GetByNameSlug(ctx, camp, Slugify("Lamp")); e != nil || err == nil {
		t.Fatalf("Lamp found %+v; Lamp 2 is not called Lamp", e)
	}
	// Only uniqueness suffixes come back, so a longer name never fills the cap.
	if got, err := NewEntityRepository(db).ListSuffixedSlugs(ctx, camp, "hessa-ord-s-workshop"); err != nil || len(got) != 1 || got[0].ID != live {
		t.Fatalf("suffixed slugs %+v, %v; want only the live page", got, err)
	}
	if got, err := NewEntityRepository(db).ListSuffixedSlugs(ctx, camp, "hessa%"); err != nil || len(got) != 0 {
		t.Fatalf("a wildcard base matched %d pages, %v", len(got), err)
	}
}

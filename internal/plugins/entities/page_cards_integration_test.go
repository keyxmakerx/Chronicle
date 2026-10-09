package entities

import (
	"context"
	"fmt"
	"testing"
)

// PageCards names live pages of one campaign in one query: a deleted page,
// another campaign's page and an unknown id are absent, and a list longer
// than one query's chunk is still served whole.
func TestPageCards_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, campA, campB := testUUID(t), testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "cards-"+userID+"@example.test", "Cards Test", "x")
	for _, c := range []string{campA, campB} {
		mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, c, "Cards", "cards-"+c[:8], userID)
	}
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id IN (?, ?)`, campA, campB)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()
	typeIn := func(c string) int64 {
		res, err := db.ExecContext(ctx, `INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, 'place', 'Place', 'Places')`, c)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	tA, tB := typeIn(campA), typeIn(campB)
	page := func(c string, typ int64, name, img string) string {
		id := testUUID(t)
		var imgArg any
		if img != "" {
			imgArg = img
		}
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, image_path, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, id, c, typ, name, "p-"+id[:8], imgArg, userID)
		return id
	}
	tavern := page(campA, tA, "Tavern", "/media/t.png")
	temple := page(campA, tA, "Temple", "")
	gone := page(campA, tA, "Gone", "")
	mustExec(t, db, `UPDATE entities SET deleted_at = NOW() WHERE id = ?`, gone)
	foreign := page(campB, tB, "Elsewhere", "")

	got, err := NewPageCards(db).InCampaign(ctx, campA, []string{tavern, temple, temple, gone, foreign, "nope", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d cards, want 2: %+v", len(got), got)
	}
	if c := got[tavern]; c.Name != "Tavern" || c.ImagePath != "/media/t.png" || c.TypeName != "Place" || c.TypeSlug != "place" {
		t.Errorf("tavern = %+v", c)
	}
	if c := got[temple]; c.Name != "Temple" || c.ImagePath != "" {
		t.Errorf("temple = %+v", c)
	}

	// More ids than one chunk: every real page still comes back.
	many := []string{tavern}
	for i := 0; i < maxPageCards+5; i++ {
		many = append(many, fmt.Sprintf("missing-%d", i))
	}
	many = append(many, temple)
	got, err = NewPageCards(db).InCampaign(ctx, campA, many)
	if err != nil || len(got) != 2 {
		t.Fatalf("chunked: %d cards, err %v", len(got), err)
	}
}

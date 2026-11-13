package entities

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// Two saves to different sheet keys at the same moment (a Foundry push and a
// shop purchase, say) must both land. Each iteration races two MergeFields
// calls through the real service and repository and checks both keys, plus a
// key neither touched, survive.
func TestEntityService_MergeFields_OverlappingWriters_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	user, campaignID := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		user, "merge-int-"+user+"@example.test", "Merge Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Merge Int Test", "merge-int-"+campaignID[:8], user)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, user)
	}()

	types := NewEntityTypeRepository(db)
	et := &EntityType{
		CampaignID: campaignID, Slug: "merge-pc", Name: "PC", NamePlural: "PCs",
		Icon: "fa-user", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true,
	}
	mustCreate(t, types, ctx, et)
	repo := NewEntityRepository(db)
	svc := NewEntityService(repo, types, NewEntityPermissionRepository(db))

	id := testUUID(t)
	mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, fields_data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		id, campaignID, et.ID, "Hero", "hero", user, `{"untouched":{"nested":[1,2]}}`)

	for i := 1; i <= 40; i++ {
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs[0] = svc.MergeFields(ctx, id, map[string]any{"hp": float64(i)})
		}()
		go func() {
			defer wg.Done()
			errs[1] = svc.MergeFields(ctx, id, map[string]any{"coins": map[string]any{"gp": float64(i)}})
		}()
		wg.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("iteration %d errors: %v, %v", i, errs[0], errs[1])
		}

		e, err := repo.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		coins, _ := e.FieldsData["coins"].(map[string]any)
		if e.FieldsData["hp"] != float64(i) || coins["gp"] != float64(i) {
			t.Fatalf("iteration %d lost an update: %v", i, e.FieldsData)
		}
		if fmt.Sprint(e.FieldsData["untouched"]) != "map[nested:[1 2]]" {
			t.Fatalf("iteration %d changed an untouched key: %v", i, e.FieldsData)
		}
	}

	t.Run("null clears a key and a missing entity is not found", func(t *testing.T) {
		if err := svc.MergeFields(ctx, id, map[string]any{"hp": nil}); err != nil {
			t.Fatal(err)
		}
		e, _ := repo.FindByID(ctx, id)
		if _, ok := e.FieldsData["hp"]; ok {
			t.Errorf("hp should be cleared: %v", e.FieldsData)
		}
		if err := svc.MergeFields(ctx, testUUID(t), map[string]any{"a": 1}); err == nil {
			t.Error("want not found for a missing entity")
		}
	})
}

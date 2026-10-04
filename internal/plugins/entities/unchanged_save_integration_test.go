package entities

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Saving values an entity already has, in the same second as its last write,
// changes no row; MariaDB then reports zero affected rows and that must not
// read as "entity not found" (every Foundry permission push hit this). A
// genuinely missing entity is still a not-found.
func TestEntityRepositories_UnchangedSaveIsNotNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	userID, campaignID, entityID := testUUID(t), testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "vis-int-"+userID+"@example.test", "Vis Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Vis Int Test", "vis-int-"+campaignID[:8], userID)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()
	et := &EntityType{
		CampaignID: campaignID, Slug: "vis-int", Name: "Vis", NamePlural: "Vis",
		Icon: "fa-circle", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(), Enabled: true,
	}
	mustCreate(t, NewEntityTypeRepository(db), ctx, et)
	mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`, entityID, campaignID, et.ID, "Vis Entity", "vis-entity", userID)

	repo := NewEntityPermissionRepository(db)
	for i := 0; i < 3; i++ {
		if err := repo.UpdateVisibility(ctx, entityID, VisibilityDefault); err != nil {
			t.Fatalf("save %d of an unchanged visibility: %v", i+1, err)
		}
	}

	entRepo := NewEntityRepository(db)
	ent, err := entRepo.FindByID(ctx, entityID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := entRepo.Update(ctx, ent); err != nil {
			t.Fatalf("entity save %d with unchanged values: %v", i+1, err)
		}
		if err := entRepo.UpdateEntry(ctx, entityID, `{"type":"doc"}`, "<p>x</p>", "x"); err != nil {
			t.Fatalf("entry save %d with unchanged values: %v", i+1, err)
		}
	}

	missing := testUUID(t)
	for name, err := range map[string]error{
		"UpdateVisibility": repo.UpdateVisibility(ctx, missing, VisibilityDefault),
		"UpdateEntry":      entRepo.UpdateEntry(ctx, missing, `{"type":"doc"}`, "", ""),
	} {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
			t.Errorf("%s on a missing entity: got %v, want a not-found", name, err)
		}
	}
}

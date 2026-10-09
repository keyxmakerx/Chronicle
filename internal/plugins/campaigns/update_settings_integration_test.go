package campaigns

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestUpdateSettings_Integration pins that saving the same settings twice in
// a row succeeds. MariaDB reports zero affected rows for a write that changes
// nothing, and the Foundry update flow saves the pin and then an unchanged pin
// mode within the same second; that must not read as a missing campaign.
//
// Skipped under -short; uses the same database setup as TestNavPins_Integration.
func TestUpdateSettings_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	repo := NewCampaignRepository(db)

	owner, campaignID := navPinsUUID(t), navPinsUUID(t)
	navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, "settings-"+owner+"@example.test", "Settings", "x")
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Settings", "settings-"+campaignID[:8], owner)

	settings := `{"foundry_module_pin":"2.0.2","foundry_module_pin_mode":"approve_first"}`
	for i := 0; i < 3; i++ {
		if err := repo.UpdateSettings(ctx, campaignID, settings); err != nil {
			t.Fatalf("write %d of unchanged settings: %v", i+1, err)
		}
	}

	err := repo.UpdateSettings(ctx, navPinsUUID(t), settings)
	if appErr, ok := err.(*apperror.AppError); !ok || appErr.Code != 404 {
		t.Fatalf("unknown campaign: got %v, want not found", err)
	}

	// Campaign import writes the sidebar and dashboard straight after the
	// campaign is created; a file whose sidebar matches what is already
	// stored must not be reported as a lost "sidebar layout".
	sidebar := `{"items":[]}`
	layout := `{"rows":[]}`
	for i := 0; i < 3; i++ {
		if err := repo.UpdateSidebarConfig(ctx, campaignID, sidebar); err != nil {
			t.Fatalf("write %d of unchanged sidebar: %v", i+1, err)
		}
		if err := repo.UpdateDashboardLayout(ctx, campaignID, &layout); err != nil {
			t.Fatalf("write %d of unchanged dashboard: %v", i+1, err)
		}
	}
	err = repo.UpdateSidebarConfig(ctx, navPinsUUID(t), sidebar)
	if appErr, ok := err.(*apperror.AppError); !ok || appErr.Code != 404 {
		t.Fatalf("unknown campaign sidebar: got %v, want not found", err)
	}
	err = repo.UpdateDashboardLayout(ctx, navPinsUUID(t), &layout)
	if appErr, ok := err.(*apperror.AppError); !ok || appErr.Code != 404 {
		t.Fatalf("unknown campaign dashboard: got %v, want not found", err)
	}
}

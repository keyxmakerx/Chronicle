package campaigns

import (
	"context"
	"encoding/json"
	"testing"
)

// TestSettingsWritesKeepOtherKeys_Integration pins that a settings writer
// holding an old copy of the campaign changes only the key it set: an admin's
// module pin landing between an owner's read and save survives the save.
//
// Skipped under -short; uses the same database setup as TestNavPins_Integration.
func TestSettingsWritesKeepOtherKeys_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	repo := NewCampaignRepository(db)
	svc := &campaignService{repo: repo}

	owner, campaignID := navPinsUUID(t), navPinsUUID(t)
	navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, "settings-"+owner+"@example.test", "Settings", "x")
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by, settings) VALUES (?, ?, ?, ?, ?)`,
		campaignID, "Settings", "settings-"+campaignID[:8], owner, `{"welcome_message":"old","brand_name":"Keep","extra_unknown":1}`)

	stale, err := repo.FindByID(ctx, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetFoundryModulePin(ctx, campaignID, "2.9.0"); err != nil {
		t.Fatalf("pin: %v", err)
	}
	settings := stale.ParseSettings()
	settings.WelcomeMessage = ""
	settings.DefaultVisibility = "dm_only"
	if err := svc.saveSettingsChanges(ctx, stale, campaignID, mustJSON(t, settings)); err != nil {
		t.Fatalf("stale save: %v", err)
	}

	got, err := repo.FindByID(ctx, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	st := got.ParseSettings()
	if st.FoundryModulePin != "2.9.0" {
		t.Errorf("the pin set in between was undone: %q", st.FoundryModulePin)
	}
	if st.DefaultVisibility != "dm_only" || st.WelcomeMessage != "" || st.BrandName != "Keep" {
		t.Errorf("settings after save: %+v", st)
	}
	var unknown int
	if err := db.QueryRow(`SELECT JSON_VALUE(settings, '$.extra_unknown') FROM campaigns WHERE id = ?`, campaignID).Scan(&unknown); err != nil || unknown != 1 {
		t.Errorf("a key this build doesn't know was dropped: %v %d", err, unknown)
	}

	if err := repo.UpdateSettingsKeys(ctx, campaignID, map[string]json.RawMessage{"x') , settings = ('": nil}); err == nil {
		t.Error("a key outside the settings key shape was accepted")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// reconcile_harptos_seasons_integration_test.go runs ReconcileHarptosSeasons
// against a real MariaDB, since it rewrites stored rows on server start and
// the in-memory fake can't prove its SQL. Skipped under `-short`.
//
// Run with: `make test-int-local`.
package calendar

import (
	"context"
	"testing"
)

func TestReconcileHarptosSeasons_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	repo := NewCalendarRepository(db)
	store := repo.(HarptosSeasonStore)
	fix := newTestCampaign(t, db, "harptos-reconcile")

	preset, err := LoadPreset("harptos")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	fixed, err := fixedHarptosSeasons(preset)
	if err != nil {
		t.Fatalf("fixedHarptosSeasons: %v", err)
	}

	seed := func(name string, seasons []Season) string {
		t.Helper()
		cal := newTestCalendar(testUUID(t), fix.CampaignID, name)
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		if err := repo.SetMonths(ctx, cal.ID, preset.Months); err != nil {
			t.Fatalf("SetMonths %s: %v", name, err)
		}
		if err := repo.SetSeasons(ctx, cal.ID, seasons); err != nil {
			t.Fatalf("SetSeasons %s: %v", name, err)
		}
		return cal.ID
	}
	idsByName := func(id string) map[string]int {
		t.Helper()
		got, err := repo.GetSeasons(ctx, id)
		if err != nil {
			t.Fatalf("GetSeasons: %v", err)
		}
		m := make(map[string]int, len(got))
		for _, s := range got {
			m[s.Name] = s.ID
		}
		return m
	}
	mustMatch := func(label, id string, want []Season) {
		t.Helper()
		got, err := repo.GetSeasons(ctx, id)
		if err != nil {
			t.Fatalf("%s: GetSeasons: %v", label, err)
		}
		if !seasonsMatchPreset(got, want) {
			t.Errorf("%s: seasons = %+v, want %+v", label, got, want)
		}
	}

	edited := append([]Season(nil), oldHarptosSeasons...)
	edited[1].EndDay = 20 // an owner moved the end of The Thaw

	oldID := seed("Old Harptos", oldHarptosSeasons)
	newID := seed("Already fixed", fixed)
	editedID := seed("Owner edited", edited)
	oldIDs := idsByName(oldID)

	n, err := ReconcileHarptosSeasons(ctx, store)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if n != 1 {
		t.Errorf("first run repaired %d calendars, want 1", n)
	}
	mustMatch("old calendar", oldID, fixed)
	mustMatch("already-fixed calendar", newID, fixed)
	mustMatch("owner-edited calendar", editedID, edited)
	for name, id := range idsByName(oldID) {
		if oldIDs[name] != id {
			t.Errorf("season %q id changed from %d to %d; rows must be updated in place", name, oldIDs[name], id)
		}
	}

	n, err = ReconcileHarptosSeasons(ctx, store)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if n != 0 {
		t.Errorf("second run repaired %d calendars, want 0", n)
	}
	mustMatch("old calendar after second run", oldID, fixed)
	mustMatch("owner-edited calendar after second run", editedID, edited)
}

// moon_lore_integration_test.go checks the moon lore migration is idempotent
// and reversible, and that the repository round-trips a moon's lore page: the
// name is read only within the calendar's own campaign, a write that does not
// mention the page keeps it, and deleting the page unlinks it.
package calendar

import (
	"context"
	"testing"
)

func TestMoonLoreMigration_Integration_IdempotentAndReversible(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	steps := []struct {
		name    string
		file    string
		present bool
	}{
		{"re-apply on top of itself", "026_moon_lore_page.up.sql", true},
		{"roll back", "026_moon_lore_page.down.sql", false},
		{"roll back twice", "026_moon_lore_page.down.sql", false},
		{"apply again", "026_moon_lore_page.up.sql", true},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			execMigration(t, db, st.file)
			if got := columnExists(t, db, "calendar_moons", "lore_entity_id"); got != st.present {
				t.Errorf("calendar_moons.lore_entity_id present = %v, want %v", got, st.present)
			}
		})
	}
}

func TestMoonLoreRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewCalendarRepository(db)
	fix := newTestCampaign(t, db, "moon-lore")
	other := newTestCampaign(t, db, "moon-lore-other")
	lore := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Selune, Our Lady Silver")
	second := newTestEntity(t, db, fix.CampaignID, fix.UserID, "The Tides")
	foreign := newTestEntity(t, db, other.CampaignID, other.UserID, "Someone else's page")

	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Moon Lore Calendar")
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SetMoons(ctx, cal.ID, []MoonInput{
		{Name: "Selune", CycleDays: 30, Color: "#eeeeee", LoreEntityID: &lore},
		{Name: "Plain", CycleDays: 9, Color: "#111111"},
		{Name: "Stray", CycleDays: 5, Color: "#222222", LoreEntityID: &foreign},
	}); err != nil {
		t.Fatalf("SetMoons: %v", err)
	}
	byName := func() map[string]Moon {
		moons, err := repo.GetMoons(ctx, cal.ID)
		if err != nil {
			t.Fatalf("GetMoons: %v", err)
		}
		out := map[string]Moon{}
		for _, m := range moons {
			out[m.Name] = m
		}
		return out
	}

	moons := byName()
	if m := moons["Selune"]; m.LoreEntityID == nil || *m.LoreEntityID != lore || m.LoreEntityName != "Selune, Our Lady Silver" {
		t.Errorf("lore did not round-trip with its name: %+v", m)
	}
	if m := moons["Plain"]; m.LoreEntityID != nil || m.LoreEntityName != "" {
		t.Errorf("a moon with no page read one: %+v", m)
	}
	// The repository never names a page from another campaign, whatever id
	// was stored; the service refuses to store one in the first place.
	if n := moons["Stray"].LoreEntityName; n != "" {
		t.Errorf("a page from another campaign was named: %q", n)
	}

	selune := moons["Selune"].ID
	t.Run("an update that does not mention the page keeps it", func(t *testing.T) {
		if err := repo.SetMoons(ctx, cal.ID, []MoonInput{{ID: &selune, Name: "Selune II", CycleDays: 31, Color: "#eeeeee"}}); err != nil {
			t.Fatal(err)
		}
		m := byName()["Selune II"]
		if m.LoreEntityID == nil || *m.LoreEntityID != lore {
			t.Errorf("page lost by an update that never named it: %+v", m)
		}
	})
	t.Run("a structure save replaces and clears only when told to", func(t *testing.T) {
		write := func(m MoonInput) {
			t.Helper()
			err := repo.ApplyStructure(ctx, cal.ID, func(*StructureState) (*StructureWrite, error) {
				return &StructureWrite{
					Months: []MonthInput{{Name: "Only", Days: 30}}, Weekdays: []WeekdayInput{{Name: "Day"}},
					Moons: []MoonInput{m}, CurrentMonth: 1, CurrentDay: 1,
				}, nil
			})
			if err != nil {
				t.Fatalf("ApplyStructure: %v", err)
			}
		}
		write(MoonInput{ID: &selune, Name: "Selune II", CycleDays: 31})
		if m := byName()["Selune II"]; m.LoreEntityID == nil || *m.LoreEntityID != lore {
			t.Errorf("a save that left the page out cleared it: %+v", m)
		}
		write(MoonInput{ID: &selune, Name: "Selune II", CycleDays: 31, LoreSet: true, LoreEntityID: &second})
		if m := byName()["Selune II"]; m.LoreEntityID == nil || *m.LoreEntityID != second {
			t.Errorf("a named page was not saved: %+v", m)
		}
		write(MoonInput{ID: &selune, Name: "Selune II", CycleDays: 31, LoreSet: true})
		if m := byName()["Selune II"]; m.LoreEntityID != nil {
			t.Errorf("an explicit clear kept the page: %+v", m)
		}
	})
	t.Run("deleting the page unlinks the moon, not removes it", func(t *testing.T) {
		write := MoonInput{ID: &selune, Name: "Selune II", CycleDays: 31, LoreSet: true, LoreEntityID: &lore}
		if err := repo.SetMoons(ctx, cal.ID, []MoonInput{write}); err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, `DELETE FROM entities WHERE id = ?`, lore)
		m, ok := byName()["Selune II"]
		if !ok || m.LoreEntityID != nil {
			t.Errorf("moon after its page was deleted = %+v (found %v)", m, ok)
		}
	})
}

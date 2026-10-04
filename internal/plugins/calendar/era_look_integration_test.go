// era_look_integration_test.go checks the era look migration is idempotent
// and reversible, and that the repository round-trips every era look
// column, including the lore page's name, which is read only within the
// era's own campaign.
package calendar

import (
	"context"
	"testing"
)

func TestEraLookMigration_Integration_IdempotentAndReversible(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	steps := []struct {
		name    string
		file    string
		present bool
	}{
		{"re-apply on top of itself", "024_era_look.up.sql", true},
		{"roll back", "024_era_look.down.sql", false},
		{"roll back twice", "024_era_look.down.sql", false},
		{"apply again", "024_era_look.up.sql", true},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			execMigration(t, db, st.file)
			for _, c := range [][2]string{
				{"calendars", "era_colors_on"}, {"calendars", "era_feel"}, {"calendars", "era_intensity"}, {"calendars", "era_speed"},
				{"calendar_eras", "color_2"}, {"calendar_eras", "style"}, {"calendar_eras", "feel"},
				{"calendar_eras", "lore_entity_id"}, {"calendar_eras", "dm_note"}, {"calendar_eras", "hidden_until_begins"},
			} {
				if got := columnExists(t, db, c[0], c[1]); got != st.present {
					t.Errorf("%s.%s present = %v, want %v", c[0], c[1], got, st.present)
				}
			}
		})
	}
}

func TestEraLookRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewCalendarRepository(db)
	fix := newTestCampaign(t, db, "era-look")
	other := newTestCampaign(t, db, "era-look-other")
	lore := newTestEntity(t, db, fix.CampaignID, fix.UserID, "The Burning Years")
	foreign := newTestEntity(t, db, other.CampaignID, other.UserID, "Someone else's page")

	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Era Look Calendar")
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EraLook != DefaultEraLook() {
		t.Errorf("new calendar look = %+v, want the default %+v", got.EraLook, DefaultEraLook())
	}

	era, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Age of Ember", StartYear: 1, StartMonth: 1, StartDay: 1,
		Color: "#6e1a2a", Color2: strp("#d6893a"), Style: EraStyleInk, Feel: strp(EraFeelLively),
		LoreEntityID: &lore, DMNote: strp("note"), HiddenUntilBegins: true})
	if err != nil {
		t.Fatalf("CreateEra: %v", err)
	}
	plain, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Plain", StartYear: 9, StartMonth: 1, StartDay: 1, Color: "#111111",
		Style: EraStyleGas, LoreEntityID: &foreign})
	if err != nil {
		t.Fatalf("CreateEra (foreign lore): %v", err)
	}

	eras, err := repo.GetEras(ctx, cal.ID)
	if err != nil {
		t.Fatalf("GetEras: %v", err)
	}
	byID := map[int]Era{}
	for _, e := range eras {
		byID[e.ID] = e
	}
	e := byID[era.ID]
	if e.Style != EraStyleInk || e.Color2 == nil || *e.Color2 != "#d6893a" || e.Feel == nil || *e.Feel != EraFeelLively ||
		e.DMNote == nil || *e.DMNote != "note" || !e.HiddenUntilBegins || e.LoreEntityID == nil || *e.LoreEntityID != lore {
		t.Errorf("era did not round-trip: %+v", e)
	}
	if e.LoreEntityName != "The Burning Years" {
		t.Errorf("lore name = %q, want the page's name", e.LoreEntityName)
	}
	// The repository never names a page from another campaign, whatever id
	// was stored; the service refuses to store one in the first place.
	if n := byID[plain.ID].LoreEntityName; n != "" {
		t.Errorf("a page from another campaign was named: %q", n)
	}

	look := EraLook{ColorsOn: false, Feel: EraFeelCustom, Intensity: 2.5, Speed: 0.2}
	if err := repo.SaveEraLook(ctx, cal.ID, look, []EraLookWrite{
		{ID: era.ID, Color: "#1d3f6e", Color2: nil, Style: EraStyleGas, Feel: nil},
	}); err != nil {
		t.Fatalf("SaveEraLook: %v", err)
	}
	got, err = repo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EraLook != look {
		t.Errorf("look = %+v, want %+v", got.EraLook, look)
	}
	after, err := repo.GetEraByID(ctx, era.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Color != "#1d3f6e" || after.Color2 != nil || after.Style != EraStyleGas || after.Feel != nil {
		t.Errorf("era look not saved: %+v", after)
	}
	if after.DMNote == nil || !after.HiddenUntilBegins {
		t.Errorf("SaveEraLook must leave the era's content fields alone: %+v", after)
	}

	// An era id from another calendar is never touched.
	cal2 := newTestCalendar(testUUID(t), other.CampaignID, "Other")
	if err := repo.Create(ctx, cal2); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveEraLook(ctx, cal2.ID, DefaultEraLook(), []EraLookWrite{{ID: era.ID, Color: "#000000", Style: EraStyleGas}}); err == nil {
		after, _ = repo.GetEraByID(ctx, era.ID)
		if after.Color == "#000000" {
			t.Error("SaveEraLook changed an era of another calendar")
		}
	}

	// Deleting the lore page clears the link rather than the era.
	mustExec(t, db, `DELETE FROM entities WHERE id = ?`, lore)
	after, err = repo.GetEraByID(ctx, era.ID)
	if err != nil || after == nil {
		t.Fatalf("era should survive its lore page: %v", err)
	}
	if after.LoreEntityID != nil {
		t.Errorf("lore link should clear when the page is deleted, got %v", *after.LoreEntityID)
	}
}

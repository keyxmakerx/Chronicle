package calendar

import (
	"context"
	"strings"
	"testing"
)

// TestApplyStructure_Integration runs a structure save against MariaDB:
// moons and seasons keep their rows (and what the editor doesn't show),
// month positions are remapped on events and eras including a swap, the
// current date and leap rule change and nothing else on the calendar row
// does, and a failure part-way writes nothing.
func TestApplyStructure_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewCalendarRepository(db)
	events := NewEventRepository(db)
	fix := newTestCampaign(t, db, "structure")

	setup := func(t *testing.T) (*Calendar, int, int) {
		t.Helper()
		cal := newTestCalendar(testUUID(t), fix.CampaignID, "Structure")
		cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 5, 3, 7
		cal.LeapYearEvery = 4
		desc := "keep me"
		cal.Description = &desc
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.SetMonths(ctx, cal.ID, []MonthInput{
			{Name: "Alpha", Days: 30, SortOrder: 0}, {Name: "Beta", Days: 30, SortOrder: 1}, {Name: "Gamma", Days: 30, SortOrder: 2},
		}); err != nil {
			t.Fatalf("SetMonths: %v", err)
		}
		tint := "#ff0000"
		if err := repo.SetMoons(ctx, cal.ID, []MoonInput{{Name: "Luna", CycleDays: 28, PhaseOffset: 3, Color: "#123456", Tint: &tint, HiddenFromPlayers: true}}); err != nil {
			t.Fatalf("SetMoons: %v", err)
		}
		moons, _ := repo.GetMoons(ctx, cal.ID)
		weather := "rain"
		if err := repo.SetSeasons(ctx, cal.ID, []Season{{Name: "Warm", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 30, Color: "#00ff00", WeatherEffect: &weather}}); err != nil {
			t.Fatalf("SetSeasons: %v", err)
		}
		seasons, _ := repo.GetSeasons(ctx, cal.ID)
		return cal, moons[0].ID, seasons[0].ID
	}

	t.Run("a save keeps moon and season rows, remaps positions and sets today", func(t *testing.T) {
		cal, moonID, seasonID := setup(t)
		end := 3
		endY, endD := 5, 2
		mk := func(name string, month int) *Event {
			return &Event{ID: testUUID(t), CalendarID: cal.ID, Name: name, Year: 5, Month: month, Day: 2, Visibility: "everyone", AllDay: true}
		}
		inAlpha, inGamma, spanning := mk("in alpha", 1), mk("in gamma", 3), mk("spanning", 1)
		spanning.EndYear, spanning.EndMonth, spanning.EndDay = &endY, &end, &endD
		for _, e := range []*Event{inAlpha, inGamma, spanning} {
			if err := events.CreateEvent(ctx, e); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
		}
		era, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Age", StartYear: 1, StartMonth: 3, StartDay: 1, Color: "#6366f1"})
		if err != nil {
			t.Fatalf("CreateEra: %v", err)
		}

		// Gamma and Alpha swap places; Beta stays second.
		w := StructureWrite{
			Months:        []MonthInput{{Name: "Gamma", Days: 30, SortOrder: 0}, {Name: "Beta", Days: 30, SortOrder: 1}, {Name: "Alpha", Days: 30, SortOrder: 2}},
			Weekdays:      []WeekdayInput{{Name: "One", SortOrder: 0}, {Name: "Two", SortOrder: 1, IsRestDay: true}},
			Moons:         []MoonInput{{ID: &moonID, Name: "Selene", CycleDays: 30, Color: "#808080"}, {Name: "Second", CycleDays: 9, Color: "#c0c0c0"}},
			Seasons:       []Season{{ID: seasonID, Name: "Summer", StartMonth: 3, StartDay: 1, EndMonth: 2, EndDay: 30, Color: "#808080"}},
			LeapYearEvery: 0,
			CurrentMonth:  1, CurrentDay: 7,
			MonthRemap: map[int]int{1: 3, 3: 1},
		}
		if err := repo.ApplyStructure(ctx, cal.ID, w); err != nil {
			t.Fatalf("ApplyStructure: %v", err)
		}

		got, _ := repo.GetByID(ctx, cal.ID)
		if got.CurrentYear != 5 || got.CurrentMonth != 1 || got.CurrentDay != 7 || got.LeapYearEvery != 0 {
			t.Errorf("calendar = year %d month %d day %d leap %d, want 5/1/7 leap 0", got.CurrentYear, got.CurrentMonth, got.CurrentDay, got.LeapYearEvery)
		}
		if got.Description == nil || *got.Description != "keep me" || got.Name != "Structure" {
			t.Errorf("a structure save must not touch other calendar columns, got name %q description %v", got.Name, got.Description)
		}

		months, _ := repo.GetMonths(ctx, cal.ID)
		if len(months) != 3 || months[0].Name != "Gamma" || months[2].Name != "Alpha" {
			t.Errorf("months = %+v", months)
		}
		weekdays, _ := repo.GetWeekdays(ctx, cal.ID)
		if len(weekdays) != 2 || !weekdays[1].IsRestDay {
			t.Errorf("weekdays = %+v", weekdays)
		}

		moons, _ := repo.GetMoons(ctx, cal.ID)
		var kept *Moon
		for i := range moons {
			if moons[i].ID == moonID {
				kept = &moons[i]
			}
		}
		if len(moons) != 2 || kept == nil {
			t.Fatalf("moons = %+v, want moon %d kept plus one new", moons, moonID)
		}
		if kept.Name != "Selene" || kept.CycleDays != 30 {
			t.Errorf("kept moon = %+v, want name and cycle updated", kept)
		}
		if !kept.HiddenFromPlayers || kept.Color != "#123456" || kept.PhaseOffset != 3 || kept.Tint == nil || *kept.Tint != "#ff0000" {
			t.Errorf("kept moon lost what the editor doesn't show: %+v", kept)
		}

		seasons, _ := repo.GetSeasons(ctx, cal.ID)
		if len(seasons) != 1 || seasons[0].ID != seasonID || seasons[0].Name != "Summer" || seasons[0].StartMonth != 3 {
			t.Errorf("seasons = %+v, want season %d updated in place", seasons, seasonID)
		}
		if seasons[0].Color != "#00ff00" || seasons[0].WeatherEffect == nil || *seasons[0].WeatherEffect != "rain" {
			t.Errorf("kept season lost its colour or weather: %+v", seasons[0])
		}

		check := func(id string, wantMonth int, wantEnd *int) {
			t.Helper()
			e, err := events.GetEvent(ctx, id)
			if err != nil {
				t.Fatalf("GetEvent: %v", err)
			}
			if e.Month != wantMonth {
				t.Errorf("event %s month = %d, want %d", e.Name, e.Month, wantMonth)
			}
			if wantEnd != nil && (e.EndMonth == nil || *e.EndMonth != *wantEnd) {
				t.Errorf("event %s end month = %v, want %d", e.Name, e.EndMonth, *wantEnd)
			}
		}
		one := 1
		check(inAlpha.ID, 3, nil)
		check(inGamma.ID, 1, nil)
		check(spanning.ID, 3, &one)

		gotEra, _ := repo.GetEraByID(ctx, era.ID)
		if gotEra.StartMonth != 1 {
			t.Errorf("era start month = %d, want 1 (Gamma's new place)", gotEra.StartMonth)
		}
	})

	t.Run("a failure part-way writes nothing", func(t *testing.T) {
		cal, moonID, _ := setup(t)
		w := StructureWrite{
			Months:   []MonthInput{{Name: "Only", Days: 10}},
			Weekdays: []WeekdayInput{{Name: "One"}},
			// A season name past its column fails the season write after the
			// calendar row, months, weekdays and moons have been written.
			Moons:        []MoonInput{{ID: &moonID, Name: "Changed", CycleDays: 5}},
			Seasons:      []Season{{Name: strings.Repeat("x", 300), StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 1, Color: "#808080"}},
			CurrentMonth: 1, CurrentDay: 1,
		}
		err := repo.ApplyStructure(ctx, cal.ID, w)
		if err == nil {
			t.Fatal("expected the oversized season name to fail")
		}
		got, _ := repo.GetByID(ctx, cal.ID)
		if got.CurrentMonth != 3 || got.CurrentDay != 7 || got.LeapYearEvery != 4 {
			t.Errorf("calendar row changed despite the rollback: %+v", got)
		}
		months, _ := repo.GetMonths(ctx, cal.ID)
		if len(months) != 3 {
			t.Errorf("months = %d, want the original 3", len(months))
		}
		moons, _ := repo.GetMoons(ctx, cal.ID)
		if len(moons) != 1 || moons[0].Name != "Luna" {
			t.Errorf("moons changed despite the rollback: %+v", moons)
		}
	})
}

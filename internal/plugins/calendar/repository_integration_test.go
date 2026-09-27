// repository_integration_test.go exercises CalendarRepository against a real
// MariaDB: CRUD, every structural sub-resource, tenant isolation between
// campaigns, and cascade behavior on delete. Skipped under `-short`.
//
// Run with: `make test-int-local`, or `make test-db-up && CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/calendar/... -run Integration`.
// openTestDB (dbtest_support_test.go) creates its own scratch schema, so the
// DSN names a server only, never a database.
package calendar

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func newTestCalendar(id, campaignID, name string) *Calendar {
	return &Calendar{
		ID: id, CampaignID: campaignID, Mode: ModeFantasy, Name: name,
		CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		Visibility: "everyone",
	}
}

func TestCalendarRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() }) // after fixture cleanups (LIFO), not before

	ctx := context.Background()
	repo := NewCalendarRepository(db)

	fixA := newTestCampaign(t, db, "cal-a")
	fixB := newTestCampaign(t, db, "cal-b")

	t.Run("Create and GetByID round-trip every V5 column, including visibility", func(t *testing.T) {
		north := HemisphereNorth
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Round Trip Calendar")
		cal.Hemisphere = &north
		cal.ForecastsEnabled = true
		cal.MonthStartsNewWeek = true
		cal.Visibility = "dm_only"
		rules := `{"allowed_users":["u9"]}`
		cal.VisibilityRules = &rules
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, err := repo.GetByID(ctx, cal.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got == nil {
			t.Fatal("GetByID: got nil for a calendar that was just created")
		}
		if got.Hemisphere == nil || *got.Hemisphere != HemisphereNorth {
			t.Errorf("Hemisphere = %v, want %q", got.Hemisphere, HemisphereNorth)
		}
		if !got.ForecastsEnabled {
			t.Error("ForecastsEnabled did not round-trip true")
		}
		if !got.MonthStartsNewWeek {
			t.Error("MonthStartsNewWeek did not round-trip true")
		}
		if got.CampaignID != fixA.CampaignID {
			t.Errorf("CampaignID = %q, want %q", got.CampaignID, fixA.CampaignID)
		}
		if got.Visibility != "dm_only" {
			t.Errorf("Visibility = %q, want %q (Create must write it, not leave the column default)", got.Visibility, "dm_only")
		}
		if got.VisibilityRules == nil || *got.VisibilityRules != rules {
			t.Errorf("VisibilityRules = %v, want %q", got.VisibilityRules, rules)
		}
	})

	t.Run("GetByID returns a not-found error for a nonexistent id, never (nil, nil)", func(t *testing.T) {
		// middleware.RequireInCampaign calls a method on the result; a bare
		// nil pointer there would panic instead of reporting not-found.
		got, err := repo.GetByID(ctx, testUUID(t))
		if got != nil {
			t.Errorf("GetByID(nonexistent) = %+v, want nil", got)
		}
		if apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("GetByID(nonexistent) err = %v, want a not-found apperror", err)
		}
	})

	t.Run("the old world-state mood-tint columns are gone from the schema", func(t *testing.T) {
		// A direct assertion against information_schema, not just "Calendar
		// has no such Go field": proves 020 actually dropped the columns
		// rather than the Go struct merely forgetting them.
		for _, col := range []string{"mood_tint_color", "mood_tint_intensity"} {
			var n int
			if err := db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM information_schema.columns
				 WHERE table_schema = DATABASE() AND table_name = 'calendars' AND column_name = ?`, col,
			).Scan(&n); err != nil {
				t.Fatalf("information_schema query: %v", err)
			}
			if n != 0 {
				t.Errorf("calendars.%s still exists; 020 should have dropped it", col)
			}
		}
	})

	t.Run("ListByCampaignID never leaks another campaign's calendar (tenant isolation)", func(t *testing.T) {
		calA := newTestCalendar(testUUID(t), fixA.CampaignID, "Isolation A")
		calB := newTestCalendar(testUUID(t), fixB.CampaignID, "Isolation B")
		if err := repo.Create(ctx, calA); err != nil {
			t.Fatalf("Create A: %v", err)
		}
		if err := repo.Create(ctx, calB); err != nil {
			t.Fatalf("Create B: %v", err)
		}

		listA, err := repo.ListByCampaignID(ctx, fixA.CampaignID)
		if err != nil {
			t.Fatalf("ListByCampaignID(A): %v", err)
		}
		for _, c := range listA {
			if c.ID == calB.ID {
				t.Fatalf("ListByCampaignID(A) returned campaign B's calendar %s: tenant isolation broken", calB.ID)
			}
			if c.CampaignID != fixA.CampaignID {
				t.Fatalf("calendar %s in campaign A's list has CampaignID %q", c.ID, c.CampaignID)
			}
		}
	})

	t.Run("SetDefault unsets every other default in the campaign", func(t *testing.T) {
		cal1 := newTestCalendar(testUUID(t), fixA.CampaignID, "Default 1")
		cal2 := newTestCalendar(testUUID(t), fixA.CampaignID, "Default 2")
		cal1.IsDefault = true
		if err := repo.Create(ctx, cal1); err != nil {
			t.Fatalf("Create cal1: %v", err)
		}
		if err := repo.Create(ctx, cal2); err != nil {
			t.Fatalf("Create cal2: %v", err)
		}
		if err := repo.SetDefault(ctx, fixA.CampaignID, cal2.ID); err != nil {
			t.Fatalf("SetDefault: %v", err)
		}
		got1, _ := repo.GetByID(ctx, cal1.ID)
		got2, _ := repo.GetByID(ctx, cal2.ID)
		if got1.IsDefault {
			t.Error("cal1 is still default after SetDefault(cal2)")
		}
		if !got2.IsDefault {
			t.Error("cal2 was not marked default")
		}
		def, err := repo.GetDefaultByCampaignID(ctx, fixA.CampaignID)
		if err != nil || def == nil || def.ID != cal2.ID {
			t.Errorf("GetDefaultByCampaignID = %+v, %v; want cal2", def, err)
		}
	})

	t.Run("SetDefault rejects a calendar id from another campaign and changes nothing", func(t *testing.T) {
		// Its own campaign fixture: idx_one_default_per_campaign allows only
		// one default per campaign, and fixA already has one from an
		// earlier subtest in this test function.
		mine := newTestCampaign(t, db, "setdefault-own")
		own := newTestCalendar(testUUID(t), mine.CampaignID, "Owns The Default")
		own.IsDefault = true
		if err := repo.Create(ctx, own); err != nil {
			t.Fatalf("Create own: %v", err)
		}
		foreign := newTestCalendar(testUUID(t), fixB.CampaignID, "Foreign Calendar")
		if err := repo.Create(ctx, foreign); err != nil {
			t.Fatalf("Create foreign: %v", err)
		}

		err := repo.SetDefault(ctx, mine.CampaignID, foreign.ID)
		if apperror.SafeCode(err) != http.StatusNotFound {
			t.Fatalf("SetDefault(own campaign, another campaign's calendar) err = %v, want not-found", err)
		}

		gotOwn, _ := repo.GetByID(ctx, own.ID)
		if !gotOwn.IsDefault {
			t.Error("a rejected SetDefault must change nothing, but it cleared the campaign's real default")
		}
		gotForeign, _ := repo.GetByID(ctx, foreign.ID)
		if gotForeign.IsDefault {
			t.Error("SetDefault(own campaign, a foreign calendar) should never mark the foreign calendar default")
		}
	})

	t.Run("Update persists every mutable column including the new switches", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Before Update")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		south := HemisphereSouth
		cal.Name = "After Update"
		cal.Hemisphere = &south
		cal.ForecastsEnabled = true
		cal.MonthStartsNewWeek = true
		cal.Visibility = "dm_only"
		if err := repo.Update(ctx, cal); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := repo.GetByID(ctx, cal.ID)
		if got.Name != "After Update" || got.Hemisphere == nil || *got.Hemisphere != HemisphereSouth ||
			!got.ForecastsEnabled || !got.MonthStartsNewWeek || got.Visibility != "dm_only" {
			t.Errorf("Update did not persist: %+v", got)
		}
	})

	t.Run("UpdateVisibility sets visibility and rules", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Vis Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		rules := `{"allowed_users":["u1"]}`
		if err := repo.UpdateVisibility(ctx, cal.ID, "dm_only", &rules); err != nil {
			t.Fatalf("UpdateVisibility: %v", err)
		}
		got, _ := repo.GetByID(ctx, cal.ID)
		if got.Visibility != "dm_only" || got.VisibilityRules == nil || *got.VisibilityRules != rules {
			t.Errorf("UpdateVisibility did not persist: %+v", got)
		}
	})

	t.Run("structural sub-resources: table-driven CRUD", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Structure Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := repo.SetMonths(ctx, cal.ID, []MonthInput{
			{Name: "Firstmonth", Days: 30, SortOrder: 0},
			{Name: "Secondmonth", Days: 31, SortOrder: 1, LeapYearDays: 1},
		}); err != nil {
			t.Fatalf("SetMonths: %v", err)
		}
		months, err := repo.GetMonths(ctx, cal.ID)
		if err != nil || len(months) != 2 {
			t.Fatalf("GetMonths = %+v, %v; want 2 months", months, err)
		}
		if months[0].Name != "Firstmonth" || months[1].LeapYearDays != 1 {
			t.Errorf("months did not round-trip: %+v", months)
		}

		if err := repo.SetWeekdays(ctx, cal.ID, []WeekdayInput{
			{Name: "Moonday", SortOrder: 0, IsRestDay: true},
			{Name: "Sunday", SortOrder: 1},
		}); err != nil {
			t.Fatalf("SetWeekdays: %v", err)
		}
		weekdays, err := repo.GetWeekdays(ctx, cal.ID)
		if err != nil || len(weekdays) != 2 || !weekdays[0].IsRestDay {
			t.Fatalf("GetWeekdays = %+v, %v", weekdays, err)
		}

		if err := repo.SetMoons(ctx, cal.ID, []MoonInput{
			{Name: "Selene", CycleDays: 29.5, Color: "#ffffff"},
		}); err != nil {
			t.Fatalf("SetMoons: %v", err)
		}
		moons, err := repo.GetMoons(ctx, cal.ID)
		if err != nil || len(moons) != 1 {
			t.Fatalf("GetMoons = %+v, %v", moons, err)
		}
		if moons[0].HiddenFromPlayers {
			t.Error("a new moon should default to visible")
		}
		if moons[0].PhaseSource != "css-clip" || moons[0].BaseDesign != "moon-realistic-selene" {
			t.Errorf("moon render-param defaults not applied: %+v", moons[0])
		}
		if err := repo.SetMoonHidden(ctx, cal.ID, moons[0].ID, true); err != nil {
			t.Fatalf("SetMoonHidden: %v", err)
		}
		moons, _ = repo.GetMoons(ctx, cal.ID)
		if !moons[0].HiddenFromPlayers {
			t.Error("SetMoonHidden(true) did not persist")
		}

		if err := repo.SetSeasons(ctx, cal.ID, []Season{
			{Name: "Winter", StartMonth: 11, StartDay: 1, EndMonth: 2, EndDay: 28, Color: "#a0a0ff"},
		}); err != nil {
			t.Fatalf("SetSeasons: %v", err)
		}
		seasons, err := repo.GetSeasons(ctx, cal.ID)
		if err != nil || len(seasons) != 1 || seasons[0].Name != "Winter" {
			t.Fatalf("GetSeasons = %+v, %v", seasons, err)
		}

		if err := repo.SetCycles(ctx, cal.ID, []CycleInput{
			{Name: "Zodiac", CycleLength: 12, Type: "yearly", Entries: []CycleEntryInput{
				{Name: "Ram", YearOffset: 0, SortOrder: 0},
				{Name: "Bull", YearOffset: 1, SortOrder: 1},
			}},
		}); err != nil {
			t.Fatalf("SetCycles: %v", err)
		}
		cycles, err := repo.GetCycles(ctx, cal.ID)
		if err != nil || len(cycles) != 1 || len(cycles[0].Entries) != 2 {
			t.Fatalf("GetCycles = %+v, %v; want 1 cycle with 2 entries", cycles, err)
		}

		if err := repo.SetFestivals(ctx, cal.ID, []FestivalInput{
			{Name: "Midwinter", AfterMonth: intPtr(6)},
		}); err != nil {
			t.Fatalf("SetFestivals: %v", err)
		}
		festivals, err := repo.GetFestivals(ctx, cal.ID)
		if err != nil || len(festivals) != 1 || festivals[0].AfterMonth == nil || *festivals[0].AfterMonth != 6 {
			t.Fatalf("GetFestivals = %+v, %v", festivals, err)
		}
	})

	t.Run("SetMoons upserts by id: a re-save keeps an existing moon's id and hidden flag", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Moon Upsert Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.SetMoons(ctx, cal.ID, []MoonInput{
			{Name: "Keeper", CycleDays: 29.5, Color: "#ffffff"},
		}); err != nil {
			t.Fatalf("SetMoons (initial): %v", err)
		}
		moons, err := repo.GetMoons(ctx, cal.ID)
		if err != nil || len(moons) != 1 {
			t.Fatalf("GetMoons = %+v, %v", moons, err)
		}
		keeperID := moons[0].ID
		if err := repo.SetMoonHidden(ctx, cal.ID, keeperID, true); err != nil {
			t.Fatalf("SetMoonHidden: %v", err)
		}

		if err := repo.SetMoons(ctx, cal.ID, []MoonInput{
			{ID: &keeperID, Name: "Keeper Renamed", CycleDays: 30, Color: "#eeeeee"},
			{Name: "Newcomer", CycleDays: 27.3, Color: "#dddddd"},
		}); err != nil {
			t.Fatalf("SetMoons (re-save): %v", err)
		}

		moons, err = repo.GetMoons(ctx, cal.ID)
		if err != nil || len(moons) != 2 {
			t.Fatalf("GetMoons after re-save = %+v, %v; want 2", moons, err)
		}
		var kept *Moon
		for i := range moons {
			if moons[i].ID == keeperID {
				kept = &moons[i]
			}
		}
		if kept == nil {
			t.Fatal("the existing moon's id did not survive SetMoons; upsert re-numbered it")
		}
		if kept.Name != "Keeper Renamed" {
			t.Errorf("kept moon Name = %q, want the updated name", kept.Name)
		}
		if !kept.HiddenFromPlayers {
			t.Error("SetMoons un-hid a moon that SetMoonHidden(true) had hidden; upsert must not touch HiddenFromPlayers on update")
		}
	})

	t.Run("eras: day-granular CRUD, per-era create/update/delete, resort on delete", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Era Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}

		e1, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111"})
		if err != nil {
			t.Fatalf("CreateEra e1: %v", err)
		}
		endYear := 100
		e2, err := repo.CreateEra(ctx, cal.ID, EraInput{
			Name: "Second Age", StartYear: 101, StartMonth: 3, StartDay: 15,
			EndYear: &endYear, EndMonth: intPtr(12), EndDay: intPtr(31), Color: "#222222",
		})
		if err != nil {
			t.Fatalf("CreateEra e2: %v", err)
		}
		if e1.SortOrder != 0 || e2.SortOrder != 1 {
			t.Errorf("auto sort_order = %d, %d; want 0, 1", e1.SortOrder, e2.SortOrder)
		}
		if e2.StartMonth != 3 || e2.StartDay != 15 || e2.EndMonth == nil || *e2.EndMonth != 12 {
			t.Errorf("era day-granular fields did not round-trip: %+v", e2)
		}

		if err := repo.UpdateEra(ctx, cal.ID, e1.ID, EraInput{Name: "First Age (renamed)", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111", SortOrder: 0}); err != nil {
			t.Fatalf("UpdateEra: %v", err)
		}
		got, err := repo.GetEraByID(ctx, e1.ID)
		if err != nil || got == nil || got.Name != "First Age (renamed)" {
			t.Fatalf("GetEraByID after UpdateEra = %+v, %v", got, err)
		}

		e3, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Third Age", StartYear: 200, Color: "#333333"})
		if err != nil {
			t.Fatalf("CreateEra e3: %v", err)
		}
		if err := repo.DeleteEra(ctx, cal.ID, e1.ID); err != nil {
			t.Fatalf("DeleteEra: %v", err)
		}
		eras, err := repo.GetEras(ctx, cal.ID)
		if err != nil || len(eras) != 2 {
			t.Fatalf("GetEras after delete = %+v, %v; want 2 remaining", eras, err)
		}
		if eras[0].SortOrder != 0 || eras[1].SortOrder != 1 {
			t.Errorf("remaining eras were not re-sorted contiguously: %+v", eras)
		}
		if eras[0].ID != e2.ID || eras[1].ID != e3.ID {
			t.Errorf("unexpected surviving eras: %+v", eras)
		}

		if err := repo.DeleteEra(ctx, cal.ID, 999999999); apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("DeleteEra on a nonexistent id = %v, want a not-found error", err)
		}
	})

	t.Run("UpdateEra with no actual change still succeeds (not a false not-found)", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "No-op Era Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		era, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Same Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#333333"})
		if err != nil {
			t.Fatalf("CreateEra: %v", err)
		}
		// Save back the exact same values: a production DSN (no
		// clientFoundRows) reports zero rows changed here even though the
		// era exists.
		if err := repo.UpdateEra(ctx, cal.ID, era.ID, EraInput{Name: "Same Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#333333", SortOrder: era.SortOrder}); err != nil {
			t.Errorf("UpdateEra with identical values = %v, want nil (existing row, no real change)", err)
		}
	})

	t.Run("UpdateEra and DeleteEra reject an era id from another calendar", func(t *testing.T) {
		calA := newTestCalendar(testUUID(t), fixA.CampaignID, "Scoping Cal A")
		if err := repo.Create(ctx, calA); err != nil {
			t.Fatalf("Create calA: %v", err)
		}
		calB := newTestCalendar(testUUID(t), fixB.CampaignID, "Scoping Cal B")
		if err := repo.Create(ctx, calB); err != nil {
			t.Fatalf("Create calB: %v", err)
		}
		era, err := repo.CreateEra(ctx, calA.ID, EraInput{Name: "A's Era", StartYear: 1, Color: "#111111"})
		if err != nil {
			t.Fatalf("CreateEra: %v", err)
		}

		err = repo.UpdateEra(ctx, calB.ID, era.ID, EraInput{Name: "Hijacked", StartYear: 1, Color: "#222222"})
		if apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("UpdateEra(wrong calendar) err = %v, want not-found", err)
		}
		got, _ := repo.GetEraByID(ctx, era.ID)
		if got == nil || got.Name != "A's Era" {
			t.Errorf("UpdateEra(wrong calendar) changed the era: %+v", got)
		}

		err = repo.DeleteEra(ctx, calB.ID, era.ID)
		if apperror.SafeCode(err) != http.StatusNotFound {
			t.Errorf("DeleteEra(wrong calendar) err = %v, want not-found", err)
		}
		got, _ = repo.GetEraByID(ctx, era.ID)
		if got == nil {
			t.Error("DeleteEra(wrong calendar) deleted an era it doesn't own")
		}
	})

	t.Run("SetEras upserts by id: a re-save with the era's own id keeps entity_era_links", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Era Upsert Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		era, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Age of Heroes", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111"})
		if err != nil {
			t.Fatalf("CreateEra: %v", err)
		}
		entityID := newTestEntity(t, db, fixA.CampaignID, fixA.UserID, "Tied Hero")
		eventRepo := NewEventRepository(db)
		if err := eventRepo.LinkEntityEra(ctx, entityID, era.ID, nil); err != nil {
			t.Fatalf("LinkEntityEra: %v", err)
		}
		linkCount := func() int {
			var n int
			if err := db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM entity_era_links WHERE era_id = ?`, era.ID).Scan(&n); err != nil {
				t.Fatalf("count entity_era_links: %v", err)
			}
			return n
		}
		if linkCount() != 1 {
			t.Fatalf("fixture: expected the tie to exist before SetEras")
		}

		// A caller that round-trips GetEras' own id back through
		// EraInput.ID (an eras-editing form, say) must not lose the tie.
		if err := repo.SetEras(ctx, cal.ID, []EraInput{
			{ID: &era.ID, Name: "Age of Heroes (renamed)", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111"},
		}); err != nil {
			t.Fatalf("SetEras: %v", err)
		}
		if linkCount() != 1 {
			t.Error("SetEras with the era's own id dropped entity_era_links; the era was deleted and reinserted instead of updated")
		}
		got, err := repo.GetEraByID(ctx, era.ID)
		if err != nil || got == nil || got.Name != "Age of Heroes (renamed)" {
			t.Errorf("GetEraByID after SetEras = %+v, %v", got, err)
		}

		// Known limitation: every import format supplies EraInput with no
		// id (there is no Chronicle id in an import file), so omitting the
		// id still replaces the era and drops its ties. This documents
		// that, rather than asserting it as a requirement.
		if err := repo.SetEras(ctx, cal.ID, []EraInput{
			{Name: "Age of Heroes (no id)", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111"},
		}); err != nil {
			t.Fatalf("SetEras (no id): %v", err)
		}
		if linkCount() != 0 {
			t.Error("SetEras without an id was expected to still replace the era and drop its old ties")
		}
	})

	t.Run("ApplyImport rolls back every write when a later step fails", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Rollback Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := repo.SetMonths(ctx, cal.ID, []MonthInput{{Name: "Original Month", Days: 30}}); err != nil {
			t.Fatalf("SetMonths: %v", err)
		}

		badCal := *cal
		badCal.Name = "Should Not Persist"
		// calendar_eras.name is VARCHAR(200); this DB runs with
		// STRICT_TRANS_TABLES, so an over-length value is a hard error, not
		// a silent truncation.
		tooLong := strings.Repeat("x", 201)
		result := &ImportResult{
			Months: []MonthInput{{Name: "New Month", Days: 31}},
			Eras:   []EraInput{{Name: tooLong, StartYear: 1, Color: "#000000"}},
		}
		if err := repo.ApplyImport(ctx, &badCal, result); err == nil {
			t.Fatal("ApplyImport with an over-length era name should fail, got nil error")
		}

		got, err := repo.GetByID(ctx, cal.ID)
		if err != nil || got.Name != "Rollback Calendar" {
			t.Errorf("calendar name changed despite the failed import: %+v, %v", got, err)
		}
		months, err := repo.GetMonths(ctx, cal.ID)
		if err != nil || len(months) != 1 || months[0].Name != "Original Month" {
			t.Errorf("months changed despite the failed import (an earlier successful step was not rolled back): %+v, %v", months, err)
		}
	})

	t.Run("ApplyImport replaces calendar fields and structure in one transaction", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Import Target")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		cal.Name = "Imported Name"
		cal.CurrentYear = 42
		result := &ImportResult{
			Months:   []MonthInput{{Name: "Imported Month", Days: 28}},
			Weekdays: []WeekdayInput{{Name: "Imported Day"}},
			Eras:     []EraInput{{Name: "Imported Era", StartYear: 5, StartMonth: 1, StartDay: 1, Color: "#000000"}},
		}
		if err := repo.ApplyImport(ctx, cal, result); err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		got, err := repo.GetByID(ctx, cal.ID)
		if err != nil || got.Name != "Imported Name" || got.CurrentYear != 42 {
			t.Fatalf("calendar fields not applied: %+v, %v", got, err)
		}
		months, _ := repo.GetMonths(ctx, cal.ID)
		if len(months) != 1 || months[0].Name != "Imported Month" {
			t.Errorf("months not applied: %+v", months)
		}
		eras, _ := repo.GetEras(ctx, cal.ID)
		if len(eras) != 1 || eras[0].StartMonth != 1 {
			t.Errorf("eras not applied: %+v", eras)
		}
	})

	t.Run("ApplyImport with no eras in the file leaves existing eras untouched", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Import No-Eras Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := repo.CreateEra(ctx, cal.ID, EraInput{Name: "Pre-existing Era", StartYear: 1, Color: "#123123"}); err != nil {
			t.Fatalf("CreateEra: %v", err)
		}
		if err := repo.ApplyImport(ctx, cal, &ImportResult{Months: []MonthInput{{Name: "Renamed month", Days: 30}}}); err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		eras, err := repo.GetEras(ctx, cal.ID)
		if err != nil || len(eras) != 1 || eras[0].Name != "Pre-existing Era" {
			t.Errorf("ApplyImport with an empty Eras list should leave existing eras alone, got %+v, %v", eras, err)
		}
	})

	t.Run("Delete cascades every structural sub-resource", func(t *testing.T) {
		cal := newTestCalendar(testUUID(t), fixA.CampaignID, "Doomed Calendar")
		if err := repo.Create(ctx, cal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		mustExec(t, db, `INSERT INTO calendar_months (calendar_id, name) VALUES (?, ?)`, cal.ID, "M")
		mustExec(t, db, `INSERT INTO calendar_weekdays (calendar_id, name) VALUES (?, ?)`, cal.ID, "D")
		mustExec(t, db, `INSERT INTO calendar_moons (calendar_id, name) VALUES (?, ?)`, cal.ID, "Moon")
		mustExec(t, db, `INSERT INTO calendar_seasons (calendar_id, name, start_month, start_day, end_month, end_day) VALUES (?, ?, 1, 1, 2, 1)`, cal.ID, "S")
		mustExec(t, db, `INSERT INTO calendar_eras (calendar_id, name, start_year) VALUES (?, ?, 1)`, cal.ID, "E")
		mustExec(t, db, `INSERT INTO calendar_cycles (calendar_id, name, cycle_length) VALUES (?, ?, 1)`, cal.ID, "C")
		mustExec(t, db, `INSERT INTO calendar_festivals (calendar_id, name) VALUES (?, ?)`, cal.ID, "F")
		mustExec(t, db, `INSERT INTO calendar_weather (calendar_id) VALUES (?)`, cal.ID)

		if err := repo.Delete(ctx, cal.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		for _, tbl := range []string{"calendar_months", "calendar_weekdays", "calendar_moons",
			"calendar_seasons", "calendar_eras", "calendar_cycles", "calendar_festivals", "calendar_weather"} {
			var n int
			if err := db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM `+tbl+` WHERE calendar_id = ?`, cal.ID).Scan(&n); err != nil {
				t.Fatalf("count %s: %v", tbl, err)
			}
			if n != 0 {
				t.Errorf("%s still has %d row(s) for the deleted calendar: cascade did not fire", tbl, n)
			}
		}
	})
}

func intPtr(v int) *int { return &v }

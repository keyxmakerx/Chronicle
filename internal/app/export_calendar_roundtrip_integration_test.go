// export_calendar_roundtrip_integration_test.go proves the calendar export/
// import adapters round-trip through a REAL calendar.CalendarService backed
// by MariaDB, end to end: create a calendar in one campaign, export it,
// import it into a second (brand new) campaign, and read the second
// campaign's calendar back to compare. Runs against a real database, in
// internal/app (the adapters live here, and building calendar.CalendarService
// is only possible with its concrete repositories), the same convention
// armory_npcs_visibility_leak_test.go established for cross-plugin,
// DB-backed adapter tests in this package.
//
// Skipped under -short. Run with `make test-int-local`, or `make docker-up
// && make migrate-up && go test ./internal/app/... -run CalendarCampaign
// -v`.
package app

import (
	"context"
	"database/sql"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newCalRoundTripCampaign inserts a minimal (user, campaign) fixture,
// registers its teardown, and returns the campaign id. Mirrors
// calendar/dbtest_support_test.go's newTestCampaign — this repo's convention
// is one small copy per package rather than a shared test-only import across
// plugin boundaries (see that file's own doc comment).
func newCalRoundTripCampaign(t *testing.T, db *sql.DB, label string) (campaignID, ownerID string) {
	t.Helper()
	userID := galleryTestUUID(t)
	campaignID = galleryTestUUID(t)
	mustGalleryExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "calv5-roundtrip-"+label+"-"+userID+"@example.test", "CalV5 RoundTrip "+label, "x")
	mustGalleryExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "CalV5 RoundTrip "+label, "calv5-roundtrip-"+label+"-"+campaignID[:8], userID)
	t.Cleanup(func() {
		mustGalleryExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustGalleryExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	})
	return campaignID, userID
}

// newCalRoundTripEntity inserts a minimal entity for entity-tie tests.
func newCalRoundTripEntity(t *testing.T, db *sql.DB, campaignID, userID, name, slug string) string {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO entity_types (campaign_id, slug, name, name_plural) VALUES (?, ?, ?, ?)`,
		campaignID, "calv5-rt-type-"+slug, "CalV5 RT Type", "CalV5 RT Types")
	if err != nil {
		t.Fatalf("insert entity_type: %v", err)
	}
	typeID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("entity_type last insert id: %v", err)
	}
	entityID := galleryTestUUID(t)
	mustGalleryExec(t, db,
		`INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		entityID, campaignID, typeID, name, slug, userID)
	return entityID
}

// TestCalendarCampaignExportImport_DBRoundTrip is the DB-backed regression
// the task calls for: export a campaign's calendar, import it into a NEW
// campaign, and compare — proving the adapters work against real SQL, not
// just the in-memory fakes in export_calendar_roundtrip_test.go.
func TestCalendarCampaignExportImport_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openGalleryTestDB(t)
	defer db.Close()
	ctx := context.Background()

	srcCampaignID, srcOwnerID := newCalRoundTripCampaign(t, db, "src")
	dstCampaignID, dstOwnerID := newCalRoundTripCampaign(t, db, "dst")

	// A real entity in the SOURCE campaign, for the linked public event.
	srcEntityID := newCalRoundTripEntity(t, db, srcCampaignID, srcOwnerID, "The Duke", "the-duke-rt")
	// A real entity in the DESTINATION campaign, standing in for what a real
	// entity import would have created — this test exercises the calendar
	// adapters, not the entity importer, so the ID map is built by hand.
	dstEntityID := newCalRoundTripEntity(t, db, dstCampaignID, dstOwnerID, "The Duke", "the-duke-rt-2")

	calRepo := calendar.NewCalendarRepository(db)
	eventRepo := calendar.NewEventRepository(db)
	kindRepo := calendar.NewEventKindRepository(db)
	weatherRepo := calendar.NewWeatherRepository(db)
	calSvc := calendar.NewCalendarService(calRepo, eventRepo, kindRepo, weatherRepo)

	const ownerRole = 3
	systemViewer := permissions.SystemViewer(ownerRole)

	// --- Build the source calendar ---
	desc := "The round-trip calendar"
	srcCal, err := calSvc.CreateCalendar(ctx, srcCampaignID, calendar.CreateCalendarInput{
		Name: "Harvest Calendar", Description: &desc, CurrentYear: 998,
	})
	if err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if err := calSvc.SetDefaultCalendar(ctx, srcCampaignID, srcCal.ID); err != nil {
		t.Fatalf("set default calendar: %v", err)
	}
	if err := calSvc.SetMonths(ctx, srcCal.ID, srcCampaignID, []calendar.MonthInput{
		{Name: "Thaw", Days: 30, SortOrder: 0},
		{Name: "Bloom", Days: 30, SortOrder: 1, LeapYearDays: 1},
	}); err != nil {
		t.Fatalf("set months: %v", err)
	}
	if err := calSvc.SetWeekdays(ctx, srcCal.ID, srcCampaignID, []calendar.WeekdayInput{
		{Name: "Sunday", SortOrder: 0}, {Name: "Moonday", SortOrder: 1},
	}); err != nil {
		t.Fatalf("set weekdays: %v", err)
	}
	if err := calSvc.SetMoons(ctx, srcCal.ID, srcCampaignID, []calendar.MoonInput{
		{Name: "Secret Moon", CycleDays: 29.5, Color: "#ffffff", HiddenFromPlayers: true},
	}); err != nil {
		t.Fatalf("set moons: %v", err)
	}
	if err := calSvc.SetSeasons(ctx, srcCal.ID, srcCampaignID, []calendar.Season{
		{Name: "Spring", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 30, Color: "#22c55e"},
	}); err != nil {
		t.Fatalf("set seasons: %v", err)
	}
	if _, err := calSvc.CreateEra(ctx, srcCal.ID, srcCampaignID, calendar.EraInput{
		Name: "First Age", StartYear: 0, StartMonth: 1, StartDay: 1, Color: "#eab308",
	}); err != nil {
		t.Fatalf("create era: %v", err)
	}
	kind, err := calSvc.CreateEventKind(ctx, srcCampaignID, calendar.EventKindInput{
		Slug: "festival", Name: "Festival", Icon: "fa-star", Color: "#10b981",
	})
	if err != nil {
		t.Fatalf("create event kind: %v", err)
	}
	if _, err := calSvc.CreateEvent(ctx, srcCal.ID, srcCampaignID, calendar.CreateEventInput{
		Name: "Harvest Festival", EntityID: &srcEntityID, Year: 998, Month: 2, Day: 20,
		Visibility: "everyone", KindID: &kind.ID, CanAuthorDmOnly: true,
	}); err != nil {
		t.Fatalf("create public event: %v", err)
	}
	visRules := `{"denied_users":["player-1"]}`
	if _, err := calSvc.CreateEvent(ctx, srcCal.ID, srcCampaignID, calendar.CreateEventInput{
		Name: "Secret War Council", Year: 998, Month: 3, Day: 1,
		Visibility: "dm_only", VisibilityRules: &visRules, CanAuthorDmOnly: true,
	}); err != nil {
		t.Fatalf("create dm_only event: %v", err)
	}

	// --- Export ---
	exportAdapter := &calendarExportAdapter{svc: calSvc}
	slugLookup := func(id string) string {
		if id == srcEntityID {
			return "the-duke-rt"
		}
		return ""
	}
	data, err := exportAdapter.ExportCalendar(ctx, srcCampaignID, slugLookup)
	if err != nil {
		t.Fatalf("export calendar: %v", err)
	}
	if len(data.Events) != 2 {
		t.Fatalf("exported %d events, want 2", len(data.Events))
	}

	// --- Import into the NEW (destination) campaign ---
	idMap := campaigns.NewIDMap(dstCampaignID)
	idMap.EntitySlugToID["the-duke-rt"] = dstEntityID
	report := campaigns.NewImportReport()
	importAdapter := &calendarImportAdapter{svc: calSvc}
	if err := importAdapter.ImportCalendar(ctx, dstCampaignID, data, idMap, report); err != nil {
		t.Fatalf("import calendar: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("clean calendar import reported failures: %s", report.Summary())
	}

	// --- Read the destination calendar back and compare ---
	dstCal, err := calSvc.GetDefaultCalendarForViewer(ctx, dstCampaignID, systemViewer)
	if err != nil {
		t.Fatalf("get imported default calendar: %v", err)
	}
	if dstCal.Name != "Harvest Calendar" {
		t.Errorf("imported calendar name = %q, want %q", dstCal.Name, "Harvest Calendar")
	}
	if len(dstCal.Months) != 2 || len(dstCal.Weekdays) != 2 {
		t.Errorf("imported calendar lost months/weekdays: months=%d weekdays=%d", len(dstCal.Months), len(dstCal.Weekdays))
	}
	if len(dstCal.Moons) != 1 || !dstCal.Moons[0].HiddenFromPlayers {
		t.Errorf("imported moon lost its HiddenFromPlayers flag: %+v", dstCal.Moons)
	}
	if len(dstCal.Seasons) != 1 {
		t.Errorf("imported calendar lost its season: %+v", dstCal.Seasons)
	}
	if len(dstCal.Eras) != 1 || dstCal.Eras[0].StartMonth != 1 || dstCal.Eras[0].StartDay != 1 {
		t.Errorf("imported era lost its day-granular start: %+v", dstCal.Eras)
	}
	if len(dstCal.EventKinds) != 1 || dstCal.EventKinds[0].Slug != "festival" {
		t.Errorf("imported calendar lost its event kind: %+v", dstCal.EventKinds)
	}

	dstEvents, err := calSvc.ListAllEventsForCalendar(ctx, dstCal.ID, dstCampaignID, systemViewer)
	if err != nil {
		t.Fatalf("list imported events: %v", err)
	}
	if len(dstEvents) != 2 {
		t.Fatalf("imported %d events, want 2", len(dstEvents))
	}
	var dmEvent, pubEvent *calendar.Event
	for i := range dstEvents {
		e := &dstEvents[i]
		switch e.Name {
		case "Secret War Council":
			dmEvent = e
		case "Harvest Festival":
			pubEvent = e
		}
	}
	if dmEvent == nil {
		t.Fatal("dm_only event did not survive the round trip")
	}
	if dmEvent.Visibility != "dm_only" {
		t.Errorf("imported dm_only event visibility = %q, want dm_only", dmEvent.Visibility)
	}
	if dmEvent.VisibilityRules == nil || *dmEvent.VisibilityRules != visRules {
		t.Error("imported dm_only event lost its per-user visibility_rules")
	}
	if pubEvent == nil {
		t.Fatal("public event did not survive the round trip")
	}
	if pubEvent.EntityID == nil || *pubEvent.EntityID != dstEntityID {
		t.Errorf("imported event entity link = %v, want %q (the destination campaign's own entity)", pubEvent.EntityID, dstEntityID)
	}
	if pubEvent.KindID == nil {
		t.Error("imported event lost its event-kind link")
	}

	// A Player viewer (not system, not co-DM) must never see the dm_only
	// event that just round-tripped — proves the import didn't accidentally
	// widen visibility on write.
	playerViewer := permissions.RequestViewer(permissions.RolePlayer, "player-2")
	playerEvents, err := calSvc.ListEventsForMonth(ctx, dstCal.ID, dstCampaignID, 998, 3, playerViewer)
	if err != nil {
		t.Fatalf("list events for month (player): %v", err)
	}
	for _, e := range playerEvents {
		if e.Name == "Secret War Council" {
			t.Error("LEAK: a Player can see the imported dm_only event")
		}
	}
}


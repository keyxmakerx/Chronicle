// service_integration_test.go: DB-backed tests for the parts of
// CalendarService whose correctness depends on real SQL — cross-campaign
// scoping enforced by foreign keys, duplicate-key conflicts, and the
// SQL role-filter (ListEventsForMonth's dm_only clause) composing correctly
// with the service's Go-side per-user visibility filter. Uses the same
// openTestDB/newTestCampaign harness as every other *_integration_test.go
// in this package (dbtest_support_test.go); skips when no test DB answers.
package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestCalendarService_Integration_CrossCampaignEraRejected(t *testing.T) {
	db := openTestDB(t)
	fixtureA := newTestCampaign(t, db, "era-a")
	fixtureB := newTestCampaign(t, db, "era-b")

	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	// A real calendar in campaign B.
	calB := &Calendar{ID: testUUID(t), CampaignID: fixtureB.CampaignID, Mode: ModeFantasy, Name: "Calendar B",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, calB); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	era, err := calRepo.CreateEra(ctx, calB.ID, EraInput{Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1})
	if err != nil {
		t.Fatalf("seed era: %v", err)
	}

	// Reached through campaign A's URL (:id=A, :calid=calB.ID): must be
	// NotFound, never a raw FK/driver error and never a silent success.
	err = svc.UpdateEra(ctx, era.ID, calB.ID, fixtureA.CampaignID, EraInput{Name: "Renamed", StartYear: 1, StartMonth: 1, StartDay: 1})
	assertIntegrationNotFound(t, err)

	err = svc.DeleteEra(ctx, era.ID, calB.ID, fixtureA.CampaignID)
	assertIntegrationNotFound(t, err)

	// Control: the SAME operation through the correct campaign succeeds.
	if err := svc.UpdateEra(ctx, era.ID, calB.ID, fixtureB.CampaignID, EraInput{Name: "Renamed", StartYear: 1, StartMonth: 1, StartDay: 1}); err != nil {
		t.Errorf("update through the correct campaign must succeed: %v", err)
	}
}

func TestCalendarService_Integration_CreateEventRejectsCrossCampaignEntity(t *testing.T) {
	db := openTestDB(t)
	fixtureA := newTestCampaign(t, db, "evt-a")
	fixtureB := newTestCampaign(t, db, "evt-b")
	foreignEntityID := newTestEntity(t, db, fixtureB.CampaignID, fixtureB.UserID, "Someone Else's NPC")

	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	calA := &Calendar{ID: testUUID(t), CampaignID: fixtureA.CampaignID, Mode: ModeFantasy, Name: "Calendar A",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, calA); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}

	_, err := svc.CreateEvent(ctx, calA.ID, fixtureA.CampaignID, CreateEventInput{
		Name: "Cross-campaign link attempt", Year: 1, Month: 1, Day: 1,
		EntityID: &foreignEntityID, CreatedBy: fixtureA.UserID,
	})
	if err == nil {
		t.Fatal("expected a validation error linking an entity from a different campaign")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an *apperror.AppError (never a raw DB error, rule 6), got %T: %v", err, err)
	}
	if appErr.Code == 500 {
		t.Errorf("cross-campaign entity link must not surface as a raw internal error, got: %v", appErr)
	}
}

func TestCalendarService_Integration_EventKindDuplicateSlugIsConflict(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "kind-dup")
	kindRepo := NewEventKindRepository(db)
	svc := NewCalendarService(NewCalendarRepository(db), NewEventRepository(db), kindRepo, NewWeatherRepository(db))
	ctx := context.Background()

	input := EventKindInput{Slug: "harvest-fair", Name: "Harvest Fair", Icon: "fa-wheat-awn", Color: "#84cc16", DefaultAnnounced: AnnouncedAhead}
	if _, err := svc.CreateEventKind(ctx, fixture.CampaignID, input); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.CreateEventKind(ctx, fixture.CampaignID, input)
	if err == nil {
		t.Fatal("expected a conflict on a duplicate slug within the same campaign")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != 409 {
		t.Fatalf("expected apperror 409 conflict (never a raw MariaDB duplicate-entry error, rule 6), got %T: %v", err, err)
	}
}

// TestCalendarService_Integration_ListEventsForMonth_SQLRoleFilterPlusGoVisibility
// exercises the real dm_only SQL filter (ListEventsForMonth) composing with
// the service's Go-side visibility_rules filter, against a real database —
// the unit test in service_test.go pins the same behavior against a fake
// that only APPROXIMATES the SQL filter's contract.
func TestCalendarService_Integration_ListEventsForMonth_SQLRoleFilterPlusGoVisibility(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "evt-vis")
	calRepo := NewCalendarRepository(db)
	eventRepo := NewEventRepository(db)
	svc := NewCalendarService(calRepo, eventRepo, NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	cal := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: "Vis Calendar",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}

	mustCreate := func(name, visibility string, rules *string) {
		t.Helper()
		if err := eventRepo.CreateEvent(ctx, &Event{
			ID: testUUID(t), CalendarID: cal.ID, Name: name, Year: 100, Month: 1, Day: 1,
			Visibility: visibility, VisibilityRules: rules, CreatedBy: &fixture.UserID,
		}); err != nil {
			t.Fatalf("seed event %q: %v", name, err)
		}
	}
	deniedRules := `{"denied_users":["u-blocked"]}`
	mustCreate("Open Feast", "everyone", nil)
	mustCreate("Restricted Feast", "everyone", &deniedRules)
	mustCreate("Secret War Council", "dm_only", nil)

	names := func(viewer permissions.Viewer) map[string]bool {
		events, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 100, 1, viewer)
		if err != nil {
			t.Fatalf("ListEventsForMonth: %v", err)
		}
		out := map[string]bool{}
		for _, e := range events {
			out[e.Name] = true
		}
		return out
	}

	blocked := names(playerViewer("u-blocked"))
	if blocked["Secret War Council"] {
		t.Error("a player must never see a dm_only event, even seeded directly in the DB")
	}
	if blocked["Restricted Feast"] {
		t.Error("the denied user must not see the event that names them, even with base visibility 'everyone'")
	}
	if !blocked["Open Feast"] {
		t.Error("the open event must still be visible")
	}

	other := names(playerViewer("u-other"))
	if !other["Restricted Feast"] {
		t.Error("a DIFFERENT player (not named in the deny list) must see the restricted event")
	}

	owner := names(ownerViewer("u-owner"))
	if !owner["Secret War Council"] || !owner["Restricted Feast"] || !owner["Open Feast"] {
		t.Errorf("owner must see every event including dm_only and deny-listed ones, got %v", owner)
	}
}

// assertIntegrationNotFound is the integration-test twin of service_test.go's
// assertNotFound (kept separate: this file has its own focused error
// helpers so it can run standalone if the two files are ever split).
func assertIntegrationNotFound(t *testing.T, err error) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != 404 {
		t.Fatalf("expected 404 not_found, got code=%d type=%s message=%q", appErr.Code, appErr.Type, appErr.Message)
	}
}

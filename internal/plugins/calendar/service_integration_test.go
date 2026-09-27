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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
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

	renamedInput := UpdateEraInput{Name: "Renamed", StartYear: patch.Of(1), StartMonth: patch.Of(1), StartDay: patch.Of(1)}

	// Reached through campaign A's URL (:id=A, :calid=calB.ID): must be
	// NotFound, never a raw FK/driver error and never a silent success.
	err = svc.UpdateEra(ctx, era.ID, calB.ID, fixtureA.CampaignID, renamedInput)
	assertIntegrationNotFound(t, err)

	err = svc.DeleteEra(ctx, era.ID, calB.ID, fixtureA.CampaignID)
	assertIntegrationNotFound(t, err)

	// Control: the SAME operation through the correct campaign succeeds.
	if err := svc.UpdateEra(ctx, era.ID, calB.ID, fixtureB.CampaignID, renamedInput); err != nil {
		t.Errorf("update through the correct campaign must succeed: %v", err)
	}
}

// TestCalendarService_Integration_CrossCalendarEraRejected (N6) is the
// SAME-campaign twin of the cross-campaign test above: an era genuinely
// belonging to calendar A cannot be edited through a URL naming calendar B,
// even when both calendars are in the SAME campaign — proving the check is
// "does this era belong to THIS calendar", not just "is the campaign right".
func TestCalendarService_Integration_CrossCalendarEraRejected(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "era-xcal")
	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	mk := func(name string) *Calendar {
		c := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: name,
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
		if err := calRepo.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	calA, calB := mk("Calendar A"), mk("Calendar B")
	era, err := calRepo.CreateEra(ctx, calA.ID, EraInput{Name: "First Age", StartYear: 1, StartMonth: 1, StartDay: 1})
	if err != nil {
		t.Fatalf("seed era: %v", err)
	}

	// Same campaign, wrong calendar: must be NotFound.
	err = svc.UpdateEra(ctx, era.ID, calB.ID, fixture.CampaignID, UpdateEraInput{Name: "Renamed"})
	assertIntegrationNotFound(t, err)
	err = svc.DeleteEra(ctx, era.ID, calB.ID, fixture.CampaignID)
	assertIntegrationNotFound(t, err)

	// Control: the correct calendar succeeds.
	if err := svc.UpdateEra(ctx, era.ID, calA.ID, fixture.CampaignID, UpdateEraInput{Name: "Renamed"}); err != nil {
		t.Errorf("update through the correct calendar must succeed: %v", err)
	}
}

// TestCalendarService_Integration_SetMoonHidden_SiblingCalendar (N6) is
// SetMoonHidden's twin: a moon genuinely belonging to a sibling calendar in
// the SAME campaign cannot be toggled through the wrong calendar's id.
func TestCalendarService_Integration_SetMoonHidden_SiblingCalendar(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "moon-xcal")
	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	mk := func(name string) *Calendar {
		c := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: name,
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
		if err := calRepo.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	calA, calB := mk("Calendar A"), mk("Calendar B")
	if err := calRepo.SetMoons(ctx, calB.ID, []MoonInput{{Name: "Sibling's Moon", CycleDays: 10, Color: "#ffffff"}}); err != nil {
		t.Fatal(err)
	}
	moons, err := calRepo.GetMoons(ctx, calB.ID)
	if err != nil || len(moons) != 1 {
		t.Fatalf("seed moon: moons=%v err=%v", moons, err)
	}

	err = svc.SetMoonHidden(ctx, moons[0].ID, calA.ID, fixture.CampaignID, true)
	assertIntegrationNotFound(t, err)

	if err := svc.SetMoonHidden(ctx, moons[0].ID, calB.ID, fixture.CampaignID, true); err != nil {
		t.Errorf("toggling through the correct calendar must succeed: %v", err)
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
		t.Fatalf("expected an *apperror.AppError (never a raw DB error), got %T: %v", err, err)
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
		t.Fatalf("expected apperror 409 conflict (never a raw MariaDB duplicate-entry error), got %T: %v", err, err)
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

// testEntityGate is a minimal, test-only EntityVisibilityGate: it queries
// entities.is_private directly rather than depending on the entities
// plugin's own service (a cross-plugin import this package's isolation rule
// forbids even from a test) — CanSeeDmOnly(role) sees everything, the same
// short-circuit the real gate's Owner/co-DM path takes.
type testEntityGate struct{ db *sql.DB }

func (g testEntityGate) FilterViewableEntityIDs(ctx context.Context, _ string, entityIDs []string, role int, _ string) (map[string]bool, error) {
	out := make(map[string]bool, len(entityIDs))
	if permissions.CanSeeDmOnly(role) {
		for _, id := range entityIDs {
			out[id] = true
		}
		return out, nil
	}
	for _, id := range entityIDs {
		var isPrivate bool
		if err := g.db.QueryRowContext(ctx, `SELECT is_private FROM entities WHERE id = ?`, id).Scan(&isPrivate); err != nil {
			return nil, err
		}
		out[id] = !isPrivate
	}
	return out, nil
}

// TestCalendarService_Integration_PrivateEntityNameRedacted (S2) proves a
// player/anonymous viewer never learns a private entity's name/id through an
// event's linked-entity join, which reads straight from entities with no
// visibility filter of its own (redactHiddenEntityLinks's doc comment).
func TestCalendarService_Integration_PrivateEntityNameRedacted(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "entity-redact")
	entID := newTestEntity(t, db, fixture.CampaignID, fixture.UserID, "The Hidden Cult")
	mustExec(t, db, `UPDATE entities SET is_private = 1 WHERE id = ?`, entID)

	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	cs, ok := svc.(*calendarService)
	if !ok {
		t.Fatalf("expected *calendarService, got %T", svc)
	}
	cs.SetEntityVisibilityGate(testEntityGate{db: db})
	ctx := context.Background()

	cal := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: "Redact Calendar",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	evt, err := svc.CreateEvent(ctx, cal.ID, fixture.CampaignID, CreateEventInput{
		Name: "Festival of Lights", Year: 5, Month: 2, Day: 3, EntityID: &entID, CreatedBy: fixture.UserID,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	got, err := svc.GetEventForViewer(ctx, evt.ID, cal.ID, fixture.CampaignID, playerViewer("u-some-player"))
	if err != nil {
		t.Fatalf("GetEventForViewer (player): %v", err)
	}
	if got.EntityID != nil || got.EntityName != "" {
		t.Errorf("player must not see the private entity's id/name, got entity_id=%v entity_name=%q", derefString(got.EntityID), got.EntityName)
	}

	list, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 5, 2, publicViewer())
	if err != nil {
		t.Fatalf("ListEventsForMonth (anonymous): %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 event, got %d", len(list))
	}
	if list[0].EntityID != nil || list[0].EntityName != "" {
		t.Errorf("anonymous visitor must not see the private entity's id/name, got entity_id=%v entity_name=%q", derefString(list[0].EntityID), list[0].EntityName)
	}

	ownerGot, err := svc.GetEventForViewer(ctx, evt.ID, cal.ID, fixture.CampaignID, ownerViewer(fixture.UserID))
	if err != nil {
		t.Fatalf("GetEventForViewer (owner): %v", err)
	}
	if ownerGot.EntityName != "The Hidden Cult" {
		t.Errorf("owner must still see the private entity's name, got %q", ownerGot.EntityName)
	}
}

// TestCalendarService_Integration_EntityGateNotWired_FailsClosed (S2) proves
// redactHiddenEntityLinks fails CLOSED when the gate is nil (a construction
// bug), blanking a linked entity that is not even private — never leaking a
// name because nothing was configured to check it.
func TestCalendarService_Integration_EntityGateNotWired_FailsClosed(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "entity-nogate")
	entID := newTestEntity(t, db, fixture.CampaignID, fixture.UserID, "Visible NPC")

	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	// No SetEntityVisibilityGate call: s.entityGate stays nil.
	ctx := context.Background()

	cal := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: "No Gate Calendar",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	evt, err := svc.CreateEvent(ctx, cal.ID, fixture.CampaignID, CreateEventInput{
		Name: "Public Fair", Year: 5, Month: 2, Day: 3, EntityID: &entID, CreatedBy: fixture.UserID,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	got, err := svc.GetEventForViewer(ctx, evt.ID, cal.ID, fixture.CampaignID, playerViewer("u-some-player"))
	if err != nil {
		t.Fatalf("GetEventForViewer (player): %v", err)
	}
	if got.EntityID != nil || got.EntityName != "" {
		t.Errorf("an unwired gate must fail CLOSED, even for a not-actually-private entity, got entity_id=%v entity_name=%q",
			derefString(got.EntityID), got.EntityName)
	}

	ownerGot, err := svc.GetEventForViewer(ctx, evt.ID, cal.ID, fixture.CampaignID, ownerViewer(fixture.UserID))
	if err != nil {
		t.Fatalf("GetEventForViewer (owner): %v", err)
	}
	if ownerGot.EntityName != "Visible NPC" {
		t.Errorf("owner must still see the entity name regardless of the gate, got %q", ownerGot.EntityName)
	}
}

// TestCalendarService_Integration_TextColumnLengths (N2) proves every text
// column the service does not already cover in validation_test.go's
// fake-backed table is bounded before it reaches the driver: event kind
// slug/name, real_time_zone (length AND time.LoadLocation validity), and an
// event description at the TEXT column's byte capacity.
func TestCalendarService_Integration_TextColumnLengths(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "col-len")
	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	cal := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: "Width Calendar",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}

	t.Run("event kind slug over VARCHAR(50)", func(t *testing.T) {
		_, err := svc.CreateEventKind(ctx, fixture.CampaignID, EventKindInput{
			Slug: strings.Repeat("a", 51), Name: "K", Icon: "fa-star", Color: "#fff",
		})
		assertCleanClientError(t, err)
	})
	t.Run("event kind name over VARCHAR(100)", func(t *testing.T) {
		_, err := svc.CreateEventKind(ctx, fixture.CampaignID, EventKindInput{
			Slug: "k-long-name", Name: strings.Repeat("n", 150), Icon: "fa-star", Color: "#fff",
		})
		assertCleanClientError(t, err)
	})
	t.Run("real_time_zone over VARCHAR(64)", func(t *testing.T) {
		on, tz := true, strings.Repeat("z", 70)
		err := svc.UpdateCalendar(ctx, cal.ID, fixture.CampaignID, UpdateCalendarInput{
			Name: "Width Calendar", SetRealTime: &on, RealTimeZone: &tz,
		})
		assertCleanClientError(t, err)
	})
	t.Run("real_time_zone not a valid IANA zone", func(t *testing.T) {
		on, tz := true, "Not/AZone"
		err := svc.UpdateCalendar(ctx, cal.ID, fixture.CampaignID, UpdateCalendarInput{
			Name: "Width Calendar", SetRealTime: &on, RealTimeZone: &tz,
		})
		assertCleanClientError(t, err)
	})
	t.Run("event description over the TEXT column's byte capacity", func(t *testing.T) {
		bigDesc := strings.Repeat("d", 70000)
		_, err := svc.CreateEvent(ctx, cal.ID, fixture.CampaignID, CreateEventInput{
			Name: "Big", Year: 1, Month: 1, Day: 1, Description: &bigDesc, CreatedBy: fixture.UserID,
		})
		assertCleanClientError(t, err)
	})
}

// assertCleanClientError fails unless err is a non-500 *apperror.AppError —
// an over-long or malformed value must be rejected before it reaches the
// driver, never surfacing as a raw "data too long" 500.
func assertCleanClientError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a client-error rejection, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an *apperror.AppError (never a raw DB error), got %T: %v", err, err)
	}
	if appErr.Code == 500 {
		t.Errorf("must not surface as a raw internal error, got: %v", appErr)
	}
}

// TestCalendarService_Integration_ListEventsForMonth_RecurringEventAcrossMonths
// (N4) proves a recurring event is narrowed to the months it actually
// OCCURS on: the repository's recurringCandidateClause widens every
// recurring row in as a candidate for ANY month query (event_repository.go),
// so without the service's OccursOn-based narrowing (filterRecurringToMonth)
// this would show up in every month of every later year, not just the one
// month it actually recurs into.
func TestCalendarService_Integration_ListEventsForMonth_RecurringEventAcrossMonths(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "recur-month")
	calRepo := NewCalendarRepository(db)
	eventRepo := NewEventRepository(db)
	svc := NewCalendarService(calRepo, eventRepo, NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	cal := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: "Recurrence Calendar",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	months := make([]MonthInput, 12)
	for i := range months {
		months[i] = MonthInput{Name: fmt.Sprintf("Month%d", i+1), Days: 30, SortOrder: i}
	}
	if err := calRepo.SetMonths(ctx, cal.ID, months); err != nil {
		t.Fatalf("seed months: %v", err)
	}
	weekdays := make([]WeekdayInput, 7)
	for i := range weekdays {
		weekdays[i] = WeekdayInput{Name: fmt.Sprintf("Day%d", i+1), SortOrder: i}
	}
	if err := calRepo.SetWeekdays(ctx, cal.ID, weekdays); err != nil {
		t.Fatalf("seed weekdays: %v", err)
	}

	recType := RecurrenceYearly
	if err := eventRepo.CreateEvent(ctx, &Event{
		ID: testUUID(t), CalendarID: cal.ID, Name: "Founding Day", Year: 1, Month: 3, Day: 15,
		Visibility: "everyone", IsRecurring: true, RecurrenceType: &recType, CreatedBy: &fixture.UserID,
	}); err != nil {
		t.Fatalf("seed recurring event: %v", err)
	}

	viewer := ownerViewer(fixture.UserID)
	sameMonthLaterYear, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 5, 3, viewer)
	if err != nil {
		t.Fatalf("ListEventsForMonth (same month, later year): %v", err)
	}
	if len(sameMonthLaterYear) != 1 || sameMonthLaterYear[0].Name != "Founding Day" {
		t.Errorf("expected the yearly recurrence to appear in year 5 month 3, got %v", sameMonthLaterYear)
	}

	differentMonth, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 5, 4, viewer)
	if err != nil {
		t.Fatalf("ListEventsForMonth (different month, later year): %v", err)
	}
	for _, e := range differentMonth {
		if e.Name == "Founding Day" {
			t.Errorf("a yearly recurrence anchored at month 3 must not appear in month 4, even though the repository widens it in as a raw candidate for every month")
		}
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

// TestCalendarService_Integration_CreateCalendarFromImport_EveryPreset (Part
// B) proves the actual missing link this PR wires up: every shipped preset
// creates a real, fully-structured calendar through CreateCalendarFromImport
// — not just a parse (presets_test.go already covers that half).
func TestCalendarService_Integration_CreateCalendarFromImport_EveryPreset(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "import-presets")
	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	names, err := PresetNames()
	if err != nil {
		t.Fatalf("PresetNames: %v", err)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			ir, err := svc.PreviewPreset(ctx, name)
			if err != nil {
				t.Fatalf("PreviewPreset(%q): %v", name, err)
			}
			// Three presets are Chronicle-native and fully specify their own
			// current date (see presets/*.json). "elven" ships in Calendaria's
			// own format (deliberately, per presets.go's doc comment — a
			// preset IS an export, so it exercises a real importer), and
			// Calendaria never carries a day-level date, only a year — so it
			// needs the same explicit override an uploaded Calendaria file
			// would, per the "never silently reset the current date" rule.
			opts := CreateCalendarFromImportOptions{}
			if ir.Today.Month == nil {
				opts.CurrentMonth = intPtrForTest(1)
			}
			if ir.Today.Day == nil {
				opts.CurrentDay = intPtrForTest(1)
			}
			cal, err := svc.CreateCalendarFromImport(ctx, fixture.CampaignID, ir, opts)
			if err != nil {
				t.Fatalf("CreateCalendarFromImport(%q): %v", name, err)
			}
			if cal.ID == "" || cal.CampaignID != fixture.CampaignID {
				t.Fatalf("created calendar looks wrong: %+v", cal)
			}
			// Structure actually landed in the DB, not just in memory: read
			// it back the way GetCalendarForViewer would.
			months, err := calRepo.GetMonths(ctx, cal.ID)
			if err != nil {
				t.Fatalf("GetMonths: %v", err)
			}
			if len(months) != len(ir.Months) {
				t.Errorf("got %d months in the DB, want %d (from the preset)", len(months), len(ir.Months))
			}
			got, err := calRepo.GetByID(ctx, cal.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got.CurrentYear != ir.Today.Year {
				t.Errorf("current_year = %d, want %d (from the preset)", got.CurrentYear, ir.Today.Year)
			}
		})
	}
}

// TestCalendarService_Integration_CreateCalendarFromImport_UploadedCalendaria
// (Part B) exercises the upload path end to end: raw bytes through
// PreviewImport, then CreateCalendarFromImport — Calendaria's missing
// day-level "today" must be rejected without an explicit override and
// accepted with one, matching the unit-level invariant test in
// service_import_test.go but against a real DB write.
func TestCalendarService_Integration_CreateCalendarFromImport_UploadedCalendaria(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "import-upload")
	calRepo := NewCalendarRepository(db)
	svc := NewCalendarService(calRepo, NewEventRepository(db), NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	raw := calendariaSeasonFixture([]int{30, 31}, []fixtureSeason{
		{Name: "Only Season", DayStart: 1, DayEnd: 30},
	})

	ir, err := svc.PreviewImport(ctx, raw)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if ir.Today.Month != nil || ir.Today.Day != nil {
		t.Fatalf("expected Calendaria's Today to have no day-level date, got %+v", ir.Today)
	}

	if _, err := svc.CreateCalendarFromImport(ctx, fixture.CampaignID, ir, CreateCalendarFromImportOptions{}); err == nil {
		t.Error("expected CreateCalendarFromImport to require an explicit current-date override for a Calendaria upload, got nil error")
	}

	cal, err := svc.CreateCalendarFromImport(ctx, fixture.CampaignID, ir, CreateCalendarFromImportOptions{
		CurrentMonth: intPtrForTest(1), CurrentDay: intPtrForTest(1),
	})
	if err != nil {
		t.Fatalf("CreateCalendarFromImport with an explicit override: %v", err)
	}
	got, err := calRepo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.CurrentMonth != 1 || got.CurrentDay != 1 {
		t.Errorf("current date = %d-%d, want the explicit 1-1 override", got.CurrentMonth, got.CurrentDay)
	}
}

// TestCalendarService_Integration_CreateCalendarFromImport_ChronicleEventsRoundTrip
// (#779, Part B) is the full-stack version of
// TestChronicleExportImport_EventsRoundTrip: a calendar with real events,
// exported, then re-imported into a fresh calendar via
// CreateCalendarFromImport, ends up with the same events in the database —
// resolved against the TARGET campaign's own event kind, not the source's.
func TestCalendarService_Integration_CreateCalendarFromImport_ChronicleEventsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "import-events")
	calRepo := NewCalendarRepository(db)
	eventRepo := NewEventRepository(db)
	kindRepo := NewEventKindRepository(db)
	svc := NewCalendarService(calRepo, eventRepo, kindRepo, NewWeatherRepository(db))
	ctx := context.Background()

	source := &Calendar{ID: testUUID(t), CampaignID: fixture.CampaignID, Mode: ModeFantasy, Name: "Source Calendar",
		CurrentYear: 10, CurrentMonth: 1, CurrentDay: 5,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, source); err != nil {
		t.Fatalf("seed source calendar: %v", err)
	}
	if err := calRepo.SetMonths(ctx, source.ID, []MonthInput{{Name: "Firstmonth", Days: 30, SortOrder: 0}}); err != nil {
		t.Fatalf("seed months: %v", err)
	}
	kind, err := kindRepo.Create(ctx, fixture.CampaignID, EventKindInput{Slug: "festival", Name: "Festival", Icon: "fa-star", Color: "#ff0000", DefaultAnnounced: AnnouncedAhead})
	if err != nil {
		t.Fatalf("seed event kind: %v", err)
	}
	if err := eventRepo.CreateEvent(ctx, &Event{
		ID: testUUID(t), CalendarID: source.ID, Name: "Founding Day", Year: 1, Month: 1, Day: 1,
		Visibility: "everyone", KindID: &kind.ID, CreatedBy: &fixture.UserID,
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	loaded, err := calRepo.GetByID(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	loaded.Months, err = calRepo.GetMonths(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetMonths: %v", err)
	}
	events, err := eventRepo.ListAllEvents(ctx, source.ID)
	if err != nil {
		t.Fatalf("ListAllEvents: %v", err)
	}
	for i := range events {
		events[i].KindSlug = kind.Slug
	}

	export := BuildExport(loaded, events, true)
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}

	ir, err := svc.PreviewImport(ctx, raw)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	target, err := svc.CreateCalendarFromImport(ctx, fixture.CampaignID, ir, CreateCalendarFromImportOptions{})
	if err != nil {
		t.Fatalf("CreateCalendarFromImport: %v", err)
	}

	targetEvents, err := eventRepo.ListAllEvents(ctx, target.ID)
	if err != nil {
		t.Fatalf("ListAllEvents on the re-imported calendar: %v", err)
	}
	if len(targetEvents) != 1 {
		t.Fatalf("got %d events on the re-imported calendar, want 1", len(targetEvents))
	}
	got := targetEvents[0]
	if got.Name != "Founding Day" || got.Year != 1 || got.Month != 1 || got.Day != 1 {
		t.Errorf("re-imported event = %+v, want the Founding Day fields preserved", got)
	}
	if got.KindID == nil || *got.KindID != kind.ID {
		t.Errorf("re-imported event KindID = %v, want %d (resolved by slug %q in the target campaign)", got.KindID, kind.ID, kind.Slug)
	}
	if got.Visibility != "dm_only" {
		t.Errorf("re-imported event Visibility = %q, want dm_only (imported events fail closed regardless of the source's visibility, which the export format doesn't carry)", got.Visibility)
	}
	// The kind slug resolved cleanly (no per-event warning for that), but the
	// fail-closed dm_only default for imported events still gets its one
	// summary warning, regardless of how cleanly everything else resolved.
	if len(ir.Warnings) != 1 || !strings.Contains(ir.Warnings[0], "Director-only") {
		t.Errorf("expected exactly one Director-only-visibility warning, got %v", ir.Warnings)
	}
}

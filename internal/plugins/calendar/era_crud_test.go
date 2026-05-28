// era_crud_test.go covers V2 Wave 0 PR 2's per-era CRUD service surface
// + the transactional ApplyImport service-level routing. The DB-level
// rollback semantics of repo.ApplyImport are covered by integration
// tests at the repo layer (require MariaDB); this file covers the
// service-layer validation + dispatch.

package calendar

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// newSvcWithBus constructs a calendarService + attaches a mockWSPublisher
// so tests can assert structure.updated publish via the actual
// CalendarEventPublisher hook the service uses.
func newSvcWithBus(repo CalendarRepository) (CalendarService, *mockWSPublisher) {
	svc := NewCalendarService(repo)
	bus := &mockWSPublisher{}
	svc.(*calendarService).events = bus
	return svc, bus
}

// stubGetByIDForCalendar provides a default getByIDFn so the service's
// publishStructureUpdated path (which calls GetByID to resolve the
// owning campaign) succeeds in WS-publish assertions.
func stubGetByIDForCalendar(calendarID, campaignID string) func(ctx context.Context, id string) (*Calendar, error) {
	return func(ctx context.Context, id string) (*Calendar, error) {
		if id == calendarID {
			return &Calendar{ID: id, CampaignID: campaignID, Mode: ModeFantasy}, nil
		}
		return nil, nil
	}
}

// TestCreateEra_HappyPath — valid input → repo.CreateEra called + WS
// structure.updated event published.
func TestCreateEra_HappyPath(t *testing.T) {
	var capturedInput EraInput
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-1", "camp-1"),
		createEraFn: func(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
			capturedInput = input
			return &Era{ID: 7, CalendarID: calendarID, Name: input.Name, StartYear: input.StartYear,
				EndYear: input.EndYear, Color: input.Color, SortOrder: input.SortOrder}, nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	era, err := svc.CreateEra(context.Background(), "cal-1", EraInput{
		Name: "Age of Magic", StartYear: 1000, Color: "#abcdef",
	})
	if err != nil {
		t.Fatalf("CreateEra: %v", err)
	}
	if era.ID != 7 || era.Name != "Age of Magic" {
		t.Errorf("returned era = %+v; want ID=7 Name=Age of Magic", era)
	}
	if capturedInput.Color != "#abcdef" {
		t.Errorf("repo received Color = %q; want %q", capturedInput.Color, "#abcdef")
	}
	if !bus.calendarStructurePublished("cal-1") {
		t.Error("expected structure.updated WS event for cal-1")
	}
}

// TestCreateEra_EmptyColorDefaults — empty Color in input gets the
// platform default #6366f1 (mirrors SetEras's per-row default-fill).
func TestCreateEra_EmptyColorDefaults(t *testing.T) {
	var capturedInput EraInput
	repo := &mockCalendarRepo{
		createEraFn: func(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
			capturedInput = input
			return &Era{ID: 1, CalendarID: calendarID, Name: input.Name}, nil
		},
	}
	svc, _ := newSvcWithBus(repo)

	if _, err := svc.CreateEra(context.Background(), "cal-1", EraInput{
		Name: "Default Color", StartYear: 1,
	}); err != nil {
		t.Fatalf("CreateEra: %v", err)
	}
	if capturedInput.Color != "#6366f1" {
		t.Errorf("default color = %q; want %q", capturedInput.Color, "#6366f1")
	}
}

// TestCreateEra_ValidationErrors — name required; end < start rejected.
func TestCreateEra_ValidationErrors(t *testing.T) {
	repo := &mockCalendarRepo{
		createEraFn: func(ctx context.Context, calendarID string, input EraInput) (*Era, error) {
			t.Error("repo.CreateEra called despite validation failure")
			return nil, nil
		},
	}
	svc, _ := newSvcWithBus(repo)

	// Missing name.
	if _, err := svc.CreateEra(context.Background(), "cal-1", EraInput{StartYear: 1}); err == nil {
		t.Error("expected validation error for empty name; got nil")
	}

	// EndYear before StartYear.
	endBefore := 500
	if _, err := svc.CreateEra(context.Background(), "cal-1", EraInput{
		Name: "Backwards", StartYear: 1000, EndYear: &endBefore,
	}); err == nil {
		t.Error("expected validation error for EndYear < StartYear; got nil")
	}
}

// TestUpdateEra_NotFound — repo returns nil from GetEraByID → service
// returns apperror.NotFound; repo.UpdateEra NEVER called.
func TestUpdateEra_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{
		getEraByIDFn: func(ctx context.Context, eraID int) (*Era, error) {
			return nil, nil
		},
		updateEraFn: func(ctx context.Context, eraID int, input EraInput) error {
			t.Error("repo.UpdateEra called despite era not found")
			return nil
		},
	}
	svc, _ := newSvcWithBus(repo)

	err := svc.UpdateEra(context.Background(), 99, EraInput{Name: "X", StartYear: 1, Color: "#000000"})
	if err == nil {
		t.Fatal("expected NotFound error; got nil")
	}
	if !isNotFoundErr(err) {
		t.Errorf("error = %v; want NotFound", err)
	}
}

// TestUpdateEra_HappyPath — existing era → validation passes → repo
// UpdateEra called + WS publishes for the era's owning calendar.
func TestUpdateEra_HappyPath(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-7", "camp-7"),
		getEraByIDFn: func(ctx context.Context, eraID int) (*Era, error) {
			return &Era{ID: eraID, CalendarID: "cal-7", Name: "Old Name"}, nil
		},
		updateEraFn: func(ctx context.Context, eraID int, input EraInput) error {
			if eraID != 42 {
				t.Errorf("repo received eraID = %d; want 42", eraID)
			}
			if input.Name != "New Name" {
				t.Errorf("repo received Name = %q; want New Name", input.Name)
			}
			return nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	if err := svc.UpdateEra(context.Background(), 42, EraInput{
		Name: "New Name", StartYear: 1, Color: "#abcdef",
	}); err != nil {
		t.Fatalf("UpdateEra: %v", err)
	}
	if !bus.calendarStructurePublished("cal-7") {
		t.Error("expected structure.updated WS event for cal-7 (era's owning calendar)")
	}
}

// TestDeleteEra_HappyPath — repo returns the owning calendarID → service
// publishes WS for that calendar.
func TestDeleteEra_HappyPath(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-9", "camp-9"),
		deleteEraFn: func(ctx context.Context, eraID int) (string, error) {
			if eraID != 11 {
				t.Errorf("repo received eraID = %d; want 11", eraID)
			}
			return "cal-9", nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	if err := svc.DeleteEra(context.Background(), 11); err != nil {
		t.Fatalf("DeleteEra: %v", err)
	}
	if !bus.calendarStructurePublished("cal-9") {
		t.Error("expected structure.updated WS event for cal-9")
	}
}

// TestDeleteEra_NotFound — repo returns empty calendarID (era didn't
// exist) → service returns NotFound; no WS event published.
func TestDeleteEra_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{
		deleteEraFn: func(ctx context.Context, eraID int) (string, error) {
			return "", nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	err := svc.DeleteEra(context.Background(), 99)
	if err == nil {
		t.Fatal("expected NotFound; got nil")
	}
	if !isNotFoundErr(err) {
		t.Errorf("error = %v; want NotFound", err)
	}
	if len(bus.publishedEvents) != 0 {
		t.Errorf("expected no WS events; got %d", len(bus.publishedEvents))
	}
}

// TestApplyImport_RoutesThroughRepoApplyImport — verifies V2 PR 2's
// transactional-ApplyImport refactor: service.ApplyImport calls
// repo.ApplyImport (one tx) rather than the old per-resource Set*
// chain. The DB-level atomic-rollback semantic is a repo-layer
// concern (integration test territory); this test pins the service-
// layer dispatch shape so accidental regressions of the refactor
// fire fast.
func TestApplyImport_RoutesThroughRepoApplyImport(t *testing.T) {
	var applyCalled bool
	var setMonthsCalled bool
	repo := &mockCalendarRepo{
		getByIDFn: func(ctx context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Mode: ModeFantasy}, nil
		},
		applyImportFn: func(ctx context.Context, cal *Calendar, result *ImportResult) error {
			applyCalled = true
			if cal.ID != "cal-1" {
				t.Errorf("repo received cal.ID = %q; want cal-1", cal.ID)
			}
			return nil
		},
		setMonthsFn: func(ctx context.Context, calendarID string, months []MonthInput) error {
			setMonthsCalled = true
			return nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	result := &ImportResult{
		CalendarName: "Imported",
		Settings: ImportedSettings{
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, CurrentYear: 1500,
		},
		Months: []MonthInput{
			{Name: "Janus", Days: 31, SortOrder: 0},
		},
	}
	if err := svc.ApplyImport(context.Background(), "cal-1", result); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if !applyCalled {
		t.Error("expected service.ApplyImport to call repo.ApplyImport (transactional path); not called")
	}
	if setMonthsCalled {
		t.Error("service.ApplyImport called legacy repo.SetMonths; should route through repo.ApplyImport instead")
	}
	if !bus.calendarStructurePublished("cal-1") {
		t.Error("expected structure.updated WS event for cal-1 post-ApplyImport")
	}
}

// TestApplyImport_ValidationShortCircuits — invalid input in Months
// validates upfront → service returns the validation error WITHOUT
// calling repo.ApplyImport (so no tx opens; calendar stays at pre-
// import state).
func TestApplyImport_ValidationShortCircuits(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(ctx context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Mode: ModeFantasy}, nil
		},
		applyImportFn: func(ctx context.Context, cal *Calendar, result *ImportResult) error {
			t.Error("repo.ApplyImport called despite invalid input")
			return nil
		},
	}
	svc, _ := newSvcWithBus(repo)

	// Month with empty name → validation error before tx opens.
	result := &ImportResult{
		Settings: ImportedSettings{HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60},
		Months: []MonthInput{
			{Name: "", Days: 31, SortOrder: 0}, // invalid
		},
	}
	if err := svc.ApplyImport(context.Background(), "cal-1", result); err == nil {
		t.Fatal("expected validation error; got nil")
	}
}

// isNotFoundErr unwraps an error to detect Chronicle's apperror.AppError
// with HTTP 404 status. apperror exposes constructor helpers but not a
// sentinel; the AppError carries Code (HTTP status) which is the
// stable check.
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code == http.StatusNotFound
	}
	return false
}

// mockWSPublisher captures PublishCalendarEvent calls so tests can
// assert "structure.updated WS event published for calendar X." Matches
// the CalendarEventPublisher interface the service uses internally
// (assigned via SetEventPublisher / direct field set in tests).
type mockWSPublisher struct {
	publishedEvents []wsEventCapture
}

type wsEventCapture struct {
	eventType  string
	campaignID string
	resourceID string
	payload    any
}

func (m *mockWSPublisher) PublishCalendarEvent(eventType, campaignID, resourceID string, payload any) {
	m.publishedEvents = append(m.publishedEvents, wsEventCapture{
		eventType: eventType, campaignID: campaignID, resourceID: resourceID, payload: payload,
	})
}

// calendarStructurePublished returns true if the publisher saw a
// `calendar.structure.updated` event whose resourceID equals the
// supplied calendarID. publishStructureUpdated emits the WS event with
// the calendar's owning campaign as `campaignID` + the calendarID as
// `resourceID` — see service.go::publishStructureUpdated.
func (m *mockWSPublisher) calendarStructurePublished(calendarID string) bool {
	for _, e := range m.publishedEvents {
		if e.eventType == "calendar.structure.updated" && e.resourceID == calendarID {
			return true
		}
	}
	return false
}

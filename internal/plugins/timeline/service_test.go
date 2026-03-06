package timeline

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

type mockTimelineRepo struct {
	createFn              func(ctx context.Context, t *Timeline) error
	getByIDFn             func(ctx context.Context, id string) (*Timeline, error)
	listFn                func(ctx context.Context, campaignID string, role int) ([]Timeline, error)
	updateFn              func(ctx context.Context, t *Timeline) error
	deleteFn              func(ctx context.Context, id string) error
	searchFn              func(ctx context.Context, campaignID, query string, role int) ([]Timeline, error)
	linkEventFn           func(ctx context.Context, link *EventLink) error
	unlinkEventFn         func(ctx context.Context, timelineID, eventID string) error
	listEventLinksFn      func(ctx context.Context, timelineID string, role int) ([]EventLink, error)
	countEventsFn         func(ctx context.Context, timelineID string) (int, error)
	updateEventLinkVisFn  func(ctx context.Context, timelineID, eventID string, visOverride *string, visRules *string) error
	createEventFn         func(ctx context.Context, e *TimelineEvent) error
	getEventFn            func(ctx context.Context, eventID string) (*TimelineEvent, error)
	updateEventFn         func(ctx context.Context, e *TimelineEvent) error
	deleteEventFn         func(ctx context.Context, eventID string) error
	listStandaloneEventsFn func(ctx context.Context, timelineID string, role int) ([]TimelineEvent, error)
	countStandaloneEventsFn func(ctx context.Context, timelineID string) (int, error)
	createEntityGroupFn   func(ctx context.Context, g *EntityGroup) error
	updateEntityGroupFn   func(ctx context.Context, g *EntityGroup) error
	deleteEntityGroupFn   func(ctx context.Context, groupID int, timelineID string) error
	listEntityGroupsFn    func(ctx context.Context, timelineID string) ([]EntityGroup, error)
	addGroupMemberFn      func(ctx context.Context, groupID int, timelineID, entityID string) error
	removeGroupMemberFn   func(ctx context.Context, groupID int, timelineID, entityID string) error
	createConnectionFn    func(ctx context.Context, c *EventConnection) error
	deleteConnectionFn    func(ctx context.Context, connectionID int, timelineID string) error
	listConnectionsFn     func(ctx context.Context, timelineID string) ([]EventConnection, error)
}

func (m *mockTimelineRepo) Create(ctx context.Context, t *Timeline) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	return nil
}

func (m *mockTimelineRepo) GetByID(ctx context.Context, id string) (*Timeline, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockTimelineRepo) List(ctx context.Context, campaignID string, role int) ([]Timeline, error) {
	if m.listFn != nil {
		return m.listFn(ctx, campaignID, role)
	}
	return nil, nil
}

func (m *mockTimelineRepo) Update(ctx context.Context, t *Timeline) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, t)
	}
	return nil
}

func (m *mockTimelineRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockTimelineRepo) Search(ctx context.Context, campaignID, query string, role int) ([]Timeline, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, campaignID, query, role)
	}
	return nil, nil
}

func (m *mockTimelineRepo) LinkEvent(ctx context.Context, link *EventLink) error {
	if m.linkEventFn != nil {
		return m.linkEventFn(ctx, link)
	}
	return nil
}

func (m *mockTimelineRepo) UnlinkEvent(ctx context.Context, timelineID, eventID string) error {
	if m.unlinkEventFn != nil {
		return m.unlinkEventFn(ctx, timelineID, eventID)
	}
	return nil
}

func (m *mockTimelineRepo) ListEventLinks(ctx context.Context, timelineID string, role int) ([]EventLink, error) {
	if m.listEventLinksFn != nil {
		return m.listEventLinksFn(ctx, timelineID, role)
	}
	return nil, nil
}

func (m *mockTimelineRepo) CountEvents(ctx context.Context, timelineID string) (int, error) {
	if m.countEventsFn != nil {
		return m.countEventsFn(ctx, timelineID)
	}
	return 0, nil
}

func (m *mockTimelineRepo) UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, visOverride *string, visRules *string) error {
	if m.updateEventLinkVisFn != nil {
		return m.updateEventLinkVisFn(ctx, timelineID, eventID, visOverride, visRules)
	}
	return nil
}

func (m *mockTimelineRepo) CreateEvent(ctx context.Context, e *TimelineEvent) error {
	if m.createEventFn != nil {
		return m.createEventFn(ctx, e)
	}
	return nil
}

func (m *mockTimelineRepo) GetEvent(ctx context.Context, eventID string) (*TimelineEvent, error) {
	if m.getEventFn != nil {
		return m.getEventFn(ctx, eventID)
	}
	return nil, nil
}

func (m *mockTimelineRepo) UpdateEvent(ctx context.Context, e *TimelineEvent) error {
	if m.updateEventFn != nil {
		return m.updateEventFn(ctx, e)
	}
	return nil
}

func (m *mockTimelineRepo) DeleteEvent(ctx context.Context, eventID string) error {
	if m.deleteEventFn != nil {
		return m.deleteEventFn(ctx, eventID)
	}
	return nil
}

func (m *mockTimelineRepo) ListStandaloneEvents(ctx context.Context, timelineID string, role int) ([]TimelineEvent, error) {
	if m.listStandaloneEventsFn != nil {
		return m.listStandaloneEventsFn(ctx, timelineID, role)
	}
	return nil, nil
}

func (m *mockTimelineRepo) CountStandaloneEvents(ctx context.Context, timelineID string) (int, error) {
	if m.countStandaloneEventsFn != nil {
		return m.countStandaloneEventsFn(ctx, timelineID)
	}
	return 0, nil
}

func (m *mockTimelineRepo) CreateEntityGroup(ctx context.Context, g *EntityGroup) error {
	if m.createEntityGroupFn != nil {
		return m.createEntityGroupFn(ctx, g)
	}
	return nil
}

func (m *mockTimelineRepo) UpdateEntityGroup(ctx context.Context, g *EntityGroup) error {
	if m.updateEntityGroupFn != nil {
		return m.updateEntityGroupFn(ctx, g)
	}
	return nil
}

func (m *mockTimelineRepo) DeleteEntityGroup(ctx context.Context, groupID int, timelineID string) error {
	if m.deleteEntityGroupFn != nil {
		return m.deleteEntityGroupFn(ctx, groupID, timelineID)
	}
	return nil
}

func (m *mockTimelineRepo) ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error) {
	if m.listEntityGroupsFn != nil {
		return m.listEntityGroupsFn(ctx, timelineID)
	}
	return nil, nil
}

func (m *mockTimelineRepo) AddGroupMember(ctx context.Context, groupID int, timelineID, entityID string) error {
	if m.addGroupMemberFn != nil {
		return m.addGroupMemberFn(ctx, groupID, timelineID, entityID)
	}
	return nil
}

func (m *mockTimelineRepo) RemoveGroupMember(ctx context.Context, groupID int, timelineID, entityID string) error {
	if m.removeGroupMemberFn != nil {
		return m.removeGroupMemberFn(ctx, groupID, timelineID, entityID)
	}
	return nil
}

func (m *mockTimelineRepo) CreateConnection(ctx context.Context, c *EventConnection) error {
	if m.createConnectionFn != nil {
		return m.createConnectionFn(ctx, c)
	}
	return nil
}

func (m *mockTimelineRepo) DeleteConnection(ctx context.Context, connectionID int, timelineID string) error {
	if m.deleteConnectionFn != nil {
		return m.deleteConnectionFn(ctx, connectionID, timelineID)
	}
	return nil
}

func (m *mockTimelineRepo) ListConnections(ctx context.Context, timelineID string) ([]EventConnection, error) {
	if m.listConnectionsFn != nil {
		return m.listConnectionsFn(ctx, timelineID)
	}
	return nil, nil
}

// --- Mock Calendar Adapters ---

type mockCalendarLister struct {
	listFn func(ctx context.Context, campaignID string) ([]CalendarRef, error)
}

func (m *mockCalendarLister) ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error) {
	if m.listFn != nil {
		return m.listFn(ctx, campaignID)
	}
	return nil, nil
}

type mockCalendarEventLister struct {
	listFn func(ctx context.Context, calendarID string, role int) ([]CalendarEventRef, error)
}

func (m *mockCalendarEventLister) ListEventsForCalendar(ctx context.Context, calendarID string, role int) ([]CalendarEventRef, error) {
	if m.listFn != nil {
		return m.listFn(ctx, calendarID, role)
	}
	return nil, nil
}

type mockCalendarEraLister struct {
	listFn func(ctx context.Context, calendarID string) ([]CalendarEra, error)
}

func (m *mockCalendarEraLister) ListEras(ctx context.Context, calendarID string) ([]CalendarEra, error) {
	if m.listFn != nil {
		return m.listFn(ctx, calendarID)
	}
	return nil, nil
}

// --- Test Helpers ---

func newTestTimelineService(repo *mockTimelineRepo) TimelineService {
	return NewTimelineService(repo, &mockCalendarLister{}, &mockCalendarEventLister{}, &mockCalendarEraLister{})
}

func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status code %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- CreateTimeline Tests ---

func TestCreateTimeline_Success(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	tl, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name:      "Age of Heroes",
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tl == nil {
		t.Fatal("expected timeline, got nil")
	}
	if tl.Name != "Age of Heroes" {
		t.Errorf("expected name 'Age of Heroes', got %q", tl.Name)
	}
	if tl.CampaignID != "camp-1" {
		t.Errorf("expected campaign_id 'camp-1', got %q", tl.CampaignID)
	}
	if tl.ID == "" {
		t.Error("expected a generated UUID, got empty string")
	}
	// Check defaults.
	if tl.Color != "#6366f1" {
		t.Errorf("expected default color '#6366f1', got %q", tl.Color)
	}
	if tl.Icon != "fa-timeline" {
		t.Errorf("expected default icon 'fa-timeline', got %q", tl.Icon)
	}
	if tl.Visibility != "everyone" {
		t.Errorf("expected default visibility 'everyone', got %q", tl.Visibility)
	}
	if tl.ZoomDefault != ZoomYear {
		t.Errorf("expected default zoom 'year', got %q", tl.ZoomDefault)
	}
}

func TestCreateTimeline_EmptyName(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name: "",
	})
	assertAppError(t, err, 422)
}

func TestCreateTimeline_NameTooLong(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	longName := make([]byte, 256)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name: string(longName),
	})
	assertAppError(t, err, 422)
}

func TestCreateTimeline_InvalidVisibility(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name:       "TL",
		Visibility: "invalid",
	})
	assertAppError(t, err, 422)
}

func TestCreateTimeline_InvalidZoom(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name:        "TL",
		ZoomDefault: "invalid_zoom",
	})
	assertAppError(t, err, 422)
}

func TestCreateTimeline_InvalidIcon(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name: "TL",
		Icon: "<script>alert(1)</script>",
	})
	assertAppError(t, err, 422)
}

func TestCreateTimeline_InvalidColor(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name:  "TL",
		Color: "not-a-color",
	})
	assertAppError(t, err, 422)
}

func TestCreateTimeline_RepoError(t *testing.T) {
	repo := &mockTimelineRepo{
		createFn: func(_ context.Context, _ *Timeline) error {
			return errors.New("db error")
		},
	}
	svc := newTestTimelineService(repo)

	_, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
		Name: "TL",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetTimeline Tests ---

func TestGetTimeline_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
			return &Timeline{ID: id, Name: "Main", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	tl, err := svc.GetTimeline(context.Background(), "tl-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tl.ID != "tl-1" {
		t.Errorf("expected ID 'tl-1', got %q", tl.ID)
	}
}

func TestGetTimeline_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	_, err := svc.GetTimeline(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

func TestGetTimeline_RepoError(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestTimelineService(repo)

	_, err := svc.GetTimeline(context.Background(), "tl-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- ListTimelines Tests ---

func TestListTimelines_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		listFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			return []Timeline{
				{ID: "tl-1", Name: "Main", Visibility: "everyone"},
				{ID: "tl-2", Name: "Secret", Visibility: "dm_only"},
			}, nil
		},
	}
	svc := newTestTimelineService(repo)

	// Owner (role 3) sees all.
	timelines, err := svc.ListTimelines(context.Background(), "camp-1", 3, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timelines) != 2 {
		t.Errorf("expected 2 timelines, got %d", len(timelines))
	}
}

func TestListTimelines_PlayerFiltersDMOnly(t *testing.T) {
	repo := &mockTimelineRepo{
		listFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			return []Timeline{
				{ID: "tl-1", Name: "Main", Visibility: "everyone"},
				{ID: "tl-2", Name: "Secret", Visibility: "dm_only"},
			}, nil
		},
	}
	svc := newTestTimelineService(repo)

	// Player (role 1) should be filtered to only "everyone".
	timelines, err := svc.ListTimelines(context.Background(), "camp-1", 1, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timelines) != 1 {
		t.Fatalf("expected 1 timeline for player, got %d", len(timelines))
	}
	if timelines[0].ID != "tl-1" {
		t.Errorf("expected timeline 'tl-1', got %q", timelines[0].ID)
	}
}

// --- UpdateTimeline Tests ---

func TestUpdateTimeline_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1", Name: "Old", CampaignID: "camp-1"}, nil
		},
		updateFn: func(_ context.Context, tl *Timeline) error {
			if tl.Name != "New Name" {
				t.Errorf("expected name 'New Name', got %q", tl.Name)
			}
			return nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.UpdateTimeline(context.Background(), "tl-1", UpdateTimelineInput{
		Name:        "New Name",
		Visibility:  "everyone",
		ZoomDefault: ZoomYear,
		Icon:        "fa-timeline",
		Color:       "#ff0000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateTimeline_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	err := svc.UpdateTimeline(context.Background(), "nonexistent", UpdateTimelineInput{
		Name:        "X",
		Visibility:  "everyone",
		ZoomDefault: ZoomYear,
	})
	assertAppError(t, err, 404)
}

func TestUpdateTimeline_EmptyName(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.UpdateTimeline(context.Background(), "tl-1", UpdateTimelineInput{
		Name:        "",
		Visibility:  "everyone",
		ZoomDefault: ZoomYear,
	})
	assertAppError(t, err, 422)
}

func TestUpdateTimeline_InvalidVisibility(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.UpdateTimeline(context.Background(), "tl-1", UpdateTimelineInput{
		Name:        "X",
		Visibility:  "invalid",
		ZoomDefault: ZoomYear,
	})
	assertAppError(t, err, 422)
}

// --- DeleteTimeline Tests ---

func TestDeleteTimeline_Success(t *testing.T) {
	deleted := false
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1", CampaignID: "camp-1"}, nil
		},
		deleteFn: func(_ context.Context, id string) error {
			deleted = true
			if id != "tl-1" {
				t.Errorf("expected delete ID 'tl-1', got %q", id)
			}
			return nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.DeleteTimeline(context.Background(), "tl-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected Delete to be called on repo")
	}
}

func TestDeleteTimeline_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	err := svc.DeleteTimeline(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// --- LinkEvent Tests ---

func TestLinkEvent_Success(t *testing.T) {
	calID := "cal-1"
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1", CalendarID: &calID}, nil
		},
		countEventsFn: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
	}
	svc := newTestTimelineService(repo)

	link, err := svc.LinkEvent(context.Background(), "tl-1", "evt-1", LinkEventInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link.EventID != "evt-1" {
		t.Errorf("expected event_id 'evt-1', got %q", link.EventID)
	}
	if link.DisplayOrder != 5 {
		t.Errorf("expected display_order 5, got %d", link.DisplayOrder)
	}
}

func TestLinkEvent_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	_, err := svc.LinkEvent(context.Background(), "nonexistent", "evt-1", LinkEventInput{})
	assertAppError(t, err, 404)
}

func TestLinkEvent_NoCalendar(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1", CalendarID: nil}, nil
		},
	}
	svc := newTestTimelineService(repo)

	_, err := svc.LinkEvent(context.Background(), "tl-1", "evt-1", LinkEventInput{})
	assertAppError(t, err, 422)
}

// --- CreateStandaloneEvent Tests ---

func TestCreateStandaloneEvent_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1", CampaignID: "camp-1"}, nil
		},
		countStandaloneEventsFn: func(_ context.Context, _ string) (int, error) {
			return 3, nil
		},
	}
	svc := newTestTimelineService(repo)

	evt, err := svc.CreateStandaloneEvent(context.Background(), "tl-1", CreateTimelineEventInput{
		Name:      "Battle of Dawn",
		Year:      1200,
		Month:     3,
		Day:       15,
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt == nil {
		t.Fatal("expected event, got nil")
	}
	if evt.Name != "Battle of Dawn" {
		t.Errorf("expected name 'Battle of Dawn', got %q", evt.Name)
	}
	if evt.DisplayOrder != 3 {
		t.Errorf("expected display_order 3, got %d", evt.DisplayOrder)
	}
	if evt.Visibility != "everyone" {
		t.Errorf("expected default visibility 'everyone', got %q", evt.Visibility)
	}
}

func TestCreateStandaloneEvent_TimelineNotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	_, err := svc.CreateStandaloneEvent(context.Background(), "nonexistent", CreateTimelineEventInput{
		Name: "Event",
	})
	assertAppError(t, err, 404)
}

func TestCreateStandaloneEvent_EmptyName(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	_, err := svc.CreateStandaloneEvent(context.Background(), "tl-1", CreateTimelineEventInput{
		Name: "",
	})
	assertAppError(t, err, 422)
}

func TestCreateStandaloneEvent_NameTooLong(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	longName := make([]byte, 256)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.CreateStandaloneEvent(context.Background(), "tl-1", CreateTimelineEventInput{
		Name: string(longName),
	})
	assertAppError(t, err, 422)
}

func TestCreateStandaloneEvent_InvalidVisibility(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	_, err := svc.CreateStandaloneEvent(context.Background(), "tl-1", CreateTimelineEventInput{
		Name:       "Event",
		Visibility: "invalid",
	})
	assertAppError(t, err, 422)
}

func TestCreateStandaloneEvent_InvalidColor(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
			return &Timeline{ID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	badColor := "not-a-color"
	_, err := svc.CreateStandaloneEvent(context.Background(), "tl-1", CreateTimelineEventInput{
		Name:  "Event",
		Color: &badColor,
	})
	assertAppError(t, err, 422)
}

// --- GetStandaloneEvent Tests ---

func TestGetStandaloneEvent_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		getEventFn: func(_ context.Context, id string) (*TimelineEvent, error) {
			return &TimelineEvent{ID: id, Name: "Battle"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	evt, err := svc.GetStandaloneEvent(context.Background(), "evt-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.ID != "evt-1" {
		t.Errorf("expected ID 'evt-1', got %q", evt.ID)
	}
}

func TestGetStandaloneEvent_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	_, err := svc.GetStandaloneEvent(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// --- UpdateStandaloneEvent Tests ---

func TestUpdateStandaloneEvent_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		getEventFn: func(_ context.Context, _ string) (*TimelineEvent, error) {
			return &TimelineEvent{ID: "evt-1", TimelineID: "tl-1", Name: "Old"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.UpdateStandaloneEvent(context.Background(), "tl-1", "evt-1", UpdateTimelineEventInput{
		Name:       "New Name",
		Visibility: "everyone",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateStandaloneEvent_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	err := svc.UpdateStandaloneEvent(context.Background(), "tl-1", "nonexistent", UpdateTimelineEventInput{
		Name:       "X",
		Visibility: "everyone",
	})
	assertAppError(t, err, 404)
}

func TestUpdateStandaloneEvent_WrongTimeline(t *testing.T) {
	repo := &mockTimelineRepo{
		getEventFn: func(_ context.Context, _ string) (*TimelineEvent, error) {
			return &TimelineEvent{ID: "evt-1", TimelineID: "tl-other"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	// Event belongs to tl-other, but we're requesting tl-1 -- should be not found.
	err := svc.UpdateStandaloneEvent(context.Background(), "tl-1", "evt-1", UpdateTimelineEventInput{
		Name:       "X",
		Visibility: "everyone",
	})
	assertAppError(t, err, 404)
}

func TestUpdateStandaloneEvent_EmptyName(t *testing.T) {
	repo := &mockTimelineRepo{
		getEventFn: func(_ context.Context, _ string) (*TimelineEvent, error) {
			return &TimelineEvent{ID: "evt-1", TimelineID: "tl-1"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.UpdateStandaloneEvent(context.Background(), "tl-1", "evt-1", UpdateTimelineEventInput{
		Name:       "",
		Visibility: "everyone",
	})
	assertAppError(t, err, 422)
}

// --- DeleteStandaloneEvent Tests ---

func TestDeleteStandaloneEvent_Success(t *testing.T) {
	deleted := false
	repo := &mockTimelineRepo{
		getEventFn: func(_ context.Context, _ string) (*TimelineEvent, error) {
			return &TimelineEvent{ID: "evt-1", TimelineID: "tl-1"}, nil
		},
		deleteEventFn: func(_ context.Context, id string) error {
			deleted = true
			return nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.DeleteStandaloneEvent(context.Background(), "tl-1", "evt-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected DeleteEvent to be called on repo")
	}
}

func TestDeleteStandaloneEvent_NotFound(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	err := svc.DeleteStandaloneEvent(context.Background(), "tl-1", "nonexistent")
	assertAppError(t, err, 404)
}

func TestDeleteStandaloneEvent_WrongTimeline(t *testing.T) {
	repo := &mockTimelineRepo{
		getEventFn: func(_ context.Context, _ string) (*TimelineEvent, error) {
			return &TimelineEvent{ID: "evt-1", TimelineID: "tl-other"}, nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.DeleteStandaloneEvent(context.Background(), "tl-1", "evt-1")
	assertAppError(t, err, 404)
}

// --- CreateEntityGroup Tests ---

func TestCreateEntityGroup_Success(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	g, err := svc.CreateEntityGroup(context.Background(), "tl-1", CreateEntityGroupInput{
		Name: "Heroes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g == nil {
		t.Fatal("expected group, got nil")
	}
	if g.Name != "Heroes" {
		t.Errorf("expected name 'Heroes', got %q", g.Name)
	}
	// Default color.
	if g.Color != "#6b7280" {
		t.Errorf("expected default color '#6b7280', got %q", g.Color)
	}
}

func TestCreateEntityGroup_EmptyName(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateEntityGroup(context.Background(), "tl-1", CreateEntityGroupInput{
		Name: "",
	})
	assertAppError(t, err, 422)
}

func TestCreateEntityGroup_NameTooLong(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	longName := make([]byte, 201)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.CreateEntityGroup(context.Background(), "tl-1", CreateEntityGroupInput{
		Name: string(longName),
	})
	assertAppError(t, err, 422)
}

func TestCreateEntityGroup_InvalidColor(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateEntityGroup(context.Background(), "tl-1", CreateEntityGroupInput{
		Name:  "Group",
		Color: "not-a-color",
	})
	assertAppError(t, err, 422)
}

// --- CreateConnection Tests ---

func TestCreateConnection_Success(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	conn, err := svc.CreateConnection(context.Background(), "tl-1", CreateConnectionInput{
		SourceID:   "evt-1",
		TargetID:   "evt-2",
		SourceType: "calendar",
		TargetType: "standalone",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
	if conn.Style != "arrow" {
		t.Errorf("expected default style 'arrow', got %q", conn.Style)
	}
}

func TestCreateConnection_MissingSourceOrTarget(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateConnection(context.Background(), "tl-1", CreateConnectionInput{
		SourceID:   "",
		TargetID:   "evt-2",
		SourceType: "calendar",
		TargetType: "calendar",
	})
	assertAppError(t, err, 422)
}

func TestCreateConnection_SelfConnect(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateConnection(context.Background(), "tl-1", CreateConnectionInput{
		SourceID:   "evt-1",
		TargetID:   "evt-1",
		SourceType: "calendar",
		TargetType: "calendar",
	})
	assertAppError(t, err, 422)
}

func TestCreateConnection_InvalidSourceType(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateConnection(context.Background(), "tl-1", CreateConnectionInput{
		SourceID:   "evt-1",
		TargetID:   "evt-2",
		SourceType: "invalid",
		TargetType: "calendar",
	})
	assertAppError(t, err, 422)
}

func TestCreateConnection_InvalidStyle(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	_, err := svc.CreateConnection(context.Background(), "tl-1", CreateConnectionInput{
		SourceID:   "evt-1",
		TargetID:   "evt-2",
		SourceType: "calendar",
		TargetType: "calendar",
		Style:      "invalid_style",
	})
	assertAppError(t, err, 422)
}

// --- SearchTimelines Tests ---

func TestSearchTimelines_Success(t *testing.T) {
	repo := &mockTimelineRepo{
		searchFn: func(_ context.Context, _ string, _ string, _ int) ([]Timeline, error) {
			return []Timeline{
				{ID: "tl-1", Name: "Age of Heroes", Icon: "fa-timeline", Color: "#6366f1"},
			}, nil
		},
	}
	svc := newTestTimelineService(repo)

	results, err := svc.SearchTimelines(context.Background(), "camp-1", "heroes", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "Age of Heroes" {
		t.Errorf("expected name 'Age of Heroes', got %q", results[0]["name"])
	}
	if results[0]["type_name"] != "Timeline" {
		t.Errorf("expected type_name 'Timeline', got %q", results[0]["type_name"])
	}
}

func TestSearchTimelines_Empty(t *testing.T) {
	repo := &mockTimelineRepo{
		searchFn: func(_ context.Context, _ string, _ string, _ int) ([]Timeline, error) {
			return []Timeline{}, nil
		},
	}
	svc := newTestTimelineService(repo)

	results, err := svc.SearchTimelines(context.Background(), "camp-1", "nonexistent", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- UpdateEventLinkVisibility Tests ---

func TestUpdateEventLinkVisibility_InvalidOverride(t *testing.T) {
	svc := newTestTimelineService(&mockTimelineRepo{})
	invalid := "invalid"
	err := svc.UpdateEventLinkVisibility(context.Background(), "tl-1", "evt-1", UpdateEventVisibilityInput{
		VisibilityOverride: &invalid,
	})
	assertAppError(t, err, 422)
}

func TestUpdateEventLinkVisibility_Success(t *testing.T) {
	repo := &mockTimelineRepo{}
	svc := newTestTimelineService(repo)

	dmOnly := "dm_only"
	err := svc.UpdateEventLinkVisibility(context.Background(), "tl-1", "evt-1", UpdateEventVisibilityInput{
		VisibilityOverride: &dmOnly,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

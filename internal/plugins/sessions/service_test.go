package sessions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

// mockSessionRepo implements SessionRepository with function pointers so
// individual tests can override only the methods they care about. Nil
// methods return sensible zero values (nil error, empty slices).
type mockSessionRepo struct {
	createFn              func(ctx context.Context, campaignID string, s *Session) error
	findByIDFn            func(ctx context.Context, id string) (*Session, error)
	listByCampaignFn      func(ctx context.Context, campaignID string) ([]Session, error)
	listByDateRangeFn     func(ctx context.Context, campaignID, startDate, endDate string) ([]Session, error)
	searchByCampaignFn    func(ctx context.Context, campaignID, query string) ([]Session, error)
	updateFn              func(ctx context.Context, s *Session) error
	updateRecapFn         func(ctx context.Context, id string, recap, recapHTML *string) error
	deleteFn              func(ctx context.Context, id string) error
	addAttendeeFn         func(ctx context.Context, sessionID, userID, status string) error
	updateAttendeeStatusFn func(ctx context.Context, sessionID, userID, status string) error
	removeAttendeeFn      func(ctx context.Context, sessionID, userID string) error
	listAttendeesFn       func(ctx context.Context, sessionID string) ([]Attendee, error)
	linkEntityFn          func(ctx context.Context, sessionID, entityID, role string) error
	unlinkEntityFn        func(ctx context.Context, sessionID, entityID string) error
	listSessionEntitiesFn func(ctx context.Context, sessionID string) ([]SessionEntity, error)
	createRSVPTokenFn     func(ctx context.Context, token *RSVPToken) error
	findRSVPTokenFn       func(ctx context.Context, tokenStr string) (*RSVPToken, error)
	markRSVPTokenUsedFn   func(ctx context.Context, tokenStr string) error
}

func (m *mockSessionRepo) Create(ctx context.Context, campaignID string, s *Session) error {
	if m.createFn != nil {
		return m.createFn(ctx, campaignID, s)
	}
	return nil
}

func (m *mockSessionRepo) FindByID(ctx context.Context, id string) (*Session, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockSessionRepo) ListByCampaign(ctx context.Context, campaignID string) ([]Session, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockSessionRepo) ListByDateRange(ctx context.Context, campaignID, startDate, endDate string) ([]Session, error) {
	if m.listByDateRangeFn != nil {
		return m.listByDateRangeFn(ctx, campaignID, startDate, endDate)
	}
	return nil, nil
}

func (m *mockSessionRepo) SearchByCampaign(ctx context.Context, campaignID, query string) ([]Session, error) {
	if m.searchByCampaignFn != nil {
		return m.searchByCampaignFn(ctx, campaignID, query)
	}
	return nil, nil
}

func (m *mockSessionRepo) Update(ctx context.Context, s *Session) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, s)
	}
	return nil
}

func (m *mockSessionRepo) UpdateRecap(ctx context.Context, id string, recap, recapHTML *string) error {
	if m.updateRecapFn != nil {
		return m.updateRecapFn(ctx, id, recap, recapHTML)
	}
	return nil
}

func (m *mockSessionRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockSessionRepo) AddAttendee(ctx context.Context, sessionID, userID, status string) error {
	if m.addAttendeeFn != nil {
		return m.addAttendeeFn(ctx, sessionID, userID, status)
	}
	return nil
}

func (m *mockSessionRepo) UpdateAttendeeStatus(ctx context.Context, sessionID, userID, status string) error {
	if m.updateAttendeeStatusFn != nil {
		return m.updateAttendeeStatusFn(ctx, sessionID, userID, status)
	}
	return nil
}

func (m *mockSessionRepo) RemoveAttendee(ctx context.Context, sessionID, userID string) error {
	if m.removeAttendeeFn != nil {
		return m.removeAttendeeFn(ctx, sessionID, userID)
	}
	return nil
}

func (m *mockSessionRepo) ListAttendees(ctx context.Context, sessionID string) ([]Attendee, error) {
	if m.listAttendeesFn != nil {
		return m.listAttendeesFn(ctx, sessionID)
	}
	return nil, nil
}

func (m *mockSessionRepo) LinkEntity(ctx context.Context, sessionID, entityID, role string) error {
	if m.linkEntityFn != nil {
		return m.linkEntityFn(ctx, sessionID, entityID, role)
	}
	return nil
}

func (m *mockSessionRepo) UnlinkEntity(ctx context.Context, sessionID, entityID string) error {
	if m.unlinkEntityFn != nil {
		return m.unlinkEntityFn(ctx, sessionID, entityID)
	}
	return nil
}

func (m *mockSessionRepo) ListSessionEntities(ctx context.Context, sessionID string) ([]SessionEntity, error) {
	if m.listSessionEntitiesFn != nil {
		return m.listSessionEntitiesFn(ctx, sessionID)
	}
	return nil, nil
}

func (m *mockSessionRepo) CreateRSVPToken(ctx context.Context, token *RSVPToken) error {
	if m.createRSVPTokenFn != nil {
		return m.createRSVPTokenFn(ctx, token)
	}
	return nil
}

func (m *mockSessionRepo) FindRSVPToken(ctx context.Context, tokenStr string) (*RSVPToken, error) {
	if m.findRSVPTokenFn != nil {
		return m.findRSVPTokenFn(ctx, tokenStr)
	}
	return nil, nil
}

func (m *mockSessionRepo) MarkRSVPTokenUsed(ctx context.Context, tokenStr string) error {
	if m.markRSVPTokenUsedFn != nil {
		return m.markRSVPTokenUsedFn(ctx, tokenStr)
	}
	return nil
}

// --- Mock Entity Campaign Checker ---

// mockEntityChecker implements EntityCampaignChecker for testing entity linking.
type mockEntityChecker struct {
	belongsFn func(ctx context.Context, entityID, campaignID string) (bool, error)
}

func (m *mockEntityChecker) EntityBelongsToCampaign(ctx context.Context, entityID, campaignID string) (bool, error) {
	if m.belongsFn != nil {
		return m.belongsFn(ctx, entityID, campaignID)
	}
	return true, nil
}

// --- Test Helpers ---

// newTestSessionService creates a session service with the given mock repo
// and no entity checker (nil). Use newTestSessionServiceWithChecker when
// testing entity linking.
func newTestSessionService(repo *mockSessionRepo) SessionService {
	return NewSessionService(repo, nil)
}

// newTestSessionServiceWithChecker creates a session service with both a
// mock repo and a mock entity campaign checker.
func newTestSessionServiceWithChecker(repo *mockSessionRepo, checker *mockEntityChecker) SessionService {
	return NewSessionService(repo, checker)
}

// assertAppError asserts that err is a non-nil *apperror.AppError with the
// expected HTTP status code.
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

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }

// intPtr returns a pointer to the given int.
func intPtr(i int) *int { return &i }

// --- CreateSession Tests ---

func TestCreateSession_Success(t *testing.T) {
	repo := &mockSessionRepo{}
	svc := newTestSessionService(repo)

	session, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name:      "Session 1",
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.Name != "Session 1" {
		t.Errorf("expected name 'Session 1', got %q", session.Name)
	}
	if session.CampaignID != "camp-1" {
		t.Errorf("expected campaign_id 'camp-1', got %q", session.CampaignID)
	}
	if session.Status != StatusPlanned {
		t.Errorf("expected status %q, got %q", StatusPlanned, session.Status)
	}
	if session.ID == "" {
		t.Error("expected a generated UUID, got empty string")
	}
	if session.CreatedBy != "user-1" {
		t.Errorf("expected created_by 'user-1', got %q", session.CreatedBy)
	}
}

func TestCreateSession_WithOptionalFields(t *testing.T) {
	repo := &mockSessionRepo{}
	svc := newTestSessionService(repo)

	recType := RecurrenceWeekly
	session, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name:           "Weekly Game",
		Summary:        strPtr("Our weekly D&D session"),
		ScheduledDate:  strPtr("2026-03-14"),
		CalendarYear:   intPtr(1492),
		CalendarMonth:  intPtr(3),
		CalendarDay:    intPtr(15),
		IsRecurring:    true,
		RecurrenceType: &recType,
		CreatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.Summary == nil || *session.Summary != "Our weekly D&D session" {
		t.Errorf("expected summary set, got %v", session.Summary)
	}
	if session.ScheduledDate == nil || *session.ScheduledDate != "2026-03-14" {
		t.Errorf("expected scheduled_date '2026-03-14', got %v", session.ScheduledDate)
	}
	if !session.IsRecurring {
		t.Error("expected is_recurring true")
	}
	if session.RecurrenceType == nil || *session.RecurrenceType != RecurrenceWeekly {
		t.Errorf("expected recurrence_type 'weekly', got %v", session.RecurrenceType)
	}
}

func TestCreateSession_EmptyName(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	_, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name: "",
	})
	assertAppError(t, err, 400)
}

func TestCreateSession_NameTooLong(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	longName := strings.Repeat("a", 201)
	_, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name: longName,
	})
	assertAppError(t, err, 400)
}

func TestCreateSession_NameExactly200Chars(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	name := strings.Repeat("a", 200)
	session, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name:      name,
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.Name != name {
		t.Error("expected 200-char name to be accepted")
	}
}

func TestCreateSession_InvalidRecurrenceType(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	badType := "daily"
	_, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name:           "Bad Recurrence",
		IsRecurring:    true,
		RecurrenceType: &badType,
	})
	assertAppError(t, err, 400)
}

func TestCreateSession_ValidRecurrenceTypes(t *testing.T) {
	validTypes := []string{RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly, RecurrenceCustom}
	for _, rt := range validTypes {
		t.Run(rt, func(t *testing.T) {
			svc := newTestSessionService(&mockSessionRepo{})
			recType := rt
			session, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
				Name:           "Recurring",
				IsRecurring:    true,
				RecurrenceType: &recType,
				CreatedBy:      "user-1",
			})
			if err != nil {
				t.Fatalf("unexpected error for recurrence type %q: %v", rt, err)
			}
			if session.RecurrenceType == nil || *session.RecurrenceType != rt {
				t.Errorf("expected recurrence type %q, got %v", rt, session.RecurrenceType)
			}
		})
	}
}

func TestCreateSession_IntervalDefaultsToOne(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	session, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name:               "Default Interval",
		RecurrenceInterval: 0,
		CreatedBy:          "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.RecurrenceInterval != 1 {
		t.Errorf("expected recurrence_interval 1 (default), got %d", session.RecurrenceInterval)
	}
}

func TestCreateSession_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		createFn: func(_ context.Context, _ string, _ *Session) error {
			return errors.New("db connection lost")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.CreateSession(context.Background(), "camp-1", CreateSessionInput{
		Name:      "Will Fail",
		CreatedBy: "user-1",
	})
	assertAppError(t, err, 500)
}

// --- GetSession Tests ---

func TestGetSession_Success(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Session 1", CampaignID: "camp-1"}, nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return []Attendee{{UserID: "user-1", Status: RSVPAccepted}}, nil
		},
		listSessionEntitiesFn: func(_ context.Context, _ string) ([]SessionEntity, error) {
			return []SessionEntity{{EntityID: "ent-1", Role: EntityRoleKey}}, nil
		},
	}
	svc := newTestSessionService(repo)

	session, err := svc.GetSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID != "sess-1" {
		t.Errorf("expected ID 'sess-1', got %q", session.ID)
	}
	if len(session.Attendees) != 1 {
		t.Errorf("expected 1 attendee, got %d", len(session.Attendees))
	}
	if len(session.Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(session.Entities))
	}
}

func TestGetSession_NotFound(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, _ string) (*Session, error) {
			return nil, apperror.NewNotFound("session not found")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.GetSession(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

func TestGetSession_AttendeesRepoError(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Session 1"}, nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.GetSession(context.Background(), "sess-1")
	assertAppError(t, err, 500)
}

func TestGetSession_EntitiesRepoError(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Session 1"}, nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return []Attendee{}, nil
		},
		listSessionEntitiesFn: func(_ context.Context, _ string) ([]SessionEntity, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.GetSession(context.Background(), "sess-1")
	assertAppError(t, err, 500)
}

// --- ListSessions Tests ---

func TestListSessions_Success(t *testing.T) {
	repo := &mockSessionRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Session, error) {
			return []Session{
				{ID: "sess-1", Name: "Session 1"},
				{ID: "sess-2", Name: "Session 2"},
			}, nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return []Attendee{{UserID: "user-1"}}, nil
		},
	}
	svc := newTestSessionService(repo)

	sessions, err := svc.ListSessions(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
	// Verify attendees were loaded.
	if len(sessions[0].Attendees) != 1 {
		t.Errorf("expected 1 attendee on first session, got %d", len(sessions[0].Attendees))
	}
}

func TestListSessions_Empty(t *testing.T) {
	repo := &mockSessionRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Session, error) {
			return []Session{}, nil
		},
	}
	svc := newTestSessionService(repo)

	sessions, err := svc.ListSessions(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Session, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.ListSessions(context.Background(), "camp-1")
	assertAppError(t, err, 500)
}

// --- ListPlannedSessions Tests ---

func TestListPlannedSessions_Success(t *testing.T) {
	repo := &mockSessionRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Session, error) {
			return []Session{
				{ID: "sess-1", Name: "Upcoming", Status: StatusPlanned},
				{ID: "sess-2", Name: "Done", Status: StatusCompleted},
				{ID: "sess-3", Name: "Also Upcoming", Status: StatusPlanned},
				{ID: "sess-4", Name: "Cancelled", Status: StatusCancelled},
			}, nil
		},
	}
	svc := newTestSessionService(repo)

	sessions, err := svc.ListPlannedSessions(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 planned sessions, got %d", len(sessions))
	}
	for _, s := range sessions {
		if s.Status != StatusPlanned {
			t.Errorf("expected only planned sessions, got status %q", s.Status)
		}
	}
}

func TestListPlannedSessions_NoneExist(t *testing.T) {
	repo := &mockSessionRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Session, error) {
			return []Session{
				{ID: "sess-1", Name: "Done", Status: StatusCompleted},
			}, nil
		},
	}
	svc := newTestSessionService(repo)

	sessions, err := svc.ListPlannedSessions(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 planned sessions, got %d", len(sessions))
	}
}

func TestListPlannedSessions_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Session, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.ListPlannedSessions(context.Background(), "camp-1")
	assertAppError(t, err, 500)
}

// --- UpdateSession Tests ---

func TestUpdateSession_Success(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Old Name", Status: StatusPlanned, CampaignID: "camp-1"}, nil
		},
		updateFn: func(_ context.Context, s *Session) error {
			if s.Name != "New Name" {
				t.Errorf("expected updated name 'New Name', got %q", s.Name)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:   "New Name",
		Status: StatusPlanned,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSession_NotFound(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, _ string) (*Session, error) {
			return nil, apperror.NewNotFound("session not found")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.UpdateSession(context.Background(), "nonexistent", UpdateSessionInput{
		Name:   "Updated",
		Status: StatusPlanned,
	})
	assertAppError(t, err, 404)
}

func TestUpdateSession_EmptyName(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:   "",
		Status: StatusPlanned,
	})
	assertAppError(t, err, 400)
}

func TestUpdateSession_InvalidStatusDefaultsToPlanned(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Session", Status: StatusPlanned}, nil
		},
		updateFn: func(_ context.Context, s *Session) error {
			if s.Status != StatusPlanned {
				t.Errorf("expected invalid status to default to 'planned', got %q", s.Status)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:   "Session",
		Status: "invalid_status",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSession_RepoUpdateError(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Session", Status: StatusPlanned}, nil
		},
		updateFn: func(_ context.Context, _ *Session) error {
			return apperror.NewInternal(errors.New("db error"))
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:   "Updated",
		Status: StatusPlanned,
	})
	assertAppError(t, err, 500)
}

func TestUpdateSession_RecurrenceIntervalDefaultsToOne(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, Name: "Session", Status: StatusPlanned}, nil
		},
		updateFn: func(_ context.Context, s *Session) error {
			if s.RecurrenceInterval != 1 {
				t.Errorf("expected interval to default to 1, got %d", s.RecurrenceInterval)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	_, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:               "Session",
		Status:             StatusPlanned,
		RecurrenceInterval: 0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSession_CompletingRecurringSessionGeneratesNext(t *testing.T) {
	createCalled := false
	recType := RecurrenceWeekly
	scheduledDate := "2026-03-07"
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{
				ID:             id,
				CampaignID:     "camp-1",
				Name:           "Weekly Game",
				Status:         StatusPlanned,
				IsRecurring:    true,
				RecurrenceType: &recType,
				ScheduledDate:  &scheduledDate,
				CreatedBy:      "user-1",
			}, nil
		},
		updateFn: func(_ context.Context, _ *Session) error {
			return nil
		},
		createFn: func(_ context.Context, _ string, s *Session) error {
			createCalled = true
			// The next occurrence should be one week later.
			if s.ScheduledDate == nil || *s.ScheduledDate != "2026-03-14" {
				t.Errorf("expected next date '2026-03-14', got %v", s.ScheduledDate)
			}
			if s.Status != StatusPlanned {
				t.Errorf("expected new session to be planned, got %q", s.Status)
			}
			return nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return []Attendee{{UserID: "user-1"}, {UserID: "user-2"}}, nil
		},
	}
	svc := newTestSessionService(repo)

	nextSession, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:           "Weekly Game",
		Status:         StatusCompleted,
		IsRecurring:    true,
		RecurrenceType: &recType,
		ScheduledDate:  &scheduledDate,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCalled {
		t.Error("expected next occurrence to be created")
	}
	if nextSession == nil {
		t.Fatal("expected next session to be returned, got nil")
	}
}

func TestUpdateSession_CompletingNonRecurringReturnsNil(t *testing.T) {
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{
				ID:          id,
				Name:        "One-off Game",
				Status:      StatusPlanned,
				IsRecurring: false,
			}, nil
		},
	}
	svc := newTestSessionService(repo)

	nextSession, err := svc.UpdateSession(context.Background(), "sess-1", UpdateSessionInput{
		Name:   "One-off Game",
		Status: StatusCompleted,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextSession != nil {
		t.Errorf("expected nil for non-recurring completion, got %v", nextSession)
	}
}

// --- DeleteSession Tests ---

func TestDeleteSession_Success(t *testing.T) {
	deleted := false
	repo := &mockSessionRepo{
		deleteFn: func(_ context.Context, id string) error {
			deleted = true
			if id != "sess-1" {
				t.Errorf("expected delete ID 'sess-1', got %q", id)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	err := svc.DeleteSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected Delete to be called on repo")
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	repo := &mockSessionRepo{
		deleteFn: func(_ context.Context, _ string) error {
			return apperror.NewNotFound("session not found")
		},
	}
	svc := newTestSessionService(repo)
	err := svc.DeleteSession(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

func TestDeleteSession_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		deleteFn: func(_ context.Context, _ string) error {
			return errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	err := svc.DeleteSession(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- UpdateSessionRecap Tests ---

func TestUpdateSessionRecap_Success(t *testing.T) {
	recapCalled := false
	repo := &mockSessionRepo{
		updateRecapFn: func(_ context.Context, id string, recap, recapHTML *string) error {
			recapCalled = true
			if id != "sess-1" {
				t.Errorf("expected id 'sess-1', got %q", id)
			}
			if recap == nil || *recap != "Session recap content" {
				t.Errorf("expected recap content, got %v", recap)
			}
			if recapHTML == nil || *recapHTML != "<p>Session recap content</p>" {
				t.Errorf("expected recap HTML, got %v", recapHTML)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	recap := "Session recap content"
	recapHTML := "<p>Session recap content</p>"
	err := svc.UpdateSessionRecap(context.Background(), "sess-1", &recap, &recapHTML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recapCalled {
		t.Error("expected UpdateRecap to be called on repo")
	}
}

func TestUpdateSessionRecap_NilValues(t *testing.T) {
	repo := &mockSessionRepo{
		updateRecapFn: func(_ context.Context, _ string, recap, recapHTML *string) error {
			if recap != nil {
				t.Errorf("expected nil recap, got %v", recap)
			}
			if recapHTML != nil {
				t.Errorf("expected nil recapHTML, got %v", recapHTML)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	err := svc.UpdateSessionRecap(context.Background(), "sess-1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSessionRecap_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		updateRecapFn: func(_ context.Context, _ string, _, _ *string) error {
			return apperror.NewNotFound("session not found")
		},
	}
	svc := newTestSessionService(repo)

	recap := "content"
	err := svc.UpdateSessionRecap(context.Background(), "nonexistent", &recap, nil)
	assertAppError(t, err, 404)
}

// --- UpdateRSVP Tests ---

func TestUpdateRSVP_Success(t *testing.T) {
	validStatuses := []string{RSVPAccepted, RSVPDeclined, RSVPTentative}
	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			updateCalled := false
			repo := &mockSessionRepo{
				updateAttendeeStatusFn: func(_ context.Context, sessionID, userID, s string) error {
					updateCalled = true
					if sessionID != "sess-1" {
						t.Errorf("expected sessionID 'sess-1', got %q", sessionID)
					}
					if userID != "user-1" {
						t.Errorf("expected userID 'user-1', got %q", userID)
					}
					if s != status {
						t.Errorf("expected status %q, got %q", status, s)
					}
					return nil
				},
			}
			svc := newTestSessionService(repo)

			err := svc.UpdateRSVP(context.Background(), "sess-1", "user-1", status)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !updateCalled {
				t.Error("expected UpdateAttendeeStatus to be called on repo")
			}
		})
	}
}

func TestUpdateRSVP_InvalidStatus(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.UpdateRSVP(context.Background(), "sess-1", "user-1", "maybe")
	assertAppError(t, err, 400)
}

func TestUpdateRSVP_InvitedStatusIsInvalid(t *testing.T) {
	// "invited" is a valid attendee status but NOT a valid RSVP response.
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.UpdateRSVP(context.Background(), "sess-1", "user-1", RSVPInvited)
	assertAppError(t, err, 400)
}

func TestUpdateRSVP_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error {
			return apperror.NewNotFound("attendee not found")
		},
	}
	svc := newTestSessionService(repo)
	err := svc.UpdateRSVP(context.Background(), "sess-1", "user-1", RSVPAccepted)
	assertAppError(t, err, 404)
}

// --- ListAttendees Tests ---

func TestListAttendees_Success(t *testing.T) {
	repo := &mockSessionRepo{
		listAttendeesFn: func(_ context.Context, sessionID string) ([]Attendee, error) {
			return []Attendee{
				{UserID: "user-1", Status: RSVPAccepted, DisplayName: "Alice"},
				{UserID: "user-2", Status: RSVPDeclined, DisplayName: "Bob"},
			}, nil
		},
	}
	svc := newTestSessionService(repo)

	attendees, err := svc.ListAttendees(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attendees) != 2 {
		t.Errorf("expected 2 attendees, got %d", len(attendees))
	}
}

func TestListAttendees_Empty(t *testing.T) {
	repo := &mockSessionRepo{
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return nil, nil
		},
	}
	svc := newTestSessionService(repo)

	attendees, err := svc.ListAttendees(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attendees) != 0 {
		t.Errorf("expected 0 attendees, got %d", len(attendees))
	}
}

func TestListAttendees_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.ListAttendees(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- InviteAll Tests ---

func TestInviteAll_Success(t *testing.T) {
	invitedUsers := make(map[string]bool)
	repo := &mockSessionRepo{
		addAttendeeFn: func(_ context.Context, sessionID, userID, status string) error {
			invitedUsers[userID] = true
			if status != RSVPInvited {
				t.Errorf("expected status %q, got %q", RSVPInvited, status)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	err := svc.InviteAll(context.Background(), "sess-1", []string{"user-1", "user-2", "user-3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invitedUsers) != 3 {
		t.Errorf("expected 3 users invited, got %d", len(invitedUsers))
	}
}

func TestInviteAll_EmptyList(t *testing.T) {
	svc := newTestSessionService(&mockSessionRepo{})
	err := svc.InviteAll(context.Background(), "sess-1", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInviteAll_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		addAttendeeFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	err := svc.InviteAll(context.Background(), "sess-1", []string{"user-1"})
	assertAppError(t, err, 500)
}

// --- LinkEntity Tests ---

func TestLinkEntity_Success(t *testing.T) {
	linkCalled := false
	checker := &mockEntityChecker{
		belongsFn: func(_ context.Context, entityID, campaignID string) (bool, error) {
			return true, nil
		},
	}
	repo := &mockSessionRepo{
		linkEntityFn: func(_ context.Context, sessionID, entityID, role string) error {
			linkCalled = true
			if role != EntityRoleKey {
				t.Errorf("expected role %q, got %q", EntityRoleKey, role)
			}
			return nil
		},
	}
	svc := newTestSessionServiceWithChecker(repo, checker)

	err := svc.LinkEntity(context.Background(), "sess-1", "ent-1", EntityRoleKey, "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !linkCalled {
		t.Error("expected LinkEntity to be called on repo")
	}
}

func TestLinkEntity_EntityNotInCampaign(t *testing.T) {
	checker := &mockEntityChecker{
		belongsFn: func(_ context.Context, _, _ string) (bool, error) {
			return false, nil
		},
	}
	svc := newTestSessionServiceWithChecker(&mockSessionRepo{}, checker)

	err := svc.LinkEntity(context.Background(), "sess-1", "ent-1", EntityRoleKey, "camp-1")
	assertAppError(t, err, 400)
}

func TestLinkEntity_CheckerError(t *testing.T) {
	checker := &mockEntityChecker{
		belongsFn: func(_ context.Context, _, _ string) (bool, error) {
			return false, errors.New("db error")
		},
	}
	svc := newTestSessionServiceWithChecker(&mockSessionRepo{}, checker)

	err := svc.LinkEntity(context.Background(), "sess-1", "ent-1", EntityRoleKey, "camp-1")
	assertAppError(t, err, 500)
}

func TestLinkEntity_InvalidRoleDefaultsToMentioned(t *testing.T) {
	repo := &mockSessionRepo{
		linkEntityFn: func(_ context.Context, _, _, role string) error {
			if role != EntityRoleMentioned {
				t.Errorf("expected invalid role to default to %q, got %q", EntityRoleMentioned, role)
			}
			return nil
		},
	}
	// No checker so the campaign ownership check is skipped.
	svc := newTestSessionService(repo)

	err := svc.LinkEntity(context.Background(), "sess-1", "ent-1", "unknown_role", "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLinkEntity_ValidRoles(t *testing.T) {
	roles := []string{EntityRoleMentioned, EntityRoleEncountered, EntityRoleKey}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			repo := &mockSessionRepo{
				linkEntityFn: func(_ context.Context, _, _, r string) error {
					if r != role {
						t.Errorf("expected role %q, got %q", role, r)
					}
					return nil
				},
			}
			svc := newTestSessionService(repo)
			err := svc.LinkEntity(context.Background(), "sess-1", "ent-1", role, "camp-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- UnlinkEntity Tests ---

func TestUnlinkEntity_Success(t *testing.T) {
	unlinkCalled := false
	repo := &mockSessionRepo{
		unlinkEntityFn: func(_ context.Context, sessionID, entityID string) error {
			unlinkCalled = true
			return nil
		},
	}
	svc := newTestSessionService(repo)

	err := svc.UnlinkEntity(context.Background(), "sess-1", "ent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !unlinkCalled {
		t.Error("expected UnlinkEntity to be called on repo")
	}
}

func TestUnlinkEntity_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		unlinkEntityFn: func(_ context.Context, _, _ string) error {
			return errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	err := svc.UnlinkEntity(context.Background(), "sess-1", "ent-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- ListSessionEntities Tests ---

func TestListSessionEntities_Success(t *testing.T) {
	repo := &mockSessionRepo{
		listSessionEntitiesFn: func(_ context.Context, _ string) ([]SessionEntity, error) {
			return []SessionEntity{
				{EntityID: "ent-1", Role: EntityRoleKey, EntityName: "Dragon"},
				{EntityID: "ent-2", Role: EntityRoleMentioned, EntityName: "Innkeeper"},
			}, nil
		},
	}
	svc := newTestSessionService(repo)

	entities, err := svc.ListSessionEntities(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(entities))
	}
}

func TestListSessionEntities_Empty(t *testing.T) {
	repo := &mockSessionRepo{
		listSessionEntitiesFn: func(_ context.Context, _ string) ([]SessionEntity, error) {
			return nil, nil
		},
	}
	svc := newTestSessionService(repo)

	entities, err := svc.ListSessionEntities(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(entities))
	}
}

// --- SearchSessions Tests ---

func TestSearchSessions_Success(t *testing.T) {
	repo := &mockSessionRepo{
		searchByCampaignFn: func(_ context.Context, campaignID, query string) ([]Session, error) {
			return []Session{
				{ID: "sess-1", Name: "Dragon's Lair"},
			}, nil
		},
	}
	svc := newTestSessionService(repo)

	results, err := svc.SearchSessions(context.Background(), "camp-1", "dragon")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "Dragon's Lair" {
		t.Errorf("expected name 'Dragon's Lair', got %q", results[0]["name"])
	}
	if results[0]["type_name"] != "Session" {
		t.Errorf("expected type_name 'Session', got %q", results[0]["type_name"])
	}
	if results[0]["type_icon"] != "fa-dice-d20" {
		t.Errorf("expected type_icon 'fa-dice-d20', got %q", results[0]["type_icon"])
	}
	if results[0]["type_color"] != "#8b5cf6" {
		t.Errorf("expected type_color '#8b5cf6', got %q", results[0]["type_color"])
	}
	expectedURL := "/campaigns/camp-1/sessions/sess-1"
	if results[0]["url"] != expectedURL {
		t.Errorf("expected url %q, got %q", expectedURL, results[0]["url"])
	}
}

func TestSearchSessions_Empty(t *testing.T) {
	repo := &mockSessionRepo{
		searchByCampaignFn: func(_ context.Context, _, _ string) ([]Session, error) {
			return []Session{}, nil
		},
	}
	svc := newTestSessionService(repo)

	results, err := svc.SearchSessions(context.Background(), "camp-1", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchSessions_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		searchByCampaignFn: func(_ context.Context, _, _ string) ([]Session, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.SearchSessions(context.Background(), "camp-1", "query")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- ListSessionsForDateRange Tests ---

func TestListSessionsForDateRange_Success(t *testing.T) {
	repo := &mockSessionRepo{
		listByDateRangeFn: func(_ context.Context, campaignID, start, end string) ([]Session, error) {
			if start != "2026-03-01" || end != "2026-03-31" {
				t.Errorf("expected date range 2026-03-01 to 2026-03-31, got %s to %s", start, end)
			}
			return []Session{
				{ID: "sess-1", Name: "March Session"},
			}, nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) {
			return []Attendee{{UserID: "user-1"}}, nil
		},
	}
	svc := newTestSessionService(repo)

	sessions, err := svc.ListSessionsForDateRange(context.Background(), "camp-1", "2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions[0].Attendees) != 1 {
		t.Errorf("expected attendees loaded, got %d", len(sessions[0].Attendees))
	}
}

func TestListSessionsForDateRange_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		listByDateRangeFn: func(_ context.Context, _, _, _ string) ([]Session, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.ListSessionsForDateRange(context.Background(), "camp-1", "2026-03-01", "2026-03-31")
	assertAppError(t, err, 500)
}

// --- CreateRSVPTokens Tests ---

func TestCreateRSVPTokens_Success(t *testing.T) {
	tokensCreated := 0
	repo := &mockSessionRepo{
		createRSVPTokenFn: func(_ context.Context, token *RSVPToken) error {
			tokensCreated++
			if token.SessionID != "sess-1" {
				t.Errorf("expected session_id 'sess-1', got %q", token.SessionID)
			}
			if token.UserID != "user-1" {
				t.Errorf("expected user_id 'user-1', got %q", token.UserID)
			}
			if token.Action != RSVPAccepted && token.Action != RSVPDeclined {
				t.Errorf("expected action 'accepted' or 'declined', got %q", token.Action)
			}
			if token.Token == "" {
				t.Error("expected non-empty token")
			}
			// Verify expiry is roughly 7 days from now.
			expectedExpiry := time.Now().UTC().Add(7 * 24 * time.Hour)
			if token.ExpiresAt.Before(expectedExpiry.Add(-time.Minute)) || token.ExpiresAt.After(expectedExpiry.Add(time.Minute)) {
				t.Errorf("expected expiry near %v, got %v", expectedExpiry, token.ExpiresAt)
			}
			return nil
		},
	}
	svc := newTestSessionService(repo)

	acceptToken, declineToken, err := svc.CreateRSVPTokens(context.Background(), "sess-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acceptToken == "" {
		t.Error("expected non-empty accept token")
	}
	if declineToken == "" {
		t.Error("expected non-empty decline token")
	}
	if acceptToken == declineToken {
		t.Error("accept and decline tokens should be different")
	}
	if tokensCreated != 2 {
		t.Errorf("expected 2 tokens created, got %d", tokensCreated)
	}
}

func TestCreateRSVPTokens_RepoError(t *testing.T) {
	repo := &mockSessionRepo{
		createRSVPTokenFn: func(_ context.Context, _ *RSVPToken) error {
			return errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, _, err := svc.CreateRSVPTokens(context.Background(), "sess-1", "user-1")
	assertAppError(t, err, 500)
}

// --- RedeemRSVPToken Tests ---

func TestRedeemRSVPToken_Success(t *testing.T) {
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, tokenStr string) (*RSVPToken, error) {
			return &RSVPToken{
				Token:     tokenStr,
				SessionID: "sess-1",
				UserID:    "user-1",
				Action:    RSVPAccepted,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
				UsedAt:    nil,
			}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, sessionID, userID, status string) error {
			if status != RSVPAccepted {
				t.Errorf("expected status %q, got %q", RSVPAccepted, status)
			}
			return nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error {
			return nil
		},
	}
	svc := newTestSessionService(repo)

	token, err := svc.RedeemRSVPToken(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == nil {
		t.Fatal("expected token, got nil")
	}
	if token.Action != RSVPAccepted {
		t.Errorf("expected action %q, got %q", RSVPAccepted, token.Action)
	}
}

func TestRedeemRSVPToken_NotFound(t *testing.T) {
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return nil, apperror.NewNotFound("invalid or expired RSVP token")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.RedeemRSVPToken(context.Background(), "bad-token")
	assertAppError(t, err, 404)
}

func TestRedeemRSVPToken_AlreadyUsed(t *testing.T) {
	usedAt := time.Now().UTC().Add(-time.Hour)
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{
				Token:     "used-token",
				SessionID: "sess-1",
				UserID:    "user-1",
				Action:    RSVPAccepted,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
				UsedAt:    &usedAt,
			}, nil
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.RedeemRSVPToken(context.Background(), "used-token")
	assertAppError(t, err, 400)
}

func TestRedeemRSVPToken_Expired(t *testing.T) {
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{
				Token:     "expired-token",
				SessionID: "sess-1",
				UserID:    "user-1",
				Action:    RSVPAccepted,
				ExpiresAt: time.Now().UTC().Add(-24 * time.Hour), // Expired yesterday.
				UsedAt:    nil,
			}, nil
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.RedeemRSVPToken(context.Background(), "expired-token")
	assertAppError(t, err, 400)
}

func TestRedeemRSVPToken_UpdateAttendeeError(t *testing.T) {
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{
				Token:     "valid-token",
				SessionID: "sess-1",
				UserID:    "user-1",
				Action:    RSVPAccepted,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
				UsedAt:    nil,
			}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error {
			return apperror.NewNotFound("attendee not found")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.RedeemRSVPToken(context.Background(), "valid-token")
	assertAppError(t, err, 404)
}

func TestRedeemRSVPToken_MarkUsedError(t *testing.T) {
	repo := &mockSessionRepo{
		findRSVPTokenFn: func(_ context.Context, _ string) (*RSVPToken, error) {
			return &RSVPToken{
				Token:     "valid-token",
				SessionID: "sess-1",
				UserID:    "user-1",
				Action:    RSVPDeclined,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
				UsedAt:    nil,
			}, nil
		},
		updateAttendeeStatusFn: func(_ context.Context, _, _, _ string) error {
			return nil
		},
		markRSVPTokenUsedFn: func(_ context.Context, _ string) error {
			return errors.New("db error")
		},
	}
	svc := newTestSessionService(repo)
	_, err := svc.RedeemRSVPToken(context.Background(), "valid-token")
	assertAppError(t, err, 500)
}

// --- computeNextOccurrence Tests ---

func TestComputeNextOccurrence_Weekly(t *testing.T) {
	recType := RecurrenceWeekly
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:  &date,
		RecurrenceType: &recType,
	}
	next := computeNextOccurrence(session)
	if next != "2026-03-14" {
		t.Errorf("expected '2026-03-14', got %q", next)
	}
}

func TestComputeNextOccurrence_BiWeekly(t *testing.T) {
	recType := RecurrenceBiWeekly
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:  &date,
		RecurrenceType: &recType,
	}
	next := computeNextOccurrence(session)
	if next != "2026-03-21" {
		t.Errorf("expected '2026-03-21', got %q", next)
	}
}

func TestComputeNextOccurrence_Monthly(t *testing.T) {
	recType := RecurrenceMonthly
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:  &date,
		RecurrenceType: &recType,
	}
	next := computeNextOccurrence(session)
	if next != "2026-04-07" {
		t.Errorf("expected '2026-04-07', got %q", next)
	}
}

func TestComputeNextOccurrence_Custom(t *testing.T) {
	recType := RecurrenceCustom
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:      &date,
		RecurrenceType:     &recType,
		RecurrenceInterval: 3, // Every 3 weeks.
	}
	next := computeNextOccurrence(session)
	if next != "2026-03-28" {
		t.Errorf("expected '2026-03-28', got %q", next)
	}
}

func TestComputeNextOccurrence_CustomDefaultInterval(t *testing.T) {
	recType := RecurrenceCustom
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:      &date,
		RecurrenceType:     &recType,
		RecurrenceInterval: 0, // Should default to 1 week.
	}
	next := computeNextOccurrence(session)
	if next != "2026-03-14" {
		t.Errorf("expected '2026-03-14' for default interval, got %q", next)
	}
}

func TestComputeNextOccurrence_NilScheduledDate(t *testing.T) {
	recType := RecurrenceWeekly
	session := &Session{
		ScheduledDate:  nil,
		RecurrenceType: &recType,
	}
	next := computeNextOccurrence(session)
	if next != "" {
		t.Errorf("expected empty string for nil scheduled date, got %q", next)
	}
}

func TestComputeNextOccurrence_NilRecurrenceType(t *testing.T) {
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:  &date,
		RecurrenceType: nil,
	}
	next := computeNextOccurrence(session)
	if next != "" {
		t.Errorf("expected empty string for nil recurrence type, got %q", next)
	}
}

func TestComputeNextOccurrence_InvalidDate(t *testing.T) {
	recType := RecurrenceWeekly
	badDate := "not-a-date"
	session := &Session{
		ScheduledDate:  &badDate,
		RecurrenceType: &recType,
	}
	next := computeNextOccurrence(session)
	if next != "" {
		t.Errorf("expected empty string for invalid date, got %q", next)
	}
}

func TestComputeNextOccurrence_UnknownRecurrenceType(t *testing.T) {
	unknownType := "daily"
	date := "2026-03-07"
	session := &Session{
		ScheduledDate:  &date,
		RecurrenceType: &unknownType,
	}
	next := computeNextOccurrence(session)
	if next != "" {
		t.Errorf("expected empty string for unknown recurrence type, got %q", next)
	}
}

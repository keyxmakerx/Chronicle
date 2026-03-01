package sessions

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SessionService defines the business logic contract for sessions.
type SessionService interface {
	CreateSession(ctx context.Context, campaignID string, input CreateSessionInput) (*Session, error)
	GetSession(ctx context.Context, id string) (*Session, error)
	ListSessions(ctx context.Context, campaignID string) ([]Session, error)
	UpdateSession(ctx context.Context, id string, input UpdateSessionInput) error
	DeleteSession(ctx context.Context, id string) error

	// Attendees / RSVP.
	InviteAll(ctx context.Context, sessionID string, userIDs []string) error
	UpdateRSVP(ctx context.Context, sessionID, userID, status string) error
	ListAttendees(ctx context.Context, sessionID string) ([]Attendee, error)

	// Entity linking.
	LinkEntity(ctx context.Context, sessionID, entityID, role string) error
	UnlinkEntity(ctx context.Context, sessionID, entityID string) error
	ListSessionEntities(ctx context.Context, sessionID string) ([]SessionEntity, error)
}

// sessionService implements SessionService.
type sessionService struct {
	repo SessionRepository
}

// NewSessionService creates a new session service.
func NewSessionService(repo SessionRepository) SessionService {
	return &sessionService{repo: repo}
}

// CreateSession validates input and creates a new session.
func (s *sessionService) CreateSession(ctx context.Context, campaignID string, input CreateSessionInput) (*Session, error) {
	if input.Name == "" {
		return nil, apperror.NewBadRequest("session name is required")
	}
	if len(input.Name) > 200 {
		return nil, apperror.NewBadRequest("session name must be at most 200 characters")
	}

	session := &Session{
		ID:            generateUUID(),
		CampaignID:    campaignID,
		Name:          input.Name,
		Summary:       input.Summary,
		ScheduledDate: input.ScheduledDate,
		CalendarYear:  input.CalendarYear,
		CalendarMonth: input.CalendarMonth,
		CalendarDay:   input.CalendarDay,
		Status:        StatusPlanned,
		CreatedBy:     input.CreatedBy,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, campaignID, session); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating session: %w", err))
	}

	return session, nil
}

// GetSession retrieves a session by ID with attendees and entities.
func (s *sessionService) GetSession(ctx context.Context, id string) (*Session, error) {
	session, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Load attendees and entities.
	attendees, err := s.repo.ListAttendees(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading attendees: %w", err))
	}
	session.Attendees = attendees

	entities, err := s.repo.ListSessionEntities(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("loading entities: %w", err))
	}
	session.Entities = entities

	return session, nil
}

// ListSessions returns all sessions for a campaign.
func (s *sessionService) ListSessions(ctx context.Context, campaignID string) ([]Session, error) {
	sessions, err := s.repo.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing sessions: %w", err))
	}

	// Load attendee counts for each session.
	for i := range sessions {
		attendees, err := s.repo.ListAttendees(ctx, sessions[i].ID)
		if err == nil {
			sessions[i].Attendees = attendees
		}
	}

	return sessions, nil
}

// UpdateSession validates and updates a session.
func (s *sessionService) UpdateSession(ctx context.Context, id string, input UpdateSessionInput) error {
	if input.Name == "" {
		return apperror.NewBadRequest("session name is required")
	}

	// Validate status.
	switch input.Status {
	case StatusPlanned, StatusCompleted, StatusCancelled:
		// Valid.
	default:
		input.Status = StatusPlanned
	}

	session, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	session.Name = input.Name
	session.Summary = input.Summary
	session.ScheduledDate = input.ScheduledDate
	session.CalendarYear = input.CalendarYear
	session.CalendarMonth = input.CalendarMonth
	session.CalendarDay = input.CalendarDay
	session.Status = input.Status

	return s.repo.Update(ctx, session)
}

// DeleteSession removes a session.
func (s *sessionService) DeleteSession(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// --- Attendees / RSVP ---

// InviteAll adds multiple users as attendees with "invited" status.
func (s *sessionService) InviteAll(ctx context.Context, sessionID string, userIDs []string) error {
	for _, userID := range userIDs {
		if err := s.repo.AddAttendee(ctx, sessionID, userID, RSVPInvited); err != nil {
			return apperror.NewInternal(fmt.Errorf("inviting user %s: %w", userID, err))
		}
	}
	return nil
}

// UpdateRSVP updates a user's attendance status for a session.
func (s *sessionService) UpdateRSVP(ctx context.Context, sessionID, userID, status string) error {
	switch status {
	case RSVPAccepted, RSVPDeclined, RSVPTentative:
		// Valid.
	default:
		return apperror.NewBadRequest("invalid RSVP status: must be accepted, declined, or tentative")
	}

	return s.repo.UpdateAttendeeStatus(ctx, sessionID, userID, status)
}

// ListAttendees returns attendees for a session.
func (s *sessionService) ListAttendees(ctx context.Context, sessionID string) ([]Attendee, error) {
	return s.repo.ListAttendees(ctx, sessionID)
}

// --- Entity Linking ---

// LinkEntity links an entity to a session.
func (s *sessionService) LinkEntity(ctx context.Context, sessionID, entityID, role string) error {
	switch role {
	case EntityRoleMentioned, EntityRoleEncountered, EntityRoleKey:
		// Valid.
	default:
		role = EntityRoleMentioned
	}
	return s.repo.LinkEntity(ctx, sessionID, entityID, role)
}

// UnlinkEntity removes an entity link from a session.
func (s *sessionService) UnlinkEntity(ctx context.Context, sessionID, entityID string) error {
	return s.repo.UnlinkEntity(ctx, sessionID, entityID)
}

// ListSessionEntities returns entities linked to a session.
func (s *sessionService) ListSessionEntities(ctx context.Context, sessionID string) ([]SessionEntity, error) {
	return s.repo.ListSessionEntities(ctx, sessionID)
}

// generateUUID creates a v4 UUID.
func generateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

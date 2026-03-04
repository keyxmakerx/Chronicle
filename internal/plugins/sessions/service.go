package sessions

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// EntityCampaignChecker verifies that an entity belongs to a given campaign.
// Used to prevent cross-campaign entity linking (IDOR prevention).
type EntityCampaignChecker interface {
	EntityBelongsToCampaign(ctx context.Context, entityID, campaignID string) (bool, error)
}

// SessionService defines the business logic contract for sessions.
type SessionService interface {
	CreateSession(ctx context.Context, campaignID string, input CreateSessionInput) (*Session, error)
	GetSession(ctx context.Context, id string) (*Session, error)
	ListSessions(ctx context.Context, campaignID string) ([]Session, error)
	ListSessionsForDateRange(ctx context.Context, campaignID, startDate, endDate string) ([]Session, error)
	UpdateSession(ctx context.Context, id string, input UpdateSessionInput) error
	DeleteSession(ctx context.Context, id string) error
	SearchSessions(ctx context.Context, campaignID, query string) ([]map[string]string, error)

	// Attendees / RSVP.
	InviteAll(ctx context.Context, sessionID string, userIDs []string) error
	UpdateRSVP(ctx context.Context, sessionID, userID, status string) error
	ListAttendees(ctx context.Context, sessionID string) ([]Attendee, error)

	// RSVP tokens for email-based responses.
	CreateRSVPTokens(ctx context.Context, sessionID, userID string) (acceptToken, declineToken string, err error)
	RedeemRSVPToken(ctx context.Context, tokenStr string) (*RSVPToken, error)

	// Entity linking. campaignID is used to verify the entity belongs to the
	// same campaign as the session, preventing cross-campaign IDOR attacks.
	LinkEntity(ctx context.Context, sessionID, entityID, role, campaignID string) error
	UnlinkEntity(ctx context.Context, sessionID, entityID string) error
	ListSessionEntities(ctx context.Context, sessionID string) ([]SessionEntity, error)
}

// sessionService implements SessionService.
type sessionService struct {
	repo           SessionRepository
	entityChecker  EntityCampaignChecker
}

// NewSessionService creates a new session service. The EntityCampaignChecker
// is used to verify entities belong to the correct campaign when linking,
// preventing cross-campaign IDOR attacks.
func NewSessionService(repo SessionRepository, ec EntityCampaignChecker) SessionService {
	return &sessionService{repo: repo, entityChecker: ec}
}

// CreateSession validates input and creates a new session.
func (s *sessionService) CreateSession(ctx context.Context, campaignID string, input CreateSessionInput) (*Session, error) {
	if input.Name == "" {
		return nil, apperror.NewBadRequest("session name is required")
	}
	if len(input.Name) > 200 {
		return nil, apperror.NewBadRequest("session name must be at most 200 characters")
	}

	// Validate recurrence settings.
	if input.IsRecurring && input.RecurrenceType != nil {
		switch *input.RecurrenceType {
		case RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly, RecurrenceCustom:
			// Valid.
		default:
			return nil, apperror.NewBadRequest("invalid recurrence type")
		}
	}

	interval := input.RecurrenceInterval
	if interval < 1 {
		interval = 1
	}

	session := &Session{
		ID:                  generateUUID(),
		CampaignID:          campaignID,
		Name:                input.Name,
		Summary:             input.Summary,
		ScheduledDate:       input.ScheduledDate,
		CalendarYear:        input.CalendarYear,
		CalendarMonth:       input.CalendarMonth,
		CalendarDay:         input.CalendarDay,
		Status:              StatusPlanned,
		IsRecurring:         input.IsRecurring,
		RecurrenceType:      input.RecurrenceType,
		RecurrenceInterval:  interval,
		RecurrenceDayOfWeek: input.RecurrenceDayOfWeek,
		RecurrenceEndDate:   input.RecurrenceEndDate,
		CreatedBy:           input.CreatedBy,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
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
	session.IsRecurring = input.IsRecurring
	session.RecurrenceType = input.RecurrenceType
	session.RecurrenceInterval = input.RecurrenceInterval
	if session.RecurrenceInterval < 1 {
		session.RecurrenceInterval = 1
	}
	session.RecurrenceDayOfWeek = input.RecurrenceDayOfWeek
	session.RecurrenceEndDate = input.RecurrenceEndDate

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

// LinkEntity links an entity to a session. Verifies the entity belongs to the
// same campaign to prevent cross-campaign IDOR attacks.
func (s *sessionService) LinkEntity(ctx context.Context, sessionID, entityID, role, campaignID string) error {
	// Verify entity belongs to the same campaign as the session.
	if s.entityChecker != nil {
		belongs, err := s.entityChecker.EntityBelongsToCampaign(ctx, entityID, campaignID)
		if err != nil {
			return apperror.NewInternal(fmt.Errorf("checking entity campaign: %w", err))
		}
		if !belongs {
			return apperror.NewBadRequest("entity does not belong to this campaign")
		}
	}

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

// SearchSessions returns sessions matching a query for the quick search system.
// Results are formatted to match the entity search JSON format.
func (s *sessionService) SearchSessions(ctx context.Context, campaignID, query string) ([]map[string]string, error) {
	sessions, err := s.repo.SearchByCampaign(ctx, campaignID, query)
	if err != nil {
		return nil, fmt.Errorf("search sessions: %w", err)
	}

	results := make([]map[string]string, 0, len(sessions))
	for _, sess := range sessions {
		results = append(results, map[string]string{
			"id":         sess.ID,
			"name":       sess.Name,
			"type_name":  "Session",
			"type_icon":  "fa-dice-d20",
			"type_color": "#8b5cf6",
			"url":        fmt.Sprintf("/campaigns/%s/sessions/%s", campaignID, sess.ID),
		})
	}
	return results, nil
}

// ListSessionsForDateRange returns sessions for a campaign that overlap the
// given date range. Used by the calendar plugin to display sessions on the grid.
func (s *sessionService) ListSessionsForDateRange(ctx context.Context, campaignID, startDate, endDate string) ([]Session, error) {
	sessions, err := s.repo.ListByDateRange(ctx, campaignID, startDate, endDate)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing sessions for date range: %w", err))
	}
	// Load attendees for each session.
	for i := range sessions {
		attendees, err := s.repo.ListAttendees(ctx, sessions[i].ID)
		if err == nil {
			sessions[i].Attendees = attendees
		}
	}
	return sessions, nil
}

// CreateRSVPTokens generates accept and decline tokens for a session invitation.
// Tokens are single-use and expire in 7 days.
func (s *sessionService) CreateRSVPTokens(ctx context.Context, sessionID, userID string) (string, string, error) {
	acceptToken := generateToken()
	declineToken := generateToken()
	now := time.Now().UTC()
	expires := now.Add(7 * 24 * time.Hour)

	for _, tok := range []struct {
		token  string
		action string
	}{
		{acceptToken, RSVPAccepted},
		{declineToken, RSVPDeclined},
	} {
		if err := s.repo.CreateRSVPToken(ctx, &RSVPToken{
			Token:     tok.token,
			SessionID: sessionID,
			UserID:    userID,
			Action:    tok.action,
			ExpiresAt: expires,
			CreatedAt: now,
		}); err != nil {
			return "", "", apperror.NewInternal(fmt.Errorf("creating rsvp token: %w", err))
		}
	}

	return acceptToken, declineToken, nil
}

// RedeemRSVPToken validates and applies an RSVP token, updating the user's attendance.
func (s *sessionService) RedeemRSVPToken(ctx context.Context, tokenStr string) (*RSVPToken, error) {
	token, err := s.repo.FindRSVPToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}

	if token.UsedAt != nil {
		return nil, apperror.NewBadRequest("this RSVP link has already been used")
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return nil, apperror.NewBadRequest("this RSVP link has expired")
	}

	// Apply the RSVP.
	if err := s.repo.UpdateAttendeeStatus(ctx, token.SessionID, token.UserID, token.Action); err != nil {
		return nil, err
	}

	// Mark token as used.
	if err := s.repo.MarkRSVPTokenUsed(ctx, tokenStr); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("marking token used: %w", err))
	}

	return token, nil
}

// generateToken creates a secure random token for RSVP email links.
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
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

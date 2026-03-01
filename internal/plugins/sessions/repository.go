package sessions

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SessionRepository defines the data access contract for session operations.
type SessionRepository interface {
	Create(ctx context.Context, campaignID string, s *Session) error
	FindByID(ctx context.Context, id string) (*Session, error)
	ListByCampaign(ctx context.Context, campaignID string) ([]Session, error)
	Update(ctx context.Context, s *Session) error
	Delete(ctx context.Context, id string) error

	// Attendee management.
	AddAttendee(ctx context.Context, sessionID, userID, status string) error
	UpdateAttendeeStatus(ctx context.Context, sessionID, userID, status string) error
	RemoveAttendee(ctx context.Context, sessionID, userID string) error
	ListAttendees(ctx context.Context, sessionID string) ([]Attendee, error)

	// Entity linking.
	LinkEntity(ctx context.Context, sessionID, entityID, role string) error
	UnlinkEntity(ctx context.Context, sessionID, entityID string) error
	ListSessionEntities(ctx context.Context, sessionID string) ([]SessionEntity, error)
}

// sessionRepository implements SessionRepository with MariaDB queries.
type sessionRepository struct {
	db *sql.DB
}

// NewSessionRepository creates a new session repository.
func NewSessionRepository(db *sql.DB) SessionRepository {
	return &sessionRepository{db: db}
}

// Create inserts a new session.
func (r *sessionRepository) Create(ctx context.Context, campaignID string, s *Session) error {
	query := `INSERT INTO sessions
		(id, campaign_id, name, summary, scheduled_date, calendar_year, calendar_month,
		 calendar_day, status, sort_order, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		s.ID, campaignID, s.Name, s.Summary, s.ScheduledDate,
		s.CalendarYear, s.CalendarMonth, s.CalendarDay,
		s.Status, s.SortOrder, s.CreatedBy, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// FindByID retrieves a session by its UUID.
func (r *sessionRepository) FindByID(ctx context.Context, id string) (*Session, error) {
	query := `SELECT s.id, s.campaign_id, s.name, s.summary, s.notes, s.notes_html,
	                 s.scheduled_date, s.calendar_year, s.calendar_month, s.calendar_day,
	                 s.status, s.sort_order, s.created_by, s.created_at, s.updated_at,
	                 u.display_name
	          FROM sessions s
	          LEFT JOIN users u ON u.id = s.created_by
	          WHERE s.id = ?`

	s := &Session{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.CampaignID, &s.Name, &s.Summary, &s.Notes, &s.NotesHTML,
		&s.ScheduledDate, &s.CalendarYear, &s.CalendarMonth, &s.CalendarDay,
		&s.Status, &s.SortOrder, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		&s.CreatorName,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying session by id: %w", err)
	}
	return s, nil
}

// ListByCampaign returns all sessions for a campaign, ordered by scheduled date
// descending (most recent first), then by sort_order.
func (r *sessionRepository) ListByCampaign(ctx context.Context, campaignID string) ([]Session, error) {
	query := `SELECT s.id, s.campaign_id, s.name, s.summary,
	                 s.scheduled_date, s.calendar_year, s.calendar_month, s.calendar_day,
	                 s.status, s.sort_order, s.created_by, s.created_at, s.updated_at,
	                 u.display_name
	          FROM sessions s
	          LEFT JOIN users u ON u.id = s.created_by
	          WHERE s.campaign_id = ?
	          ORDER BY CASE s.status
	              WHEN 'planned' THEN 0
	              WHEN 'completed' THEN 1
	              WHEN 'cancelled' THEN 2
	          END,
	          s.scheduled_date IS NULL, s.scheduled_date ASC,
	          s.sort_order ASC, s.created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.ID, &s.CampaignID, &s.Name, &s.Summary,
			&s.ScheduledDate, &s.CalendarYear, &s.CalendarMonth, &s.CalendarDay,
			&s.Status, &s.SortOrder, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&s.CreatorName,
		); err != nil {
			return nil, fmt.Errorf("scanning session row: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// Update updates a session's editable fields.
func (r *sessionRepository) Update(ctx context.Context, s *Session) error {
	query := `UPDATE sessions SET
		name = ?, summary = ?, notes = ?, notes_html = ?,
		scheduled_date = ?, calendar_year = ?, calendar_month = ?, calendar_day = ?,
		status = ?, updated_at = ?
		WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		s.Name, s.Summary, s.Notes, s.NotesHTML,
		s.ScheduledDate, s.CalendarYear, s.CalendarMonth, s.CalendarDay,
		s.Status, time.Now().UTC(), s.ID,
	)
	if err != nil {
		return fmt.Errorf("updating session: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("session not found")
	}
	return nil
}

// Delete removes a session by ID.
func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("session not found")
	}
	return nil
}

// --- Attendee Management ---

// AddAttendee invites a user to a session.
func (r *sessionRepository) AddAttendee(ctx context.Context, sessionID, userID, status string) error {
	query := `INSERT INTO session_attendees (session_id, user_id, status)
	          VALUES (?, ?, ?)
	          ON DUPLICATE KEY UPDATE status = VALUES(status), responded_at = NOW()`

	_, err := r.db.ExecContext(ctx, query, sessionID, userID, status)
	if err != nil {
		return fmt.Errorf("adding attendee: %w", err)
	}
	return nil
}

// UpdateAttendeeStatus updates a user's RSVP status.
func (r *sessionRepository) UpdateAttendeeStatus(ctx context.Context, sessionID, userID, status string) error {
	query := `UPDATE session_attendees SET status = ?, responded_at = NOW()
	          WHERE session_id = ? AND user_id = ?`

	result, err := r.db.ExecContext(ctx, query, status, sessionID, userID)
	if err != nil {
		return fmt.Errorf("updating attendee status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("attendee not found")
	}
	return nil
}

// RemoveAttendee removes a user from a session's attendee list.
func (r *sessionRepository) RemoveAttendee(ctx context.Context, sessionID, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM session_attendees WHERE session_id = ? AND user_id = ?`,
		sessionID, userID,
	)
	if err != nil {
		return fmt.Errorf("removing attendee: %w", err)
	}
	return nil
}

// ListAttendees returns all attendees for a session with user display data.
func (r *sessionRepository) ListAttendees(ctx context.Context, sessionID string) ([]Attendee, error) {
	query := `SELECT sa.id, sa.session_id, sa.user_id, sa.status, sa.responded_at,
	                 u.display_name, u.avatar_path
	          FROM session_attendees sa
	          INNER JOIN users u ON u.id = sa.user_id
	          WHERE sa.session_id = ?
	          ORDER BY FIELD(sa.status, 'accepted', 'tentative', 'invited', 'declined'),
	                   u.display_name`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing attendees: %w", err)
	}
	defer rows.Close()

	var attendees []Attendee
	for rows.Next() {
		var a Attendee
		if err := rows.Scan(
			&a.ID, &a.SessionID, &a.UserID, &a.Status, &a.RespondedAt,
			&a.DisplayName, &a.AvatarPath,
		); err != nil {
			return nil, fmt.Errorf("scanning attendee row: %w", err)
		}
		attendees = append(attendees, a)
	}
	return attendees, rows.Err()
}

// --- Entity Linking ---

// LinkEntity links an entity to a session with a role.
func (r *sessionRepository) LinkEntity(ctx context.Context, sessionID, entityID, role string) error {
	query := `INSERT INTO session_entities (session_id, entity_id, role)
	          VALUES (?, ?, ?)
	          ON DUPLICATE KEY UPDATE role = VALUES(role)`

	_, err := r.db.ExecContext(ctx, query, sessionID, entityID, role)
	if err != nil {
		return fmt.Errorf("linking entity: %w", err)
	}
	return nil
}

// UnlinkEntity removes an entity from a session.
func (r *sessionRepository) UnlinkEntity(ctx context.Context, sessionID, entityID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM session_entities WHERE session_id = ? AND entity_id = ?`,
		sessionID, entityID,
	)
	if err != nil {
		return fmt.Errorf("unlinking entity: %w", err)
	}
	return nil
}

// ListSessionEntities returns all entities linked to a session.
func (r *sessionRepository) ListSessionEntities(ctx context.Context, sessionID string) ([]SessionEntity, error) {
	query := `SELECT se.id, se.session_id, se.entity_id, se.role,
	                 e.name, e.slug
	          FROM session_entities se
	          INNER JOIN entities e ON e.id = se.entity_id
	          WHERE se.session_id = ?
	          ORDER BY FIELD(se.role, 'key', 'encountered', 'mentioned'), e.name`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing session entities: %w", err)
	}
	defer rows.Close()

	var entities []SessionEntity
	for rows.Next() {
		var se SessionEntity
		if err := rows.Scan(
			&se.ID, &se.SessionID, &se.EntityID, &se.Role,
			&se.EntityName, &se.EntitySlug,
		); err != nil {
			return nil, fmt.Errorf("scanning session entity row: %w", err)
		}
		entities = append(entities, se)
	}
	return entities, rows.Err()
}

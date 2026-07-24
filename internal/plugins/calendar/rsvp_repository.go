package calendar

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Persistence for calendar-event RSVPs (C-CAL-RSVP-P1). Its OWN repository over
// the shared *sql.DB — deliberately NOT the calendarRepository — so the RSVP
// lane stays disjoint from the CalendarService/CalendarRepository surface (and
// from the parallel entity-ties leak fix that touches those shared interfaces).
// Every table it owns was added by migration 013.

// RSVPRepository is the RSVP persistence boundary (house rule: interface at the
// repo seam for testability).
type RSVPRepository interface {
	// UpsertRSVP inserts or updates one member's RSVP (UNIQUE(event_id,user_id)).
	// A nil note leaves any existing note untouched; a non-nil note replaces it.
	UpsertRSVP(ctx context.Context, eventID, userID, status string, note *string) error
	// GetMyRSVP returns a user's own RSVP for an event, or (nil, nil) if none.
	GetMyRSVP(ctx context.Context, eventID, userID string) (*EventRSVP, error)
	// CountRSVPs returns the yes/maybe/no aggregate for an event.
	CountRSVPs(ctx context.Context, eventID string) (RSVPCounts, error)
	// ListRSVPs returns every RSVP for an event with the responder's display
	// name/avatar joined (the Owner/co-DM per-person breakdown).
	ListRSVPs(ctx context.Context, eventID string) ([]EventRSVP, error)

	// collect_rsvps opt-in flag lives on calendar_events (migration 013). The
	// RSVP repo owns its read/write so the column never has to thread through the
	// large event CRUD path.
	GetCollectRSVPs(ctx context.Context, eventID string) (bool, error)
	SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) error

	// Emailed one-click token lifecycle (single-use + expiring).
	CreateToken(ctx context.Context, eventID, userID, action, token string, expiresAt time.Time) error
	GetToken(ctx context.Context, token string) (*EventRSVPToken, error)
	MarkTokenUsed(ctx context.Context, id int) error
}

// rsvpRepo is the concrete RSVPRepository over MariaDB.
type rsvpRepo struct {
	db *sql.DB
}

// NewRSVPRepository constructs the RSVP repository over the shared DB handle.
func NewRSVPRepository(db *sql.DB) RSVPRepository {
	return &rsvpRepo{db: db}
}

// UpsertRSVP writes the member's RSVP. INSERT ... ON DUPLICATE KEY UPDATE keeps
// it a single round-trip; the UNIQUE(event_id,user_id) index is the upsert key.
// note is COALESCE-preserved when nil so a plain yes/maybe/no click never wipes
// a previously-suggested time.
func (r *rsvpRepo) UpsertRSVP(ctx context.Context, eventID, userID, status string, note *string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_rsvps (event_id, user_id, status, note, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   status = VALUES(status),
		   note = COALESCE(VALUES(note), note),
		   updated_at = VALUES(updated_at)`,
		eventID, userID, status, note, now)
	if err != nil {
		return fmt.Errorf("upserting rsvp: %w", err)
	}
	return nil
}

// GetMyRSVP returns the caller's own RSVP row, or (nil, nil) when absent.
func (r *rsvpRepo) GetMyRSVP(ctx context.Context, eventID, userID string) (*EventRSVP, error) {
	var rs EventRSVP
	var note sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT event_id, user_id, status, note, updated_at
		 FROM calendar_event_rsvps WHERE event_id = ? AND user_id = ?`,
		eventID, userID).Scan(&rs.EventID, &rs.UserID, &rs.Status, &note, &rs.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting rsvp: %w", err)
	}
	if note.Valid {
		rs.Note = &note.String
	}
	return &rs, nil
}

// CountRSVPs aggregates the three statuses in one grouped query.
func (r *rsvpRepo) CountRSVPs(ctx context.Context, eventID string) (RSVPCounts, error) {
	var counts RSVPCounts
	rows, err := r.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM calendar_event_rsvps WHERE event_id = ? GROUP BY status`,
		eventID)
	if err != nil {
		return counts, fmt.Errorf("counting rsvps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return counts, fmt.Errorf("scanning rsvp count: %w", err)
		}
		switch status {
		case RSVPStatusYes:
			counts.Yes = n
		case RSVPStatusMaybe:
			counts.Maybe = n
		case RSVPStatusNo:
			counts.No = n
		}
	}
	return counts, rows.Err()
}

// ListRSVPs returns every RSVP for an event, joined to users for display. Used
// ONLY on the Owner/co-DM detail path — the handler gates the call.
func (r *rsvpRepo) ListRSVPs(ctx context.Context, eventID string) ([]EventRSVP, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.event_id, r.user_id, r.status, r.note, r.updated_at,
		        u.display_name, u.avatar_path
		 FROM calendar_event_rsvps r
		 JOIN users u ON u.id = r.user_id
		 WHERE r.event_id = ?
		 ORDER BY FIELD(r.status,'yes','maybe','no'), u.display_name`,
		eventID)
	if err != nil {
		return nil, fmt.Errorf("listing rsvps: %w", err)
	}
	defer rows.Close()
	var out []EventRSVP
	for rows.Next() {
		var rs EventRSVP
		var note, avatar sql.NullString
		if err := rows.Scan(&rs.EventID, &rs.UserID, &rs.Status, &note, &rs.UpdatedAt,
			&rs.DisplayName, &avatar); err != nil {
			return nil, fmt.Errorf("scanning rsvp: %w", err)
		}
		if note.Valid {
			rs.Note = &note.String
		}
		if avatar.Valid {
			rs.AvatarPath = &avatar.String
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

// GetCollectRSVPs reads the per-event opt-in flag.
func (r *rsvpRepo) GetCollectRSVPs(ctx context.Context, eventID string) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT collect_rsvps FROM calendar_events WHERE id = ?`, eventID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading collect_rsvps: %w", err)
	}
	return enabled, nil
}

// SetCollectRSVPs flips the per-event opt-in flag.
func (r *rsvpRepo) SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events SET collect_rsvps = ? WHERE id = ?`, enabled, eventID)
	if err != nil {
		return fmt.Errorf("setting collect_rsvps: %w", err)
	}
	return nil
}

// CreateToken inserts one single-use emailed RSVP token.
func (r *rsvpRepo) CreateToken(ctx context.Context, eventID, userID, action, token string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_rsvp_tokens (token, event_id, user_id, action, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		token, eventID, userID, action, expiresAt, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("creating rsvp token: %w", err)
	}
	return nil
}

// GetToken loads a token row by its opaque value (validity checks live in the
// service so both the GET-confirm and POST-apply halves share one rule).
func (r *rsvpRepo) GetToken(ctx context.Context, token string) (*EventRSVPToken, error) {
	var t EventRSVPToken
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, token, event_id, user_id, action, used_at, expires_at, created_at
		 FROM calendar_event_rsvp_tokens WHERE token = ?`, token).
		Scan(&t.ID, &t.Token, &t.EventID, &t.UserID, &t.Action, &usedAt, &t.ExpiresAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting rsvp token: %w", err)
	}
	if usedAt.Valid {
		t.UsedAt = &usedAt.Time
	}
	return &t, nil
}

// MarkTokenUsed consumes a token so it can never be replayed. Scoped to the
// still-unused row so a concurrent double-submit can't double-apply.
func (r *rsvpRepo) MarkTokenUsed(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_event_rsvp_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("marking rsvp token used: %w", err)
	}
	return nil
}

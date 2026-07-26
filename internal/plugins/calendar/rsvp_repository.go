package calendar

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RSVPRepository owns every query against the two RSVP tables plus the single
// calendar_events.collect_rsvps column.
//
// It is a SEPARATE repository from CalendarRepository on purpose (C-CAL-RSVP-P1
// "code isolation"): CalendarRepository is a ~60-method interface consumed by
// the syncapi stub and mirrored by a hand-written mock, so widening it makes
// this lane collide with every other lane touching the calendar aggregate. A
// narrow, self-contained interface keeps the blast radius to this file.
type RSVPRepository interface {
	// Responses.
	UpsertRSVP(ctx context.Context, r *EventRSVP) error
	SetRSVPNote(ctx context.Context, eventID, userID, note string) error
	GetUserRSVP(ctx context.Context, eventID, userID string) (*EventRSVP, error)
	ListRSVPsForEvent(ctx context.Context, eventID string) ([]EventRSVP, error)

	// Per-event opt-in flag. Read lives on the Event aggregate (collect_rsvps is
	// part of eventCols); this is the write half, deliberately kept OFF the
	// shared UpdateEvent path so the drawer's lossless quick-save — which
	// re-sends the whole stored event shape — can never clobber the flag.
	SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) error

	// Tokens.
	CreateRSVPToken(ctx context.Context, t *EventRSVPToken) error
	FindRSVPToken(ctx context.Context, token string) (*EventRSVPToken, error)
	MarkRSVPTokenUsed(ctx context.Context, token string) error
}

// rsvpRepo is the MariaDB implementation of RSVPRepository.
type rsvpRepo struct {
	db *sql.DB
}

// NewRSVPRepository creates the RSVP repository.
func NewRSVPRepository(db *sql.DB) RSVPRepository {
	return &rsvpRepo{db: db}
}

// UpsertRSVP writes the member's answer, replacing any previous one.
//
// ON DUPLICATE KEY on uq_cal_event_rsvp(event_id, user_id) is what makes
// re-answering idempotent: one member can never contribute two rows to a count,
// and there is no response history to leak. The note is only overwritten when a
// non-nil one is supplied, so answering "yes" after leaving a "suggest" note
// keeps the note (VALUES(note) would blank it).
func (r *rsvpRepo) UpsertRSVP(ctx context.Context, rec *EventRSVP) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_rsvps (id, event_id, user_id, status, note, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		     status     = VALUES(status),
		     note       = COALESCE(VALUES(note), note),
		     updated_at = VALUES(updated_at)`,
		rec.ID, rec.EventID, rec.UserID, rec.Status, rec.Note, rec.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upserting event rsvp: %w", err)
	}
	return nil
}

// SetRSVPNote attaches a note WITHOUT changing the member's status.
//
// This is the "suggest another time" write: a member who already answered
// "maybe" and then suggests a better time stays "maybe". If they have no row
// yet, one is created as 'maybe' — the honest reading of "I can't do this slot,
// try another" is an unsettled answer, not a decline.
func (r *rsvpRepo) SetRSVPNote(ctx context.Context, eventID, userID, note string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_rsvps (id, event_id, user_id, status, note, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE note = VALUES(note), updated_at = VALUES(updated_at)`,
		generateRSVPID(), eventID, userID, RSVPMaybe, note, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("setting event rsvp note: %w", err)
	}
	return nil
}

// GetUserRSVP returns one member's own answer, or (nil, nil) if they have none.
func (r *rsvpRepo) GetUserRSVP(ctx context.Context, eventID, userID string) (*EventRSVP, error) {
	rec := &EventRSVP{}
	var note sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, event_id, user_id, status, note, updated_at
		 FROM calendar_event_rsvps WHERE event_id = ? AND user_id = ?`, eventID, userID).
		Scan(&rec.ID, &rec.EventID, &rec.UserID, &rec.Status, &note, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting event rsvp: %w", err)
	}
	if note.Valid {
		rec.Note = &note.String
	}
	return rec, nil
}

// ListRSVPsForEvent returns every answer for an event.
//
// Deliberately returns raw rows (user IDs, not display names): resolving a name
// is a campaigns concern, so the handler maps IDs through campaigns.MemberLister
// (rule 8) instead of this query joining the core users table. That also means a
// member who has since LEFT the campaign simply drops out of the detail list.
func (r *rsvpRepo) ListRSVPsForEvent(ctx context.Context, eventID string) ([]EventRSVP, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, event_id, user_id, status, note, updated_at
		 FROM calendar_event_rsvps WHERE event_id = ? ORDER BY updated_at ASC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("listing event rsvps: %w", err)
	}
	defer rows.Close()

	out := make([]EventRSVP, 0, 8)
	for rows.Next() {
		var rec EventRSVP
		var note sql.NullString
		if err := rows.Scan(&rec.ID, &rec.EventID, &rec.UserID, &rec.Status, &note, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning event rsvp: %w", err)
		}
		if note.Valid {
			rec.Note = &note.String
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// SetCollectRSVPs flips the per-event opt-in.
func (r *rsvpRepo) SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events SET collect_rsvps = ? WHERE id = ?`, enabled, eventID)
	if err != nil {
		return fmt.Errorf("setting collect_rsvps: %w", err)
	}
	return nil
}

// CreateRSVPToken inserts one emailed action link.
func (r *rsvpRepo) CreateRSVPToken(ctx context.Context, t *EventRSVPToken) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_rsvp_tokens (token, event_id, user_id, action, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.Token, t.EventID, t.UserID, t.Action, t.ExpiresAt, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating event rsvp token: %w", err)
	}
	return nil
}

// FindRSVPToken resolves a token string. A miss is (nil, nil) — the service
// turns that into the same generic "invalid or expired" message an expired or
// spent token gets, so the endpoint never distinguishes "never existed" from
// "already used" to an unauthenticated caller.
func (r *rsvpRepo) FindRSVPToken(ctx context.Context, token string) (*EventRSVPToken, error) {
	t := &EventRSVPToken{}
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, token, event_id, user_id, action, used_at, expires_at, created_at
		 FROM calendar_event_rsvp_tokens WHERE token = ?`, token).
		Scan(&t.ID, &t.Token, &t.EventID, &t.UserID, &t.Action, &usedAt, &t.ExpiresAt, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding event rsvp token: %w", err)
	}
	if usedAt.Valid {
		t.UsedAt = &usedAt.Time
	}
	return t, nil
}

// MarkRSVPTokenUsed consumes a token.
//
// The `used_at IS NULL` predicate makes consumption atomic: two concurrent
// POSTs of the same link race on this UPDATE and exactly one sees a non-zero
// RowsAffected. The service treats zero as "already used" rather than applying
// the action twice.
func (r *rsvpRepo) MarkRSVPTokenUsed(ctx context.Context, token string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_event_rsvp_tokens SET used_at = ? WHERE token = ? AND used_at IS NULL`,
		time.Now().UTC(), token)
	if err != nil {
		return fmt.Errorf("marking event rsvp token used: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		// Driver can't report; the UPDATE itself succeeded, so don't fail the
		// redemption on a diagnostic gap.
		return nil
	}
	if n == 0 {
		return errRSVPTokenSpent
	}
	return nil
}

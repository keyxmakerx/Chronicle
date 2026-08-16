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
	// SetCollectRSVPs flips the per-event opt-in and reports whether THIS call
	// was the one that changed it — the fan-out trigger must be the write, not a
	// separate earlier read.
	SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) (changed bool, err error)

	// Tokens.
	CreateRSVPToken(ctx context.Context, t *EventRSVPToken) error
	FindRSVPToken(ctx context.Context, token string) (*EventRSVPToken, error)
	MarkRSVPTokenUsed(ctx context.Context, token string) error

	// Schedule-ask send bookkeeping (C-CALV4-RSVP-P8B, migration 015). The two
	// PERSISTED rate limits are reads off this one log; the third layer (per
	// actor, in memory) never touches the database.
	RecordScheduleAsk(ctx context.Context, a *ScheduleAsk) error
	// LastScheduleAskAt is the campaign cooldown's input: MAX(sent_at) for one
	// campaign, or the zero time when nobody has ever been asked.
	LastScheduleAskAt(ctx context.Context, campaignID string) (time.Time, error)
	// ScheduleAskRecipientsSince is the per-recipient floor's input: the
	// distinct members of one campaign emailed at or after `since`.
	ScheduleAskRecipientsSince(ctx context.Context, campaignID string, since time.Time) ([]string, error)
}

// ScheduleAsk is one recorded schedule-ask send: one row per recipient ACTUALLY
// HANDED TO THE MAILER WITHOUT ERROR.
//
// The invariant this type exists to carry: NOTHING IS RECORDED WHEN NOTHING WAS
// SENT. SMTP unconfigured, no address on file, or a send error writes no row,
// because a cooldown must never lock out a campaign that received no mail.
//
// EventID is a pointer because it is genuinely optional: the schedule question
// does not need a session, and the ask outlives whichever session it happened
// to mention (the column is ON DELETE SET NULL for the same reason).
type ScheduleAsk struct {
	ID              string
	CampaignID      string
	EventID         *string
	RecipientUserID string
	ActorUserID     string
	SentAt          time.Time
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

// SetCollectRSVPs flips the per-event opt-in and reports whether it changed.
//
// The `collect_rsvps <> ?` predicate makes the OFF→ON transition atomic, so the
// invite fan-out is triggered by the WRITE rather than by a separate earlier
// read. Without it, two Scribes arming the gate at the same moment both read
// collect_rsvps=0, both decided they had caused the transition, and both started
// a fan-out — double-mailing the roster, because the 24h per-recipient floor's
// rows are written only AFTER each send returns and so absorb nothing in a tight
// race.
func (r *rsvpRepo) SetCollectRSVPs(ctx context.Context, eventID string, enabled bool) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_events SET collect_rsvps = ? WHERE id = ? AND collect_rsvps <> ?`,
		enabled, eventID, enabled)
	if err != nil {
		return false, fmt.Errorf("setting collect_rsvps: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		// The driver can't report. Treat it as "already in this state" rather
		// than as a transition: over-reporting a transition mails the roster.
		return false, nil
	}
	return n > 0, nil
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

// --- Schedule-ask send bookkeeping (C-CALV4-RSVP-P8B) ---

// RecordScheduleAsk appends one send row. Append-only by design: there is no
// update and no delete, because the log's whole purpose is to answer "when was
// this roster last mailed" honestly, and a row that can be edited answers a
// different question.
func (r *rsvpRepo) RecordScheduleAsk(ctx context.Context, a *ScheduleAsk) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_schedule_asks
		     (id, campaign_id, event_id, recipient_user_id, actor_user_id, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.CampaignID, a.EventID, a.RecipientUserID, a.ActorUserID, a.SentAt)
	if err != nil {
		return fmt.Errorf("recording schedule ask: %w", err)
	}
	return nil
}

// LastScheduleAskAt reports when this campaign's roster was last mailed, or the
// zero time when it never has been.
//
// MAX over an empty set is SQL NULL, not an error, so the NULL scan IS the
// "never asked" answer rather than a missing-row special case.
func (r *rsvpRepo) LastScheduleAskAt(ctx context.Context, campaignID string) (time.Time, error) {
	var last sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(sent_at) FROM calendar_schedule_asks WHERE campaign_id = ?`,
		campaignID).Scan(&last)
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("reading last schedule ask: %w", err)
	}
	if !last.Valid {
		return time.Time{}, nil
	}
	return last.Time.UTC(), nil
}

// ScheduleAskRecipientsSince lists the distinct members of one campaign emailed
// at or after `since` — the per-recipient floor's skip set.
//
// DISTINCT because the answer is set membership, not a count: a member mailed
// twice inside the window is skipped exactly as much as a member mailed once.
func (r *rsvpRepo) ScheduleAskRecipientsSince(ctx context.Context, campaignID string, since time.Time) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT recipient_user_id FROM calendar_schedule_asks
		 WHERE campaign_id = ? AND sent_at >= ?`,
		campaignID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("listing recent schedule ask recipients: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning schedule ask recipient: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating schedule ask recipients: %w", err)
	}
	return out, nil
}

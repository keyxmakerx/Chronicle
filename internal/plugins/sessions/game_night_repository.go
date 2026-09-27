package sessions

// Game-night RSVP storage (issue #741): soft delete/restore, the
// attendee note + tally-exclusion + needs-recheck columns, per-occurrence
// RSVPs for a repeating session, the "suggest another time" record, and the
// private calendar-feed credential + campaign kill switch.
//
// Kept in its own file per this plugin's convention of splitting out a large
// concern (see availability_repository.go, notifications_repository.go)
// rather than growing repository.go further.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Soft delete -------------------------------------------------------

// SoftDeleteSession marks a session removed without losing its row (so a
// co-Director/Owner can restore it). Idempotent: deleting an already-deleted
// session matches zero rows and returns nil, matching this file's existing
// same-state-resubmit convention (see UpdateAttendeeStatus) rather than
// reporting "not found" for a call that already achieved its goal.
func (r *sessionRepository) SoftDeleteSession(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET deleted_at = NOW() WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("soft-deleting session: %w", err)
	}
	return nil
}

// RestoreSession is SoftDeleteSession's inverse, equally idempotent.
func (r *sessionRepository) RestoreSession(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("restoring session: %w", err)
	}
	return nil
}

// --- session_attendees: note, exclusion, needs-recheck ------------------

// SetAttendeeNote sets or clears one attendee's note. An UPSERT rather than a
// bare UPDATE: a note can be attached before an RSVP row otherwise would
// exist (mirrors AddAttendee/UpdateAttendeeStatus's own upsert shape).
//
// VALUE CONVENTION (documented once, here): nil OR an empty string both clear
// the note (stored as SQL NULL); any non-empty string sets it. This method is
// only ever invoked when the caller means to touch the note at all — the
// service simply skips calling it when a request didn't mention one — so
// there is no third "leave alone" state to represent at this layer.
func (r *sessionRepository) SetAttendeeNote(ctx context.Context, sessionID, userID string, note *string) error {
	var n sql.NullString
	if note != nil && *note != "" {
		n = sql.NullString{String: *note, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_attendees (session_id, user_id, status, note)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE note = VALUES(note)`,
		sessionID, userID, RSVPInvited, n)
	if err != nil {
		return fmt.Errorf("setting attendee note: %w", err)
	}
	return nil
}

// SetAttendeeExcluded is the Director's own "leave myself out of the tally"
// switch. An UPSERT for the same reason as SetAttendeeNote: the organizer may
// flip this before ever answering their own RSVP.
func (r *sessionRepository) SetAttendeeExcluded(ctx context.Context, sessionID, userID string, excluded bool) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_attendees (session_id, user_id, status, excluded_from_count)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE excluded_from_count = VALUES(excluded_from_count)`,
		sessionID, userID, RSVPInvited, excluded)
	if err != nil {
		return fmt.Errorf("setting attendee excluded: %w", err)
	}
	return nil
}

// MarkSeriesNeedsRecheck bulk-flags every answered (non-invited) attendee row
// for a non-recurring session that just moved or was cancelled/restored. The
// answer itself is untouched — this only asks the member to look again.
func (r *sessionRepository) MarkSeriesNeedsRecheck(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE session_attendees SET needs_recheck = 1 WHERE session_id = ? AND status <> ?`,
		sessionID, RSVPInvited)
	if err != nil {
		return fmt.Errorf("marking series needs-recheck: %w", err)
	}
	return nil
}

// ListRespondedUserIDs returns user ids with a non-"invited" answer on a
// non-recurring session — who a move/cancel/restore should notify. An
// untouched invite carries no expectation to notify anyone of.
func (r *sessionRepository) ListRespondedUserIDs(ctx context.Context, sessionID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM session_attendees WHERE session_id = ? AND status <> ?`,
		sessionID, RSVPInvited)
	if err != nil {
		return nil, fmt.Errorf("listing responded user ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning responded user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- session_occurrence_rsvps: per-night answers on a recurring session --

// UpsertOccurrenceRSVP records one member's answer for ONE night of a
// repeating session, independent of every other night — the row's identity
// is (session, user, occurrence_date), so answering "yes" for next Tuesday
// never touches the row for the Tuesday after.
//
// note follows the same nil-preserves / pointer-sets-or-clears convention as
// SetAttendeeNote: nil means "don't touch the note column at all" (so a
// status-only re-answer can never clobber a note set earlier), a pointer to
// "" clears it, any other pointer sets it.
func (r *sessionRepository) UpsertOccurrenceRSVP(ctx context.Context, sessionID, userID, occurrenceDate, status string, note *string) error {
	if note == nil {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO session_occurrence_rsvps (session_id, user_id, occurrence_date, status, responded_at)
			VALUES (?, ?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE status = VALUES(status), needs_recheck = 0, responded_at = NOW()`,
			sessionID, userID, occurrenceDate, status)
		if err != nil {
			return fmt.Errorf("upserting occurrence rsvp: %w", err)
		}
		return nil
	}
	var n sql.NullString
	if *note != "" {
		n = sql.NullString{String: *note, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_occurrence_rsvps (session_id, user_id, occurrence_date, status, note, responded_at)
		VALUES (?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE status = VALUES(status), note = VALUES(note), needs_recheck = 0, responded_at = NOW()`,
		sessionID, userID, occurrenceDate, status, n)
	if err != nil {
		return fmt.Errorf("upserting occurrence rsvp: %w", err)
	}
	return nil
}

// ListOccurrenceRSVPs returns every member's answer for one specific night,
// with display data joined in for rendering a roster (mirrors ListAttendees).
func (r *sessionRepository) ListOccurrenceRSVPs(ctx context.Context, sessionID, occurrenceDate string) ([]OccurrenceRSVP, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sor.id, sor.session_id, sor.user_id, sor.occurrence_date, sor.status,
		       sor.note, sor.excluded_from_count, sor.needs_recheck, sor.responded_at,
		       u.display_name, u.avatar_path
		FROM session_occurrence_rsvps sor
		INNER JOIN users u ON u.id = sor.user_id
		WHERE sor.session_id = ? AND sor.occurrence_date = ?
		ORDER BY FIELD(sor.status, 'accepted', 'carried_yes', 'tentative', 'invited', 'declined'),
		         u.display_name`,
		sessionID, occurrenceDate)
	if err != nil {
		return nil, fmt.Errorf("listing occurrence rsvps: %w", err)
	}
	defer rows.Close()
	var out []OccurrenceRSVP
	for rows.Next() {
		var o OccurrenceRSVP
		var od time.Time
		if err := rows.Scan(
			&o.ID, &o.SessionID, &o.UserID, &od, &o.Status,
			&o.Note, &o.ExcludedFromCount, &o.NeedsRecheck, &o.RespondedAt,
			&o.DisplayName, &o.AvatarPath,
		); err != nil {
			return nil, fmt.Errorf("scanning occurrence rsvp row: %w", err)
		}
		o.OccurrenceDate = od.Format("2006-01-02")
		out = append(out, o)
	}
	return out, rows.Err()
}

// SetOccurrenceExcluded is SetAttendeeExcluded's per-occurrence twin.
func (r *sessionRepository) SetOccurrenceExcluded(ctx context.Context, sessionID, userID, occurrenceDate string, excluded bool) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_occurrence_rsvps (session_id, user_id, occurrence_date, status, excluded_from_count)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE excluded_from_count = VALUES(excluded_from_count)`,
		sessionID, userID, occurrenceDate, RSVPInvited, excluded)
	if err != nil {
		return fmt.Errorf("setting occurrence excluded: %w", err)
	}
	return nil
}

// MarkOccurrenceNeedsRecheck bulk-flags every answered row for ONE night.
func (r *sessionRepository) MarkOccurrenceNeedsRecheck(ctx context.Context, sessionID, occurrenceDate string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE session_occurrence_rsvps SET needs_recheck = 1
		 WHERE session_id = ? AND occurrence_date = ? AND status <> ?`,
		sessionID, occurrenceDate, RSVPInvited)
	if err != nil {
		return fmt.Errorf("marking occurrence needs-recheck: %w", err)
	}
	return nil
}

// ListOccurrenceRespondedUserIDs is ListRespondedUserIDs' per-occurrence twin.
func (r *sessionRepository) ListOccurrenceRespondedUserIDs(ctx context.Context, sessionID, occurrenceDate string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM session_occurrence_rsvps WHERE session_id = ? AND occurrence_date = ? AND status <> ?`,
		sessionID, occurrenceDate, RSVPInvited)
	if err != nil {
		return nil, fmt.Errorf("listing occurrence responded user ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning occurrence responded user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkAllOccurrencesNeedsRecheck is MarkOccurrenceNeedsRecheck without a date
// filter — needed when the RECURRING SESSION ITSELF (not one specific night)
// is moved, cancelled or restored, so every future night's existing answer is
// flagged, not just one.
func (r *sessionRepository) MarkAllOccurrencesNeedsRecheck(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE session_occurrence_rsvps SET needs_recheck = 1 WHERE session_id = ? AND status <> ?`,
		sessionID, RSVPInvited)
	if err != nil {
		return fmt.Errorf("marking all occurrences needs-recheck: %w", err)
	}
	return nil
}

// ListAllOccurrenceRespondedUserIDs is ListOccurrenceRespondedUserIDs without
// a date filter — the distinct set of members who answered ANY night of the
// series, deduplicated so a mass notify never messages someone twice.
func (r *sessionRepository) ListAllOccurrenceRespondedUserIDs(ctx context.Context, sessionID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT user_id FROM session_occurrence_rsvps WHERE session_id = ? AND status <> ?`,
		sessionID, RSVPInvited)
	if err != nil {
		return nil, fmt.Errorf("listing all occurrence responded user ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning occurrence responded user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- session_reschedule_suggestions ---------------------------------------

// CreateRescheduleSuggestion inserts one "suggest another time" record. The
// caller (ValidateAndRecordSuggestion) is responsible for consuming the
// originating token BEFORE calling this — see that method's doc comment for
// why the order matters.
func (r *sessionRepository) CreateRescheduleSuggestion(ctx context.Context, s *RescheduleSuggestion) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_reschedule_suggestions
			(session_id, user_id, occurrence_date, suggested_date, suggested_time, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.SessionID, s.UserID, s.OccurrenceDate, s.SuggestedDate, s.SuggestedTime, s.Note, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating reschedule suggestion: %w", err)
	}
	return nil
}

// --- session_calendar_feed_tokens / session_calendar_feed_settings --------

// generateFeedToken mints a URL-safe, unguessable feed credential. 32 random
// bytes base64url-encoded (no padding) is 43 characters — matching the
// token CHAR(43) column.
func generateFeedToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// GetOrCreateFeedToken returns a member's existing feed token, minting one on
// first call. The UNIQUE (campaign_id, user_id) key makes the insert-then-
// reread on a race safe: the loser of a concurrent first call simply reads
// back the winner's row instead of erroring.
func (r *sessionRepository) GetOrCreateFeedToken(ctx context.Context, campaignID, userID string) (*CalendarFeedToken, error) {
	if tok, err := r.findFeedTokenByMember(ctx, campaignID, userID); err == nil {
		return tok, nil
	}
	now := time.Now().UTC()
	token := generateFeedToken()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_calendar_feed_tokens (campaign_id, user_id, token, created_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE token = token`, // no-op on a race; re-read below gets the winner's token
		campaignID, userID, token, now)
	if err != nil {
		return nil, fmt.Errorf("creating feed token: %w", err)
	}
	return r.findFeedTokenByMember(ctx, campaignID, userID)
}

// ReplaceFeedToken deletes the member's current token (if any) and mints a
// new one, so a leaked link stops resolving immediately rather than staying
// valid until some future rotation.
func (r *sessionRepository) ReplaceFeedToken(ctx context.Context, campaignID, userID string) (*CalendarFeedToken, error) {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM session_calendar_feed_tokens WHERE campaign_id = ? AND user_id = ?`,
		campaignID, userID); err != nil {
		return nil, fmt.Errorf("deleting old feed token: %w", err)
	}
	now := time.Now().UTC()
	token := generateFeedToken()
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO session_calendar_feed_tokens (campaign_id, user_id, token, created_at)
		VALUES (?, ?, ?, ?)`,
		campaignID, userID, token, now); err != nil {
		return nil, fmt.Errorf("creating replacement feed token: %w", err)
	}
	return &CalendarFeedToken{CampaignID: campaignID, UserID: userID, Token: token, CreatedAt: now}, nil
}

// findFeedTokenByMember reads a member's current feed token, if any.
func (r *sessionRepository) findFeedTokenByMember(ctx context.Context, campaignID, userID string) (*CalendarFeedToken, error) {
	t := &CalendarFeedToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, user_id, token, created_at FROM session_calendar_feed_tokens
		 WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).
		Scan(&t.ID, &t.CampaignID, &t.UserID, &t.Token, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("no feed token")
	}
	if err != nil {
		return nil, fmt.Errorf("finding feed token: %w", err)
	}
	return t, nil
}

// FindFeedToken resolves a token string back to its owner — the read behind
// the public /sessions/feed/:token.ics route, where the token IS the
// credential.
func (r *sessionRepository) FindFeedToken(ctx context.Context, token string) (*CalendarFeedToken, error) {
	t := &CalendarFeedToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, user_id, token, created_at FROM session_calendar_feed_tokens WHERE token = ?`,
		token).Scan(&t.ID, &t.CampaignID, &t.UserID, &t.Token, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("invalid feed link")
	}
	if err != nil {
		return nil, fmt.Errorf("finding feed token: %w", err)
	}
	return t, nil
}

// IsCalendarFeedEnabled reports the campaign-wide kill switch. ABSENCE OF A
// ROW MEANS ENABLED (matching member_availability_status's "absence is a
// first-class state" convention), so the feed works for every campaign
// unless an owner explicitly inserted a disabling row.
func (r *sessionRepository) IsCalendarFeedEnabled(ctx context.Context, campaignID string) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT enabled FROM session_calendar_feed_settings WHERE campaign_id = ?`, campaignID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading feed settings: %w", err)
	}
	return enabled, nil
}

// SetCalendarFeedEnabled upserts the campaign-wide kill switch.
func (r *sessionRepository) SetCalendarFeedEnabled(ctx context.Context, campaignID string, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_calendar_feed_settings (campaign_id, enabled) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE enabled = VALUES(enabled)`,
		campaignID, enabled)
	if err != nil {
		return fmt.Errorf("setting feed enabled: %w", err)
	}
	return nil
}

package sessions

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// This file adds availability persistence to the existing sessionRepository
// (it already holds *sql.DB). Availability lives in its own tables
// (member_availability, availability_exceptions) — never session_attendees —
// so it stays out of export egress by construction (design §5).

// ListUserAvailability returns a member's own recurring blocks for a campaign,
// ordered for stable rendering.
func (r *sessionRepository) ListUserAvailability(ctx context.Context, campaignID, userID string) ([]AvailabilityBlock, error) {
	const q = `SELECT id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, week_parity, updated_at
	           FROM member_availability
	           WHERE campaign_id = ? AND user_id = ?
	           ORDER BY day_of_week, start_minute, week_parity`
	rows, err := r.db.QueryContext(ctx, q, campaignID, userID)
	if err != nil {
		return nil, fmt.Errorf("listing user availability: %w", err)
	}
	defer rows.Close()
	return scanAvailabilityBlocks(rows)
}

// ListCampaignAvailability returns every member's recurring blocks for a
// campaign — the raw input to the DM overlay projection.
func (r *sessionRepository) ListCampaignAvailability(ctx context.Context, campaignID string) ([]AvailabilityBlock, error) {
	const q = `SELECT id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, week_parity, updated_at
	           FROM member_availability
	           WHERE campaign_id = ?
	           ORDER BY user_id, day_of_week, start_minute, week_parity`
	rows, err := r.db.QueryContext(ctx, q, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing campaign availability: %w", err)
	}
	defer rows.Close()
	return scanAvailabilityBlocks(rows)
}

// scanAvailabilityBlocks materializes rows into AvailabilityBlock structs.
func scanAvailabilityBlocks(rows *sql.Rows) ([]AvailabilityBlock, error) {
	var out []AvailabilityBlock
	for rows.Next() {
		var b AvailabilityBlock
		if err := rows.Scan(&b.ID, &b.CampaignID, &b.UserID, &b.DayOfWeek,
			&b.StartMinute, &b.EndMinute, &b.State, &b.TZ, &b.WeekCadence, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning availability block: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ReplaceUserAvailability atomically replaces a member's entire recurring
// pattern for a campaign (delete-all then insert). The paint grid always sends
// the complete current grid, so replace-all is the simplest correct semantics
// and keeps the unique constraint from fighting partial updates.
//
// It also stamps member_availability_status IN THE SAME TRANSACTION, because
// "this member has answered" and "these are their blocks" are one fact recorded
// twice, and a save that wrote one without the other would leave the roster
// lying in whichever direction the failure fell. tz is passed separately rather
// than read off the blocks: an EMPTY grid is a valid answer and carries no
// block to read a zone from, and that is precisely the answer the status table
// exists to capture.
func (r *sessionRepository) ReplaceUserAvailability(ctx context.Context, campaignID, userID, tz string, blocks []AvailabilityBlock) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin availability tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM member_availability WHERE campaign_id = ? AND user_id = ?`,
		campaignID, userID); err != nil {
		return fmt.Errorf("clearing availability: %w", err)
	}

	const ins = `INSERT INTO member_availability
		(id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, week_parity, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	now := time.Now().UTC()
	for _, b := range blocks {
		if _, err := tx.ExecContext(ctx, ins,
			generateUUID(), campaignID, userID, b.DayOfWeek,
			b.StartMinute, b.EndMinute, b.State, b.TZ, b.WeekCadence, now); err != nil {
			return fmt.Errorf("inserting availability block: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO member_availability_status (campaign_id, user_id, answered_at, tz)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE answered_at = VALUES(answered_at), tz = VALUES(tz)`,
		campaignID, userID, now, tz); err != nil {
		return fmt.Errorf("stamping availability answer: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit availability tx: %w", err)
	}
	return nil
}

// ListAnsweredUserIDs returns the ids of members who have answered the
// availability question for a campaign, in no particular order.
//
// A SET of ids rather than a per-member lookup: both callers (the overlay
// roster and the nudge) need the whole campaign at once, and asking per member
// would put a query inside a roster loop.
func (r *sessionRepository) ListAnsweredUserIDs(ctx context.Context, campaignID string) (map[string]time.Time, error) {
	const q = `SELECT user_id, answered_at FROM member_availability_status WHERE campaign_id = ?`
	rows, err := r.db.QueryContext(ctx, q, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing availability answers: %w", err)
	}
	defer rows.Close()

	out := make(map[string]time.Time)
	for rows.Next() {
		var uid string
		var at time.Time
		if err := rows.Scan(&uid, &at); err != nil {
			return nil, fmt.Errorf("scanning availability answer: %w", err)
		}
		out[uid] = at
	}
	return out, rows.Err()
}

// ListUserExceptions returns a member's own per-date overrides for a campaign.
func (r *sessionRepository) ListUserExceptions(ctx context.Context, campaignID, userID string) ([]AvailabilityException, error) {
	const q = `SELECT id, campaign_id, user_id, DATE_FORMAT(on_date, '%Y-%m-%d'), start_minute, end_minute, state, tz, updated_at
	           FROM availability_exceptions
	           WHERE campaign_id = ? AND user_id = ?
	           ORDER BY on_date, start_minute`
	rows, err := r.db.QueryContext(ctx, q, campaignID, userID)
	if err != nil {
		return nil, fmt.Errorf("listing user exceptions: %w", err)
	}
	defer rows.Close()
	return scanExceptions(rows)
}

// ListCampaignExceptionsInRange returns every member's exceptions whose date
// falls within [startDate, endDate] — the overlay only needs the target week.
func (r *sessionRepository) ListCampaignExceptionsInRange(ctx context.Context, campaignID, startDate, endDate string) ([]AvailabilityException, error) {
	const q = `SELECT id, campaign_id, user_id, DATE_FORMAT(on_date, '%Y-%m-%d'), start_minute, end_minute, state, tz, updated_at
	           FROM availability_exceptions
	           WHERE campaign_id = ? AND on_date BETWEEN ? AND ?
	           ORDER BY user_id, on_date, start_minute`
	rows, err := r.db.QueryContext(ctx, q, campaignID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("listing campaign exceptions: %w", err)
	}
	defer rows.Close()
	return scanExceptions(rows)
}

// scanExceptions materializes rows into AvailabilityException structs.
func scanExceptions(rows *sql.Rows) ([]AvailabilityException, error) {
	var out []AvailabilityException
	for rows.Next() {
		var e AvailabilityException
		if err := rows.Scan(&e.ID, &e.CampaignID, &e.UserID, &e.OnDate,
			&e.StartMinute, &e.EndMinute, &e.State, &e.TZ, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning exception: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddException inserts (or upserts, on the unique block key) a per-date
// override for a member.
//
// NO PRODUCTION CALLER, AND THAT IS THE POINT. Writing ONE exception row for a
// date silently deletes the rest of that date, because exception rows fully
// REPLACE the recurring pattern for their date (effectiveBlocks,
// availability_overlay.go). AddMyException used to call this and turned "I'm
// ALSO free 07:00–08:00" into a member who was free for one hour at 7am and busy
// every evening. It now composes the whole day and writes it through
// ReplaceDayExceptions instead. Kept for completeness of the repository's CRUD
// surface — if you reach for it, you almost certainly want ReplaceDayExceptions.
func (r *sessionRepository) AddException(ctx context.Context, e *AvailabilityException) error {
	const q = `INSERT INTO availability_exceptions
		(id, campaign_id, user_id, on_date, start_minute, end_minute, state, tz, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE state = VALUES(state), tz = VALUES(tz), updated_at = VALUES(updated_at)`
	_, err := r.db.ExecContext(ctx, q,
		e.ID, e.CampaignID, e.UserID, e.OnDate,
		e.StartMinute, e.EndMinute, e.State, e.TZ, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("adding exception: %w", err)
	}
	return nil
}

// CountUserExceptions returns how many exception rows a member currently has in
// a campaign — the input to the per-user cap that keeps a malformed or hostile
// client from inserting an unbounded number of override rows (C-SCHED-P2 0d).
func (r *sessionRepository) CountUserExceptions(ctx context.Context, campaignID, userID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM availability_exceptions WHERE campaign_id = ? AND user_id = ?`,
		campaignID, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting user exceptions: %w", err)
	}
	return n, nil
}

// ReplaceDayExceptions atomically replaces ALL of a member's exception rows for
// one date (delete-all-for-date then insert). This is the storage side of the
// "compose the day" UI (C-SCHED-P2 0c): the editor pre-fills from the recurring
// pattern and re-sends the whole day, so replace-day semantics are preserved
// while a one-hour busy mark no longer erases the rest of the day. An empty
// blocks slice clears the day's overrides (reverting it to the recurring pattern).
func (r *sessionRepository) ReplaceDayExceptions(ctx context.Context, campaignID, userID, onDate string, excs []AvailabilityException) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin exception day tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM availability_exceptions WHERE campaign_id = ? AND user_id = ? AND on_date = ?`,
		campaignID, userID, onDate); err != nil {
		return fmt.Errorf("clearing day exceptions: %w", err)
	}

	const ins = `INSERT INTO availability_exceptions
		(id, campaign_id, user_id, on_date, start_minute, end_minute, state, tz, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	now := time.Now().UTC()
	for _, e := range excs {
		if _, err := tx.ExecContext(ctx, ins,
			generateUUID(), campaignID, userID, onDate,
			e.StartMinute, e.EndMinute, e.State, e.TZ, now); err != nil {
			return fmt.Errorf("inserting day exception: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit exception day tx: %w", err)
	}
	return nil
}

// DeleteException removes one of a member's own exceptions. Scoping the delete
// to (campaign_id, user_id) as well as id prevents an IDOR delete of another
// member's exception.
func (r *sessionRepository) DeleteException(ctx context.Context, campaignID, userID, exceptionID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM availability_exceptions WHERE id = ? AND campaign_id = ? AND user_id = ?`,
		exceptionID, campaignID, userID)
	if err != nil {
		return fmt.Errorf("deleting exception: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("exception not found")
	}
	return nil
}

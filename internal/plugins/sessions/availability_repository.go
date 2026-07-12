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
	const q = `SELECT id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, updated_at
	           FROM member_availability
	           WHERE campaign_id = ? AND user_id = ?
	           ORDER BY day_of_week, start_minute`
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
	const q = `SELECT id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, updated_at
	           FROM member_availability
	           WHERE campaign_id = ?
	           ORDER BY user_id, day_of_week, start_minute`
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
			&b.StartMinute, &b.EndMinute, &b.State, &b.TZ, &b.UpdatedAt); err != nil {
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
func (r *sessionRepository) ReplaceUserAvailability(ctx context.Context, campaignID, userID string, blocks []AvailabilityBlock) error {
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
		(id, campaign_id, user_id, day_of_week, start_minute, end_minute, state, tz, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	now := time.Now().UTC()
	for _, b := range blocks {
		if _, err := tx.ExecContext(ctx, ins,
			generateUUID(), campaignID, userID, b.DayOfWeek,
			b.StartMinute, b.EndMinute, b.State, b.TZ, now); err != nil {
			return fmt.Errorf("inserting availability block: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit availability tx: %w", err)
	}
	return nil
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

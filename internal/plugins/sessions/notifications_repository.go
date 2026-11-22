package sessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Notification persistence on the existing sessionRepository. The store is
// generic and plugin-agnostic; every read/write is scoped by user_id so one
// user can never see or mutate another's notifications.

// CreateNotification inserts one notification row.
func (r *sessionRepository) CreateNotification(ctx context.Context, n *Notification) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (id, user_id, campaign_id, type, payload, link, read_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.UserID, n.CampaignID, n.Type, n.Payload, n.Link, n.ReadAt, n.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}
	return nil
}

// MergeUnreadNotification rewrites the recipient's still-unread notification
// of the same type and link with n's payload and time, so a burst of answers
// to one thing stays one bell row. It inserts n when there is nothing unread
// to merge into; once the recipient has read the row, the next answer starts
// a fresh one.
func (r *sessionRepository) MergeUnreadNotification(ctx context.Context, n *Notification) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET payload = ?, created_at = ?
		 WHERE user_id = ? AND type = ? AND link = ? AND read_at IS NULL`,
		n.Payload, n.CreatedAt, n.UserID, n.Type, n.Link)
	if err != nil {
		return fmt.Errorf("merging notification: %w", err)
	}
	if affected, aErr := res.RowsAffected(); aErr == nil && affected > 0 {
		return nil
	}
	return r.CreateNotification(ctx, n)
}

// FindUnreadNotification returns the user's newest unread notification of the
// type and link, or nil when there is none.
func (r *sessionRepository) FindUnreadNotification(ctx context.Context, userID, ntype, link string) (*Notification, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, campaign_id, type, payload, link, read_at, created_at
		 FROM notifications WHERE user_id = ? AND type = ? AND link = ? AND read_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`, userID, ntype, link)
	if err != nil {
		return nil, fmt.Errorf("finding unread notification: %w", err)
	}
	defer rows.Close()
	ns, err := scanNotifications(rows)
	if err != nil || len(ns) == 0 {
		return nil, err
	}
	return &ns[0], nil
}

// RewriteUnreadNotification replaces a row's payload and time, only while the
// row is still unread and still holds the payload the caller built on, so a
// fold never overwrites a change it did not see or a row the recipient has
// read. It reports whether the row was rewritten.
func (r *sessionRepository) RewriteUnreadNotification(ctx context.Context, id string, was, payload *string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET payload = ?, created_at = ?
		 WHERE id = ? AND read_at IS NULL AND payload <=> ?`,
		payload, at, id, was)
	if err != nil {
		return false, fmt.Errorf("rewriting notification: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rewriting notification: %w", err)
	}
	if n > 0 {
		return true, nil
	}
	// MariaDB counts changed rows, not matched ones: a rewrite that says what
	// the row already says, in the same second, changes nothing. That is a
	// success, not a lost race, or the caller would add a second row.
	var one int
	err = r.db.QueryRowContext(ctx,
		`SELECT 1 FROM notifications WHERE id = ? AND read_at IS NULL AND payload <=> ?`, id, payload).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("rewriting notification: %w", err)
	}
	return true, nil
}

// LatestCampaignNotificationAt is the newest created_at among the campaign's
// notifications of ntype, or the zero time when there are none.
func (r *sessionRepository) LatestCampaignNotificationAt(ctx context.Context, campaignID, ntype string) (time.Time, error) {
	var at sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(created_at) FROM notifications WHERE campaign_id = ? AND type = ?`,
		campaignID, ntype).Scan(&at)
	if err != nil {
		return time.Time{}, fmt.Errorf("finding latest notification: %w", err)
	}
	if !at.Valid {
		return time.Time{}, nil
	}
	return at.Time, nil
}

// ListNotifications returns a user's notifications, newest first, capped at limit.
func (r *sessionRepository) ListNotifications(ctx context.Context, userID string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, campaign_id, type, payload, link, read_at, created_at
		 FROM notifications WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

// ListUnreadNotificationsOfType returns a user's unread notifications of one
// type, newest first, capped at limit.
func (r *sessionRepository) ListUnreadNotificationsOfType(ctx context.Context, userID, ntype string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, campaign_id, type, payload, link, read_at, created_at
		 FROM notifications WHERE user_id = ? AND type = ? AND read_at IS NULL
		 ORDER BY created_at DESC LIMIT ?`, userID, ntype, limit)
	if err != nil {
		return nil, fmt.Errorf("listing unread notifications of a type: %w", err)
	}
	defer rows.Close()
	return scanNotifications(rows)
}

// scanNotifications reads notification rows selected in the column order the
// two lists above share.
func scanNotifications(rows *sql.Rows) ([]Notification, error) {
	var out []Notification
	for rows.Next() {
		var n Notification
		var campaignID, payload, link sql.NullString
		var readAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.UserID, &campaignID, &n.Type, &payload, &link, &readAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning notification: %w", err)
		}
		if campaignID.Valid {
			n.CampaignID = &campaignID.String
		}
		if payload.Valid {
			n.Payload = &payload.String
		}
		if link.Valid {
			n.Link = &link.String
		}
		if readAt.Valid {
			n.ReadAt = &readAt.Time
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountUnreadNotifications returns the user's unread count (the topbar badge).
func (r *sessionRepository) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}
	return n, nil
}

// MarkNotificationRead marks one notification read, scoped to its owner so a
// user cannot mark another user's notification (IDOR guard).
func (r *sessionRepository) MarkNotificationRead(ctx context.Context, userID, notificationID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE id = ? AND user_id = ? AND read_at IS NULL`,
		time.Now().UTC(), notificationID, userID)
	if err != nil {
		return fmt.Errorf("marking notification read: %w", err)
	}
	return nil
}

// MarkNotificationsReadForLink marks the user's unread notifications of one
// type pointing at link read, scoped to their owner like MarkNotificationRead.
func (r *sessionRepository) MarkNotificationsReadForLink(ctx context.Context, userID, ntype, link string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE user_id = ? AND type = ? AND link = ? AND read_at IS NULL`,
		time.Now().UTC(), userID, ntype, link)
	if err != nil {
		return fmt.Errorf("marking notifications read for a link: %w", err)
	}
	return nil
}

// MarkAllNotificationsRead marks every unread notification for a user read.
func (r *sessionRepository) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`,
		time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}
	return nil
}

package sessions

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Scheduler-scoped notification persistence on the existing sessionRepository
// (C-SCHED-P2). The store is generic + removable (T-B2); the scheduler feature
// is its only writer this slice. Every read/write is scoped by user_id so one
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

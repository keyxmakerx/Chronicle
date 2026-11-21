package sessions

import (
	"context"
	"fmt"
)

// ListDismissedEventIDs implements SessionRepository.
func (r *sessionRepository) ListDismissedEventIDs(ctx context.Context, campaignID string) ([]string, error) {
	return r.stringColumn(ctx, "listing dismissed events",
		`SELECT calendar_event_id FROM session_event_dismissals WHERE campaign_id = ?`, campaignID)
}

// DismissEvent implements SessionRepository. A second press keeps the first
// presser's name, so the insert ignores a row that is already there.
func (r *sessionRepository) DismissEvent(ctx context.Context, campaignID, eventID, userID string) error {
	var by any
	if userID != "" {
		by = userID
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO session_event_dismissals (campaign_id, calendar_event_id, dismissed_by) VALUES (?, ?, ?)`,
		campaignID, eventID, by); err != nil {
		return fmt.Errorf("dismissing event: %w", err)
	}
	return nil
}

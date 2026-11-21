package sessions

import (
	"context"
	"fmt"
)

// ListLinkedEventIDs implements SessionRepository. No deleted_at filter: a
// night in the Trash still holds its event's unique link, so the event must
// count as having a night.
func (r *sessionRepository) ListLinkedEventIDs(ctx context.Context, campaignID string) ([]string, error) {
	return r.stringColumn(ctx, "listing linked events",
		`SELECT calendar_event_id FROM sessions WHERE campaign_id = ? AND calendar_event_id IS NOT NULL`, campaignID)
}

func (r *sessionRepository) stringColumn(ctx context.Context, what, query string, args ...any) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

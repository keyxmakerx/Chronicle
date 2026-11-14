package campaigns

// A member's "seen up to" time for the campaign home's "Since you were last
// here" block. It is kept apart from CampaignRepository so the many fakes of
// that interface need no new method, but it is the same column family as
// nav_pins: one cell on the member's own row, addressed by campaign and user
// together, so it can only ever be the caller's own.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// HomeSeen is what the block needs to know about one member: when they last
// pressed "Mark as seen" (nil if never) and when they joined, which stands in
// until they have.
type HomeSeen struct {
	SeenAt   *time.Time
	JoinedAt time.Time
}

// SeenUpTo is the time the block reads from: the marker, else the join time.
func (h HomeSeen) SeenUpTo() time.Time {
	if h.SeenAt != nil {
		return *h.SeenAt
	}
	return h.JoinedAt
}

// HomeSeenStore reads and moves a member's marker.
type HomeSeenStore interface {
	// Get returns the member's marker; ok is false when the user is not a
	// member of the campaign.
	Get(ctx context.Context, campaignID, userID string) (seen HomeSeen, ok bool, err error)
	// Mark moves the member's marker to at, and reports whether a member row
	// was there to move.
	Mark(ctx context.Context, campaignID, userID string, at time.Time) (bool, error)
}

type sqlHomeSeenStore struct{ db *sql.DB }

// NewHomeSeenStore creates the MariaDB-backed store.
func NewHomeSeenStore(db *sql.DB) HomeSeenStore { return &sqlHomeSeenStore{db: db} }

// Get reads the member row by its primary key (campaign_id, user_id).
func (s *sqlHomeSeenStore) Get(ctx context.Context, campaignID, userID string) (HomeSeen, bool, error) {
	var seen sql.NullTime
	var h HomeSeen
	err := s.db.QueryRowContext(ctx,
		`SELECT home_seen_at, joined_at FROM campaign_members WHERE campaign_id = ? AND user_id = ?`,
		campaignID, userID).Scan(&seen, &h.JoinedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return HomeSeen{}, false, nil
	}
	if err != nil {
		return HomeSeen{}, false, fmt.Errorf("reading home seen marker: %w", err)
	}
	if seen.Valid {
		t := seen.Time
		h.SeenAt = &t
	}
	return h, true, nil
}

// Mark never moves the marker backwards: two tabs pressing the button, or a
// replayed request, cannot bring old news back.
func (s *sqlHomeSeenStore) Mark(ctx context.Context, campaignID, userID string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE campaign_members SET home_seen_at = ?
		 WHERE campaign_id = ? AND user_id = ? AND (home_seen_at IS NULL OR home_seen_at < ?)`,
		at, campaignID, userID, at)
	if err != nil {
		return false, fmt.Errorf("moving home seen marker: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return true, nil
	}
	// Nothing changed: either the marker is already at or past `at`, or there
	// is no such member. Tell them apart so a non-member is not told "seen".
	_, ok, err := s.Get(ctx, campaignID, userID)
	return ok, err
}

package decks

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FreshRow is a face-down copy dealt to a hero a player plays, found by the
// pop-up check. It names the copy and its deck and nothing of the card.
type FreshRow struct {
	CopyID string
	DeckID string
}

// ScreenRepository stores what the DM Screen's Card decks section remembers
// and which dealt cards a player has been shown. It sits beside Repository, as
// the moments store does, so the decks core does not depend on it.
//
// SECURITY: every statement filters on campaign_id, and a pick is stored only
// for a deck of that campaign, so an id from another campaign never reads or
// changes anything here.
type ScreenRepository interface {
	// Picks lists the decks a Director chose for the section, in their order.
	Picks(ctx context.Context, campaignID, userID string) ([]string, error)
	// SetPicks replaces the Director's picks; an empty list clears them.
	SetPicks(ctx context.Context, campaignID, userID string, deckIDs []string) error
	// LastMoves gives, per deck, the id of its newest log line. A deck with no
	// line is absent.
	LastMoves(ctx context.Context, campaignID string) (map[string]int64, error)
	// Fresh lists, newest deal first and at most limit, the face-down copies in
	// hand of these heroes that someone other than userID dealt within the last
	// window and that userID has not been shown.
	Fresh(ctx context.Context, campaignID, userID string, heroIDs []string, window time.Duration, limit int) ([]FreshRow, error)
	// MarkSeen records that userID was shown the copy. Marking twice is fine.
	MarkSeen(ctx context.Context, campaignID, userID, copyID string) error
}

type sqlScreen struct{ db *sql.DB }

// NewScreenRepository creates the MariaDB-backed store.
func NewScreenRepository(db *sql.DB) ScreenRepository { return &sqlScreen{db: db} }

func (s *sqlScreen) Picks(ctx context.Context, campaignID, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deck_id FROM deck_screen_picks WHERE campaign_id = ? AND user_id = ? ORDER BY sort_order, deck_id`,
		campaignID, userID)
	if err != nil {
		return nil, fmt.Errorf("list screen picks: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan screen pick: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *sqlScreen) SetPicks(ctx context.Context, campaignID, userID string, deckIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin screen picks: %w", err)
	}
	defer rollback(tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM deck_screen_picks WHERE campaign_id = ? AND user_id = ?`, campaignID, userID); err != nil {
		return fmt.Errorf("clear screen picks: %w", err)
	}
	for i, id := range deckIDs {
		// The join keeps a pick to a deck of this campaign even if the caller
		// did not check, and a deck deleted a moment ago stores nothing.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO deck_screen_picks (campaign_id, user_id, deck_id, sort_order)
			 SELECT ?, ?, d.id, ? FROM decks d WHERE d.id = ? AND d.campaign_id = ?`,
			campaignID, userID, i, id, campaignID); err != nil {
			return fmt.Errorf("store screen pick: %w", err)
		}
	}
	return tx.Commit()
}

func (s *sqlScreen) LastMoves(ctx context.Context, campaignID string) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deck_id, MAX(id) FROM deck_log WHERE campaign_id = ? GROUP BY deck_id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("last deck moves: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan last move: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (s *sqlScreen) Fresh(ctx context.Context, campaignID, userID string, heroIDs []string, window time.Duration, limit int) ([]FreshRow, error) {
	if len(heroIDs) == 0 || limit <= 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(heroIDs)), ",")
	args := []any{campaignID, int(window.Seconds()), userID}
	for _, id := range heroIDs {
		args = append(args, id)
	}
	args = append(args, userID, limit)
	// The window is measured on the database clock that stamped dealt_at.
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.deck_id FROM deck_copies c
		 WHERE c.campaign_id = ? AND c.zone = 'hand' AND c.face_up = 0
		   AND c.dealt_at IS NOT NULL AND c.dealt_at >= NOW() - INTERVAL ? SECOND
		   AND c.dealt_by IS NOT NULL AND c.dealt_by <> ?
		   AND c.holder_entity_id IN (`+marks+`)
		   AND NOT EXISTS (SELECT 1 FROM deck_fresh_seen f WHERE f.copy_id = c.id AND f.user_id = ?)
		 ORDER BY c.dealt_at DESC, c.id LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("fresh copies: %w", err)
	}
	defer rows.Close()
	var out []FreshRow
	for rows.Next() {
		var r FreshRow
		if err := rows.Scan(&r.CopyID, &r.DeckID); err != nil {
			return nil, fmt.Errorf("scan fresh copy: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// freshKept is how long a "seen" mark is kept, far past the pop-up window.
const freshKept = 24 * time.Hour

func (s *sqlScreen) MarkSeen(ctx context.Context, campaignID, userID, copyID string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO deck_fresh_seen (copy_id, user_id, campaign_id) VALUES (?, ?, ?)`,
		copyID, userID, campaignID); err != nil {
		return fmt.Errorf("mark fresh seen: %w", err)
	}
	// Old marks can never matter again, so each write clears them.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM deck_fresh_seen WHERE seen_at < NOW() - INTERVAL ? SECOND`, int(freshKept.Seconds())); err != nil {
		return fmt.Errorf("prune fresh seen: %w", err)
	}
	return nil
}

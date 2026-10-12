package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// CustomizeDraftRepository stores the owner's unsaved Customize work, one
// row per owner per campaign. Every write that changes an existing row is
// guarded by its version, so a stale tab can never overwrite newer work.
type CustomizeDraftRepository interface {
	// Get returns the draft, or nil, nil when there is none.
	Get(ctx context.Context, campaignID, userID string) (*CustomizeDraft, error)
	// Insert creates the first version of a draft. It reports false, nil
	// when a row already exists, which is how a second tab's "no draft yet"
	// write loses the race.
	Insert(ctx context.Context, campaignID, userID string, draft []byte) (bool, error)
	// Update replaces the draft and bumps its version, only while the stored
	// version is still the one given. It reports false, nil on a mismatch.
	Update(ctx context.Context, campaignID, userID string, version int, draft []byte) (bool, error)
	// Delete removes the draft while its stored version matches; version 0
	// removes it whatever its version. It reports whether a row was removed.
	Delete(ctx context.Context, campaignID, userID string, version int) (bool, error)
}

type customizeDraftRepository struct{ db *sql.DB }

// NewCustomizeDraftRepository returns the MariaDB draft store.
func NewCustomizeDraftRepository(db *sql.DB) CustomizeDraftRepository {
	return &customizeDraftRepository{db: db}
}

func (r *customizeDraftRepository) Get(ctx context.Context, campaignID, userID string) (*CustomizeDraft, error) {
	var d CustomizeDraft
	var raw string
	err := r.db.QueryRowContext(ctx,
		`SELECT draft_json, version, updated_at FROM customize_drafts
		  WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).
		Scan(&raw, &d.Version, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading customize draft: %w", err)
	}
	d.Draft = []byte(raw)
	return &d, nil
}

func (r *customizeDraftRepository) Insert(ctx context.Context, campaignID, userID string, draft []byte) (bool, error) {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO customize_drafts (campaign_id, user_id, draft_json, version)
		 VALUES (?, ?, ?, 1)`, campaignID, userID, string(draft))
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return false, nil
		}
		return false, fmt.Errorf("inserting customize draft: %w", err)
	}
	return true, nil
}

func (r *customizeDraftRepository) Update(ctx context.Context, campaignID, userID string, version int, draft []byte) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE customize_drafts SET draft_json = ?, version = version + 1
		  WHERE campaign_id = ? AND user_id = ? AND version = ?`,
		string(draft), campaignID, userID, version)
	if err != nil {
		return false, fmt.Errorf("updating customize draft: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("updating customize draft: %w", err)
	}
	return n > 0, nil
}

func (r *customizeDraftRepository) Delete(ctx context.Context, campaignID, userID string, version int) (bool, error) {
	q := `DELETE FROM customize_drafts WHERE campaign_id = ? AND user_id = ?`
	args := []any{campaignID, userID}
	if version > 0 {
		q += ` AND version = ?`
		args = append(args, version)
	}
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("deleting customize draft: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("deleting customize draft: %w", err)
	}
	return n > 0, nil
}

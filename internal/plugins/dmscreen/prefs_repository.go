package dmscreen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type prefsRepository struct {
	db *sql.DB
}

// NewPrefsRepository returns the MariaDB-backed store of screen preferences.
func NewPrefsRepository(db *sql.DB) PrefsRepository {
	return &prefsRepository{db: db}
}

func (r *prefsRepository) Get(ctx context.Context, campaignID, userID, screen string) ([]byte, bool, error) {
	var doc []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT prefs FROM dm_screen_prefs WHERE campaign_id = ? AND user_id = ? AND screen = ?`,
		campaignID, userID, screen).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading screen prefs: %w", err)
	}
	return doc, true, nil
}

// Update reads the row FOR UPDATE inside a transaction, so a second save of
// the same person's row waits for the first and merges over its result.
func (r *prefsRepository) Update(ctx context.Context, campaignID, userID, screen string, fn func(cur []byte) ([]byte, error)) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("saving screen prefs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cur []byte
	err = tx.QueryRowContext(ctx,
		`SELECT prefs FROM dm_screen_prefs WHERE campaign_id = ? AND user_id = ? AND screen = ? FOR UPDATE`,
		campaignID, userID, screen).Scan(&cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reading screen prefs: %w", err)
	}
	next, err := fn(cur)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO dm_screen_prefs (campaign_id, user_id, screen, prefs) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE prefs = VALUES(prefs)`,
		campaignID, userID, screen, next); err != nil {
		return fmt.Errorf("saving screen prefs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("saving screen prefs: %w", err)
	}
	return nil
}

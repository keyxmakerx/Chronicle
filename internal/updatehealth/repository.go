package updatehealth

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Repository is the MariaDB Store, over the update_health table.
type Repository struct {
	db *sql.DB
}

// NewRepository builds the store.
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// EnsureSeen inserts a row for each feature not seen before. INSERT IGNORE
// keeps the first sighting when several servers start at once.
func (r *Repository) EnsureSeen(ctx context.Context, ids []string, now time.Time) error {
	for _, id := range ids {
		if _, err := r.db.ExecContext(ctx,
			`INSERT IGNORE INTO update_health (feature_id, first_seen_at) VALUES (?, ?)`,
			id, now.UTC()); err != nil {
			return fmt.Errorf("noting changed feature %s: %w", id, err)
		}
	}
	return nil
}

// Add adds counts to a feature's row, creating it if the first sighting has
// not been stored yet. Counts are added in the database, so several servers
// can share one table.
func (r *Repository) Add(ctx context.Context, id string, uses, errors int, lastError *time.Time, lastErrorMsg string) error {
	var at any
	var msg any
	if lastError != nil {
		at, msg = lastError.UTC(), lastErrorMsg
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO update_health (feature_id, first_seen_at, uses, errors, last_error_at, last_error)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			uses = uses + VALUES(uses),
			errors = errors + VALUES(errors),
			last_error = IF(VALUES(last_error_at) IS NOT NULL AND (last_error_at IS NULL OR VALUES(last_error_at) > last_error_at), VALUES(last_error), last_error),
			last_error_at = IF(VALUES(last_error_at) IS NOT NULL AND (last_error_at IS NULL OR VALUES(last_error_at) > last_error_at), VALUES(last_error_at), last_error_at)`,
		id, time.Now().UTC(), uses, errors, at, msg)
	if err != nil {
		return fmt.Errorf("storing update health for %s: %w", id, err)
	}
	return nil
}

// All returns every stored row by feature id.
func (r *Repository) All(ctx context.Context) (map[string]Counts, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT feature_id, first_seen_at, uses, errors, last_error_at, COALESCE(last_error, '') FROM update_health`)
	if err != nil {
		return nil, fmt.Errorf("reading update health: %w", err)
	}
	defer rows.Close()
	out := map[string]Counts{}
	for rows.Next() {
		var id string
		var c Counts
		var at sql.NullTime
		if err := rows.Scan(&id, &c.FirstSeen, &c.Uses, &c.Errors, &at, &c.LastErrorMsg); err != nil {
			return nil, fmt.Errorf("reading update health: %w", err)
		}
		if at.Valid {
			t := at.Time
			c.LastError = &t
		}
		out[id] = c
	}
	return out, rows.Err()
}

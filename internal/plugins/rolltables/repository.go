// Hand-written SQL for campaign_roll_tables.
//
// SECURITY: every method filters on campaign_id, which is the primary key, so
// one campaign can never read or replace another's document.
package rolltables

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository is the persistence boundary for roll tables.
type Repository interface {
	// Get returns the stored JSON document, or nil when the campaign has none.
	Get(ctx context.Context, campaignID string) ([]byte, error)
	// Put stores the document, replacing any existing one. userID is empty
	// when there is no acting user.
	Put(ctx context.Context, campaignID string, data []byte, userID string) error
}

type sqlRepository struct {
	db *sql.DB
}

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepository{db: db}
}

func (r *sqlRepository) Get(ctx context.Context, campaignID string) ([]byte, error) {
	var data string
	err := r.db.QueryRowContext(ctx,
		`SELECT data FROM campaign_roll_tables WHERE campaign_id = ?`, campaignID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get roll tables: %w", err)
	}
	return []byte(data), nil
}

func (r *sqlRepository) Put(ctx context.Context, campaignID string, data []byte, userID string) error {
	var by any
	if userID != "" {
		by = userID
	}
	// A single upsert so two concurrent saves cannot race between an existence
	// check and the insert; last write wins.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_roll_tables (campaign_id, data, updated_by)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE data = VALUES(data), updated_by = VALUES(updated_by)`,
		campaignID, string(data), by)
	if err != nil {
		return fmt.Errorf("put roll tables: %w", err)
	}
	return nil
}

// Hand-written SQL for the page_block_content table.
//
// SECURITY: every statement filters on campaign_id as well as the page and
// block ids, so ids from another campaign never read or overwrite a document
// here.
package pageblocks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// Stored is one saved block document.
type Stored struct {
	Kind    string
	Data    []byte
	Version int
}

// Repository is the persistence boundary for block documents.
type Repository interface {
	// Get returns the stored document; found is false when the block has no
	// row yet.
	Get(ctx context.Context, campaignID, entityID, blockID string) (s Stored, found bool, err error)
	// Save writes the document if the stored version is still
	// expectedVersion (0 means "no row yet") and bumps it. ok is false on a
	// lost race. The kind of an existing row is never changed.
	Save(ctx context.Context, campaignID, entityID, blockID, kind string, data []byte, expectedVersion int, userID string) (ok bool, err error)
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

func (r *sqlRepository) Get(ctx context.Context, campaignID, entityID, blockID string) (Stored, bool, error) {
	var s Stored
	var data string
	err := r.db.QueryRowContext(ctx,
		`SELECT kind, data, version FROM page_block_content
		 WHERE entity_id = ? AND block_id = ? AND campaign_id = ?`,
		entityID, blockID, campaignID).Scan(&s.Kind, &data, &s.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Stored{}, false, nil
	}
	if err != nil {
		return Stored{}, false, fmt.Errorf("get page block: %w", err)
	}
	s.Data = []byte(data)
	return s, true, nil
}

func (r *sqlRepository) Save(ctx context.Context, campaignID, entityID, blockID, kind string, data []byte, expected int, userID string) (bool, error) {
	var by any
	if userID != "" {
		by = userID
	}
	if expected == 0 {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO page_block_content (entity_id, block_id, campaign_id, kind, data, version, updated_by)
			 VALUES (?, ?, ?, ?, ?, 1, ?)`,
			entityID, blockID, campaignID, kind, string(data), by)
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return false, nil // someone saved the first version first
		}
		if err != nil {
			return false, fmt.Errorf("insert page block: %w", err)
		}
		return true, nil
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE page_block_content SET data = ?, version = version + 1, updated_by = ?
		 WHERE entity_id = ? AND block_id = ? AND campaign_id = ? AND version = ?`,
		string(data), by, entityID, blockID, campaignID, expected)
	if err != nil {
		return false, fmt.Errorf("update page block: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update page block: %w", err)
	}
	return n == 1, nil
}

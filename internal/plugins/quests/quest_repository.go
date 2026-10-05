// Hand-written SQL for the quests table.
//
// SECURITY: every statement filters on campaign_id as well as the page id, so
// a page id from another campaign never reads or overwrites a sheet here.
package quests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// QuestRepository is the persistence boundary for quest sheets.
type QuestRepository interface {
	// Get returns the stored document and its version; found is false when
	// the page has no sheet yet.
	Get(ctx context.Context, campaignID, entityID string) (data []byte, version int, found bool, err error)
	// GetMany returns the stored documents of the given pages, keyed by id.
	GetMany(ctx context.Context, campaignID string, entityIDs []string) (map[string][]byte, error)
	// Save writes the document if the stored version is still expectedVersion
	// (0 means "no row yet") and bumps it. ok is false on a lost race.
	Save(ctx context.Context, campaignID, entityID string, data []byte, expectedVersion int, userID string) (ok bool, err error)
}

type sqlQuestRepository struct{ db *sql.DB }

// NewQuestRepository creates a MariaDB-backed repository.
func NewQuestRepository(db *sql.DB) QuestRepository { return &sqlQuestRepository{db: db} }

func (r *sqlQuestRepository) Get(ctx context.Context, campaignID, entityID string) ([]byte, int, bool, error) {
	var data string
	var version int
	err := r.db.QueryRowContext(ctx,
		`SELECT data, version FROM quests WHERE entity_id = ? AND campaign_id = ?`,
		entityID, campaignID).Scan(&data, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("get quest: %w", err)
	}
	return []byte(data), version, true, nil
}

func (r *sqlQuestRepository) GetMany(ctx context.Context, campaignID string, ids []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{campaignID}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT entity_id, data FROM quests WHERE campaign_id = ? AND entity_id IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("get quests: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("scan quest: %w", err)
		}
		out[id] = []byte(data)
	}
	return out, rows.Err()
}

func (r *sqlQuestRepository) Save(ctx context.Context, campaignID, entityID string, data []byte, expected int, userID string) (bool, error) {
	var by any
	if userID != "" {
		by = userID
	}
	if expected == 0 {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO quests (entity_id, campaign_id, data, version, updated_by) VALUES (?, ?, ?, 1, ?)`,
			entityID, campaignID, string(data), by)
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return false, nil // someone saved the first version first
		}
		if err != nil {
			return false, fmt.Errorf("insert quest: %w", err)
		}
		return true, nil
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE quests SET data = ?, version = version + 1, updated_by = ?
		 WHERE entity_id = ? AND campaign_id = ? AND version = ?`,
		string(data), by, entityID, campaignID, expected)
	if err != nil {
		return false, fmt.Errorf("update quest: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update quest: %w", err)
	}
	return n == 1, nil
}

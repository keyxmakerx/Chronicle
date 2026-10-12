package dmscreen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// mysqlDuplicateEntry is the MariaDB error number for a unique-key violation.
const mysqlDuplicateEntry = 1062

type boardRepository struct {
	db *sql.DB
}

// NewBoardRepository returns the MariaDB-backed board store.
func NewBoardRepository(db *sql.DB) BoardRepository {
	return &boardRepository{db: db}
}

func (r *boardRepository) List(ctx context.Context, campaignID string) ([]BoardItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, kind, COALESCE(entity_id, ''), COALESCE(body, ''), COALESCE(created_by, ''), created_at
		   FROM dm_screen_board_items
		  WHERE campaign_id = ?
		  ORDER BY created_at, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing board items: %w", err)
	}
	defer rows.Close()
	var out []BoardItem
	for rows.Next() {
		var it BoardItem
		if err := rows.Scan(&it.ID, &it.CampaignID, &it.Kind, &it.EntityID, &it.Body, &it.CreatedBy, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning board item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *boardRepository) Count(ctx context.Context, campaignID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dm_screen_board_items WHERE campaign_id = ?`, campaignID).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting board items: %w", err)
	}
	return n, nil
}

func (r *boardRepository) Add(ctx context.Context, it *BoardItem) error {
	var entity, body, by any
	if it.EntityID != "" {
		entity = it.EntityID
	}
	if it.Body != "" {
		body = it.Body
	}
	if it.CreatedBy != "" {
		by = it.CreatedBy
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dm_screen_board_items (id, campaign_id, kind, entity_id, body, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		it.ID, it.CampaignID, it.Kind, entity, body, by)
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == mysqlDuplicateEntry {
		return ErrAlreadyPinned
	}
	if err != nil {
		return fmt.Errorf("adding board item: %w", err)
	}
	return nil
}

func (r *boardRepository) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM dm_screen_board_items WHERE id = ? AND campaign_id = ?`, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("deleting board item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("deleting board item: %w", err)
	}
	return n > 0, nil
}

package armory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ItemLookRepository stores at most one look per item entity.
type ItemLookRepository interface {
	// Get returns the stored look, or (nil, nil) when the item has none.
	Get(ctx context.Context, campaignID, itemEntityID string) (json.RawMessage, error)
	// Upsert replaces the item's look, recording who saved it.
	Upsert(ctx context.Context, campaignID, itemEntityID, userID string, look json.RawMessage) error
	// Delete removes the item's look; removing a look that is not there is fine.
	Delete(ctx context.Context, campaignID, itemEntityID string) error
	// List returns up to limit looks in the campaign, keyed by page id, and
	// whether more were left unread.
	List(ctx context.Context, campaignID string, limit int) (map[string]json.RawMessage, bool, error)
}

type itemLookRepository struct {
	db *sql.DB
}

// NewItemLookRepository creates the repository.
func NewItemLookRepository(db *sql.DB) ItemLookRepository {
	return &itemLookRepository{db: db}
}

// Every query is scoped by campaign_id as well as the item, so an id from
// another campaign matches nothing.
func (r *itemLookRepository) Get(ctx context.Context, campaignID, itemEntityID string) (json.RawMessage, error) {
	var look string
	err := r.db.QueryRowContext(ctx,
		`SELECT look FROM armory_item_looks WHERE campaign_id = ? AND item_entity_id = ?`,
		campaignID, itemEntityID).Scan(&look)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return json.RawMessage(look), nil
}

func (r *itemLookRepository) Upsert(ctx context.Context, campaignID, itemEntityID, userID string, look json.RawMessage) error {
	var by any
	if userID != "" {
		by = userID
	}
	// campaign_id is not in the UPDATE list: an item never changes campaign.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO armory_item_looks (item_entity_id, campaign_id, look, updated_by)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE look = VALUES(look), updated_by = VALUES(updated_by)`,
		itemEntityID, campaignID, string(look), by)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *itemLookRepository) Delete(ctx context.Context, campaignID, itemEntityID string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM armory_item_looks WHERE campaign_id = ? AND item_entity_id = ?`,
		campaignID, itemEntityID); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *itemLookRepository) List(ctx context.Context, campaignID string, limit int) (map[string]json.RawMessage, bool, error) {
	// One row past the limit says whether the list was cut.
	rows, err := r.db.QueryContext(ctx,
		`SELECT item_entity_id, look FROM armory_item_looks WHERE campaign_id = ?
		 ORDER BY item_entity_id LIMIT ?`,
		campaignID, limit+1)
	if err != nil {
		return nil, false, apperror.NewInternal(err)
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	more := false
	for rows.Next() {
		var id, look string
		if err := rows.Scan(&id, &look); err != nil {
			return nil, false, apperror.NewInternal(err)
		}
		if len(out) == limit {
			more = true
			break
		}
		out[id] = json.RawMessage(look)
	}
	if err := rows.Err(); err != nil {
		return nil, false, apperror.NewInternal(err)
	}
	return out, more, nil
}

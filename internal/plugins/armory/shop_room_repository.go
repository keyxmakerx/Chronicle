package armory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ShopRoomRepository stores one saved room layout per shop entity.
type ShopRoomRepository interface {
	// Get returns the stored layout, or (nil, nil) when the shop has none.
	Get(ctx context.Context, campaignID, shopEntityID string) (json.RawMessage, error)
	// Upsert replaces the shop's layout, recording who saved it.
	Upsert(ctx context.Context, campaignID, shopEntityID, userID string, layout json.RawMessage) error
}

type shopRoomRepository struct {
	db *sql.DB
}

// NewShopRoomRepository creates the repository.
func NewShopRoomRepository(db *sql.DB) ShopRoomRepository {
	return &shopRoomRepository{db: db}
}

// Get is scoped by campaign_id as well as the shop so an id from another
// campaign matches nothing.
func (r *shopRoomRepository) Get(ctx context.Context, campaignID, shopEntityID string) (json.RawMessage, error) {
	var layout string
	err := r.db.QueryRowContext(ctx,
		`SELECT layout FROM shop_rooms WHERE campaign_id = ? AND shop_entity_id = ?`,
		campaignID, shopEntityID).Scan(&layout)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return json.RawMessage(layout), nil
}

func (r *shopRoomRepository) Upsert(ctx context.Context, campaignID, shopEntityID, userID string, layout json.RawMessage) error {
	var by any
	if userID != "" {
		by = userID
	}
	// campaign_id is not in the UPDATE list: a shop never changes campaign.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO shop_rooms (shop_entity_id, campaign_id, layout, updated_by)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE layout = VALUES(layout), updated_by = VALUES(updated_by)`,
		shopEntityID, campaignID, string(layout), by)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

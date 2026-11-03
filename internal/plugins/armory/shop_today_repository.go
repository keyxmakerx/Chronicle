package armory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ShopShelfRepository stores how many places a shop's today's shelf has.
type ShopShelfRepository interface {
	// GetSpots returns the stored number, or 0 when the shop has none.
	GetSpots(ctx context.Context, campaignID, shopEntityID string) (int, error)
	UpsertSpots(ctx context.Context, campaignID, shopEntityID, userID string, spots int) error
}

type shopShelfRepository struct {
	db *sql.DB
}

// NewShopShelfRepository creates the repository.
func NewShopShelfRepository(db *sql.DB) ShopShelfRepository {
	return &shopShelfRepository{db: db}
}

// GetSpots is scoped by campaign as well as shop, so an id from another
// campaign matches nothing.
func (r *shopShelfRepository) GetSpots(ctx context.Context, campaignID, shopEntityID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT today_spots FROM armory_shop_shelves WHERE campaign_id = ? AND shop_entity_id = ?`,
		campaignID, shopEntityID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	return n, nil
}

func (r *shopShelfRepository) UpsertSpots(ctx context.Context, campaignID, shopEntityID, userID string, spots int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO armory_shop_shelves (shop_entity_id, campaign_id, today_spots, updated_by)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE today_spots = VALUES(today_spots), updated_by = VALUES(updated_by)`,
		shopEntityID, campaignID, spots, userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

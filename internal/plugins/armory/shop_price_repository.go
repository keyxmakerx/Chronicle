package armory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DefaultFoodPrice is what a day's food costs in the campaign's main coin
// until a Director says otherwise.
const DefaultFoodPrice = 1.0

// ItemPrice is a price an item page already sells at in some shop.
type ItemPrice struct {
	Price    float64
	Currency string
}

// ShopPriceRepository stores the one number everyday prices scale from and
// reads prices the campaign has already set on listings.
type ShopPriceRepository interface {
	// FoodPrice is a day's food in the main coin; DefaultFoodPrice with no row.
	FoodPrice(ctx context.Context, campaignID string) (float64, error)
	SaveFoodPrice(ctx context.Context, campaignID, userID string, price float64) error
	// ItemPrices maps item page ids to the newest positive price a shop lists
	// them at, so a fill reuses what the Director already decided.
	ItemPrices(ctx context.Context, campaignID string) (map[string]ItemPrice, error)
}

type shopPriceRepository struct{ db *sql.DB }

// NewShopPriceRepository creates the repository.
func NewShopPriceRepository(db *sql.DB) ShopPriceRepository { return &shopPriceRepository{db: db} }

func (r *shopPriceRepository) FoodPrice(ctx context.Context, campaignID string) (float64, error) {
	var p float64
	err := r.db.QueryRowContext(ctx,
		`SELECT food_price FROM armory_campaign_prices WHERE campaign_id = ?`, campaignID).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultFoodPrice, nil
	}
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	return p, nil
}

func (r *shopPriceRepository) SaveFoodPrice(ctx context.Context, campaignID, userID string, price float64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO armory_campaign_prices (campaign_id, food_price, updated_by) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE food_price = VALUES(food_price), updated_by = VALUES(updated_by)`,
		campaignID, price, userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *shopPriceRepository) ItemPrices(ctx context.Context, campaignID string) (map[string]ItemPrice, error) {
	// Newest listing first, so the first row seen per item wins.
	rows, err := r.db.QueryContext(ctx,
		`SELECT er.target_entity_id,
		        CAST(JSON_EXTRACT(er.metadata, '$.price') AS DECIMAL(14,2)),
		        COALESCE(JSON_UNQUOTE(JSON_EXTRACT(er.metadata, '$.currency')), '')
		 FROM entity_relations er
		 WHERE er.campaign_id = ? AND er.relation_type = 'sells'
		   AND er.metadata IS NOT NULL
		   AND CAST(JSON_EXTRACT(er.metadata, '$.price') AS DECIMAL(14,2)) > 0
		 ORDER BY er.id DESC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("reading listed prices: %w", err)
	}
	defer rows.Close()
	out := map[string]ItemPrice{}
	for rows.Next() {
		var id string
		var p ItemPrice
		if err := rows.Scan(&id, &p.Price, &p.Currency); err != nil {
			return nil, fmt.Errorf("scanning listed price: %w", err)
		}
		if _, seen := out[id]; !seen {
			out[id] = p
		}
	}
	return out, rows.Err()
}

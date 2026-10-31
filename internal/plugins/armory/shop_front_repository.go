package armory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ShopFrontRepository stores one storefront per shop entity.
type ShopFrontRepository interface {
	// Get returns the stored storefront, or (nil, nil) when the shop has none.
	Get(ctx context.Context, campaignID, shopEntityID string) (json.RawMessage, error)
	// ListFor returns the stored storefronts of the given shops, keyed by shop.
	ListFor(ctx context.Context, campaignID string, shopEntityIDs []string) (map[string]json.RawMessage, error)
	Upsert(ctx context.Context, campaignID, shopEntityID, userID string, front json.RawMessage) error
}

type shopFrontRepository struct {
	db *sql.DB
}

// NewShopFrontRepository creates the repository.
func NewShopFrontRepository(db *sql.DB) ShopFrontRepository {
	return &shopFrontRepository{db: db}
}

// Get is scoped by campaign as well as shop, so an id from another campaign
// matches nothing.
func (r *shopFrontRepository) Get(ctx context.Context, campaignID, shopEntityID string) (json.RawMessage, error) {
	var front string
	err := r.db.QueryRowContext(ctx,
		`SELECT front FROM armory_shop_fronts WHERE campaign_id = ? AND shop_entity_id = ?`,
		campaignID, shopEntityID).Scan(&front)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return json.RawMessage(front), nil
}

// shopFrontChunk bounds the placeholders in one IN list.
const shopFrontChunk = 200

func (r *shopFrontRepository) ListFor(ctx context.Context, campaignID string, ids []string) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(ids))
	for start := 0; start < len(ids); start += shopFrontChunk {
		end := min(start+shopFrontChunk, len(ids))
		chunk := ids[start:end]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, campaignID)
		for _, id := range chunk {
			args = append(args, id)
		}
		rows, err := r.db.QueryContext(ctx,
			`SELECT shop_entity_id, front FROM armory_shop_fronts
			 WHERE campaign_id = ? AND shop_entity_id IN (?`+strings.Repeat(",?", len(chunk)-1)+`)`, args...)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		for rows.Next() {
			var id, front string
			if err := rows.Scan(&id, &front); err != nil {
				rows.Close()
				return nil, apperror.NewInternal(err)
			}
			out[id] = json.RawMessage(front)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	return out, nil
}

func (r *shopFrontRepository) Upsert(ctx context.Context, campaignID, shopEntityID, userID string, front json.RawMessage) error {
	var by any
	if userID != "" {
		by = userID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO armory_shop_fronts (shop_entity_id, campaign_id, front, updated_by)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE front = VALUES(front), updated_by = VALUES(updated_by)`,
		shopEntityID, campaignID, string(front), by)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

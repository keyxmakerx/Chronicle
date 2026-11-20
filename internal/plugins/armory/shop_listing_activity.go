package armory

import (
	"context"
	"database/sql"
	"strings"
)

// ListingActivity finds which listings of a shop have a player's record
// attached: a counter roll, a purchase, or a Director's note on the good.
// Removing such a listing would delete the roll and the note with it (they
// follow the listing), so Undo leaves it in place.
type ListingActivity interface {
	// Active returns the relation ids among ids that have any record.
	Active(ctx context.Context, campaignID, shopEntityID string, ids []int) (map[int]bool, error)
}

type listingActivity struct{ db *sql.DB }

// NewListingActivity reads the armory's own tables.
func NewListingActivity(db *sql.DB) ListingActivity { return &listingActivity{db: db} }

func (r *listingActivity) Active(ctx context.Context, campaignID, shopEntityID string, ids []int) (map[int]bool, error) {
	out := map[int]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	for _, table := range []string{"shop_counter_rolls", "shop_good_secrets", "shop_transactions"} {
		args := []any{campaignID, shopEntityID}
		for _, id := range ids {
			args = append(args, id)
		}
		rows, err := r.db.QueryContext(ctx,
			"SELECT DISTINCT relation_id FROM "+table+" WHERE campaign_id = ? AND shop_entity_id = ? AND relation_id IN ("+marks+")", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

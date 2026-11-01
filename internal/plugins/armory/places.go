// places.go is where a hero carries each item: in hand, worn, a belt pouch
// or the backpack. It only arranges the hero page; counts, moves and what an
// item remembers are unchanged by it.
package armory

import (
	"context"
	"database/sql"
)

// Place is one of a hero's containers.
type Place string

const (
	PlaceHand Place = "hand"
	PlaceWorn Place = "worn"
	PlaceBelt Place = "belt"
	PlacePack Place = "pack"
)

// Places lists the containers in the order the hero page draws them.
var Places = []Place{PlaceHand, PlaceWorn, PlaceBelt, PlacePack}

// Label is the container's name on the page.
func (p Place) Label() string {
	switch p {
	case PlaceHand:
		return "In hand"
	case PlaceWorn:
		return "Worn"
	case PlaceBelt:
		return "Belt pouch"
	}
	return "Backpack"
}

// ParsePlace reads a submitted place; anything unknown is refused.
func ParsePlace(s string) (Place, bool) {
	for _, p := range Places {
		if string(p) == s {
			return p, true
		}
	}
	return "", false
}

// PlaceStore keeps where a character carries each item.
type PlaceStore interface {
	// List returns the character's placed items by item id; an item not in
	// the map is in the backpack.
	List(ctx context.Context, campaignID, characterID string) (map[string]Place, error)
	// Set puts an item in a container; the backpack removes the row.
	Set(ctx context.Context, campaignID, characterID, itemID string, p Place) error
}

type placeRepository struct{ db *sql.DB }

// NewPlaceRepository returns the SQL place store.
func NewPlaceRepository(db *sql.DB) PlaceStore { return &placeRepository{db: db} }

func (r *placeRepository) List(ctx context.Context, campaignID, characterID string) (map[string]Place, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT item_entity_id, place FROM armory_item_places
		WHERE campaign_id = ? AND character_id = ?`, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Place{}
	for rows.Next() {
		var id, p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		if pl, ok := ParsePlace(p); ok {
			out[id] = pl
		}
	}
	return out, rows.Err()
}

func (r *placeRepository) Set(ctx context.Context, campaignID, characterID, itemID string, p Place) error {
	if p == PlacePack {
		_, err := r.db.ExecContext(ctx, `DELETE FROM armory_item_places
			WHERE campaign_id = ? AND character_id = ? AND item_entity_id = ?`, campaignID, characterID, itemID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO armory_item_places (campaign_id, character_id, item_entity_id, place)
		VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE place = VALUES(place)`, campaignID, characterID, itemID, string(p))
	return err
}

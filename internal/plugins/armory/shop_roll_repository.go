package armory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// errAlreadyTried is what InsertRoll reports when the character has already
// made this roll on this good today. The service turns it into a 409.
var errAlreadyTried = errors.New("shop roll: already tried today")

// ShopRollRepository stores the owner's hidden notes and the counter rolls.
// Every query is scoped by campaign and shop, so an id from another campaign
// or shop matches nothing.
type ShopRollRepository interface {
	// Secrets returns every hidden note of a shop by relation id.
	Secrets(ctx context.Context, campaignID, shopEntityID string) (map[int]GoodSecret, error)
	// SaveSecret stores a good's note and extra; both empty deletes the row.
	SaveSecret(ctx context.Context, campaignID, shopEntityID string, relationID int, s GoodSecret) error
	// InsertRoll records a roll. The table's unique key is the one-try-a-day
	// rule, so a repeat is errAlreadyTried and nothing is stored.
	InsertRoll(ctx context.Context, r ShopRoll) (int64, error)
	// CharacterRolls returns every roll a character has made at a shop, on any day.
	CharacterRolls(ctx context.Context, campaignID, shopEntityID, characterID string) ([]ShopRoll, error)
	// Haggles returns the price change in percent of each good the character
	// haggled over on one shelf day, by relation id. It is the buy route's read.
	Haggles(ctx context.Context, campaignID, shopEntityID, characterID string, shelfDay int64) (map[int]int, error)
}

type shopRollRepository struct {
	db *sql.DB
}

// NewShopRollRepository creates the repository.
func NewShopRollRepository(db *sql.DB) ShopRollRepository {
	return &shopRollRepository{db: db}
}

func (r *shopRollRepository) Secrets(ctx context.Context, campaignID, shopEntityID string) (map[int]GoodSecret, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT relation_id, note, extra FROM shop_good_secrets WHERE campaign_id = ? AND shop_entity_id = ?`,
		campaignID, shopEntityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()
	out := map[int]GoodSecret{}
	for rows.Next() {
		var id int
		var s GoodSecret
		if err := rows.Scan(&id, &s.Note, &s.Extra); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out[id] = s
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *shopRollRepository) SaveSecret(ctx context.Context, campaignID, shopEntityID string, relationID int, s GoodSecret) error {
	var err error
	if s.Note == "" && s.Extra == "" {
		_, err = r.db.ExecContext(ctx,
			`DELETE FROM shop_good_secrets WHERE campaign_id = ? AND shop_entity_id = ? AND relation_id = ?`,
			campaignID, shopEntityID, relationID)
	} else {
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO shop_good_secrets (campaign_id, shop_entity_id, relation_id, note, extra)
			 VALUES (?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE note = VALUES(note), extra = VALUES(extra)`,
			campaignID, shopEntityID, relationID, s.Note, s.Extra)
	}
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *shopRollRepository) InsertRoll(ctx context.Context, row ShopRoll) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO shop_counter_rolls (campaign_id, shop_entity_id, relation_id, character_id, action, shelf_day, tier, pct, roll_total)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.CampaignID, row.ShopEntityID, row.RelationID, row.CharacterID, row.Action, row.ShelfDay, row.Tier, row.Pct, row.Total)
	if err != nil {
		if isDuplicateKey(err) {
			return 0, errAlreadyTried
		}
		return 0, apperror.NewInternal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	return id, nil
}

func (r *shopRollRepository) CharacterRolls(ctx context.Context, campaignID, shopEntityID, characterID string) ([]ShopRoll, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, relation_id, action, shelf_day, tier, pct, roll_total FROM shop_counter_rolls
		 WHERE campaign_id = ? AND shop_entity_id = ? AND character_id = ? ORDER BY id`,
		campaignID, shopEntityID, characterID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()
	var out []ShopRoll
	for rows.Next() {
		x := ShopRoll{CampaignID: campaignID, ShopEntityID: shopEntityID, CharacterID: characterID}
		if err := rows.Scan(&x.ID, &x.RelationID, &x.Action, &x.ShelfDay, &x.Tier, &x.Pct, &x.Total); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *shopRollRepository) Haggles(ctx context.Context, campaignID, shopEntityID, characterID string, shelfDay int64) (map[int]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT relation_id, pct FROM shop_counter_rolls
		 WHERE campaign_id = ? AND shop_entity_id = ? AND character_id = ? AND action = ? AND shelf_day = ?`,
		campaignID, shopEntityID, characterID, RollHaggle, shelfDay)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, pct int
		if err := rows.Scan(&id, &pct); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out[id] = pct
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

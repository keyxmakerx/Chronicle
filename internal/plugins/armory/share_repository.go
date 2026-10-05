// share_repository.go holds the SQL for item shares: which view grants on a
// hidden item came from a holder sharing it (armory_item_shares, a plugin
// migration). Every statement is scoped by campaign_id.
package armory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ItemShare is one holder sharing one item with one player. MadeGrant is set
// on the row whose share added the player's view grant, so taking the share
// back knows the grant is sharing's to remove.
type ItemShare struct {
	CampaignID  string
	CharacterID string
	ItemID      string
	UserID      string
	MadeGrant   bool
	SharedBy    string
}

// ShareStore is the data access contract for item shares.
type ShareStore interface {
	// ListByCharacter returns every share the character's holdings carry.
	ListByCharacter(ctx context.Context, campaignID, characterID string) ([]ItemShare, error)
	// ListByItem returns every holder's shares of the item.
	ListByItem(ctx context.Context, campaignID, itemID string) ([]ItemShare, error)
	// Add records a share; recording one that exists changes nothing.
	Add(ctx context.Context, s ItemShare) error
	// Remove forgets one share.
	Remove(ctx context.Context, campaignID, characterID, itemID, userID string) error
	// SetMadeGrant hands the grant's ownership to another share of the same
	// item and player when the one that made it goes.
	SetMadeGrant(ctx context.Context, campaignID, characterID, itemID, userID string) error
}

type shareRepository struct{ db *sql.DB }

// NewShareRepository creates the item share repository.
func NewShareRepository(db *sql.DB) ShareStore { return &shareRepository{db: db} }

func shareDBError(what string, err error) error {
	return apperror.NewInternal(fmt.Errorf("%s: %w", what, err))
}

func (r *shareRepository) list(ctx context.Context, query string, args ...any) ([]ItemShare, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, shareDBError("listing item shares", err)
	}
	defer rows.Close()
	var out []ItemShare
	for rows.Next() {
		var s ItemShare
		if err := rows.Scan(&s.CampaignID, &s.CharacterID, &s.ItemID, &s.UserID, &s.MadeGrant, &s.SharedBy); err != nil {
			return nil, shareDBError("reading an item share", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, shareDBError("listing item shares", err)
	}
	return out, nil
}

const shareColumns = `campaign_id, character_id, item_entity_id, user_id, made_grant, shared_by`

func (r *shareRepository) ListByCharacter(ctx context.Context, campaignID, characterID string) ([]ItemShare, error) {
	return r.list(ctx, `SELECT `+shareColumns+` FROM armory_item_shares
		WHERE campaign_id = ? AND character_id = ? ORDER BY item_entity_id, created_at, user_id`, campaignID, characterID)
}

func (r *shareRepository) ListByItem(ctx context.Context, campaignID, itemID string) ([]ItemShare, error) {
	return r.list(ctx, `SELECT `+shareColumns+` FROM armory_item_shares
		WHERE campaign_id = ? AND item_entity_id = ? ORDER BY created_at, character_id, user_id`, campaignID, itemID)
}

func (r *shareRepository) Add(ctx context.Context, s ItemShare) error {
	_, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO armory_item_shares (`+shareColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		s.CampaignID, s.CharacterID, s.ItemID, s.UserID, s.MadeGrant, s.SharedBy)
	if err != nil {
		return shareDBError("recording an item share", err)
	}
	return nil
}

func (r *shareRepository) Remove(ctx context.Context, campaignID, characterID, itemID, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM armory_item_shares
		WHERE campaign_id = ? AND character_id = ? AND item_entity_id = ? AND user_id = ?`, campaignID, characterID, itemID, userID)
	if err != nil {
		return shareDBError("removing an item share", err)
	}
	return nil
}

func (r *shareRepository) SetMadeGrant(ctx context.Context, campaignID, characterID, itemID, userID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE armory_item_shares SET made_grant = 1
		WHERE campaign_id = ? AND character_id = ? AND item_entity_id = ? AND user_id = ?`, campaignID, characterID, itemID, userID)
	if err != nil {
		return shareDBError("updating an item share", err)
	}
	return nil
}

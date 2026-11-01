package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
)

// aiCarriedAdapter implements records.CarriedAPI over the Armory's stash
// service, so a carried-item change the owner approves is a give or a take
// in the item history. Where the Armory does not keep the count (it is off
// for the campaign, the item is not an Armory item, or the reviewer is a
// Scribe, who may not give), the AI writes the relation as before.
type aiCarriedAdapter struct {
	stash  armory.StashService
	dir    armory.StashDirectory
	addons aiAddonChecker
}

var _ records.CarriedAPI = aiCarriedAdapter{}

func (c aiCarriedAdapter) Counts(ctx context.Context, campaignID string, a records.Actor, characterID, itemID string) bool {
	if !(armory.Actor{UserID: a.UserID, Role: a.Role}).IsOwner() {
		return false
	}
	if on, err := c.addons.IsEnabledForCampaign(ctx, campaignID, armory.AddonSlug); err != nil || !on {
		return false
	}
	char, err := c.dir.GetEntity(ctx, campaignID, characterID)
	if err != nil || char == nil || !char.IsCharacter {
		return false
	}
	item, err := c.dir.GetEntity(ctx, campaignID, itemID)
	return err == nil && item != nil && item.IsItem
}

func (c aiCarriedAdapter) SetCarried(ctx context.Context, campaignID string, a records.Actor, characterID, itemID string, qty int) error {
	_, err := c.stash.SetCarried(ctx, campaignID, armory.Actor{UserID: a.UserID, Role: a.Role}, characterID, itemID, qty)
	return err
}

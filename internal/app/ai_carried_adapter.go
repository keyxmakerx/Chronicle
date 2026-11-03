package app

import (
	"context"
	"strconv"
	"strings"

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

var _ records.CopiesAPI = aiCarriedAdapter{}

// Remembered reads the character's pack through the Armory, which applies its
// own rule on who may look (the character's player or a GM); anyone else, a
// campaign without the Armory, or a page that isn't a character reads as
// nothing remembered rather than an error.
func (c aiCarriedAdapter) Remembered(ctx context.Context, campaignID string, a records.Actor, characterID string) (map[string]string, error) {
	if on, err := c.addons.IsEnabledForCampaign(ctx, campaignID, armory.AddonSlug); err != nil || !on {
		return nil, nil
	}
	pack, err := c.stash.CraftPack(ctx, campaignID, armory.Actor{UserID: a.UserID, Role: a.Role}, characterID)
	if err != nil || pack == nil {
		return nil, nil
	}
	out := map[string]string{}
	for _, it := range pack.Items {
		if w := copiesWords(it.Copies); w != "" {
			out[it.ItemID] = w
		}
	}
	return out, nil
}

// copiesWords is an item's remembered copies in a line, as the pack shows
// them: "1 Fine · Moonlit (Made by Orrin, 3 Frostmoot); 1 Poor".
func copiesWords(copies []armory.Copy) string {
	var parts []string
	for _, cp := range copies {
		var tags []string
		for _, t := range cp.Memory.Tags() {
			tags = append(tags, t.Label)
		}
		w := strconv.Itoa(cp.Quantity)
		if len(tags) > 0 {
			w += " " + strings.Join(tags, " · ")
		}
		if more := cp.Memory.Words(); more != "" {
			w += " (" + more + ")"
		}
		if len(tags) > 0 || cp.Memory.Words() != "" {
			parts = append(parts, w)
		}
	}
	return strings.Join(parts, "; ")
}

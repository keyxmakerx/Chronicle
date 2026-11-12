package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/designs"
)

// designsDirectory answers the saved-designs plugin's questions about
// campaigns over the campaigns service, so that plugin never imports it.
type designsDirectory struct{ svc campaigns.CampaignService }

var _ designs.CampaignDirectory = designsDirectory{}

func (d designsDirectory) Campaign(ctx context.Context, id string) (*designs.CampaignInfo, error) {
	c, err := d.svc.GetByID(ctx, id)
	if err != nil {
		// A missing or trashed campaign is a clean "none", not a failure.
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &designs.CampaignInfo{ID: c.ID, Name: c.Name, SystemID: c.ParseSettings().SystemID}, nil
}

// Live is true for a campaign that exists, is not in the Trash and is not
// archived.
func (d designsDirectory) Live(ctx context.Context, id string) (bool, error) {
	c, err := d.svc.GetByID(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return !c.IsArchived(), nil
}

// Member counts only the Owner role. A co-DM shares the Owner's visibility
// role but is not the campaign's Owner, and saved designs are the Owner's.
func (d designsDirectory) Member(ctx context.Context, campaignID, userID string) (string, bool, error) {
	m, err := d.svc.GetMember(ctx, campaignID, userID)
	if err != nil {
		if isNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return m.DisplayName, m.Role >= campaigns.RoleOwner, nil
}

func (d designsDirectory) OwnedCampaigns(ctx context.Context, userID string) ([]designs.CampaignInfo, error) {
	owned, err := d.svc.OwnedCampaigns(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out []designs.CampaignInfo
	for _, o := range owned {
		// OwnedCampaigns also lists campaigns the user created but no longer
		// owns, and archived ones, which are read-only; neither can take a design.
		if _, isOwner, err := d.Member(ctx, o.ID, userID); err != nil || !isOwner {
			continue
		}
		c, err := d.svc.GetByID(ctx, o.ID)
		if err != nil || c.ArchivedAt != nil {
			continue
		}
		out = append(out, designs.CampaignInfo{ID: c.ID, Name: c.Name, SystemID: c.ParseSettings().SystemID})
	}
	return out, nil
}

// roomThemeKind is the armory's say over designs of kind room-theme: the same
// counter-theme rules a saved room is held to. It has no needs of the
// receiving campaign, so Lacks says nothing.
type roomThemeKind struct{}

var _ designs.Kind = roomThemeKind{}

func (roomThemeKind) Validate(raw json.RawMessage) (json.RawMessage, error) {
	return armory.NormalizeShopRoomCounter(raw)
}

func (roomThemeKind) Lacks(context.Context, string, json.RawMessage) ([]string, error) {
	return nil, nil
}

// newDesignKinds is the registry every feature adds its design kinds to.
// Crafting registers crafting-circle, crafting-item-look and crafting-bench on
// it when its plugin is up (see the designs plugin's .ai.md).
func newDesignKinds() *designs.Kinds {
	k := designs.NewKinds()
	k.Register(designs.KindRoomTheme, roomThemeKind{})
	return k
}

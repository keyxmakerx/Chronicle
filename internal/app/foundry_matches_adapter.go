package app

import (
	"context"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// foundryCreaturePagesMax caps one walk of the creature pages. Past it the
// match list says it is cut rather than walking without end.
const foundryCreaturePagesMax = 2000

// foundryCreaturePages lists a campaign's creature pages for Foundry
// matching, at the DM's visibility: matching is DM-only, and a page hidden
// from players is still the DM's creature.
type foundryCreaturePages struct {
	entities entities.EntityService
}

// typeIDs are the campaign's creature page types.
func (a foundryCreaturePages) typeIDs(ctx context.Context, campaignID string) ([]int, error) {
	types, err := a.entities.GetEntityTypesByPresetCategory(ctx, campaignID, "creature")
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(types))
	for _, t := range types {
		out = append(out, t.ID)
	}
	return out, nil
}

func (a foundryCreaturePages) CreaturePages(ctx context.Context, campaignID string) ([]syncapi.CreaturePage, bool, error) {
	ids, err := a.typeIDs(ctx, campaignID)
	if err != nil {
		return nil, false, err
	}
	var out []syncapi.CreaturePage
	for _, typeID := range ids {
		for page := 1; ; page++ {
			list, total, err := a.entities.List(ctx, campaignID, typeID, int(campaigns.RoleOwner), "", entities.ListOptions{Page: page, PerPage: 100, Sort: "name"})
			if err != nil {
				return nil, false, err
			}
			for _, e := range list {
				if len(out) == foundryCreaturePagesMax {
					return out, true, nil
				}
				out = append(out, syncapi.CreaturePage{ID: e.ID, Name: e.Name})
			}
			if len(list) == 0 || page*100 >= total {
				break
			}
		}
	}
	return out, false, nil
}

func (a foundryCreaturePages) CreaturePage(ctx context.Context, campaignID, entityID string) (*syncapi.CreaturePage, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil, nil
	}
	ids, err := a.typeIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(ids, e.EntityTypeID) {
		return nil, nil
	}
	return &syncapi.CreaturePage{ID: e.ID, Name: e.Name}, nil
}

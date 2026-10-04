package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// page_extras.go is the one-time reconciler for the page pieces that became
// blocks (sub-pages, posts, writing prompts, backlinks, game-system panels,
// items and money). Before, every page drew them outside its layout; now a
// page shows only what its layout places. So that no existing page loses
// anything, this places them once into every existing entity type's layout,
// where they used to appear. It runs once per site: afterwards owners decide,
// and a block they remove must stay removed.

// pageExtrasPlacedKey is the site setting that records the reconciler ran.
const pageExtrasPlacedKey = "reconcile.page_extras_placed"

// pageExtrasSettings is the slice of the settings repository it needs.
type pageExtrasSettings interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// pageExtrasCampaigns lists every campaign, a page at a time.
type pageExtrasCampaigns interface {
	ListAll(ctx context.Context, opts campaigns.ListOptions) ([]campaigns.Campaign, int, error)
}

// pageExtrasAddons says whether a campaign has the armory on.
type pageExtrasAddons interface {
	IsEnabledForCampaign(ctx context.Context, campaignID, addonSlug string) (bool, error)
}

// pageExtrasEntities is the slice of the entity service it needs.
type pageExtrasEntities interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	PlacePageExtras(ctx context.Context, campaignID string, plans map[int]entities.PageExtrasPlan) (int, error)
}

// placePageExtrasOnce places the old page pieces into every campaign's
// layouts unless that already happened. The setting is written only after
// every campaign was attempted, so a crash part-way repeats the sweep, which
// is harmless: a block already placed is never added twice.
func placePageExtrasOnce(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, addons pageExtrasAddons, ents pageExtrasEntities) (int, error) {
	if v, err := st.Get(ctx, pageExtrasPlacedKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", pageExtrasPlacedKey, err)
		}
	}

	changed := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return changed, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			types, err := ents.GetEntityTypes(ctx, c.ID)
			if err != nil {
				slog.Warn("page extras: entity types unavailable",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			armory, err := addons.IsEnabledForCampaign(ctx, c.ID, "armory")
			if err != nil {
				slog.Warn("page extras: armory switch unavailable",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
			}
			n, err := ents.PlacePageExtras(ctx, c.ID, pageExtrasPlans(types, armory))
			if err != nil {
				slog.Warn("page extras: placing failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			changed += n
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, pageExtrasPlacedKey, "1"); err != nil {
		return changed, fmt.Errorf("recording %s: %w", pageExtrasPlacedKey, err)
	}
	return changed, nil
}

// pageExtrasPlans says, per entity type, which panels its pages showed
// outside the layout: game-system panels on NPC pages (newSystemPanelResolver)
// and, with the armory on, the items-and-money panel on the types that asked
// for it and that the armory treats as characters (armoryStashDirectoryAdapter).
func pageExtrasPlans(types []entities.EntityType, armory bool) map[int]entities.PageExtrasPlan {
	plans := make(map[int]entities.PageExtrasPlan, len(types))
	for _, id := range npcTypeIDs(types) {
		p := plans[id]
		p.SystemPanels = true
		plans[id] = p
	}
	if !armory {
		return plans
	}
	byID := make(map[int]*entities.EntityType, len(types))
	for i := range types {
		byID[types[i].ID] = &types[i]
	}
	for _, id := range characterFamilyTypeIDs(types, true) {
		if !entities.ShowedCharacterPanel(byID[id]) {
			continue
		}
		p := plans[id]
		p.CharacterItems = true
		plans[id] = p
	}
	return plans
}

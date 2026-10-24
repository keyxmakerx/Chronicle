package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// creature_panels.go is the one-time reconciler that gives existing creature
// types the game-system panels block. Game systems can aim panels at creature
// pages (a stat block), and a page shows panels only where its layout places
// the block; new creature types start with it, and this places it once on the
// ones that already exist. It runs once per site, so a block an owner later
// removes stays removed.

// creaturePanelsPlacedKey is the site setting that records the reconciler ran.
const creaturePanelsPlacedKey = "reconcile.creature_panels_placed"

// creaturePanelsNewCampaignsKey records the second sweep: campaigns made after
// the first one ran were seeded without the block, because a new campaign's
// starting categories did not go through the path that adds it.
const creaturePanelsNewCampaignsKey = "reconcile.creature_panels_new_campaigns"

// creaturePanelsSince is the day the first sweep shipped. Campaigns made
// before it were covered by that sweep, and an owner there may have removed
// the block since, so the second sweep leaves them alone.
var creaturePanelsSince = time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC)

// creaturePanelsEntities is the slice of the entity service it needs.
type creaturePanelsEntities interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	PlaceSystemPanels(ctx context.Context, campaignID string, typeIDs []int) (int, error)
}

// placeCreaturePanelsOnce places the panels block on every campaign's
// creature types unless that already happened. The setting is written only
// after every campaign was attempted, so a crash part-way repeats the sweep,
// which is harmless: a layout that has the block is left alone.
func placeCreaturePanelsOnce(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, ents creaturePanelsEntities) (int, error) {
	return placeCreaturePanels(ctx, st, camps, ents, creaturePanelsPlacedKey, time.Time{})
}

// placeCreaturePanelsOnNewCampaigns is the same sweep over campaigns made
// since creaturePanelsSince, once.
func placeCreaturePanelsOnNewCampaigns(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, ents creaturePanelsEntities) (int, error) {
	return placeCreaturePanels(ctx, st, camps, ents, creaturePanelsNewCampaignsKey, creaturePanelsSince)
}

// placeCreaturePanels runs one sweep recorded under key, over the campaigns
// made at or after since (every campaign when since is zero).
func placeCreaturePanels(ctx context.Context, st pageExtrasSettings, camps pageExtrasCampaigns, ents creaturePanelsEntities, key string, since time.Time) (int, error) {
	if v, err := st.Get(ctx, key); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", key, err)
		}
	}

	changed := 0
	for page := 1; ; page++ {
		list, total, err := camps.ListAll(ctx, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return changed, fmt.Errorf("listing campaigns: %w", err)
		}
		for _, c := range list {
			if !since.IsZero() && c.CreatedAt.Before(since) {
				continue
			}
			types, err := ents.GetEntityTypes(ctx, c.ID)
			if err != nil {
				slog.Warn("creature panels: entity types unavailable",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			ids := entities.CreatureTypeIDs(types)
			if len(ids) == 0 {
				continue
			}
			n, err := ents.PlaceSystemPanels(ctx, c.ID, ids)
			if err != nil {
				slog.Warn("creature panels: placing failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			changed += n
		}
		if len(list) == 0 || page*100 >= total {
			break
		}
	}
	if err := st.Set(ctx, key, "1"); err != nil {
		return changed, fmt.Errorf("recording %s: %w", key, err)
	}
	return changed, nil
}

// Package app wires together all application dependencies.
//
// This file holds one-time, idempotent startup backfills: data fix-ups that
// replay an addon's enable-effects across campaigns that enabled the addon
// BEFORE the effect existed. They run through the owning services (never
// hand-rolled SQL), so they can never drift from the service's definition of
// the data, and they are safe to run on every boot.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pcBackfillAddons is the slice of the addon service the player-character-type
// backfill needs (narrowed for testability).
type pcBackfillAddons interface {
	ListCampaignsUsingAddon(ctx context.Context, addonSlug string) ([]string, error)
}

// pcBackfillEntities is the slice of the entity service the backfill needs.
type pcBackfillEntities interface {
	EnsurePlayerCharacterType(ctx context.Context, campaignID string) error
}

// backfillPlayerCharacterTypes premakes the claimable "Player Character" type
// for every campaign that already has the Player Character Claiming addon
// enabled. The enable hook (ApplyAddonEnableEffects) only fires on a fresh
// enable, so campaigns that turned the addon on before that effect shipped
// would otherwise lack the premade type until they re-toggled it. Running this
// at startup heals them.
//
// EnsurePlayerCharacterType is a no-op where a player-character type already
// exists, so this is idempotent and self-healing — safe to run on every boot.
// Per-campaign failures are logged and skipped so one bad campaign neither
// aborts the sweep nor blocks startup. Returns the number of campaigns
// processed without error.
func backfillPlayerCharacterTypes(ctx context.Context, addonSvc pcBackfillAddons, entitySvc pcBackfillEntities) (int, error) {
	campaignIDs, err := addonSvc.ListCampaignsUsingAddon(ctx, entities.AddonPlayerCharacterClaiming)
	if err != nil {
		return 0, fmt.Errorf("listing campaigns with the claiming addon: %w", err)
	}

	processed := 0
	for _, campaignID := range campaignIDs {
		if err := entitySvc.EnsurePlayerCharacterType(ctx, campaignID); err != nil {
			slog.Warn("player-character-type backfill: ensure failed for campaign",
				slog.String("campaign_id", campaignID),
				slog.Any("error", err),
			)
			continue
		}
		processed++
	}
	return processed, nil
}

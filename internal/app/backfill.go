// Package app wires together all application dependencies.
//
// This file holds one-time, idempotent startup backfills: data fix-ups that
// replay an addon's enable-effects for campaigns that enabled it before the
// effect existed. They run through the owning services, never hand-rolled
// SQL, so they stay safe to run on every boot.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
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

// backfillPlayerCharacterTypes ensures the claimable "Player Character"
// sub-type is present and nested under the default "Characters" category for
// every campaign with the Player Character Claiming addon enabled — heals
// campaigns that enabled the addon before ApplyAddonEnableEffects created
// this type, since that hook only fires on a fresh enable.
//
// EnsurePlayerCharacterType is idempotent (re-parents, creates, or no-ops as
// needed), so this is safe to run on every boot. Per-campaign failures are
// logged and skipped rather than aborting the sweep. Returns the number of
// campaigns processed without error.
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

// sheetFieldReconciler is the slice of the preset applier the sheet-field
// sweep needs.
type sheetFieldReconciler interface {
	ReconcileSystemPresets(ctx context.Context, campaignID, systemSlug string) (int, error)
}

// reconcileSystemSheetFields adds newly declared package fields to the sheets of
// every campaign that already uses each given game system, so a package update
// reaches existing campaigns without the GM toggling the system. It only adds
// missing fields (see ReconcileSystemPresets), so it is safe on every boot and
// after every package install. Per-campaign failures are logged and skipped.
// Returns the total fields added.
func reconcileSystemSheetFields(ctx context.Context, addonSvc pcBackfillAddons, rec sheetFieldReconciler, systemSlugs []string) int {
	total := 0
	for _, slug := range systemSlugs {
		campaignIDs, err := addonSvc.ListCampaignsUsingAddon(ctx, slug)
		if err != nil {
			slog.Warn("sheet-field sync: listing campaigns failed",
				slog.String("system", slug), slog.Any("error", err))
			continue
		}
		for _, campaignID := range campaignIDs {
			n, err := rec.ReconcileSystemPresets(ctx, campaignID, slug)
			if err != nil {
				slog.Warn("sheet-field sync: reconcile failed for campaign",
					slog.String("campaign_id", campaignID),
					slog.String("system", slug),
					slog.Any("error", err))
				continue
			}
			total += n
		}
	}
	if total > 0 {
		slog.Info("sheet-field sync added package fields to existing campaigns",
			slog.Int("fields_added", total))
	}
	return total
}

// installedSystemSlugs lists the game systems currently served by the loader.
func installedSystemSlugs() []string {
	infos := systems.AddonInfos()
	slugs := make([]string, 0, len(infos))
	for _, info := range infos {
		slugs = append(slugs, info.Slug)
	}
	return slugs
}

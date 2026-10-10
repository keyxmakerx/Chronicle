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
	ReconcileSystemPresets(ctx context.Context, campaignID, systemSlug string, known presetFieldKeys) (int, error)
}

// reconcileSystemSheetFields adds the fields a package update introduced to the
// sheets of every campaign already using that system, so the GM doesn't have to
// toggle the system. before is the pre-update snapshot: only fields absent from
// it are added, so a field a GM deleted is never brought back, and a system
// with no earlier version loaded is skipped (enabling it applies everything).
// It only adds (see ReconcileSystemPresets). Per-campaign failures are logged
// and skipped. Returns the total fields added.
func reconcileSystemSheetFields(ctx context.Context, addonSvc pcBackfillAddons, rec sheetFieldReconciler, before map[string]presetFieldKeys) int {
	total := 0
	for slug, known := range before {
		campaignIDs, err := addonSvc.ListCampaignsUsingAddon(ctx, slug)
		if err != nil {
			slog.Warn("sheet-field sync: listing campaigns failed",
				slog.String("system", slug), slog.Any("error", err))
			continue
		}
		for _, campaignID := range campaignIDs {
			n, err := rec.ReconcileSystemPresets(ctx, campaignID, slug, known)
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
		slog.Info("sheet-field sync added new package fields to existing campaigns",
			slog.Int("fields_added", total))
	}
	return total
}

// characterHomeReconciler is the slice of the entity service the character
// preset-home sweep needs.
type characterHomeReconciler interface {
	ReconcileCharacterPresetHome(ctx context.Context) (int, error)
}

// reconcileCharacterPresetHome moves a system's character sheet onto the
// default "Characters" type in campaigns that got a separate, empty system
// type (see entities.ReconcileCharacterPresetHome for the rules). It never
// deletes and is idempotent, so it runs on every boot; failure is logged, not
// fatal. Returns the number of campaigns changed.
func reconcileCharacterPresetHome(ctx context.Context, svc characterHomeReconciler) int {
	n, err := svc.ReconcileCharacterPresetHome(ctx)
	if err != nil {
		slog.Warn("character preset home sweep failed", slog.Any("error", err))
		return 0
	}
	if n > 0 {
		slog.Info("character preset home sweep complete", slog.Int("campaigns", n))
	}
	return n
}

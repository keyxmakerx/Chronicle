package sessions

import (
	"context"
	"fmt"
	"log/slog"
)

// SessionsAddonSlug is the Game nights switch. The slug predates the name:
// it was the dashboard block's addon, and keeping it means campaigns that
// had that block on keep game nights on.
const SessionsAddonSlug = "sessions"

// reconcileEnabledBy is the enabled_by recorded for a row this reconciler
// creates: no person made the choice, so no user id is claimed.
const reconcileEnabledBy = ""

// AddonEnablementStore is the addons-service surface the reconciler needs.
// Satisfied by addons.AddonService. HasCampaignAddonRecord, not
// IsEnabledForCampaign, because "never decided" and "switched off" must be
// told apart.
type AddonEnablementStore interface {
	HasCampaignAddonRecord(ctx context.Context, campaignID string, addonSlug string) (bool, error)
	EnableForCampaignBySlug(ctx context.Context, campaignID string, addonSlug string, userID string) error
}

// ReconcileAddonEnablement turns the Game nights switch on for every
// campaign that already uses game nights but has never had the switch
// recorded either way, so giving game nights a switch of its own (off for
// new campaigns) does not take them away from anyone. A recorded row, on or
// off, is an owner's choice and is left alone. Idempotent; safe every boot.
// Returns how many campaigns it switched on.
func ReconcileAddonEnablement(ctx context.Context, svc SessionService, store AddonEnablementStore) (int, error) {
	if svc == nil || store == nil {
		return 0, fmt.Errorf("sessions.ReconcileAddonEnablement: nil dependency")
	}
	ids, err := svc.ListCampaignIDsUsingGameNights(ctx)
	if err != nil {
		return 0, fmt.Errorf("sessions.ReconcileAddonEnablement: listing campaigns: %w", err)
	}
	enabled := 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		has, err := store.HasCampaignAddonRecord(ctx, id, SessionsAddonSlug)
		if err != nil {
			return enabled, fmt.Errorf("sessions.ReconcileAddonEnablement: reading campaign %s: %w", id, err)
		}
		if has {
			continue
		}
		if err := store.EnableForCampaignBySlug(ctx, id, SessionsAddonSlug, reconcileEnabledBy); err != nil {
			return enabled, fmt.Errorf("sessions.ReconcileAddonEnablement: enabling for campaign %s: %w", id, err)
		}
		enabled++
		slog.Info("game nights switched on for a campaign that already used them", slog.String("campaign_id", id))
	}
	return enabled, nil
}

package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
)

// jot_notes_split.go is the one-time reconciler for the Notes switch becoming
// two: "notes" now turns on only the Journal, and "jot-notes" the floating Jot
// notes tab. Every campaign that had Notes on had both, so this turns Jot
// notes on for them once. It runs once per site, so a campaign that turns the
// Journal on later is not handed jots it never asked for.

// jotNotesSplitKey is the site setting that records the reconciler ran.
const jotNotesSplitKey = "reconcile.jot_notes_split"

// jotNotesAddons is the slice of the addons service it needs. Only a campaign
// with no Jot notes row is changed: a row means someone already decided.
type jotNotesAddons interface {
	ListCampaignsUsingAddon(ctx context.Context, addonSlug string) ([]string, error)
	HasCampaignAddonRecord(ctx context.Context, campaignID, addonSlug string) (bool, error)
	EnableForCampaignBySlug(ctx context.Context, campaignID, addonSlug, userID string) error
}

// splitJotNotesOnce turns Jot notes on for every campaign with the Journal on
// unless that already happened, and returns how many it turned on. The
// setting is written only after every campaign was handled, so a failure
// part-way repeats the sweep, which skips the ones already done.
func splitJotNotesOnce(ctx context.Context, st pageExtrasSettings, svc jotNotesAddons) (int, error) {
	if v, err := st.Get(ctx, jotNotesSplitKey); err == nil && v == "1" {
		return 0, nil
	} else if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			return 0, fmt.Errorf("reading %s: %w", jotNotesSplitKey, err)
		}
	}
	ids, err := svc.ListCampaignsUsingAddon(ctx, "notes")
	if err != nil {
		return 0, fmt.Errorf("listing campaigns with the Journal on: %w", err)
	}
	enabled := 0
	for _, id := range ids {
		has, err := svc.HasCampaignAddonRecord(ctx, id, addons.JotNotesAddonSlug)
		if err != nil {
			return enabled, fmt.Errorf("reading Jot notes for campaign %s: %w", id, err)
		}
		if has {
			continue
		}
		// No person made this choice, so no user is recorded as making it.
		if err := svc.EnableForCampaignBySlug(ctx, id, addons.JotNotesAddonSlug, ""); err != nil {
			return enabled, fmt.Errorf("turning Jot notes on for campaign %s: %w", id, err)
		}
		enabled++
	}
	if err := st.Set(ctx, jotNotesSplitKey, "1"); err != nil {
		return enabled, fmt.Errorf("recording %s: %w", jotNotesSplitKey, err)
	}
	return enabled, nil
}

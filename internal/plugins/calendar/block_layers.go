// block_layers.go — the per-viewer layer RESOLUTION, in one place
// (C-CALV4-LAYERS-P9, W-F).
//
// Three producers hand the Block a layer set — blockDefaultLayers (DEF),
// entityBlockLayers and benchBlockLayers — and all three carried the same
// byte-identical paragraph for three waves: "HasSwitchboard stays false: layer
// preferences are per-viewer and PERSISTED (L20/L26/L29) and that store is
// W-F's." The store now exists, so the sentence is false at all three sites and
// the resolution they now share lives here rather than being re-derived three
// times.
package calendar

import (
	"context"
	"log/slog"

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// blockPrefsPath is the campaign-scoped endpoint the switchboard posts to. It
// is built HERE, in the plugin, because the widget package is plugin-agnostic:
// no router, no campaign id, and it renders under context.Background(), so it
// can neither construct a URL nor read one off a request (r54).
//
// An empty campaign id yields "" and therefore NO SWITCHBOARD, which is the
// honest answer for any host composing a Block outside a campaign.
func blockPrefsPath(campaignID string) string {
	if campaignID == "" {
		return ""
	}
	return "/campaigns/" + campaignID + "/calendar/prefs"
}

// blockLayerPrefs is what a producer needs from the request to resolve layers,
// carried as one value so a caller cannot supply half of it.
//
// Stored is the VIEWER'S OWN set, or nil when they have never chosen. PersistURL
// is empty when there is nowhere to persist — an anonymous viewer, or a Block
// composed outside a campaign.
type blockLayerPrefs struct {
	Stored     []string
	PersistURL string
}

// resolveBlockLayers composes the host's SEED with the viewer's stored set.
//
// THE SEED RULE, signed as [LYR-3]: the host's layer set is a SEED, not an
// override. With no stored row the Block renders the host's set exactly as it
// did before this slice — which is what keeps every wave-1/2 screenshot valid on
// day one, because a fresh viewer has no row. The first explicit switchboard
// write persists the viewer's set, and from then on the store wins at EVERY host
// in that campaign.
//
// nil AND EMPTY ARE DIFFERENT ANSWERS, all the way down from the column:
//
//	stored == nil        never chosen  → the seed
//	stored == []string{} chose nothing → a bare month
//
// Collapsing them would make the bare month unreachable, and a default nobody
// can leave is not a default.
//
// THE ONE-CAMPAIGN GRAIN IS DELIBERATE, not a simplification. calendar_active's
// PK is (user_id, campaign_id) and migration 007 already chose to extend that
// row rather than mint a prefs table, under a live coordinator decision (PR #368
// stop-and-flag #3). L20 describes a VIEWER's preference for how they read
// calendars, and a viewer who turns eras off almost certainly means "off,
// everywhere" — so the entity page and the Bench honour the same row even
// though their seeds differ.
//
// HasSwitchboard and PersistURL are set TOGETHER, always, because r54 pins them
// as one fact and both ways of breaking it are silent.
func resolveBlockLayers(seed []string, prefs blockLayerPrefs) calblock.LayerState {
	enabled := seed
	if prefs.Stored != nil {
		enabled = prefs.Stored
	}
	return calblock.LayerState{
		Enabled:        enabled,
		HasSwitchboard: prefs.PersistURL != "",
		PersistURL:     prefs.PersistURL,
	}
}

// blockLayerPrefsFor reads the viewer's stored set and builds the endpoint, ONCE
// per page, so a Bench composing four Blocks makes one query rather than four.
//
// A read failure is NOT fatal and NOT a fallback to "no switchboard": the
// preference is display state, and a degraded read should give the viewer their
// host's seed with a working switchboard rather than a calendar with a dead
// control. Same shape as the sync pill's degraded probe, for the same reason —
// a degraded read must never blank a surface.
func blockLayerPrefsFor(ctx context.Context, svc CalendarService, userID, campaignID string) blockLayerPrefs {
	if svc == nil || userID == "" || campaignID == "" {
		// Nowhere to persist: the host's seed renders and the ⋯ invoker stays
		// inert, which is exactly the wave-1/2 behaviour and the honest one.
		return blockLayerPrefs{}
	}
	p := blockLayerPrefs{PersistURL: blockPrefsPath(campaignID)}
	if stored, err := svc.GetBlockLayers(ctx, userID, campaignID); err == nil {
		p.Stored = stored
	} else {
		slog.Warn("calendar: Block layer preference read failed; falling back to the host's seed",
			slog.String("user_id", userID),
			slog.String("campaign_id", campaignID),
			slog.Any("error", err))
	}
	return p
}

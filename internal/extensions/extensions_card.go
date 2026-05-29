// extensions_card.go — C-EXT-HUB Phase 1 adapter: lets the campaigns
// plugin's top-level Extensions hub embed the per-campaign Content
// Packs list as a card inside its own chrome.
//
// The hub at `/campaigns/:id/extensions` (campaigns plugin) replaced
// the standalone `ListCampaignExtensions` GET that previously owned
// the same path. The rendering markup (campaignExtensionListFragment)
// is unchanged; this method exposes it through the campaigns plugin's
// ContentPacksCardRenderer interface, inverting the import direction
// (extensions imports campaigns, never the reverse).
//
// Per-pack enable/disable POSTs at /campaigns/:id/extensions/:extID/
// {enable,disable} continue to render campaignExtensionListFragment
// for in-place HTMX swap, so toggling Content Packs from inside the
// hub card still works without further wiring.

package extensions

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RenderCampaignExtensionList loads the per-campaign installed
// Content Packs and returns the existing list-fragment templ
// component for the campaigns plugin to embed inside its Extensions
// hub. Implements `campaigns.ContentPacksCardRenderer`.
//
// A nil-but-non-error result (no packs installed) renders an empty
// list card; the hub's surrounding chrome supplies the section
// header. Error cases bubble up so the hub can log and degrade
// gracefully (the hub renders without the Content Packs card if this
// returns an error).
func (h *Handler) RenderCampaignExtensionList(ctx context.Context, cc *campaigns.CampaignContext) (templ.Component, error) {
	exts, err := h.svc.ListForCampaign(ctx, cc.Campaign.ID)
	if err != nil {
		return nil, err
	}
	if exts == nil {
		exts = []CampaignExtension{}
	}
	return campaignExtensionListFragment(cc, exts), nil
}

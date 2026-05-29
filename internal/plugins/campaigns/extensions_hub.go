// extensions_hub.go — C-EXT-HUB Phase 1 backing types + per-request
// helpers for the top-level Extensions hub at
// `GET /campaigns/:id/extensions`.
//
// The Extensions hub is the operator's new top-level entry point to
// every per-campaign feature ("extension"): one card per addon,
// owner-gated enable/disable via the existing addons-store toggle
// (`PUT /campaigns/:id/addons/:addonID/toggle`), and — Phase 2 — an
// inline-expandable dashboard panel slot per card. The hub absorbs
// and retires the Features Settings tab; Content Packs (per-campaign
// installable packs from the admin install surface) becomes one card
// inside the hub instead of a standalone page at the same path.
//
// Architecture refinement vs. the dispatch's literal "extend
// PluginInfo" instruction:
//
// The C-EXT-HUB-PHASE-1 dispatch directed extending the admin-side
// `PluginInfo` registry (`internal/plugins/admin/plugin_registry.go`)
// with HasDashboard / HasEntitySetup / OperatorFacing flags. In
// implementation the operator-facing catalog is `PluginHubAddon` via
// `AddonLister.ListForPluginHub` — not `PluginInfo`, which feeds the
// admin Plugins page and over-includes infrastructure plugins (auth,
// audit, syncapi, ...). To keep a single source of truth, the
// capability flags live on `PluginHubAddon` and are populated by the
// addons-side adapter from the slug tables below. `OperatorFacing` is
// implicit — every addon `ListForPluginHub` returns is operator-facing
// by construction.

package campaigns

import (
	"context"

	"github.com/a-h/templ"
)

// extensionDashboardSlugs marks which extension slugs ship an inline
// dashboard fragment, registered via Phase 2's
// `RegisterExtensionDashboard` factory. Today only calendar; timeline
// joins after C-TIMELINE-V2 lands.
var extensionDashboardSlugs = map[string]bool{
	"calendar": true,
}

// extensionEntitySetupSlugs marks which extension slugs ship a
// per-entity setup card (Phase 4 work; surfaced here so the catalog
// already carries the capability metadata when Phase 4 begins). Today
// only calendar; maps already has its setup card and isn't a Phase 4
// build target.
var extensionEntitySetupSlugs = map[string]bool{
	"calendar": true,
}

// HasExtensionDashboard reports whether the given addon slug exposes
// an inline dashboard fragment for the Extensions hub. Called by the
// addons-side lister adapter (`internal/app/routes.go`'s
// `addonListerAdapter.ListForPluginHub`) to populate
// `PluginHubAddon.HasDashboard`.
func HasExtensionDashboard(slug string) bool {
	return extensionDashboardSlugs[slug]
}

// HasExtensionEntitySetup reports whether the given addon slug exposes
// a per-entity setup card (Phase 4). Same wiring path as
// HasExtensionDashboard.
func HasExtensionEntitySetup(slug string) bool {
	return extensionEntitySetupSlugs[slug]
}

// ContentPacksCardRenderer is the inversion of the campaigns ↔
// extensions import direction: the `extensions` plugin already imports
// `campaigns` (for `GetCampaignContext`), so to embed the per-campaign
// Content Packs list inside the Extensions hub we let the extensions
// plugin register a renderer with the campaigns Handler at startup.
// Mirrors the established interface-injection pattern
// (`AddonLister`, `MediaUploader`, `SMTPChecker`, ...).
//
// Returning a `templ.Component` rather than rendering directly keeps
// the extensions plugin oblivious to the hub's chrome — the hub
// embeds the returned component inside its own card wrapper.
type ContentPacksCardRenderer interface {
	RenderCampaignExtensionList(ctx context.Context, cc *CampaignContext) (templ.Component, error)
}

// SetContentPacksCardRenderer wires the extensions plugin's renderer
// into the campaigns Handler. Called once at startup from
// `internal/app/routes.go` after both plugins' handlers exist. nil is
// tolerated — the hub renders without a Content Packs card if the
// extensions plugin is not wired (matches addons-store's tolerance of
// a nil AddonLister).
func (h *Handler) SetContentPacksCardRenderer(r ContentPacksCardRenderer) {
	h.contentPacksRenderer = r
}

// extensions_hub.go — backing types + per-request helpers for the
// top-level Extensions hub at `GET /campaigns/:id/extensions`.
//
// The Extensions hub is the operator's top-level entry point to every
// per-campaign feature ("extension"): one card per addon, owner-gated
// enable/disable via the existing addons-store toggle
// (`PUT /campaigns/:id/addons/:addonID/toggle`), and an inline-expandable
// dashboard panel slot per card. Content Packs (per-campaign installable
// packs from the admin install surface) is one card inside the hub
// instead of a standalone page.
//
// The capability flags (HasDashboard / HasEntitySetup) live on
// `PluginHubAddon` (via `AddonLister.ListForPluginHub`), not on the
// admin-side `PluginInfo` registry, which feeds the admin Plugins page
// and over-includes infrastructure plugins (auth, audit, syncapi, ...).
// They are populated by the addons-side adapter from the slug tables
// below.

package campaigns

import (
	"context"
	"fmt"

	"github.com/a-h/templ"
)

// extensionDashboardSlugs marks which extension slugs ship an inline
// dashboard fragment, registered via `RegisterExtensionDashboard`.
// Today only calendar.
var extensionDashboardSlugs = map[string]bool{
	"calendar": true,
}

// extensionEntitySetupSlugs marks which extension slugs ship a
// per-entity setup card. Today only calendar; maps already has its own
// setup card outside this mechanism.
var extensionEntitySetupSlugs = map[string]bool{
	"calendar": true,
}

// extensionDashboardPages maps an addon slug to a DEDICATED dashboard page
// (a full route), as opposed to the inline-panel fragment. When an entry
// exists, the hub's "Open dashboard" affordance navigates to the page
// instead of HTMX-swapping the inline panel. Other apps keep the inline
// panel until they gain a dedicated page. The value is a path template
// taking the campaign ID. See ExtensionDashboardPageURL.
var extensionDashboardPages = map[string]string{
	"calendar": "/campaigns/%s/apps/calendar",
}

// ExtensionDashboardPageURL returns the dedicated dashboard-page URL for a
// slug (and true) when one exists, else ("", false) — in which case the hub
// falls back to the inline-panel fragment affordance.
func ExtensionDashboardPageURL(slug, campaignID string) (string, bool) {
	tmpl, ok := extensionDashboardPages[slug]
	if !ok {
		return "", false
	}
	return fmt.Sprintf(tmpl, campaignID), true
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
// a per-entity setup card. Same wiring path as HasExtensionDashboard.
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

// addonCategorySystem is the addons store's category for game systems,
// spelled out here because campaigns does not import the addons package.
const addonCategorySystem = "system"

// featureAddons drops game systems from the Features grid. The game system
// card is the only control for them: a second toggle there could switch a
// system on without the campaign's settings knowing.
func featureAddons(addons []PluginHubAddon) []PluginHubAddon {
	out := make([]PluginHubAddon, 0, len(addons))
	for _, a := range addons {
		if a.Category != addonCategorySystem {
			out = append(out, a)
		}
	}
	return out
}

// currentSystemID is the system the picker shows as chosen: the campaign's
// setting, or else the system switched on in the addons store, which is what
// campaigns set up before the picker saved its choice still have.
func currentSystemID(settingID string, addons []PluginHubAddon) string {
	if settingID != "" {
		return settingID
	}
	for _, a := range addons {
		if a.Category == addonCategorySystem && a.Enabled {
			return a.Slug
		}
	}
	return ""
}

// extension_dashboards.go — C-EXT-HUB Phase 2 inline-dashboard
// registry for the top-level Extensions hub.
//
// Mirrors the proven `RegisterSettingsTab` pattern from
// settings_tabs.go: each plugin owns its own dashboard templ; the
// campaigns plugin owns only the registry + the inline expand
// chrome. Composition over a shared monolith — when timeline V2 or
// maps add their own dashboards later they register the same way
// without touching this file.
//
// Wiring path (mirrors `ai_workspace.SettingsTabFactory()` in
// `internal/app/routes.go:2465`):
//
//   campaignHandler.RegisterExtensionDashboard(calendarHandler.ExtensionDashboardFactory())
//
// Per-request resolution: BuildExtensionDashboards invokes each
// registered factory with the live CampaignContext, returning a
// slug → ExtensionDashboard map. Slug collisions: last-registered
// wins (matches RegisterSettingsTab's append semantics).

package campaigns

import (
	"context"
	"log/slog"

	"github.com/a-h/templ"
)

// ExtensionDashboard is the declarative description of one
// extension's inline dashboard fragment. The Content component is
// computed inside the registered factory closure, which means each
// invocation captures live per-request state (campaign context,
// active calendar, etc.) without leaking dependencies into this
// package.
type ExtensionDashboard struct {
	// Slug is the addon slug the dashboard binds to (must match
	// PluginHubAddon.Slug). Drives lookup from the hub fragment
	// route + the disabled/missing placeholder dispatch.
	Slug string

	// Content is the rendered dashboard body (V2-styled card chrome
	// per the calendar dashboard's reference shape). Wrapped by the
	// hub fragment route inside the expandable panel; the templ
	// itself does not need to render its own outer panel chrome.
	Content templ.Component
}

// ExtensionDashboardError is the narrow interface a dashboard
// factory's content-builder may return when underlying data loading
// fails. Returned-but-rendered (rather than bubbled) so a single
// dashboard's failure cannot 500 the hub page; the fragment route
// logs the error and renders a placeholder.
//
// Today no factory actually returns errors — content is computed
// synchronously inside the templ render — but the interface is
// reserved for future plugins whose dashboards must fetch external
// data with bounded failure modes.
type ExtensionDashboardError interface {
	Error() string
}

// RegisterExtensionDashboard appends a dashboard factory to the
// Handler's plugin-contributed registry. Called by other plugins at
// startup (after the campaigns handler is constructed and after
// their own services are wired). Same shape + invariants as
// RegisterSettingsTab.
//
// nil factory tolerated (logged + skipped) so a plugin that
// conditionally exposes a dashboard (e.g. only when its dependencies
// are wired) can pass its own nil through without crashing wire-up.
func (h *Handler) RegisterExtensionDashboard(factory func(*CampaignContext) ExtensionDashboard) {
	if factory == nil {
		slog.Warn("RegisterExtensionDashboard: nil factory ignored")
		return
	}
	h.extensionDashboardFactories = append(h.extensionDashboardFactories, factory)
}

// BuildExtensionDashboards returns the slug → ExtensionDashboard map
// for the current request. Invoked from the extensions hub fragment
// route to resolve `:slug → registered dashboard`. Returning a map
// (rather than a slice) makes the unknown-slug case a single
// map-lookup with a safe placeholder fallback in the caller.
//
// Stable behavior on slug collision: later-registered wins. A
// warning is logged so collisions are visible in production logs
// even though they don't break rendering.
func (h *Handler) BuildExtensionDashboards(cc *CampaignContext) map[string]ExtensionDashboard {
	out := make(map[string]ExtensionDashboard, len(h.extensionDashboardFactories))
	for _, factory := range h.extensionDashboardFactories {
		d := factory(cc)
		if d.Slug == "" {
			slog.Warn("BuildExtensionDashboards: factory returned empty slug; skipped")
			continue
		}
		if prev, ok := out[d.Slug]; ok {
			slog.Warn("BuildExtensionDashboards: slug collision; later registration wins",
				slog.String("slug", d.Slug),
				slog.String("previous_content_type", typeNameOf(prev.Content)),
			)
		}
		out[d.Slug] = d
	}
	return out
}

// typeNameOf is a tiny helper for the collision-warning log — keeps
// the dependency surface zero (no reflect import elsewhere in this
// package) by returning a short marker when reflect would be
// overkill.
func typeNameOf(_ templ.Component) string {
	return "templ.Component"
}

// ExtensionEnableChecker is the narrow interface the hub fragment
// route calls to decide whether to render an extension's dashboard
// or its `disabled` placeholder. Satisfied by addons.AddonService's
// IsEnabledForCampaign method (same shape entities plugin uses).
//
// nil-tolerant: a campaigns Handler built without a checker (test
// fixtures, early init) renders all dashboards as if enabled — the
// hub UI is the gate. Production wiring lives in
// internal/app/routes.go alongside the other addons-service
// adapters.
type ExtensionEnableChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

// SetExtensionEnableChecker wires the addons-store check into the
// campaigns Handler. Matches the SetAddonLister / SetSMTPChecker
// shape; called once at startup.
func (h *Handler) SetExtensionEnableChecker(c ExtensionEnableChecker) {
	h.extensionEnableChecker = c
}

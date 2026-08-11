// plugins.go declares the lightweight plugin registration model — a
// metadata-only registry that each plugin contributes to at App startup.
//
// Per cordinator/decisions/2026-05-23-plugin-registration.md (Chunk A —
// the registration shape) + cordinator/decisions/2026-05-25-plugin-static-assets.md
// (Chunk F — the StaticFS extension). See also
// reports/chronicle/2026-05-23-c-plugin-isolation-audit.md §2.4 for the
// broader plugin-interface question this chunk resolves option (c) of.
//
// PluginRegistration started as metadata-only (slug + health hook) and
// grows organically as NW-2.2 chunks land. Each addition gets a
// decision doc + a use case (e.g. Chunk F's StaticFS field powers
// per-plugin static-asset serving). The struct stays opt-in — every
// field is optional except Slug.

package app

import (
	"io/fs"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// PluginRegistration is the per-plugin entry in the App's registry.
// Each plugin contributes exactly one entry, populated inline from
// RegisterRoutes at the plugin's setup point.
//
// Fields:
//   - Slug: the canonical plugin identifier (matches the plugin's
//     exported PluginSlug const). Required.
//   - HealthCheck: optional callback returning nil if healthy; may be
//     nil for plugins without a schema dependency.
//   - StaticFS: optional embedded filesystem of static assets (JS, CSS,
//     images). When non-nil, App.mountPluginStatic() registers it with
//     Echo at /static/plugins/<slug>/. Use echo.MustSubFS(<embed.FS>,
//     "static") at the registration site to strip the leading "static"
//     dir from the embed so URLs map cleanly. Per
//     cordinator/decisions/2026-05-25-plugin-static-assets.md.
type PluginRegistration struct {
	// Slug is the canonical identifier for this plugin. MUST match the
	// owning plugin's exported PluginSlug const so the lookup is
	// symmetric (slug → plugin code, plugin code → slug).
	Slug string

	// HealthCheck is an optional callback returning nil if the plugin
	// is operational, or an error if not. Used by introspection +
	// the removable-plugin test (NW-2.4). May be nil — not every
	// plugin has a schema or other failable health signal.
	HealthCheck func() error

	// StaticFS is an optional embedded filesystem of plugin-owned
	// static assets. When non-nil, App.mountPluginStatic() registers it
	// with Echo at /static/plugins/<Slug>/. nil = no static assets.
	StaticFS fs.FS
}

// registerPlugin appends a registration entry to the App's registry.
// Package-private — called only from RegisterRoutes at each plugin's
// setup point. The pilot only registers two plugins (foundry_vtt +
// smtp); future chunks add more.
func (a *App) registerPlugin(p PluginRegistration) {
	a.registeredPlugins = append(a.registeredPlugins, p)
}

// RegisteredPlugins returns a copy of the App's registry slice. Used
// by the removable-plugin test (NW-2.4 future) and by introspection
// (e.g. an /admin/diagnostics endpoint could list registered plugins).
//
// Returns a copy so callers can't mutate the App's internal slice.
func (a *App) RegisteredPlugins() []PluginRegistration {
	out := make([]PluginRegistration, len(a.registeredPlugins))
	copy(out, a.registeredPlugins)
	return out
}

// mountPluginStatic registers each registered plugin's StaticFS with
// Echo at /static/plugins/<slug>/. Called from RegisterRoutes after
// all plugin registrations have happened.
//
// Plugins with StaticFS == nil are skipped. Per
// cordinator/decisions/2026-05-25-plugin-static-assets.md.
//
// URL convention: /static/plugins/<slug>/<path-within-plugin-static>.
// The leading "static/" dir from each plugin's embed is stripped at
// registration via echo.MustSubFS(<embedFS>, "static") on the caller
// side, so URLs map cleanly without /static/plugins/<slug>/static/...
// doubling.
// pluginStaticPrefix is the single definition of where a plugin's embedded
// assets are served. It exists so the mount and the host.embedded diagnostic
// that REPORTS the mount cannot drift apart — a diagnostic printing a URL the
// server does not actually serve would be worse than printing none.
func pluginStaticPrefix(slug string) string { return "/static/plugins/" + slug }

func (a *App) mountPluginStatic() {
	for _, p := range a.registeredPlugins {
		if p.StaticFS == nil {
			continue
		}
		prefix := pluginStaticPrefix(p.Slug)
		// C-ASSET-VERSIONING: register the embed FS with the asset-URL helper so
		// layouts.AssetURL can content-hash plugin assets exactly like on-disk
		// ones. Without this they'd still bust on deploy (via the per-build
		// token fallback) but every deploy would invalidate all of them.
		layouts.RegisterAssetFS(prefix+"/", p.StaticFS)
		a.Echo.StaticFS(prefix, p.StaticFS)
	}
}

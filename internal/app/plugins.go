// plugins.go declares the lightweight plugin registration model — a
// metadata-only registry that each plugin contributes to at App startup.
//
// Per cordinator/decisions/2026-05-23-plugin-registration.md (the
// architectural shape decision) + NW-2.2 Chunk A (the implementing
// dispatch). See also reports/chronicle/2026-05-23-c-plugin-isolation-audit.md
// §2.4 for the broader plugin-interface question this chunk resolves
// option (c) of.
//
// Today the registry holds metadata only (slug + optional health hook).
// Future NW-2.2 chunks may grow PluginRegistration's surface (Init
// lifecycle, asset paths, etc.) as the dep-injection question is
// solved via a Host interface in a separate `internal/pluginhost/`
// package — see the decision doc's "open question deferred" section.

package app

// PluginRegistration is the per-plugin entry in the App's registry.
// Each plugin contributes exactly one entry, populated inline from
// RegisterRoutes at the plugin's setup point.
//
// Today's surface is metadata-only:
//   - Slug: the canonical plugin identifier (matches the plugin's
//     exported PluginSlug const)
//   - HealthCheck: optional callback returning nil if healthy; may be
//     nil for plugins without a schema dependency
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

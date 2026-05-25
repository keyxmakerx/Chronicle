// registration.go exposes the plugin's canonical slug for the App's
// lightweight plugin registry.
//
// Per cordinator/decisions/2026-05-23-plugin-registration.md (the
// architectural shape decision) + NW-2.2 Chunk A.
//
// Today this file holds only the PluginSlug const — the App's registry
// is metadata-only. There is intentionally NO Registration() function
// exported from this file because PluginRegistration lives in
// internal/app, and internal/app already imports this package; a
// reciprocal import would create a cycle. The decision doc records
// the "deferred Host-interface chunk" that resolves this in a future
// pass.

package smtp

// PluginSlug is the canonical identifier for the smtp plugin in the
// App's PluginRegistration registry. Note that smtp's routes are
// currently mounted from inside admin.RegisterRoutes (the admin
// plugin owns the /admin group); decoupling that is a separate
// migration, also recorded in the decision doc.
const PluginSlug = "smtp"

// PluginHealthKey is the identifier the database.PluginHealthRegistry
// uses for this plugin. Happens to match PluginSlug because smtp's
// Go package name + external slug coincide (no dash/underscore split).
// Exported separately from PluginSlug so cross-package callers stay
// symmetric with plugins where the two values differ (e.g.
// foundry_vtt).
const PluginHealthKey = "smtp"

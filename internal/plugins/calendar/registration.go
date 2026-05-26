// registration.go exposes the calendar plugin's identifiers for the App's
// lightweight plugin registry.
//
// Per cordinator/decisions/2026-05-23-plugin-registration.md (the
// registration shape) + cordinator/decisions/2026-05-25-plugin-static-assets.md
// (the StaticFS extension that this plugin uses to serve calendar_widget.js).
//
// As with foundry_vtt + smtp (Chunk A pilots), no Registration() function is
// exported here — the App's PluginRegistration struct lives in internal/app,
// and internal/app already imports this package; a reciprocal import would
// create a cycle. The decision doc records the deferred Host-interface
// chunk that resolves this in a future pass. For now the registration is
// constructed inline in internal/app/routes.go using these consts +
// StaticAssetsFS from embed.go.

package calendar

// PluginSlug is the canonical identifier for the calendar plugin in the
// App's PluginRegistration registry. Matches the plugin's URL prefix
// shape (/static/plugins/calendar/, /campaigns/:id/calendar/, etc.) and
// the database.PluginHealthRegistry key.
const PluginSlug = "calendar"

// PluginHealthKey is the identifier the database.PluginHealthRegistry
// uses for this plugin. Happens to match PluginSlug because calendar's
// Go package name + external slug coincide (no dash/underscore split,
// unlike foundry_vtt). Exported separately for symmetry with plugins
// where the two values differ.
const PluginHealthKey = "calendar"

// registration.go exposes the plugin's canonical slug for the App's
// plugin registry: own settings tab via campaigns.RegisterSettingsTab,
// own routes via RegisterOwnerRoutes.
//
// There is intentionally no Registration() function exported from
// this file — PluginRegistration lives in internal/app, which already
// imports this package, so a reciprocal import would cycle.

package ai_workspace

// PluginSlug is the canonical EXTERNAL identifier for the ai_workspace
// plugin in the App's PluginRegistration registry. Hyphen form matches
// the CSS sub-layer naming convention (`@layer plugins.ai-workspace`)
// + the URL prefix (`/ai-workspace/…`).
const PluginSlug = "ai-workspace"

// PluginHealthKey is the INTERNAL identifier the
// database.PluginHealthRegistry uses. Underscored to match the Go
// package directory name.
const PluginHealthKey = "ai_workspace"

// MemorySchemaSlug is the migrations slug for the tables of what AI Import
// applied (answer parts, block fingerprints). Kept apart from PluginHealthKey
// so a failed migration leaves only that memory out, never the whole AI tool.
const MemorySchemaSlug = "ai_import_memory"

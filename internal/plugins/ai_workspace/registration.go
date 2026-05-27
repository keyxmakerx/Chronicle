// registration.go exposes the plugin's canonical slug for the App's
// lightweight plugin registry. Per
// cordinator/decisions/2026-05-23-plugin-registration.md.
//
// AI Workspace is the first plugin added under the post-NW-2.2
// isolation rules. It doubles as a showcase of the pattern: own
// settings tab via campaigns.RegisterSettingsTab, own routes via
// RegisterOwnerRoutes, own static assets via embed.FS when needed
// (none for V1 Phase 2 — the AI Export modal reuses the existing
// global /static/js/widgets/ai_export.js for the Copy widget).
//
// As with foundry_vtt's registration.go there is intentionally NO
// Registration() function exported from this file — PluginRegistration
// lives in internal/app, and internal/app already imports this
// package; a reciprocal import would cycle.

package ai_workspace

// PluginSlug is the canonical EXTERNAL identifier for the ai_workspace
// plugin in the App's PluginRegistration registry. Hyphen form matches
// the CSS sub-layer naming convention (`@layer plugins.ai-workspace`)
// + the URL / settings-tab id (`?tab=ai-workspace`).
const PluginSlug = "ai-workspace"

// PluginHealthKey is the INTERNAL identifier the
// database.PluginHealthRegistry uses. Underscored to match the Go
// package directory name. V1 Phase 2 ships no migrations so the
// PluginHealthRegistry doesn't actually track this plugin yet, but
// the constant is exported for future use (Phase 4-5 may introduce
// an import-history table).
const PluginHealthKey = "ai_workspace"

// Package timeline provides the timeline addon for campaigns.
// This file embeds the plugin's SQL migrations and static files so they are available
// in the compiled binary regardless of the runtime working directory.
package timeline

import "embed"

// MigrationsFS contains the embedded SQL migration files for the timeline plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// PluginSlug is the timeline plugin's registry key; its static files are
// served at /static/plugins/timeline/.
const PluginSlug = "timeline"

// StaticAssetsFS holds the timeline's widget scripts, loaded on sight
// through the plugin's Widgets registration.
//
//go:embed static
var StaticAssetsFS embed.FS

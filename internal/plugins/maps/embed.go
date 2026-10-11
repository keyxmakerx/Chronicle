// Package maps provides the interactive maps addon for campaigns.
// This file embeds the plugin's SQL migrations and scripts so they are
// available in the compiled binary regardless of the runtime working directory.
package maps

import "embed"

// PluginSlug names this plugin in the registry and its static URL prefix.
const PluginSlug = "maps"

// MigrationsFS contains the embedded SQL migration files for the maps plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the map scripts, served at /static/plugins/maps/. The
// widgets load on sight (ADR-063); the viewer fetches the rest on demand.
//
//go:embed static
var StaticAssetsFS embed.FS

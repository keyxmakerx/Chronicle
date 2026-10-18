// Package sessions provides the game session tracking addon for campaigns.
// This file embeds the plugin's SQL migration files so they are available
// in the compiled binary regardless of the runtime working directory.
package sessions

import "embed"

// PluginSlug names this plugin in the registry and its static URL prefix.
const PluginSlug = "sessions"

// MigrationsFS contains the embedded SQL migration files for the sessions plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the Last seen block's script, served at
// /static/plugins/sessions/ and loaded on sight (ADR-063).
//
//go:embed static
var StaticAssetsFS embed.FS

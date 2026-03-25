// Package bestiary provides the Community Bestiary plugin for sharing homebrew
// creatures across a Chronicle instance. Users can publish, browse, rate,
// favorite, and import creature statblocks. Instance admins can moderate content.
//
// This file embeds the plugin's SQL migration files so they are available
// in the compiled binary regardless of the runtime working directory.
package bestiary

import "embed"

// MigrationsFS contains the embedded SQL migration files for the bestiary plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// Package maps provides the interactive maps addon for campaigns.
// This file embeds the plugin's SQL migration files so they are available
// in the compiled binary regardless of the runtime working directory.
package maps

import "embed"

// MigrationsFS contains the embedded SQL migration files for the maps plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

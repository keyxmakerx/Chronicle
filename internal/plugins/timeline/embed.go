// Package timeline provides the timeline addon for campaigns.
// This file embeds the plugin's SQL migration files so they are available
// in the compiled binary regardless of the runtime working directory.
package timeline

import "embed"

// MigrationsFS contains the embedded SQL migration files for the timeline plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

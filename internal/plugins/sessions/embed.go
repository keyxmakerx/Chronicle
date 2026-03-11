// Package sessions provides the game session tracking addon for campaigns.
// This file embeds the plugin's SQL migration files so they are available
// in the compiled binary regardless of the runtime working directory.
package sessions

import "embed"

// MigrationsFS contains the embedded SQL migration files for the sessions plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

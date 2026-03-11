// Package calendar provides the calendar/events addon for campaigns.
// This file embeds the plugin's SQL migration files so they are available
// in the compiled binary regardless of the runtime working directory.
package calendar

import "embed"

// MigrationsFS contains the embedded SQL migration files for the calendar plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

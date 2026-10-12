package dmscreen

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping. Only the Adventures tab needs a table; a failed migration
// leaves the rest of the screen working and the tab out.
const PluginSlug = "dmscreen"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

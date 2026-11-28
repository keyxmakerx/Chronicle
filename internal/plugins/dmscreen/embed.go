package dmscreen

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping. Only the Adventures tab and the saved screen layouts need
// tables; a failed migration leaves the rest of the screen working, the tab
// out and layouts kept in the browser alone.
const PluginSlug = "dmscreen"

// MigrationsFS contains the embedded SQL migration files so the tables are
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

package fieldrules

import "embed"

// PluginSlug names this plugin in the migration bookkeeping and the health
// registry, so a failed migration here leaves only these tools out.
const PluginSlug = "fieldrules"

// MigrationsFS contains the embedded SQL migrations.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

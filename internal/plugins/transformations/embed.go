package transformations

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping, so a failed migration leaves only transformations out.
const PluginSlug = "transformations"

// AddonSlug is the campaign switch for transformations, one of the
// Characters app's add-ons.
const AddonSlug = "transformations"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

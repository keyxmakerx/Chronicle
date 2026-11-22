package decks

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping, so a failed migration leaves only decks out.
const PluginSlug = "decks"

// AddonSlug is the campaign switch for card decks. It is off until a
// campaign turns it on.
const AddonSlug = "decks"

// MigrationsFS contains the embedded SQL migration files so the tables are
// created from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

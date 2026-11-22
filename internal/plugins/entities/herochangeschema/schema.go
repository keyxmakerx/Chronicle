// Package herochangeschema holds the migration for when players may change
// their hero's picks. It is its own package so the table runs under its own
// slug: a failed migration leaves only that feature out (every hero stays
// open), and the rest of the entities plugin keeps working.
package herochangeschema

import "embed"

// SchemaSlug names the hero change table in the plugin migration bookkeeping
// and the health registry.
const SchemaSlug = "hero_changes"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// Package practiceschema holds the migration for a player's practice heroes.
// It is its own package so the table runs under its own slug: a failed
// migration leaves only practice heroes out, and the Standing block (which
// shares the entities plugin) keeps working.
package practiceschema

import "embed"

// SchemaSlug names the practice heroes table in the plugin migration
// bookkeeping and the health registry.
const SchemaSlug = "entity_practice"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

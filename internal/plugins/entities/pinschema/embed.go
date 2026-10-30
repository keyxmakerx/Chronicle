// Package pinschema holds the migration for the pages the Director pins for
// everyone on the campaign home. It is its own package so the table runs
// under its own slug: a failed migration leaves only the Director's pins out,
// and each member's own pins (their starred pages) still show.
package pinschema

import "embed"

// PluginSlug names the Director's pins table in the plugin migration
// bookkeeping and the health registry.
const PluginSlug = "page_pins"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

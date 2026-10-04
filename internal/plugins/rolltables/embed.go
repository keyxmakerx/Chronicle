// Package rolltables stores each campaign's own rolling tables: named lists of
// weighted entries a DM rolls on at the table. A campaign has one document
// holding all its tables, replaced as a unit; Chronicle validates and stores
// it but never rolls it, so the dice logic stays in the client.
package rolltables

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "rolltables"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// Package layoutschema holds the migrations for a category's named page
// layouts. It is its own package so the tables run under their own slug: a
// failed migration leaves only that feature out, and every category then has
// its one main layout, as before.
package layoutschema

import "embed"

// PluginSlug names the named page layouts' tables in the plugin migration
// bookkeeping and the health registry.
const PluginSlug = "page_layouts"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

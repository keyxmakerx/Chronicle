package rumours

import "embed"

// PluginSlug names the Rumours block in the plugin migration bookkeeping and
// the health registry. It has a slug and a migrations directory of its own so
// a failed migration here leaves only the Rumours block out, and never marks
// another plugin degraded or runs another plugin's files.
const PluginSlug = "rumours"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the block's script, served at /static/plugins/rumours/
// and loaded on sight the first time a page shows the block.
//
//go:embed static
var StaticAssetsFS embed.FS

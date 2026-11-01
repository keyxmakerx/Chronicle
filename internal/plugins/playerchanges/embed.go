package playerchanges

import "embed"

// PluginSlug names this plugin in the migration bookkeeping and the health
// registry, so a failed migration here turns off only "What players changed".
const PluginSlug = "player_changes"

// MigrationsFS holds the plugin's SQL migrations, built into the binary.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the page's script, served at
// /static/plugins/player_changes/.
//
//go:embed static
var StaticAssetsFS embed.FS

// Package rolls is the campaign's roll log: rolls made at Chronicle's Table
// tab, rolls other apps report (Foundry), and rolls other plugins make on the
// server (crafting). Chronicle decides its own rolls here with the system's
// secure random source, so a browser never picks a result, and every roll is
// sent live only to the people its mode lets see it.
package rolls

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "rolls"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the Table tab's script, served at /static/plugins/rolls/.
//
//go:embed static
var StaticAssetsFS embed.FS

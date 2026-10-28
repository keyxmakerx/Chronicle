// Package crafting keeps a campaign's crafting: the recipes its Directors
// write, the crafting tags on Armory items, the benches recipes are made at,
// and the campaign's crafting settings.
//
// Nothing here knows a game system or a campaign. How a project is rolled
// (the dice, the characteristics, what an edge is worth) comes read-only from
// the enabled system's manifest; steps, tags and bench kinds are this
// plugin's fixed library, so nothing a Director or their AI writes can break
// the bench that plays a recipe.
package crafting

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "crafting"

// AddonSlug is the addon that turns the Crafting app on for a campaign.
const AddonSlug = "crafting"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the app's script, served at /static/plugins/crafting/.
//
//go:embed static
var StaticAssetsFS embed.FS

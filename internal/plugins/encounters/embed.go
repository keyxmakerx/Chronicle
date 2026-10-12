// Package encounters stores the fights a DM plans on a page (an Adventures
// trial, a location, any page) so Foundry can load one on game night.
//
// An encounter belongs to one page. It names the heroes it was built for and
// one row per creature: which creature page, how many, its initiative group
// and how it starts the fight (seen, hidden or arriving later). Everything
// here is the DM's: no route answers a viewer below Owner visibility, so a
// planned ambush never reaches a player.
package encounters

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "encounters"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the Encounters widget script, served at
// /static/plugins/encounters/ and loaded only on pages that show the block.
//
//go:embed static
var StaticAssetsFS embed.FS

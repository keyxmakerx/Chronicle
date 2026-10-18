// Package trackers keeps a campaign's trackers and clocks: kinds its
// Directors define, placed on pages, each with a dated log whose sum is the
// tracker's value.
//
// Nothing here knows any game system or campaign: a kind is data (a number,
// named steps or a clock, with bands), and the controls are always this
// plugin's own. Hidden trackers and their logs never reach players.
package trackers

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "trackers"

// BlockType is the page layout block that shows a page's trackers.
const BlockType = "trackers"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the block widget and its styles, served at /static/plugins/trackers/.
//
//go:embed static
var StaticAssetsFS embed.FS

// Package quests holds the quest sheet behind a page's Quest board block and
// the notice boards that place pages (taverns, temples) pin quests, notes,
// pages and maps to. Chronicle validates, stores and gates the data; the
// pinboard drawing lives in the widgets.
//
// Everything the service returns is already filtered for the viewer: the
// widgets never receive a hidden step, a DM-only item or the name of a page
// the viewer cannot open, so a client bug cannot leak them.
package quests

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "quests"

// MigrationsFS contains the embedded SQL migration files so the tables are
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the quest board and notice board scripts, served at
// /static/plugins/quests/ and loaded only on pages that show a board.
//
//go:embed static
var StaticAssetsFS embed.FS

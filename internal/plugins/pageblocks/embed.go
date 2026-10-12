// Package pageblocks stores the content of the layout blocks a DM types in
// place on a page: the Read-aloud box, the Table and the Steps list.
//
// Block config belongs to the page's category and is shared by every page of
// it, so what a DM writes on one page cannot live there. Content is kept per
// (page, block) here instead. The service returns only what the viewer may
// see: a DM-only table column never reaches a viewer below Owner visibility,
// so a client bug cannot leak it.
package pageblocks

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "pageblocks"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the block widget script, served at
// /static/plugins/pageblocks/ and loaded only on pages that show one of the
// blocks.
//
//go:embed static
var StaticAssetsFS embed.FS

// Package systemstate keeps per-page game-system state: small JSON documents a
// system package's widget reads and writes for one entity (for example a
// negotiation tracker on an NPC page). Chronicle stores and gates the
// documents; it never interprets them, so any system can use the table
// without a Chronicle release.
//
// Each document has two halves. The public half is visible to everyone who can
// view the page; the gm half is only ever returned to the campaign owner and
// members granted DM access.
package systemstate

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "systemstate"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory (same
// pattern as the other plugins).
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

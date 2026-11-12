// Package designs is the campaign library of saved designs and the ways an
// Owner shares them: a share code, a link, or named owners on this instance.
//
// A design is data only, a list of choices from known parts. Each kind
// (room-theme, crafting-circle, ...) is validated by the feature that owns it
// through a KindRegistry that app wiring fills, so this package imports no
// other plugin and a design that names an unknown part is refused by the
// feature that knows the parts.
package designs

import "embed"

// PluginSlug names this plugin in the registry and the health bookkeeping.
const PluginSlug = "designs"

// MigrationsFS holds the embedded SQL migrations, so the tables are created
// from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

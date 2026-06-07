// Package widgetbindings — embedded SQL migrations so the binding table is
// created from the compiled binary regardless of the runtime working dir
// (same pattern as the other plugins; ADR-030).
package widgetbindings

import "embed"

// MigrationsFS contains the embedded SQL migration files for the
// widget-binding plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

package ai_workspace

import "embed"

// MigrationsFS holds the plugin's own migrations (what AI Import applied in a
// campaign). Embedded so the binary carries them whatever its
// working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

package armory

import "embed"

// MigrationsFS holds the armory's own plugin migrations (its older tables are
// core migrations). Embedded so the binary carries them whatever its working
// directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

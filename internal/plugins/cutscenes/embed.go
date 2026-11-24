package cutscenes

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping, so a failed migration leaves only cut scenes out.
const PluginSlug = "cutscenes"

// AddonSlug is the campaign switch for cut scenes. It is off until a campaign
// turns it on.
const AddonSlug = "cutscenes"

// MigrationsFS contains the embedded SQL migration files so the tables are
// created from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the Director's workshop script and stylesheet, served
// at /static/plugins/cutscenes/.
//
//go:embed static
var StaticAssetsFS embed.FS

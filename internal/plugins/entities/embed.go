package entities

import "embed"

// PluginSlug is the entities plugin's registry key. Used to mount the plugin's
// embedded static assets at /static/plugins/entities/.
const PluginSlug = "entities"

// StaticAssetsFS contains the entities plugin's static assets (JS/CSS),
// served by Echo at /static/plugins/entities/ once registered in the
// App's plugin registry.
//
//go:embed static
var StaticAssetsFS embed.FS

// StandingPluginSlug names the Standing block's tables in the plugin
// migration bookkeeping and the health registry. It is not "entities" on
// purpose: a failed standing migration must leave only the Standing block out,
// never mark the page system itself degraded.
const StandingPluginSlug = "entity_standing"

// MigrationsFS contains the embedded SQL migration files for the Standing
// block's tables (the entities plugin owns entity pages, so it owns them),
// created from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

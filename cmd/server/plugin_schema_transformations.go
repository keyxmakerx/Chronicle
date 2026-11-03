package main

import "github.com/keyxmakerx/chronicle/internal/plugins/transformations"

// transformations: layers on a hero's page. Its own slug, so a failed
// migration turns off only transformations. Foreign keys point at core
// tables (entities, campaigns) only.
func init() {
	addPluginSchema(280, transformations.PluginSlug, transformations.MigrationsFS)
}

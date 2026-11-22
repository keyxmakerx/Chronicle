package main

import "github.com/keyxmakerx/chronicle/internal/plugins/entities/herochangeschema"

// hero_changes: when players may change their hero's picks. Its foreign keys
// point at core campaigns and entities.
func init() {
	addPluginSchema(196, herochangeschema.SchemaSlug, herochangeschema.MigrationsFS)
}

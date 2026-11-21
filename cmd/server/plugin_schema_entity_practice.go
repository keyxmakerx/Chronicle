package main

import "github.com/keyxmakerx/chronicle/internal/plugins/entities/practiceschema"

// entity_practice: a player's practice heroes. The only foreign key points at
// core campaigns.
func init() {
	addPluginSchema(191, practiceschema.SchemaSlug, practiceschema.MigrationsFS)
}

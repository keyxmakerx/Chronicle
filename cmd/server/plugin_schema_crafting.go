package main

import "github.com/keyxmakerx/chronicle/internal/plugins/crafting"

// crafting: recipes, material tags, benches and settings. Its own slug, so a
// failed migration turns off only the Crafting app.
func init() {
	addPluginSchema(250, crafting.PluginSlug, crafting.MigrationsFS)
}

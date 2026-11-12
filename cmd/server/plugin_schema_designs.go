package main

import "github.com/keyxmakerx/chronicle/internal/plugins/designs"

// designs: the saved-designs library and its shares. Foreign keys point at the
// core campaigns and users tables and at its own tables only.
func init() {
	addPluginSchema(300, designs.PluginSlug, designs.MigrationsFS)
}

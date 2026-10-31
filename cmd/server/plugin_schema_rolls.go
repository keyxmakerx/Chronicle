package main

import "github.com/keyxmakerx/chronicle/internal/plugins/rolls"

// rolls: the roll log and the table's dice look. Its own slug, so a failed
// migration turns off only the Table tab's rolls.
func init() {
	addPluginSchema(260, rolls.PluginSlug, rolls.MigrationsFS)
}

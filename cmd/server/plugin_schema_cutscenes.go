package main

import "github.com/keyxmakerx/chronicle/internal/plugins/cutscenes"

// cutscenes: cut scenes, their showings and who each was for. Its own slug,
// so a failed migration turns off only cut scenes. Foreign keys point at core
// tables (campaigns, users) only.
func init() {
	addPluginSchema(320, cutscenes.PluginSlug, cutscenes.MigrationsFS)
}

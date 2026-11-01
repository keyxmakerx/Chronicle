package main

import "github.com/keyxmakerx/chronicle/internal/plugins/playerchanges"

// player_changes: what players changed on pages and the packed months behind
// it. Its own slug, so a failed migration turns off only that page.
func init() {
	addPluginSchema(270, playerchanges.PluginSlug, playerchanges.MigrationsFS)
}

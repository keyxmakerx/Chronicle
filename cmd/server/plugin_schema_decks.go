package main

import "github.com/keyxmakerx/chronicle/internal/plugins/decks"

// decks: card decks, their copies and move log. Its own slug, so a failed
// migration turns off only decks. Foreign keys point at core tables
// (campaigns, entities, users) only.
func init() {
	addPluginSchema(310, decks.PluginSlug, decks.MigrationsFS)
}

package main

import "github.com/keyxmakerx/chronicle/internal/plugins/entities/pinschema"

// page_pins: the pages the Director pins for everyone on the campaign home.
// Foreign keys point at core campaigns and entities only.
func init() {
	addPluginSchema(197, pinschema.PluginSlug, pinschema.MigrationsFS)
}

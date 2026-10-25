package main

import "github.com/keyxmakerx/chronicle/internal/plugins/entities/layoutschema"

// page_layouts: a category's named page layouts and each page's choice.
// Foreign keys point at core campaigns, entity_types and entities only.
func init() {
	addPluginSchema(195, layoutschema.PluginSlug, layoutschema.MigrationsFS)
}

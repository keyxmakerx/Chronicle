package main

import "github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"

// ai_connector: AI app connections (hashed keys) and the calls they made. Its
// own slug, so a failed migration turns off only connecting an AI.
func init() {
	addPluginSchema(240, connector.SchemaSlug, connector.MigrationsFS)
}

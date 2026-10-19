package main

import "github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace"

// ai_import_memory: what AI Import applied in a campaign (answer parts, block
// fingerprints). Its foreign keys point at core campaigns only.
func init() {
	addPluginSchema(230, ai_workspace.MemorySchemaSlug, ai_workspace.MigrationsFS)
}

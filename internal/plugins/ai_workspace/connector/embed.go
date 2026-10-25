package connector

import "embed"

// MigrationsFS holds the connector's own migrations (connections and their
// call log), under SchemaSlug, so a failed migration leaves only the
// connector out, never the rest of the AI tool.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// SchemaSlug is the connector's migrations slug.
const SchemaSlug = "ai_connector"

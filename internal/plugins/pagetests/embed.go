package pagetests

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping, so a failed migration leaves only the Tests block out.
const PluginSlug = "page_tests"

// MigrationsFS contains the embedded SQL migration files so the table is
// created from the compiled binary regardless of the working directory.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the Tests widget script, served at
// /static/plugins/page_tests/ and loaded only on pages that show the block.
//
//go:embed static
var StaticAssetsFS embed.FS

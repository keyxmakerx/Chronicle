// Package homebrew keeps a campaign's Homebrew Workshop recipes: parts (the
// everyday widgets and the kinds other plugins register) wired together by
// When/Then rules, saved as one document per recipe.
//
// This package owns the recipe format and the registries. It knows no kind of
// part itself: every kind, event and action is registered by the plugin that
// owns it, through adapters in internal/app, so no plugin imports another.
package homebrew

import "embed"

// PluginSlug names this plugin in the registry and the health/migration
// bookkeeping.
const PluginSlug = "homebrew"

// MigrationsFS contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS holds the Workshop widget and its styles, served at
// /static/plugins/homebrew/.
//
//go:embed static
var StaticAssetsFS embed.FS

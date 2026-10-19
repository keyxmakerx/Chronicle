package main

import (
	"io/fs"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/bestiary"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/pageblocks"
	"github.com/keyxmakerx/chronicle/internal/plugins/pagetests"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
	"github.com/keyxmakerx/chronicle/internal/plugins/rumours"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

// pluginSchemaSource is one built-in plugin's embedded migrations. Rank sets
// the run order: lower runs first, and equal ranks keep registration order.
type pluginSchemaSource struct {
	Rank       int
	Slug       string
	Migrations fs.FS
}

// pluginSchemaSources is filled from init, so a new plugin adds a file with
// its own addPluginSchema call instead of editing a shared list. A plugin
// whose tables point at another plugin's takes a higher rank than that one.
var pluginSchemaSources []pluginSchemaSource

func addPluginSchema(rank int, slug string, migrations fs.FS) {
	pluginSchemaSources = append(pluginSchemaSources, pluginSchemaSource{Rank: rank, Slug: slug, Migrations: migrations})
}

// registeredPlugins returns the built-in plugins with their embedded
// migration filesystems (via Go's embed package, so they're available
// regardless of working directory), in run order.
func registeredPlugins() []database.PluginSchema {
	sources := append([]pluginSchemaSource(nil), pluginSchemaSources...)
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Rank < sources[j].Rank })
	out := make([]database.PluginSchema, 0, len(sources))
	for _, s := range sources {
		sub, err := fs.Sub(s.Migrations, database.PluginMigrationsSubdir)
		if err != nil {
			panic("embedded migrations sub-dir: " + err.Error())
		}
		out = append(out, database.PluginSchema{Slug: s.Slug, MigrationsFS: sub})
	}
	return out
}

// The plugins that existed before plugins registered themselves, in their run
// order. Ranks step by 10 so a new plugin can run between any two.
func init() {
	addPluginSchema(10, "bestiary", bestiary.MigrationsFS)
	addPluginSchema(20, "calendar", calendar.MigrationsFS)
	addPluginSchema(30, "maps", maps.MigrationsFS)
	addPluginSchema(40, "sessions", sessions.MigrationsFS)
	addPluginSchema(50, "timeline", timeline.MigrationsFS)
	// widgetbindings: the generic host↔widget-type↔instance binding table.
	// FK-free and polymorphic, so plugin order vs calendar/maps/timeline
	// doesn't matter.
	addPluginSchema(60, "widgetbindings", widgetbindings.MigrationsFS)
	// systemstate: per-page game-system state. Its foreign keys point at
	// core tables only, which have migrated before any plugin runs.
	addPluginSchema(70, systemstate.PluginSlug, systemstate.MigrationsFS)
	// rolltables: one per-campaign document; its only foreign key points at
	// the core campaigns table.
	addPluginSchema(80, rolltables.PluginSlug, rolltables.MigrationsFS)
	// armory: its own tables beyond the core ones (item shares). Foreign
	// keys point at core campaigns and entities only.
	addPluginSchema(90, armory.AddonSlug, armory.MigrationsFS)
	// quests: foreign keys point at core tables (entities, campaigns) and at
	// its own boards table; no reference to any other plugin's tables.
	addPluginSchema(100, quests.PluginSlug, quests.MigrationsFS)
	// pageblocks: one table of per-block page content; foreign keys point at
	// core tables (entities, campaigns) only.
	addPluginSchema(110, pageblocks.PluginSlug, pageblocks.MigrationsFS)
	// rumours: the Rumours block's table, in a migrations directory and
	// under a slug of its own so a failure here leaves only that block
	// out. Foreign keys point at core tables (entities, campaigns) only.
	addPluginSchema(120, rumours.PluginSlug, rumours.MigrationsFS)
	// fieldrules: homebrew rules and local words. Foreign keys point at core
	// tables (campaigns, entities) only.
	addPluginSchema(130, fieldrules.PluginSlug, fieldrules.MigrationsFS)
	// trackers: kinds, trackers and their log. Foreign keys point at core
	// tables (entities, campaigns) and at its own tables only.
	addPluginSchema(140, trackers.PluginSlug, trackers.MigrationsFS)
	// homebrew: Workshop recipes. Its one foreign key points at campaigns.
	addPluginSchema(150, homebrew.PluginSlug, homebrew.MigrationsFS)
	// encounters: fights planned on a page; foreign keys point at core
	// tables (entities, campaigns) only.
	addPluginSchema(160, encounters.PluginSlug, encounters.MigrationsFS)
	// page_tests: the Tests block's table; foreign keys point at core
	// tables (entities, campaigns) only.
	addPluginSchema(170, pagetests.PluginSlug, pagetests.MigrationsFS)
	// dmscreen: the Adventures tab's pins and loose notes. Foreign keys
	// point at core campaigns and entities only.
	addPluginSchema(180, dmscreen.PluginSlug, dmscreen.MigrationsFS)
	// entity_standing: the Standing block's tables (how a page feels about
	// the party, and why). Foreign keys point at core campaigns and
	// entities only; its slug is separate from "entities" so a failure
	// here leaves only that block out.
	addPluginSchema(190, entities.StandingPluginSlug, entities.MigrationsFS)
	addPluginSchema(200, "syncapi", syncapi.MigrationsFS)
	addPluginSchema(210, "packages", packages.MigrationsFS)
	// foundry_vtt's migration 001 renames foundry_module_campaign_tokens
	// to foundry_vtt_campaign_tokens and drops the orphaned
	// foundry_module_versions table; foundry_vtt.PreMigrationCheck aborts
	// startup first if foundry_module_versions has rows, to avoid
	// destroying an in-flight manual upload.
	//
	// The Slug is taken from the plugin's own constant rather than a
	// literal because foundry_vtt.ReconcileConsolidationState writes a
	// plugin_schema_versions row under it: if the two ever drifted, the
	// reconciler's row would be invisible to the runner and the
	// fresh-install crash would come straight back.
	addPluginSchema(220, foundry_vtt.PluginHealthKey, foundry_vtt.MigrationsFS)
}

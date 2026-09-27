package entities

import "github.com/keyxmakerx/chronicle/internal/database"

// IconColumns lists this plugin's stored icon columns and the default that
// replaces an unusable value, for the boot-time icon reconciler.
func IconColumns() []database.IconColumn {
	return []database.IconColumn{
		{Table: "entity_types", Column: "icon", Default: "fa-circle"},
		{Table: "content_templates", Column: "icon", Default: "fa-file-lines"},
		{Table: "worldbuilding_prompts", Column: "icon", Default: "fa-lightbulb"},
		{Table: "layout_presets", Column: "icon", Default: "fa-table-columns"},
	}
}

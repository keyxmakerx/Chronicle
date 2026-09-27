package armory

import "github.com/keyxmakerx/chronicle/internal/database"

// IconColumns lists this plugin's stored icon columns and the default that
// replaces an unusable value, for the boot-time icon reconciler.
func IconColumns() []database.IconColumn {
	return []database.IconColumn{
		{Table: "inventory_instances", Column: "icon", Default: "fa-box"},
	}
}

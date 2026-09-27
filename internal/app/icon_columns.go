package app

import (
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

// IconColumns gathers every plugin's stored icon columns for the boot-time
// icon reconciler (database.ReconcileIconColumns). A plugin that adds an icon
// column adds it to its own IconColumns and to this list.
func IconColumns() []database.IconColumn {
	var cols []database.IconColumn
	for _, f := range []func() []database.IconColumn{
		entities.IconColumns,
		armory.IconColumns,
		addons.IconColumns,
		maps.IconColumns,
		timeline.IconColumns,
	} {
		cols = append(cols, f()...)
	}
	return cols
}

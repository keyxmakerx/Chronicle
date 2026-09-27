// Package calendar is, for now, a migrations-and-identity carrier.
//
// CALV5-PLACEHOLDER: the plugin was deleted for a ground-up rebuild (V5,
// issue #741); only migrations, MigrationsFS, PluginSlug and the widget type
// constants remain, kept so applied migrations stay immutable, the plugin
// stays registered, and existing addon/widget-binding rows don't orphan.
//
// TODO(#778): this doc has been stale since service.go/handler.go/routes.go
// landed; rewrite it once V5 is complete instead of patching it slice by
// slice.
package calendar

import "embed"

// MigrationsFS contains the embedded SQL migration files for the calendar
// plugin, registered by cmd/server/main.go with the startup migration runner.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// PluginSlug is the addon slug the calendar plugin gates on.
//
// calendar/routes.go's own RegisterRoutes gates directly on it via
// addons.RequireAddon. The entity_calendar/entity_worldstate/skybox/
// upcoming_events dashboard blocks (internal/app/routes.go) do not call
// RequireAddon themselves — they register with entities.BlockRegistry,
// passing this slug as their BlockMeta.Addon, and BlockRegistry.Render is
// what checks it before rendering each block.
const PluginSlug = "calendar"

// WidgetTypeCalendar and WidgetTypeWorldstate are the widgetbindings widget
// types a GM's saved entity bindings point at.
//
// CALV5-PLACEHOLDER: no widget answers to them while the calendar is
// rebuilt; V5 must re-wire widgets to these types. TODO(#778): needs
// calendar.WidgetTypeCalendar/WidgetTypeWorldstate to implement
// widgetbindings.WidgetType — see internal/app/routes.go's entity_calendar/
// entity_worldstate block comments for the specifics deferred here.
const (
	WidgetTypeCalendar   = "calendar"
	WidgetTypeWorldstate = "worldstate"
)

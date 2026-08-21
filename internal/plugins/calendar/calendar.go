// Package calendar is, for now, a migrations-and-identity carrier.
//
// CALV5-PLACEHOLDER: the calendar plugin was deleted wholesale (~37k lines of
// Go, ~58k lines of tests, 243 files, three UI generations) so V5 can be built
// on a clean slate. The operator ruled on 2026-08-21 that no old calendar data
// is preserved.
//
// WHAT SURVIVES HERE, AND WHY EACH THING HAD TO:
//
//   - migrations/ — Chronicle's migrations are APPEND-ONLY and immutable, and
//     a CI guard enforces it (deleting an applied migration crash-loops boot;
//     the 000030 incident, ADR-044/045). 001-018 stay exactly as they shipped
//     and are never edited. The clean slate is migration 019, which DROPS what
//     they created. On a fresh database that is create-then-drop — wasteful and
//     correct; on the operator's live database it is the wipe.
//
//   - MigrationsFS — cmd/server/main.go registers this with the startup
//     migration runner. Without it the plugin's lineage would stop being
//     applied and 019 would never run, which is the whole point.
//
//   - PluginSlug — the addon slug every calendar route gated on. The addon row
//     still exists, campaigns still have it enabled or not, and
//     tools/check-plugin-isolation.sh (T-B2) rejects a plugin name spelled
//     outside its owning plugin. Deleting it would force a re-typed literal
//     somewhere worse.
//
//   - WidgetTypeCalendar / WidgetTypeWorldstate — widgetbindings stores saved
//     entity-to-widget bindings keyed by these strings. Deleting the constants
//     would orphan every binding a GM already made; keeping them lets the
//     bindings survive the blackout and re-attach when V5 lands.
//
// Everything else — the service, repository, handlers, the Bench, the Block
// spine, the V2 shell, the Theater, the widgets — is gone and is V5's job.
// The design that replaces it: cordinator/plans/2026-08-21-calendar-v5-design-brief.md
package calendar

import "embed"

// MigrationsFS contains the embedded SQL migration files for the calendar
// plugin. Registered by cmd/server/main.go with the startup migration runner.
//
// CALV5-PLACEHOLDER: StaticAssetsFS was declared beside this and embedded the
// plugin's static/ directory (calendar_widget.js and friends, served at
// /static/plugins/calendar/). The directory is deleted, so the embed is gone
// too; layouts/assets.go no longer mounts that prefix.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// PluginSlug is the addon slug the calendar's routes gate on.
//
// CALV5-PLACEHOLDER: no routes gate on it today — there are none — but the
// addon row, the operator diagnostics and the isolation guard all still name
// it, and V5's routes gate on it again.
const PluginSlug = "calendar"

// WidgetTypeCalendar and WidgetTypeWorldstate are the widgetbindings widget
// types a GM's saved entity bindings point at.
//
// CALV5-PLACEHOLDER: no widget answers to them while the calendar is rebuilt —
// the block host renders the rebuild notice instead — but the bindings rows
// keep pointing at live constants rather than orphaning.
const (
	WidgetTypeCalendar   = "calendar"
	WidgetTypeWorldstate = "worldstate"
)

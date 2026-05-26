// Package calendar provides the calendar/events addon for campaigns.
// This file embeds the plugin's SQL migration files and static assets
// so they are available in the compiled binary regardless of the runtime
// working directory.
package calendar

import "embed"

// MigrationsFS contains the embedded SQL migration files for the calendar plugin.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// StaticAssetsFS contains the plugin's static assets (JS, CSS, images)
// served by Echo at /static/plugins/calendar/. Currently holds
// js/calendar_widget.js (migrated from central static/ in NW-2.2 Chunk F).
// Per cordinator/decisions/2026-05-25-plugin-static-assets.md.
//
//go:embed static
var StaticAssetsFS embed.FS

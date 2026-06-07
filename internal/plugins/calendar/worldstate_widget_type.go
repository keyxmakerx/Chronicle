// worldstate_widget_type.go — registers "worldstate" with the widget-binding
// framework (C-WIDGET-BINDING-P2-WORLDSTATE-TIMELINE).
//
// 🔑 The worldstate ("hourglass shelf") is a VIEW OVER A CALENDAR'S CLOCK —
// it has no instance table of its own; its bindable instance is a CALENDAR id,
// the same instance space as the `calendar` widget. So it reuses the shared
// calendarInstanceBacking (validate/scope via GetCalendarByID; default via the
// campaign's default calendar). It is a DISTINCT widget_type slug so a host can
// point its hourglass at a different calendar than its calendar embed.
package calendar

import "github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"

// WidgetTypeWorldstate is the persisted widget_type discriminator for the
// worldstate hourglass — an append-only namespace value; never rename.
const WidgetTypeWorldstate = "worldstate"

type worldStateWidgetType struct {
	calendarInstanceBacking
}

// NewWorldStateWidgetType builds the worldstate WidgetType (instance = calendar
// id) for registration into the widget-binding registry at app startup.
func NewWorldStateWidgetType(svc CalendarService) widgetbindings.WidgetType {
	return &worldStateWidgetType{calendarInstanceBacking{svc: svc}}
}

func (w *worldStateWidgetType) Slug() string { return WidgetTypeWorldstate }

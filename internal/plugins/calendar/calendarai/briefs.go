package calendarai

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

var (
	_ aiplug.Briefed = CalendarKind{}
	_ aiplug.Staged  = CalendarKind{}
	_ aiplug.Briefed = EventKind{}
	_ aiplug.Briefed = WeatherKind{}
	_ aiplug.Briefed = RollNightKind{}
	_ aiplug.Briefed = MoonKind{}
	_ aiplug.Briefed = GeneratorKind{}
)

// The prompt's opening "what you can do" line for each kind.

func (CalendarKind) Brief() string {
	return "make the campaign calendar, or change its months, weekdays, seasons, moons, year label, eras and current date"
}
func (EventKind) Brief() string {
	return "calendar events on the campaign calendar, one-off or repeating"
}
func (WeatherKind) Brief() string { return "one day's weather on the calendar" }
func (RollNightKind) Brief() string {
	return "one night of an event's yearly roll: roll it again, give it a result by hand, lock it, or choose whether players are told"
}
func (MoonKind) Brief() string {
	return "a moon the Director moves by hand: the day it turns full, dark or vanished, and what players see"
}
func (GeneratorKind) Brief() string {
	return "run Chronicle's own generators (weather for a range of days, festivals and other events, sky events, names into a rolling table) instead of inventing the result"
}

// Stage lets the rows after a calendar create, or a switch of the main
// calendar, be checked against the calendar that row will make main.
func (k CalendarKind) Stage(ctx context.Context, campaignID string, a Actor, r Record) context.Context {
	if r.Action != ActionCreate && !r.Has("main") {
		return ctx
	}
	if _, cal := k.plan(ctx, campaignID, a, r); cal != nil {
		return withPendingCalendar(ctx, cal)
	}
	return ctx
}

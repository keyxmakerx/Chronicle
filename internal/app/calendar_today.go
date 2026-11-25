package app

// calendar_today.go reads the main calendar's current date as one viewer sees
// it. Worked-out sheet fields and card tests both judge on it, so a moon, an
// event and the time of day are read the same way for both.

import (
	"context"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// calendarToday is the main calendar's current date for one viewer.
type calendarToday struct {
	// DayPart is dawn, day, dusk or night; "" when the calendar can't say.
	DayPart string
	// Moons are the moons the viewer may see.
	Moons []calendarMoonNow
	// Events are the lower-cased names of today's events and, for a viewer
	// who may see the calendar's structure, the slugs and names of their
	// kinds; nil when the events could not be read.
	Events map[string]bool
}

// calendarMoonNow is one moon on the current date. Day is the day of its
// cycle, the new moon's day being 1, or 0 for a moon set by hand.
type calendarMoonNow struct {
	Name  string
	Day   int
	Words []string
}

// readCalendarToday reads the viewer's main calendar, so a moon hidden from
// players or an event they can't see is never in a player's answer. ok is
// false when the viewer has no main calendar or it has no current date. who
// names the reader in a log line.
func readCalendarToday(ctx context.Context, cals calendar.CalendarService, campaignID string, pv permissions.Viewer, who string) (calendarToday, bool) {
	var out calendarToday
	c, err := cals.GetDefaultCalendarForViewer(ctx, campaignID, pv)
	if err != nil || c == nil || c.CurrentMonth < 1 || c.CurrentDay < 1 {
		return out, false
	}
	y, m, d := c.CurrentYear, c.CurrentMonth, c.CurrentDay
	abs := c.CurrentDayIndex()
	out.DayPart = c.DayPart(y, m, d, c.CurrentHour, c.CurrentMinute)
	for i := range c.Moons {
		mo := &c.Moons[i]
		out.Moons = append(out.Moons, calendarMoonNow{Name: strings.TrimSpace(mo.Name), Day: mo.CycleDay(abs), Words: mo.PhaseWords(abs)})
	}
	events, err := cals.ListEventsForMonth(ctx, c.ID, campaignID, y, m, pv)
	if err != nil {
		slog.Warn(who+": cannot read today's events", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return out, true
	}
	var kinds map[int]calendar.EventKind
	out.Events = map[string]bool{}
	for _, e := range events {
		if !eventOn(e, y, m, d) {
			continue
		}
		out.Events[strings.ToLower(strings.TrimSpace(e.Name))] = true
		// Event kinds are Owner-only structure: a player's answer never
		// rests on one, or a card or field would tell them its kind.
		if e.KindID == nil || !pv.SkipsPerUserRules() {
			continue
		}
		if kinds == nil {
			kinds = map[int]calendar.EventKind{}
			list, err := cals.ListEventKinds(ctx, campaignID)
			if err != nil {
				slog.Warn(who+": cannot read event kinds", slog.String("campaign_id", campaignID), slog.Any("error", err))
			}
			for _, k := range list {
				kinds[k.ID] = k
			}
		}
		if k, ok := kinds[*e.KindID]; ok {
			out.Events[strings.ToLower(k.Slug)] = true
			out.Events[strings.ToLower(strings.TrimSpace(k.Name))] = true
		}
	}
	return out, true
}

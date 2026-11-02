package calendarai

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// maxCalMonths caps the months one events or weather lookup reads.
const maxCalMonths = 24

// The `what:` of each calendar lookup. WhatCalendar lets the engine's
// Calendar content choice ask for it without spelling it out.
const (
	WhatCalendar     = "calendar"
	WhatWeatherKinds = "weather-kinds"
	whatEvents       = "events"
	whatWeather      = "weather"
)

// Lookups answers the calendar lookups. Every answer reads as the Actor
// sees the calendar, so a player-role Actor gets no hidden events, moons or
// eras. Without Cal every answer says the calendar can't be looked up; the
// others add to the answers when set.
type Lookups struct {
	Cal     CalendarAPI
	Weather WeatherSettingsAPI
	// Rolls adds each rolled night to the `events` answer; nil leaves them out.
	Rolls EventRollAPI
	// Tiers adds the event importance levels to the calendar answer.
	Tiers TierLister
}

// All is the calendar lookups in the prompt's order. Without a calendar
// service each still answers, saying it can't be looked up.
func (l *Lookups) All() []aiplug.Lookup {
	if l == nil {
		return nil
	}
	return []aiplug.Lookup{
		calLookup{l, aiplug.LookupInfo{
			What: WhatCalendar, Keys: "none",
			Brief: "the calendar itself: months, weekdays, seasons, moons with today's phase and the next new and full moon, eras, festivals, today's date, and its climate",
			Tool: aiplug.Tool{Name: "calendar_overview", Title: "The calendar", Did: "Read the calendar",
				Desc: "The main calendar: months, weekdays, seasons, moons and their phases, eras, festivals, today's date and the climate."},
		}, "Calendar", func(run *lookupRun, _ Record) (LookupAnswer, error) { return run.calendarInfo() }},
		calLookup{l, aiplug.LookupInfo{
			What: WhatWeatherKinds, Aliases: []string{"weather-kind"}, Keys: "none",
			Brief: "every climate, built-in weather label and sky effect the calendar can animate, and the owner's own kinds of weather",
			Tool: aiplug.Tool{Name: "weather_kinds", Title: "Kinds of weather", Did: "Read the kinds of weather",
				Desc: "Every climate and kind of weather the calendar knows."},
		}, "Kinds of weather", func(run *lookupRun, _ Record) (LookupAnswer, error) { return run.weatherKinds() }},
		calLookup{l, aiplug.LookupInfo{
			What: whatEvents, Keys: "`from` and `to` (e.g. `Deepwinter 1 1492`), or `year` and optional `month`",
			Brief: "calendar events in a date range",
			Tool: aiplug.Tool{Name: "calendar_events", Title: "Calendar events", Did: "Looked up events",
				Desc: "Calendar events in a range of dates: from and to, or a year and a month.", Args: dateArgs},
		}, "Events", (*lookupRun).events},
		calLookup{l, aiplug.LookupInfo{
			What: whatWeather, Keys: "`from` and `to`, or `year` and `month`",
			Brief: "each day's weather in a date range",
			Tool: aiplug.Tool{Name: "weather", Title: "Weather by day", Did: "Looked up the weather",
				Desc: "Each day's weather in a range of dates: from and to, or a year and a month.", Args: dateArgs},
		}, "Weather", (*lookupRun).weather},
	}
}

// dateArgs are the events and weather tools' inputs.
var dateArgs = []aiplug.Arg{
	{Key: "from", Desc: "First day, as the calendar writes it (for example \"Deepwinter 1 1492\")"},
	{Key: "to", Desc: "Last day, as the calendar writes it"},
	{Key: "year", Desc: "A year (instead of from and to), for example \"1492\""},
	{Key: "month", Desc: "A month of that year, by name or number"},
}

// LookupAnswer is one calendar lookup's chip and markdown.
type LookupAnswer struct {
	Chip, Text string
}

// lookupRun is one answer's reads, as the Actor.
type lookupRun struct {
	l          *Lookups
	ctx        context.Context
	campaignID string
	a          Actor
}

// director reports whether answers may include what players can't see.
func (run *lookupRun) director() bool { return run.a.CanAuthorDmOnly() }

var (
	_ aiplug.Lookup = calLookup{}
	_ aiplug.Dated  = calLookup{}
)

// calLookup is one calendar `what:` as an aiplug.Lookup.
type calLookup struct {
	l      *Lookups
	info   aiplug.LookupInfo
	chip   string
	answer func(run *lookupRun, r Record) (LookupAnswer, error)
}

func (k calLookup) LookupInfo() aiplug.LookupInfo { return k.info }

// WorldToday gives the main calendar's date through the calendar lookup
// alone, so the prompt asks for it once.
func (k calLookup) WorldToday(ctx context.Context, campaignID string, a Actor) string {
	if k.info.What != WhatCalendar {
		return ""
	}
	return k.l.WorldToday(ctx, campaignID, a)
}

// Answer runs the lookup. With no main calendar the answer is how to set
// one up, not a failure.
func (k calLookup) Answer(ctx context.Context, campaignID string, a Actor, r Record) (string, string, error) {
	run := &lookupRun{l: k.l, ctx: ctx, campaignID: campaignID, a: a}
	ans, err := k.answer(run, r)
	if isNoCalendar(err) {
		ans = run.noCalendar(k.chip)
		return ans.Chip, ans.Text, nil
	}
	if ans.Chip == "" {
		ans.Chip = k.chip
	}
	return ans.Chip, ans.Text, err
}

// isNoCalendar reports a refusal caused by the campaign having no main
// calendar, which a lookup answers with how to make one rather than as a
// failure.
func isNoCalendar(err error) bool {
	return err != nil && strings.HasPrefix(planError(err), needsCalendarPrefix)
}

// noCalendar is the answer to a calendar, events or weather lookup when
// there is no main calendar: which calendars exist (as the Actor sees
// them) and the block that fixes it.
func (run *lookupRun) noCalendar(chip string) LookupAnswer {
	ans := LookupAnswer{Chip: chip + " · no main calendar"}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", chip)
	if chip != "Calendar" {
		b.WriteString("None: the campaign has no main calendar yet. Ask `what: calendar` for how to set one up.\n")
		ans.Text = b.String()
		return ans
	}
	var names []string
	if l, ok := run.l.Cal.(calendarLister); ok {
		if cals, err := l.ListCalendars(run.ctx, run.campaignID, run.a.Viewer()); err == nil {
			for _, c := range cals {
				names = append(names, quote(c.Name))
			}
		}
	}
	if len(names) > 0 {
		fmt.Fprintf(&b, "Not set up yet: the campaign has %s, but none is the main calendar. Events, weather and generators all use the main one.\n\n", plural(len(names), "calendar ("+names[0]+")", "calendars ("+strings.Join(names, ", ")+")"))
		b.WriteString("To make one of them the main calendar (the owner only), send:\n\n```\n---\nkind: calendar\naction: update\nname: " + names[0] + "\nmain: true\n---\n```\n\n")
		b.WriteString("Any other `kind: calendar` key in that block changes the calendar at the same time. To make a new calendar instead, use `action: create`; ask `what: format` with `block: calendar` for every key.\n")
	} else {
		b.WriteString("The campaign has no calendar yet. Events, weather and generators all need one.\n\n")
		b.WriteString("To make one (the owner only), send a `kind: calendar` block with `action: create`. It needs `name`, `months`, `weekdays` and today's date (`current_year`, `current_month`, `current_day`), and becomes the main calendar. Its format:\n\n")
		b.WriteString(strings.TrimSpace(CalendarKind{}.Doc()) + "\n")
	}
	ans.Text = b.String()
	return ans
}

// WorldToday is the main calendar's current date for the prompt's opening,
// as the Actor may see the calendar; "" without one.
func (l *Lookups) WorldToday(ctx context.Context, campaignID string, a Actor) string {
	if l == nil || l.Cal == nil {
		return ""
	}
	cal, err := l.Cal.GetDefaultCalendarForViewer(ctx, campaignID, a.Viewer())
	if err != nil || cal == nil || len(cal.Months) == 0 {
		return ""
	}
	return todayText(cal)
}

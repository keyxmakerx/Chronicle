// calendar_v2_mobile_agenda.go — pure Go helpers for the phone-width Month
// view replacement (C-CAL-MOBILE-AGENDA): a mini-month NAVIGATOR + a
// scrolling AGENDA list, swapped in for the 7-column grid at <768px per the
// signed mockup (mockups/calendar-redesign-concept.html, viewed <768px) and
// mockups/renders/cal-mobile-390.png. Kept in its own file (mirroring
// calendar_v2_ledger.go / calendar_v2_worldstate_helpers.go's precedent of
// feature-scoped helper files) so the mobile slice reviews as one unit.
//
// Data source: data.Events is ALREADY the full displayed month (loaded by
// ShowV2 via ListEventsForMonth for the Month view) and data.ActiveCalendar
// already carries Moons — both helpers below derive everything from data
// already on CalendarV2ViewData. No new query, no new endpoint.
package calendar

// --- Mobile mini-month navigator ---------------------------------------
//
// Derived from miniMonthV2Sidebar's data (:148 in calendar_v2.templ) — the
// SAME day-cursor/today/selected logic and the SAME per-day event source the
// desktop grid's pips use (monthCellEvents) — but with its own struct and
// render path so the always-visible desktop sidebar (miniMonthDay /
// miniMonthDays / miniMonthV2Sidebar) stays byte-for-byte unchanged
// ("Desktop is unchanged" per the dispatch).

// mobileMiniMonthDay carries one mini-month cell's data for the mobile
// navigator: the day-cursor state miniMonthDay already has, plus capped
// event-presence dots (the sidebar's current payload has no such field —
// this is computed fresh from data.Events, not a new endpoint).
type mobileMiniMonthDay struct {
	Day        int
	IsToday    bool
	IsSelected bool
	DotColors  []string // capped pip colors for the day's events; nil = no events
}

// mobileMiniMonthDotsCap bounds dots-per-day so a busy day can't overflow the
// mini-month cell's fixed line box. Matches monthCellVisibleCap()'s cap so
// the mini-month and the Month grid agree on "how many events fit."
func mobileMiniMonthDotsCap() int { return monthCellVisibleCap() }

// mobileMiniMonthDays builds the mobile mini-month's day cells for the
// displayed month. Same leading-blank/offset shape as miniMonthDays (shared
// v2MonthLeadOffset), so the two mini-months (desktop sidebar, mobile
// navigator) always agree on which weekday column day 1 falls under.
func mobileMiniMonthDays(data CalendarV2ViewData) []mobileMiniMonthDay {
	if data.ActiveCalendar == nil || data.Month < 1 || data.Month > len(data.ActiveCalendar.Months) {
		return nil
	}
	cal := data.ActiveCalendar
	dim := cal.Months[data.Month-1].Days
	offset := v2MonthLeadOffset(data)
	out := make([]mobileMiniMonthDay, 0, dim+offset)
	for i := 0; i < offset; i++ {
		out = append(out, mobileMiniMonthDay{Day: 0})
	}
	for i := 0; i < dim; i++ {
		day := i + 1
		out = append(out, mobileMiniMonthDay{
			Day:        day,
			IsToday:    data.Year == cal.CurrentYear && data.Month == cal.CurrentMonth && day == cal.CurrentDay,
			IsSelected: day == data.Day,
			DotColors:  mobileMiniMonthDotColors(data, day),
		})
	}
	return out
}

// mobileMiniMonthDotColors resolves the SAME per-day event colors the
// desktop grid's pips use (monthCellEvents → eventToCardDataWithTiers),
// capped. Deliberately not deduplicated by color — a day with two social
// events still gets two dots, matching what the agenda list below actually
// shows for that day (up to the cap).
func mobileMiniMonthDotColors(data CalendarV2ViewData, day int) []string {
	evs := monthCellEvents(data, day)
	if len(evs) == 0 {
		return nil
	}
	dotsCap := mobileMiniMonthDotsCap()
	if len(evs) > dotsCap {
		evs = evs[:dotsCap]
	}
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		cd := eventToCardDataWithTiers(e, data.ActiveCalendar, data.TierDefinitions)
		out = append(out, monthLinePipColor(cd.CategoryColor))
	}
	return out
}

// mobileMiniMonthDayClasses returns the Tailwind classes for a mobile
// mini-month cell (the mockup's `.mmd` pattern): selected = filled accent
// pill, today (unselected) = an inset accent ring, plain = default text.
// The day NUMBER always renders centered in a fixed h-10 cell — dots render
// in a separate `absolute` layer (mobileMiniMonth in calendar_v2.templ) that
// is removed from flow, so no dot count can ever nudge the number
// (the operator's alignment rule, applied verbatim per the dispatch).
func mobileMiniMonthDayClasses(d mobileMiniMonthDay) string {
	switch {
	case d.IsSelected:
		return "bg-accent text-white"
	case d.IsToday:
		return "ring-2 ring-inset ring-accent text-fg"
	}
	return "text-fg hover:bg-surface-2 transition-colors duration-micro"
}

// mobileMiniMonthDotColor resolves one dot's rendered color: white when the
// day is selected (mirrors the mockup's `.mmd.sel .dots i{background:#fff}`
// — a colored dot would lose contrast against the filled accent pill),
// otherwise the event's own category color.
func mobileMiniMonthDotColor(color string, selected bool) string {
	if selected {
		return "#fff"
	}
	return color
}

// mobileSidebarClasses returns the classes for the LEFT mini-month sidebar
// (miniMonthV2Sidebar). Per C-CAL-SKYPANE-DETACH the desktop left column is
// removed — the signed design keeps the mini-month solely as the MOBILE Month
// navigator (mobileMonthAssembly) — so the sidebar is now DESKTOP-DETACHED
// while its MOBILE presentation is left byte-for-byte unchanged:
//
//   - Month view: hidden at every width. On mobile it was already hidden
//     (mobileMonthAssembly's navigator replaces it); on desktop it is now
//     hidden too (the detach). Dropping `md:block` is the whole desktop change.
//   - Week/Day/Timeline: `md:hidden` detaches it from DESKTOP (≥768px) while
//     `base` keeps its pre-existing <768px mobile presentation intact — those
//     views' own mobile pass remains a separate dispatch slice, untouched here.
//
// Net: no mini-month sidebar renders on desktop for any view (matching the
// signed desktop render), and mobile rendering is identical to before.
func mobileSidebarClasses(data CalendarV2ViewData) string {
	base := "w-60 flex-shrink-0 border-r border-edge px-3 py-4 overflow-y-auto"
	if data.View == "month" {
		return "hidden " + base
	}
	return "md:hidden " + base
}

// --- Mobile agenda list --------------------------------------------------
//
// Reuses the day-popover's data source (calendar_v2_shell.js's
// renderPopoverList: data.Events filtered to one day, multi-day/ribbon
// events excluded) but server-rendered against the SAME recurrence-aware
// projection the Month grid pips use (monthCellEvents/eventsForDay), so the
// agenda and the grid always agree on what a given day shows.

// mobileAgendaEvent is one rendered agenda card.
type mobileAgendaEvent struct {
	EventID   string
	Title     string
	TimeLabel string // "All day" or the event's full time range/start
	Category  string // category chip text; "" hides the chip
	Color     string
	IsPublic  bool
}

// mobileAgendaDay groups one day's agenda cards under a day header.
type mobileAgendaDay struct {
	Day    int
	Header string // "Weekday Day Month" (e.g. "Thu 16 Harvestwane")
	Moon   string // moon-phase glyph for this day; "" when the calendar has no moons
	Events []mobileAgendaEvent
}

// mobileAgendaGroups builds the agenda: one group per day-with-events, from
// data.Day (the selected/current day — e.g. from a mini-month tap) through
// the end of the displayed month. data.Events is already the full month
// (ListEventsForMonth backs the Month view in ShowV2), so this needs no new
// query. Days with no events are skipped entirely (mirrors the mockup — no
// empty headers). Crossing into the next month needs the existing month-nav
// (the ">" command-bar control), same as the desktop grid.
func mobileAgendaGroups(data CalendarV2ViewData) []mobileAgendaDay {
	if data.ActiveCalendar == nil || data.Month < 1 || data.Month > len(data.ActiveCalendar.Months) {
		return nil
	}
	dim := data.ActiveCalendar.Months[data.Month-1].Days
	start := data.Day
	if start < 1 {
		start = 1
	}
	var out []mobileAgendaDay
	for day := start; day <= dim; day++ {
		evs := monthCellEvents(data, day)
		if len(evs) == 0 {
			continue
		}
		out = append(out, mobileAgendaDay{
			Day:    day,
			Header: mobileAgendaDayHeader(data, day),
			Moon:   mobileAgendaMoonGlyph(data, day),
			Events: mobileAgendaEventsForDay(data, evs),
		})
	}
	return out
}

// mobileAgendaDayHeader composes "Weekday Day Month" (e.g. "Thu 16
// Harvestwane") — the same weekday-name + month-name sources weekDayLabel
// uses for the Week view, so the vocabulary matches across views.
func mobileAgendaDayHeader(data CalendarV2ViewData, day int) string {
	cal := data.ActiveCalendar
	weekday := ""
	if idx := v2WeekdayIndex(data, day); idx >= 0 && idx < len(cal.Weekdays) {
		weekday = cal.Weekdays[idx].Name
	}
	monthName := ""
	if data.Month >= 1 && data.Month <= len(cal.Months) {
		monthName = cal.Months[data.Month-1].Name
	}
	if weekday != "" {
		return weekday + " " + itoaCal(day) + " " + monthName
	}
	return monthName + " " + itoaCal(day)
}

// mobileAgendaMoonGlyph resolves a compact moon-phase glyph for the day from
// the calendar's first-defined moon (Moons[0] — the mockup shows a single
// glyph per day header, not one per moon). Pure math via Calendar.AbsoluteDay
// + Moon.MoonPhaseName — already-loaded data, zero extra queries. Weather is
// deliberately NOT shown per day-group here: unlike moon phase it's
// GM-authored per-day state with only a single-day fetch available
// server-side (BuildWorldStateSeed); showing it for every agenda header would
// need either a new batch endpoint or N client-side fetches. Flagged per the
// dispatch's stop-and-flag rather than adding new surface solo.
func mobileAgendaMoonGlyph(data CalendarV2ViewData, day int) string {
	cal := data.ActiveCalendar
	if cal == nil || len(cal.Moons) == 0 {
		return ""
	}
	abs := cal.AbsoluteDay(data.Year, data.Month, day)
	return moonPhaseGlyph(cal.Moons[0].MoonPhaseName(abs))
}

// moonPhaseGlyph maps Moon.MoonPhaseName's 8 canonical strings to their
// Unicode phase glyph. Presentation-only mirror — MoonPhaseName stays the
// source of truth for phase naming; this never invents a new phase value.
func moonPhaseGlyph(name string) string {
	switch name {
	case "New Moon":
		return "🌑"
	case "Waxing Crescent":
		return "🌒"
	case "First Quarter":
		return "🌓"
	case "Waxing Gibbous":
		return "🌔"
	case "Full Moon":
		return "🌕"
	case "Waning Gibbous":
		return "🌖"
	case "Last Quarter":
		return "🌗"
	case "Waning Crescent":
		return "🌘"
	default:
		return ""
	}
}

// mobileAgendaEventsForDay projects a day's (already-capped-free — the
// agenda has room the grid doesn't) events into render-ready agenda cards.
func mobileAgendaEventsForDay(data CalendarV2ViewData, evs []Event) []mobileAgendaEvent {
	out := make([]mobileAgendaEvent, 0, len(evs))
	for _, e := range evs {
		cd := eventToCardDataWithTiers(e, data.ActiveCalendar, data.TierDefinitions)
		timeLabel := cd.TimeLabel
		if timeLabel == "" {
			timeLabel = "All day"
		}
		out = append(out, mobileAgendaEvent{
			EventID:   cd.ID,
			Title:     cd.Name,
			TimeLabel: timeLabel,
			Category:  cd.CategoryName,
			Color:     cd.CategoryColor,
			IsPublic:  cd.IsPublic,
		})
	}
	return out
}

// mobileAgendaCardColor resolves an agenda card's left-accent-bar color,
// falling back to the same muted token the Month grid pips use when an event
// carries no category color (monthLinePipColor) so every card reserves the
// same accent slot.
func mobileAgendaCardColor(color string) string {
	return monthLinePipColor(color)
}

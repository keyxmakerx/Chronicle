// calendar_v2_ledger.go — server-side assembly for the Timeline Ledger,
// the calendar's 4th V2 view (C-CAL-TIMELINE-V2-W1). The Ledger renders the
// displayed year as the world's own chronicle: an era header → year gutter →
// dense, scannable event rows. Tier sets typographic weight, categories are
// colored dots, multi-day events draw a span bar, entity ties + the dm_only
// lock annotate inline.
//
// Design pick: Ledger (Design A) per decisions/2026-07-03-timeline-v2-design-pick.md.
// This is the production port of the demo (internal/templates/demo/
// timeline_ledger.templ) re-expressed on Tailwind tokens — the demo's isolated
// inline OKLCH CSS is NOT copied here.
//
// W1 scope: ONE displayed year (one era via EraForYear + one year label +
// rows). The era→year→rows shape is kept forward-compatible with the W2
// multi-year window. Weather glyphs, markers, and the worldstate band are W2+
// and deliberately absent. The row set is projected from the SAME pre-filtered
// events the month grid receives (filterEventsByUser runs in the service, so
// dm_only rows never reach a player).

package calendar

import (
	calwidget "github.com/keyxmakerx/chronicle/internal/widgets/calendar_v2"
)

// ledgerRowData is the projected view of a single event occurrence in the
// Ledger. It's the flat, render-ready shape the ledgerView templ consumes:
// a computed date label, category dot + chip, tier-weighted title, the
// multi-day span flag, an inline entity annotation, and the GM-only lock.
type ledgerRowData struct {
	EventID    string         // for data-event-id (opens the existing drawer)
	DateLabel  string         // "Mirtul 5" or "Ches 12 – 19" for a span
	Title      string         // event name
	Tier       calwidget.Tier // drives the title's typographic weight
	CatColor   string         // category dot color ("" → neutral dot)
	CatName    string         // category chip label ("" → no chip)
	EntityName string         // inline entity annotation ("" → none)
	DMOnly     bool           // dm_only lock (only reaches GMs; players filtered upstream)
	Span       bool           // multi-day event → span bar
}

// ledgerYearGroup is one year's worth of rows under an era header. W1 emits
// exactly one; the slice shape lets W2 stack multiple years per era.
type ledgerYearGroup struct {
	Label string // epoch-suffixed year, e.g. "1492 DR"
	Rows  []ledgerRowData
}

// ledgerEraGroup is one era band with its contained years. W1 emits exactly
// one (the era containing the displayed year, or a nameless group when no era
// matches). The structure mirrors the demo's era→year→rows nesting.
type ledgerEraGroup struct {
	Name  string // "" → no era matched; render the years without a band
	Span  string // "1488 – 1490 DR" / "1491 DR – ongoing"
	Color string // operator era color; tints the header band
	Years []ledgerYearGroup
}

// ledgerEraGroups assembles the full Ledger structure for the displayed year.
// For W1 this is a single era group wrapping a single year group; the nesting
// is intentional so the W2 multi-year window is a pure additive change.
func ledgerEraGroups(data CalendarV2ViewData) []ledgerEraGroup {
	cal := data.ActiveCalendar
	if cal == nil {
		return nil
	}
	yearGroup := ledgerYearGroup{
		Label: ledgerYearLabel(data),
		Rows:  ledgerRows(data),
	}
	group := ledgerEraGroup{Years: []ledgerYearGroup{yearGroup}}
	if era := cal.EraForYear(data.Year); era != nil {
		group.Name = era.Name
		group.Color = era.Color
		group.Span = ledgerEraSpan(era, cal)
	}
	return []ledgerEraGroup{group}
}

// ledgerRows projects the pre-filtered events into chronological Ledger rows
// for the displayed year. It iterates month → day → events (per the dispatch),
// so rows emerge already ordered by date. Single-day and recurring events emit
// one row per in-year occurrence via Event.OccursOn (the single recurrence
// predicate the other views use); multi-day events emit ONE anchored row with
// a span bar (they are not expanded per-day).
func ledgerRows(data CalendarV2ViewData) []ledgerRowData {
	cal := data.ActiveCalendar
	if cal == nil {
		return nil
	}
	var rows []ledgerRowData
	for m := 1; m <= len(cal.Months); m++ {
		dim := cal.MonthDays(m-1, data.Year)
		for d := 1; d <= dim; d++ {
			for _, e := range data.Events {
				if isMultiDayEvent(e) {
					// Anchor a span exactly once, on the day it first shows in
					// the displayed year (its start day, or Jan 1 for a span
					// that began in a prior year but reaches into this one).
					if am, ad, ok := ledgerMultiDayAnchor(e, data.Year, cal); ok && am == m && ad == d {
						rows = append(rows, ledgerRowForMultiDay(e, cal, data.TierDefinitions))
					}
					continue
				}
				if e.OccursOn(cal, data.Year, m, d) {
					rows = append(rows, ledgerRowForOccurrence(e, cal, data.TierDefinitions, m, d))
				}
			}
		}
	}
	return rows
}

// ledgerYearWindowEnd returns the (month, day) of the displayed year's final
// day for the Ledger's one-year event window. It MUST be leap-aware
// (MonthDays, not Months[i].Days): a calendar whose last month gains
// LeapYearDays would otherwise exclude events stored on the trailing leap
// days from the SQL window even though ledgerRows iterates those days.
func ledgerYearWindowEnd(cal *Calendar, year int) (lastMonth, lastDay int) {
	lastMonth = len(cal.Months)
	lastDay = 1
	if lastMonth >= 1 {
		lastDay = cal.MonthDays(lastMonth-1, year)
	}
	return lastMonth, lastDay
}

// ledgerMultiDayAnchor returns the (month, day) inside the displayed year at
// which a multi-day event's row should render, and whether it renders at all.
// A span whose start falls in the year anchors on its start day. A span that
// began in a prior year but reaches into this one anchors on the year's first
// day — NOTE: with W1's data layer this branch is defensive only. The repo
// SQL behind ListEventsForDateRange returns rows based in the displayed year
// (plus recurring candidates), so a non-recurring prior-year span never
// reaches the handler pipeline today; W2's multi-year query makes this branch
// real. A span wholly before/after the year is skipped.
func ledgerMultiDayAnchor(e Event, year int, cal *Calendar) (month, day int, ok bool) {
	if e.Year == year {
		return e.Month, e.Day, true
	}
	if e.Year < year {
		endYear := e.Year
		if e.EndYear != nil {
			endYear = *e.EndYear
		}
		if endYear >= year {
			return 1, 1, true
		}
	}
	return 0, 0, false
}

// ledgerRowForOccurrence builds a single-day/recurring row for a specific
// in-year occurrence date (not the event's stored base date — recurrence
// occurrences carry their own day). Tier/category/color reuse the shared
// eventToCardDataWithTiers projection so the Ledger stays visually coherent
// with the month/week/day cards.
func ledgerRowForOccurrence(e Event, cal *Calendar, tiers []TierDefinitionAlias, month, day int) ledgerRowData {
	cd := eventToCardDataWithTiers(e, cal, tiers)
	return ledgerRowData{
		EventID:    e.ID,
		DateLabel:  ledgerDayLabel(cal, month, day),
		Title:      e.Name,
		Tier:       cd.Tier,
		CatColor:   cd.CategoryColor,
		CatName:    cd.CategoryName,
		EntityName: e.EntityName,
		DMOnly:     e.Visibility == "dm_only",
		Span:       false,
	}
}

// ledgerRowForMultiDay builds the single anchored row for a multi-day event,
// with a start–end date label and the span flag set.
func ledgerRowForMultiDay(e Event, cal *Calendar, tiers []TierDefinitionAlias) ledgerRowData {
	cd := eventToCardDataWithTiers(e, cal, tiers)
	return ledgerRowData{
		EventID:    e.ID,
		DateLabel:  ledgerSpanDateLabel(e, cal),
		Title:      e.Name,
		Tier:       cd.Tier,
		CatColor:   cd.CategoryColor,
		CatName:    cd.CategoryName,
		EntityName: e.EntityName,
		DMOnly:     e.Visibility == "dm_only",
		Span:       true,
	}
}

// ledgerDayLabel formats a "MonthName Day" date (e.g. "Mirtul 5"), falling
// back to the bare day number when the month is out of range.
func ledgerDayLabel(cal *Calendar, month, day int) string {
	if cal != nil && month >= 1 && month <= len(cal.Months) {
		return cal.Months[month-1].Name + " " + itoaCal(day)
	}
	return itoaCal(day)
}

// ledgerSpanDateLabel formats a multi-day event's date range. Same month →
// "Ches 12 – 19"; crossing months → "Ches 28 – Tarsakh 3".
func ledgerSpanDateLabel(e Event, cal *Calendar) string {
	start := ledgerDayLabel(cal, e.Month, e.Day)
	endMonth, endDay := e.Month, e.Day
	if e.EndMonth != nil {
		endMonth = *e.EndMonth
	}
	if e.EndDay != nil {
		endDay = *e.EndDay
	}
	if endMonth == e.Month {
		return start + " – " + itoaCal(endDay)
	}
	return start + " – " + ledgerDayLabel(cal, endMonth, endDay)
}

// ledgerYearLabel renders the epoch-suffixed displayed year (e.g. "1492 DR").
func ledgerYearLabel(data CalendarV2ViewData) string {
	epoch := ""
	if data.ActiveCalendar != nil && data.ActiveCalendar.EpochName != nil && *data.ActiveCalendar.EpochName != "" {
		epoch = " " + *data.ActiveCalendar.EpochName
	}
	return itoaCal(data.Year) + epoch
}

// ledgerEraSpan renders an era's year range for the header band
// (e.g. "1488 – 1490 DR" or "1491 DR – ongoing").
func ledgerEraSpan(era *Era, cal *Calendar) string {
	epoch := ""
	if cal != nil && cal.EpochName != nil && *cal.EpochName != "" {
		epoch = " " + *cal.EpochName
	}
	if era.EndYear == nil {
		return itoaCal(era.StartYear) + epoch + " – ongoing"
	}
	return itoaCal(era.StartYear) + " – " + itoaCal(*era.EndYear) + epoch
}

// ledgerHeading is the Ledger view's top heading — the displayed year framed
// as a chronicle (e.g. "Timeline · 1492 DR"). Matches the "Timeline" pill.
func ledgerHeading(data CalendarV2ViewData) string {
	return "Timeline · " + ledgerYearLabel(data)
}

// ledgerEraBandStyle tints the era header band with the operator's era color,
// faded so the header text stays legible. Empty color falls back to the
// neutral surface token. Re-expresses the demo's OKLCH gradient using
// color-mix over the dynamic era color + Tailwind CSS variables.
func ledgerEraBandStyle(color string) string {
	if color == "" {
		return "background-color: var(--color-surface-2, transparent);"
	}
	tint := "color-mix(in srgb, " + color + " 22%, transparent)"
	return "background: linear-gradient(90deg, " + tint + ", transparent 75%);"
}

// ledgerDotStyle returns the inline background for a row's category dot. The
// category color is operator-defined (dynamic), so it can't be a Tailwind
// class; a neutral dot is rendered via a class when the color is empty (see
// ledgerRow), so this is only called with a non-empty color.
func ledgerDotStyle(color string) string {
	return "background:" + color + ";"
}

// ledgerTitleClasses returns the Tailwind classes for a row title, mapping the
// three-level tier to typographic weight: Major = bold + larger, Standard =
// medium, Minor = normal + muted. `flex-1 min-w-0 truncate` lets the title
// grow to fill the row and clip cleanly, pushing the trailing metadata right.
func ledgerTitleClasses(tier calwidget.Tier) string {
	switch tier {
	case calwidget.TierMajor:
		return "flex-1 min-w-0 truncate font-semibold text-fg text-[15px]"
	case calwidget.TierMinor:
		return "flex-1 min-w-0 truncate font-normal text-fg-secondary text-[13px]"
	default:
		return "flex-1 min-w-0 truncate font-medium text-fg text-sm"
	}
}

// ledgerIsEmpty reports whether the assembled Ledger has no rows to show — the
// view then renders a friendly empty state instead of an empty chronicle box.
func ledgerIsEmpty(eras []ledgerEraGroup) bool {
	for _, era := range eras {
		for _, yr := range era.Years {
			if len(yr.Rows) > 0 {
				return false
			}
		}
	}
	return true
}

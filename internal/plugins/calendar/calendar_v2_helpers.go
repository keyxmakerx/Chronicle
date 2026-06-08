// calendar_v2_helpers.go — pure Go helpers consumed by calendar_v2.templ.
// Kept separate from the templ file so the helpers can be unit-tested
// independently and so the .templ file stays focused on rendering.

package calendar

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/a-h/templ"

	calwidget "github.com/keyxmakerx/chronicle/internal/widgets/calendar_v2"
)

// jsonMarshalImpl is the encoding/json indirection used by
// jsonMarshalSafe. Separating the implementation lets tests stub the
// marshaller (currently unused — the indirection is here for future
// extension).
func jsonMarshalImpl(v any) ([]byte, error) {
	return json.Marshal(v)
}

// v2PageTitle builds the browser title for the V2 shell. Falls back
// to "Calendar" when no active calendar is set (zero-calendar campaign).
func v2PageTitle(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil {
		return "Calendar"
	}
	return data.ActiveCalendar.Name
}

// v2CreateCalendarHref points to the V1 setup chooser. Until later
// Wave 1 PRs ship a V2 create flow, the existing setup surface is
// the create affordance.
func v2CreateCalendarHref(campaignID string) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/campaigns/%s/calendars", campaignID))
}

// v2ViewHref builds the URL for switching to a different view of the
// currently-active calendar (preserves year/month/day cursor).
func v2ViewHref(data CalendarV2ViewData, view string) templ.SafeURL {
	if data.ActiveCalendar == nil {
		return templ.SafeURL(fmt.Sprintf("/campaigns/%s/calendar/v2", data.CampaignID))
	}
	return templ.SafeURL(fmt.Sprintf("/campaigns/%s/calendar/v2/%s/%s?year=%d&month=%d&day=%d",
		data.CampaignID, data.ActiveCalendar.ID, view, data.Year, data.Month, data.Day))
}

// v2ViewNavHref builds prev/today/next navigation URLs for the current
// view. delta=-1 prev period, delta=0 today, delta=1 next period.
//
// "Today" maps to the calendar's CurrentYear/CurrentMonth/CurrentDay
// (the stored in-world clock), NOT the OS clock. Per V2 design canon:
// the calendar is a world-clock; "today" is whatever the GM advanced
// the date to last.
func v2ViewNavHref(data CalendarV2ViewData, delta int) templ.SafeURL {
	if data.ActiveCalendar == nil {
		return templ.SafeURL(fmt.Sprintf("/campaigns/%s/calendar/v2", data.CampaignID))
	}
	year, month, day := data.Year, data.Month, data.Day
	switch delta {
	case 0:
		year = data.ActiveCalendar.CurrentYear
		month = data.ActiveCalendar.CurrentMonth
		day = data.ActiveCalendar.CurrentDay
	case -1:
		year, month, day = v2Step(data, -1)
	case 1:
		year, month, day = v2Step(data, 1)
	}
	return templ.SafeURL(fmt.Sprintf("/campaigns/%s/calendar/v2/%s/%s?year=%d&month=%d&day=%d",
		data.CampaignID, data.ActiveCalendar.ID, data.View, year, month, day))
}

// v2Step computes prev/next cursor for the current view. Month +/-1
// is straightforward; Week/Day use day-step math. Crossing year/month
// boundaries respects calendar.Months[].Days.
func v2Step(data CalendarV2ViewData, dir int) (int, int, int) {
	year, month, day := data.Year, data.Month, data.Day
	switch data.View {
	case "month":
		month += dir
		if month < 1 {
			month = len(data.ActiveCalendar.Months)
			year--
		} else if month > len(data.ActiveCalendar.Months) {
			month = 1
			year++
		}
		return year, month, day
	case "week":
		day += 7 * dir
	default: // day
		day += dir
	}
	// Roll over month/year boundaries.
	for day < 1 {
		month--
		if month < 1 {
			month = len(data.ActiveCalendar.Months)
			year--
		}
		day += data.ActiveCalendar.Months[month-1].Days
	}
	for {
		dim := data.ActiveCalendar.Months[month-1].Days
		if day <= dim {
			break
		}
		day -= dim
		month++
		if month > len(data.ActiveCalendar.Months) {
			month = 1
			year++
		}
	}
	return year, month, day
}

// monthHeading renders the human-readable heading for the Month view
// (e.g. "Mirtul 1492 DR"). EpochName is optional.
func monthHeading(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil || data.Month < 1 || data.Month > len(data.ActiveCalendar.Months) {
		return "Calendar"
	}
	name := data.ActiveCalendar.Months[data.Month-1].Name
	epoch := ""
	if data.ActiveCalendar.EpochName != nil && *data.ActiveCalendar.EpochName != "" {
		epoch = " " + *data.ActiveCalendar.EpochName
	}
	return fmt.Sprintf("%s %d%s", name, data.Year, epoch)
}

// monthGridStyle builds the CSS Grid template for the Month view's
// weekday-column layout. Uses the calendar's per-week structure
// (len(Weekdays)) so a fantasy 10-day week renders 10 columns.
func monthGridStyle(data CalendarV2ViewData) string {
	cols := 7
	if data.ActiveCalendar != nil && len(data.ActiveCalendar.Weekdays) > 0 {
		cols = len(data.ActiveCalendar.Weekdays)
	}
	return fmt.Sprintf("grid-template-columns: repeat(%d, minmax(0, 1fr));", cols)
}

// monthDay carries everything a Month-view cell needs. Kept tiny —
// cell rendering doesn't need the full Event payload (PR 4 layers
// that in via a separate slot).
type monthDay struct {
	Day       int
	IsToday   bool
	IsRestDay bool
	// Filler is true for leading/trailing cells that belong to the
	// previous/next month (visual continuity). Renders dimmed.
	Filler bool
}

// monthWeekdayHeaders returns the weekday header labels for the
// Month view's top row.
func monthWeekdayHeaders(data CalendarV2ViewData) []Weekday {
	if data.ActiveCalendar == nil {
		return defaultGregorianWeekdays()
	}
	return data.ActiveCalendar.Weekdays
}

// defaultGregorianWeekdays is the fallback header set used when an
// active calendar has no weekdays defined yet (edge case during
// initial setup; the Show* path normally has weekdays populated).
func defaultGregorianWeekdays() []Weekday {
	return []Weekday{
		{Name: "Sun"}, {Name: "Mon"}, {Name: "Tue"},
		{Name: "Wed"}, {Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"},
	}
}

// monthDays builds the list of cells for the Month view. Cells run
// from day 1 to days-in-month (no leading/trailing fillers in this
// PR — PR 2-4 may add them when the placeholder grows into a full
// renderer). Today + rest-day markers come from the active calendar.
func monthDays(data CalendarV2ViewData) []monthDay {
	if data.ActiveCalendar == nil || data.Month < 1 || data.Month > len(data.ActiveCalendar.Months) {
		return nil
	}
	cal := data.ActiveCalendar
	dim := cal.Months[data.Month-1].Days
	out := make([]monthDay, dim)
	for i := 0; i < dim; i++ {
		day := i + 1
		isToday := data.Year == cal.CurrentYear && data.Month == cal.CurrentMonth && day == cal.CurrentDay
		isRest := false
		// Rest-day tint: year-aware weekday (matches the corrected grid
		// placement), mark cells whose weekday has is_rest_day=1.
		if len(cal.Weekdays) > 0 {
			weekdayIdx := v2WeekdayIndex(data, day)
			if weekdayIdx >= 0 && weekdayIdx < len(cal.Weekdays) && cal.Weekdays[weekdayIdx].IsRestDay {
				isRest = true
			}
		}
		out[i] = monthDay{Day: day, IsToday: isToday, IsRestDay: isRest}
	}
	return out
}

// v2DayOfYear returns the 1-indexed day-of-year for (month, day) in
// the given calendar. Sums Months[0..month-2].Days then adds day.
func v2DayOfYear(cal *Calendar, month, day int) int {
	doy := day
	for i := 0; i < month-1 && i < len(cal.Months); i++ {
		doy += cal.Months[i].Days
	}
	return doy
}

// monthDayClasses returns the Tailwind classes for a Month-view cell
// based on today/rest-day markers. Token-driven (uses --accent-* +
// neutral tints already published by Wave 0 PR #357 + theme system).
func monthDayClasses(_ CalendarV2ViewData, day monthDay) string {
	switch {
	case day.IsToday:
		// NOTE: `today-pulse` was removed here (C-CAL-V2-MONTH-GRID-ALIGN-FIX
		// #2). It is a one-shot keyframe ending at `opacity: 0` with
		// `animation-fill-mode: both`, and it was applied to the CELL itself —
		// so today's cell faded to invisible and stayed blank. The static
		// ring + tint is the persistent today marker; the legible day number
		// stays visible.
		return "bg-accent/10 ring-2 ring-accent"
	case day.IsRestDay:
		return "bg-surface-2"
	case day.Filler:
		return "opacity-50"
	default:
		return ""
	}
}

// --- Event rendering glue (Wave 1 PR 4 / C-CAL-V2-EVENT-CARD-COMPOSITE) ---

// v2CalendarID returns the active calendar's ID for the page root
// data attribute. Empty when no calendar is loaded (zero-cal campaign).
func v2CalendarID(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil {
		return ""
	}
	return data.ActiveCalendar.ID
}

// v2EventsJSON serializes the full event list as JSON for the
// event_grid.js widget. The widget reads this once on mount and uses
// it as the source of truth for drag/edit operations. Failure → "[]"
// so the page still renders the grid even if event data is malformed.
func v2EventsJSON(data CalendarV2ViewData) string {
	if len(data.Events) == 0 {
		return "[]"
	}
	b, err := jsonMarshalSafe(data.Events)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// buildEventVisibilityEditor projects the V2 view data into the
// VisibilityEditorData shape the calwidget consumes. PR 4 ships
// minimal AvailableUsers/AvailableRoles — Members + standard roles
// populate later when the drawer's pickers wire to the membership
// service. For PR 4, the widget renders the chip-row with whatever
// the operator already has set; rule add resolves client-side.
func buildEventVisibilityEditor(data CalendarV2ViewData) calwidget.VisibilityEditorData {
	return calwidget.VisibilityEditorData{
		IsPublic:    true, // default; JS overrides on open
		Rules:       nil,
		FieldPrefix: "event_visibility",
		AvailableRoles: []calwidget.RoleOption{
			{Name: "owner", Label: "Owners"},
			{Name: "scribe", Label: "Scribes"},
			{Name: "player", Label: "Players"},
		},
	}
}

// jsonMarshalSafe wraps json.Marshal to avoid pulling the import into
// the helpers file's main import list (already imported by service.go).
// Plain indirection here; mainly improves readability of the call site.
func jsonMarshalSafe(v any) ([]byte, error) {
	return jsonMarshalImpl(v)
}

// monthCellVisibleCap is the per-cell event chip cap. Anything beyond
// surfaces in the "+N more" affordance. Tuned for default Month-view
// cell sizing (~80px min-height); PR 5 may make this dynamic per
// viewport.
func monthCellVisibleCap() int { return 3 }

// monthCellEvents returns all single-day events for one Month cell.
// Multi-day events render via the ribbon layer above the cell row.
func monthCellEvents(data CalendarV2ViewData, day int) []Event {
	return eventsForDay(data.Events, data.Year, data.Month, day)
}

// v2WeekdayIndex returns the 0-based weekday column for (data.Year, data.Month,
// day) — year-aware (C-CAL-V2-MONTH-GRID-ALIGN-FIX). It mirrors the V1 grid's
// CalendarViewData.WeekdayIndex EXACTLY (absolute day = Year*YearLength +
// prior-month days + day, mod week length) so the V2 Month grid + mini-month
// place each day under the SAME true weekday V1 already does. The prior V2
// placement was offset-blind (day 1 always in column 0) and the rest-day shading
// used a year-BLIND day-of-year, so e.g. real-life June 8 2026 (a Monday)
// rendered under the wrong column. The year term is what makes it correct.
func v2WeekdayIndex(data CalendarV2ViewData, day int) int {
	cal := data.ActiveCalendar
	if cal == nil {
		return 0
	}
	wl := cal.WeekLength()
	if wl == 0 {
		return 0
	}
	abs := data.Year * cal.YearLength()
	for i := 0; i < data.Month-1 && i < len(cal.Months); i++ {
		abs += cal.Months[i].Days
	}
	abs += day
	idx := abs % wl
	if idx < 0 {
		idx += wl
	}
	return idx
}

// v2MonthLeadOffset is the number of leading blank cells before day 1 of the
// displayed month — the weekday column day 1 falls under. This is the SINGLE
// shared offset that the day cells (daysInRow), the multi-day ribbons
// (monthRibbonRows), and the era bands (monthEraBands) all consume, so every
// layer aligns to the same columns.
func v2MonthLeadOffset(data CalendarV2ViewData) int {
	return v2WeekdayIndex(data, 1)
}

// monthDayFor builds the monthDay struct for a specific day-of-month,
// reusing the same today/rest-day logic as `monthDays` so per-row
// rendering matches the original flat grid.
func monthDayFor(data CalendarV2ViewData, day int) monthDay {
	if data.ActiveCalendar == nil || day < 1 {
		return monthDay{Day: day}
	}
	cal := data.ActiveCalendar
	isToday := data.Year == cal.CurrentYear && data.Month == cal.CurrentMonth && day == cal.CurrentDay
	isRest := false
	if len(cal.Weekdays) > 0 {
		// Year-aware weekday so the rest-day tint lines up with the corrected
		// column placement (C-CAL-V2-MONTH-GRID-ALIGN-FIX) — not the old
		// year-blind day-of-year, which would shade the wrong column.
		weekdayIdx := v2WeekdayIndex(data, day)
		if weekdayIdx >= 0 && weekdayIdx < len(cal.Weekdays) && cal.Weekdays[weekdayIdx].IsRestDay {
			isRest = true
		}
	}
	return monthDay{Day: day, IsToday: isToday, IsRestDay: isRest}
}

// monthCellVisible returns up to monthCellVisibleCap() single-day
// events. The rest go to the "+N more" overflow affordance.
func monthCellVisible(data CalendarV2ViewData, day int) []Event {
	all := monthCellEvents(data, day)
	if len(all) <= monthCellVisibleCap() {
		return all
	}
	return all[:monthCellVisibleCap()]
}

// monthCellOverflow returns the count of hidden events for a cell.
func monthCellOverflow(data CalendarV2ViewData, day int) int {
	all := monthCellEvents(data, day)
	if len(all) <= monthCellVisibleCap() {
		return 0
	}
	return len(all) - monthCellVisibleCap()
}

// --- Multi-day ribbon layout (Wave 1 PR 5 §A) ---

// monthRibbonSegment is one ribbon segment in a Month-view week row.
// A multi-day event spanning across week boundaries renders as
// multiple segments (one per week row); same event_id across segments
// so click opens the same drawer.
type monthRibbonSegment struct {
	EventID    string
	Name       string
	Color      string
	IsPublic   bool
	Tier       string // "minor" | "standard" | "major"
	// 1-indexed grid column where the segment starts/ends (relative
	// to the week row's column count).
	StartCol int
	Span     int
	// Continuity flags: when true, the segment is mid-event (event
	// continues out of this week-row edge); flat-cut rather than
	// rounded.
	OpenLeft  bool
	OpenRight bool
	// StackRow tells the renderer which stack slot this segment lives
	// in (0 = top of band; PR 5 caps at 3 with overflow).
	StackRow int
}

// ribbonStackCap is the max ribbons stacked per week row before the
// "+N more" affordance kicks in. Tuned to keep cells readable at
// default sizing.
func ribbonStackCap() int { return 3 }

// monthRibbonRows returns the ribbon segments per visible week row.
// Outer slice index = week row (0-based); inner = segments to render
// in that row, post-stacking. Cells whose day is in any segment's
// range MUST suppress that event from their single-day render —
// monthCellEvents already filters multi-day events out.
func monthRibbonRows(data CalendarV2ViewData) [][]monthRibbonSegment {
	if data.ActiveCalendar == nil || len(data.Events) == 0 {
		return nil
	}
	cal := data.ActiveCalendar
	cols := monthColumnCount(data)
	if cols < 1 {
		return nil
	}
	dim := cal.Months[data.Month-1].Days
	// Shared leading offset so ribbon columns line up with the day cells
	// (C-CAL-V2-MONTH-GRID-ALIGN-FIX): a day sits at grid position
	// (day-1+offset), so its row + per-row start day fold in the offset below.
	offset := v2MonthLeadOffset(data)
	rowCount := (dim + offset + cols - 1) / cols
	rows := make([][]monthRibbonSegment, rowCount)

	// Tier sort key: major(2) > standard(1) > minor(0); ties by start day.
	priority := func(t string) int {
		switch t {
		case "major":
			return 2
		case "minor":
			return 0
		}
		return 1
	}

	type pending struct {
		ev   Event
		tier string
	}
	var multiDay []pending
	for _, e := range data.Events {
		if !isMultiDayEvent(e) {
			continue
		}
		// Only show events that intersect the visible month.
		if !ribbonIntersectsVisibleMonth(e, data.Year, data.Month) {
			continue
		}
		multiDay = append(multiDay, pending{ev: e, tier: "standard"})
	}
	// Sort: tier-major first, then start day ascending. Simple
	// O(n²) bubble keeps the implementation small for typical event
	// counts (TTRPG campaigns rarely exceed dozens of multi-day events).
	for i := 0; i < len(multiDay); i++ {
		for j := i + 1; j < len(multiDay); j++ {
			pi, pj := priority(multiDay[i].tier), priority(multiDay[j].tier)
			swap := false
			if pj > pi {
				swap = true
			} else if pj == pi {
				if multiDay[j].ev.Month < multiDay[i].ev.Month ||
					(multiDay[j].ev.Month == multiDay[i].ev.Month && multiDay[j].ev.Day < multiDay[i].ev.Day) {
					swap = true
				}
			}
			if swap {
				multiDay[i], multiDay[j] = multiDay[j], multiDay[i]
			}
		}
	}

	// Per-row stack tracking: rows[r][stackSlot] = first free day-of-row.
	// We pack segments greedily into the lowest stack slot where the
	// segment's column range doesn't overlap.
	occupied := make([][]int, rowCount) // [row][slot] = first free col
	for r := range occupied {
		occupied[r] = []int{}
	}

	for _, p := range multiDay {
		startDay, endDay := ribbonRangeInVisibleMonth(p.ev, data.Year, data.Month, dim)
		if startDay < 1 || endDay < startDay {
			continue
		}
		startRow := (startDay - 1 + offset) / cols
		endRow := (endDay - 1 + offset) / cols
		for r := startRow; r <= endRow; r++ {
			// Per-row column slice (offset-aware: day at column 0 of row r is
			// r*cols+1-offset, so startCol below maps to the corrected columns):
			rowStartDay := r*cols + 1 - offset
			rowEndDay := rowStartDay + cols - 1
			segStartDay := startDay
			segEndDay := endDay
			if segStartDay < rowStartDay {
				segStartDay = rowStartDay
			}
			if segEndDay > rowEndDay {
				segEndDay = rowEndDay
			}
			startCol := segStartDay - rowStartDay + 1
			endCol := segEndDay - rowStartDay + 1
			span := endCol - startCol + 1
			openLeft := startDay < rowStartDay || ribbonStartsBeforeMonth(p.ev, data.Year, data.Month)
			openRight := endDay > rowEndDay || ribbonEndsAfterMonth(p.ev, data.Year, data.Month)

			// Find lowest stack slot whose first-free-col <= startCol.
			slot := -1
			for i, free := range occupied[r] {
				if free <= startCol {
					slot = i
					break
				}
			}
			if slot == -1 {
				slot = len(occupied[r])
				occupied[r] = append(occupied[r], 0)
			}
			occupied[r][slot] = endCol + 1
			if slot >= ribbonStackCap() {
				continue // overflow; "+N more" affordance ratifies in §J
			}

			color := ""
			if p.ev.Color != nil {
				color = *p.ev.Color
			}
			if color == "" && p.ev.Category != nil {
				for _, c := range cal.EventCategories {
					if c.Slug == *p.ev.Category {
						color = c.Color
						break
					}
				}
			}
			rows[r] = append(rows[r], monthRibbonSegment{
				EventID:   p.ev.ID,
				Name:      p.ev.Name,
				Color:     color,
				IsPublic:  p.ev.Visibility == "everyone",
				Tier:      p.tier,
				StartCol:  startCol,
				Span:      span,
				OpenLeft:  openLeft,
				OpenRight: openRight,
				StackRow:  slot,
			})
		}
	}
	return rows
}

// monthColumnCount returns the number of column-equivalent cells in
// each Month-view week row. Falls back to 7 (Gregorian) when the
// calendar has no weekdays configured.
func monthColumnCount(data CalendarV2ViewData) int {
	if data.ActiveCalendar != nil && len(data.ActiveCalendar.Weekdays) > 0 {
		return len(data.ActiveCalendar.Weekdays)
	}
	return 7
}

// ribbonRangeInMonth clips an event's full date range to the days of
// the current month (1..dim). Multi-day events that start before the
// month return startDay = 1; events ending after the month return
// endDay = dim. The 2-arg form preserves PR 5 internal callers;
// the 4-arg form takes visible (year, month) context for cross-month
// events.
func ribbonRangeInMonth(e Event, dim int) (startDay, endDay int) {
	return ribbonRangeInVisibleMonth(e, e.Year, e.Month, dim)
}

// ribbonRangeInVisibleMonth clips an event range to the visible
// (year, month). When the event starts before the visible month, the
// returned startDay is 1; when it ends after, endDay is dim.
func ribbonRangeInVisibleMonth(e Event, visYear, visMonth, dim int) (startDay, endDay int) {
	// Default to the event's own range.
	startDay = e.Day
	endDay = e.Day
	if e.EndDay != nil {
		endDay = *e.EndDay
	}
	// If the event starts before the visible month, the segment
	// begins at day 1.
	if e.Year < visYear || (e.Year == visYear && e.Month < visMonth) {
		startDay = 1
	}
	// If the event ends after the visible month, the segment goes
	// through the last day of the month.
	endY, endM := e.Year, e.Month
	if e.EndYear != nil {
		endY = *e.EndYear
	}
	if e.EndMonth != nil {
		endM = *e.EndMonth
	}
	if endY > visYear || (endY == visYear && endM > visMonth) {
		endDay = dim
	}
	if startDay < 1 {
		startDay = 1
	}
	if endDay > dim {
		endDay = dim
	}
	return startDay, endDay
}

// ribbonIntersectsVisibleMonth reports whether the event's date range
// touches (year, month). PR 5 §A.3 cross-month boundary case.
func ribbonIntersectsVisibleMonth(e Event, year, month int) bool {
	// Start in or before visible month; end in or after.
	startsBeforeOrIn := e.Year < year ||
		(e.Year == year && e.Month <= month)
	endY, endM := e.Year, e.Month
	if e.EndYear != nil {
		endY = *e.EndYear
	}
	if e.EndMonth != nil {
		endM = *e.EndMonth
	}
	endsInOrAfter := endY > year ||
		(endY == year && endM >= month)
	return startsBeforeOrIn && endsInOrAfter
}

// ribbonStartsBeforeMonth reports whether the event starts before the
// visible month — used to flat-cut the left edge.
func ribbonStartsBeforeMonth(e Event, year, month int) bool {
	if e.Year < year {
		return true
	}
	if e.Year > year {
		return false
	}
	return e.Month < month
}

// ribbonEndsAfterMonth reports whether the event ends after the
// visible month — used to flat-cut the right edge.
func ribbonEndsAfterMonth(e Event, year, month int) bool {
	endY, endM := e.Year, e.Month
	if e.EndYear != nil {
		endY = *e.EndYear
	}
	if e.EndMonth != nil {
		endM = *e.EndMonth
	}
	if endY > year {
		return true
	}
	if endY < year {
		return false
	}
	return endM > month
}

// ribbonClasses returns the Tailwind classes for a ribbon segment.
// Composes the widget-package's tier styling with rounded-corner
// overrides per continuity (cross-row events get flat edges).
func ribbonClasses(seg monthRibbonSegment) string {
	base := "card border-y border-edge text-xs px-2 truncate flex items-center cursor-pointer ribbon-enter"
	switch seg.Tier {
	case "major":
		base += " border-t-accent ring-1 ring-accent/30 font-medium"
	case "minor":
		base += " opacity-40"
	}
	left := "rounded-l-md"
	right := "rounded-r-md"
	if seg.OpenLeft {
		left = "rounded-l-none"
	}
	if seg.OpenRight {
		right = "rounded-r-none"
	}
	return base + " " + left + " " + right
}

// ribbonInlineStyle composes grid-column + tint into one style string.
func ribbonInlineStyle(seg monthRibbonSegment) string {
	span := seg.Span
	if span < 1 {
		span = 1
	}
	tint := "var(--color-surface, #fff)"
	if seg.Color != "" {
		tint = "color-mix(in srgb, " + seg.Color + " 18%, transparent)"
	}
	return "grid-column: " + itoaCal(seg.StartCol) + " / span " + itoaCal(span) +
		"; background-color: " + tint + ";"
}

// --- Era band display (Wave 1 PR 5 §E) ---

// eraBand represents one era stretched as a horizontal band behind
// the Month grid. Surfaces per week row; each row gets a band slice
// for the era's portion of that row.
type eraBand struct {
	Name     string
	Color    string
	StartCol int
	Span     int
	OpenLeft bool
	OpenRight bool
	Row      int // week-row index
}

// monthEraBands returns the era-band slices for each visible week row.
// Eras stack BELOW ribbons (z-order via separate stack slot per
// dispatch §E.2). PR 5 surfaces eras that intersect the visible
// (year, month); deeper era band design (column-row spans across
// month boundaries) is a deferred enhancement.
func monthEraBands(data CalendarV2ViewData) []eraBand {
	if data.ActiveCalendar == nil {
		return nil
	}
	cal := data.ActiveCalendar
	cols := monthColumnCount(data)
	if cols < 1 || data.Month < 1 || data.Month > len(cal.Months) {
		return nil
	}
	dim := cal.Months[data.Month-1].Days
	// Shared leading offset so era bands cover the same columns as the day cells
	// (C-CAL-V2-MONTH-GRID-ALIGN-FIX): row 0 starts after `offset` blanks.
	offset := v2MonthLeadOffset(data)
	rowCount := (dim + offset + cols - 1) / cols

	var bands []eraBand
	for _, era := range cal.Eras {
		if era.StartYear > data.Year {
			continue
		}
		if era.EndYear != nil && *era.EndYear < data.Year {
			continue
		}
		// Era covers the full year → all weeks in this month.
		for r := 0; r < rowCount; r++ {
			rowStartDay := r*cols + 1 - offset
			rowEndDay := rowStartDay + cols - 1
			firstDay := rowStartDay
			if firstDay < 1 {
				firstDay = 1 // skip the leading blanks in row 0
			}
			if rowEndDay > dim {
				rowEndDay = dim // skip trailing blanks in the last row
			}
			if firstDay > rowEndDay {
				continue
			}
			// 1-indexed grid column of the first real day in this row.
			startCol := firstDay - rowStartDay + 1
			span := rowEndDay - firstDay + 1
			bands = append(bands, eraBand{
				Name:     era.Name,
				Color:    era.Color,
				StartCol: startCol,
				Span:     span,
				Row:      r,
				OpenLeft: r > 0 || era.StartYear < data.Year,
				OpenRight: r < rowCount-1 || (era.EndYear == nil || *era.EndYear > data.Year),
			})
		}
	}
	return bands
}

// eraBandStyle returns the inline CSS for an era band — grid-column
// + faded operator color + low z-index so events render legibly above.
func eraBandStyle(band eraBand) string {
	span := band.Span
	if span < 1 {
		span = 1
	}
	tint := "color-mix(in srgb, " + band.Color + " 28%, transparent)"
	if band.Color == "" {
		tint = "var(--color-surface-2, transparent)"
	}
	return "grid-column: " + itoaCal(band.StartCol) + " / span " + itoaCal(span) +
		"; background-color: " + tint + ";"
}

// eraBandsForRow returns the era bands targeting a specific week row.
// Templ filters during rendering.
func eraBandsForRow(bands []eraBand, row int) []eraBand {
	var out []eraBand
	for _, b := range bands {
		if b.Row == row {
			out = append(out, b)
		}
	}
	return out
}

// ribbonsForRow returns the ribbon segments for a specific week row,
// indexed for templ iteration.
func ribbonsForRow(rows [][]monthRibbonSegment, row int) []monthRibbonSegment {
	if row < 0 || row >= len(rows) {
		return nil
	}
	return rows[row]
}

// monthWeekRowCount returns the number of week rows for the current
// month. Used by the templ to iterate.
func monthWeekRowCount(data CalendarV2ViewData) int {
	if data.ActiveCalendar == nil || data.Month < 1 || data.Month > len(data.ActiveCalendar.Months) {
		return 0
	}
	cols := monthColumnCount(data)
	if cols < 1 {
		return 0
	}
	dim := data.ActiveCalendar.Months[data.Month-1].Days
	// Include the leading weekday offset: a month whose 1st sits mid-week can
	// spill into one extra row (C-CAL-V2-MONTH-GRID-ALIGN-FIX).
	cells := dim + v2MonthLeadOffset(data)
	return (cells + cols - 1) / cols
}

// daysInRow returns the day numbers belonging to one week row.
// Last row may be short of full columns; templ uses 0 as a "filler"
// signal.
func daysInRow(data CalendarV2ViewData, row int) []int {
	cols := monthColumnCount(data)
	if cols < 1 || data.ActiveCalendar == nil {
		return nil
	}
	dim := data.ActiveCalendar.Months[data.Month-1].Days
	offset := v2MonthLeadOffset(data)
	out := make([]int, cols)
	for c := 0; c < cols; c++ {
		// Grid position (row*cols+c) maps to day (pos+1-offset): the leading
		// `offset` cells of row 0 are blanks (0) before day 1, and any trailing
		// cells past the month length are blanks too (C-CAL-V2-MONTH-GRID-ALIGN).
		d := row*cols + c + 1 - offset
		if d < 1 || d > dim {
			out[c] = 0
		} else {
			out[c] = d
		}
	}
	return out
}

// --- Wave 1.5 §C: Week + Day hour-grid layout helpers ---

// weekDay carries everything a Week-view day column needs to render
// its header + hour-slot grid. Caller iterates the slice in column
// order; day=0 indicates a column that falls outside the calendar's
// month (e.g. when a fantasy week's column count would push the
// window past the last day of the year).
type weekDay struct {
	Day       int // day-of-month (1..dim) or 0 for filler
	Month     int
	Year      int
	IsToday   bool
	IsRestDay bool
	Weekday   string // weekday name for header
}

// weekDays builds the day-column descriptors for the visible Week
// window centered on the cursor (data.Year, data.Month, data.Day).
// The window length matches the calendar's per-week column count.
//
// PR #367 used `addDaysSimple(month, day, n)` for the +6 step; here
// we extend with backward stepping so the cursor sits roughly mid-
// week. Days that roll past month boundaries surface their actual
// (year, month, day) so the header reads "Apr 28 · Apr 29 · Apr 30
// · May 1 · ..." across boundaries.
func weekDays(data CalendarV2ViewData) []weekDay {
	if data.ActiveCalendar == nil {
		return nil
	}
	cal := data.ActiveCalendar
	cols := monthColumnCount(data)
	if cols < 1 {
		return nil
	}
	// Compute the start of the week: cursor minus N days where N is
	// half the column count (rounded down).
	startBack := cols / 2
	startMonth, startDay := stepDaysBackward(cal, data.Month, data.Day, startBack)
	startYear := data.Year
	if startMonth > data.Month {
		startYear--
	}
	out := make([]weekDay, cols)
	curYear, curMonth, curDay := startYear, startMonth, startDay
	for i := 0; i < cols; i++ {
		isToday := curYear == cal.CurrentYear && curMonth == cal.CurrentMonth && curDay == cal.CurrentDay
		// Rest-day check: day-of-year modulo week-length lands on a
		// weekday with IsRestDay=true.
		isRest := false
		weekdayName := ""
		if curMonth >= 1 && curMonth <= len(cal.Months) {
			doy := v2DayOfYear(cal, curMonth, curDay)
			idx := (doy - 1) % cols
			if idx >= 0 && idx < len(cal.Weekdays) {
				isRest = cal.Weekdays[idx].IsRestDay
				weekdayName = cal.Weekdays[idx].Name
			}
		}
		out[i] = weekDay{
			Day:       curDay,
			Month:     curMonth,
			Year:      curYear,
			IsToday:   isToday,
			IsRestDay: isRest,
			Weekday:   weekdayName,
		}
		// Step forward 1 day; detect year rollover only when the
		// step actually wrapped through the last month of the year
		// (prevMonth was the last month AND curDay reset to 1).
		// This avoids false positives in single-month calendars
		// where every step would otherwise look like a year boundary.
		prevMonth := curMonth
		curMonth, curDay = stepDaysForward(cal, curMonth, curDay, 1)
		if prevMonth == len(cal.Months) && curMonth == 1 && curDay == 1 {
			curYear++
		}
	}
	return out
}

// stepDaysBackward steps (month, day) back by n days within the
// calendar's month structure. Returns the resulting (month, day);
// year rollover is left to the caller.
func stepDaysBackward(cal *Calendar, month, day, n int) (int, int) {
	for n > 0 {
		if day > n {
			day -= n
			break
		}
		n -= day
		month--
		if month < 1 {
			month = len(cal.Months)
		}
		day = cal.Months[month-1].Days
	}
	return month, day
}

// stepDaysForward steps (month, day) forward by n days.
func stepDaysForward(cal *Calendar, month, day, n int) (int, int) {
	for n > 0 {
		remaining := cal.Months[month-1].Days - day
		if n <= remaining {
			day += n
			break
		}
		n -= remaining + 1
		day = 1
		month++
		if month > len(cal.Months) {
			month = 1
		}
	}
	return month, day
}

// hourRows returns the hour labels for the Week + Day hour grid.
// Uses the calendar's HoursPerDay (already in the Calendar model;
// no schema add needed — Wave 1.5 §1 finding). Falls back to 24 if
// the field is somehow zero.
func hourRows(cal *Calendar) []int {
	if cal == nil || cal.HoursPerDay < 1 {
		out := make([]int, 24)
		for i := range out {
			out[i] = i
		}
		return out
	}
	out := make([]int, cal.HoursPerDay)
	for i := range out {
		out[i] = i
	}
	return out
}

// fmtHourLabel renders a 2-digit "HH:00" hour label.
func fmtHourLabel(h int) string {
	hs := itoaCal(h)
	if len(hs) == 1 {
		hs = "0" + hs
	}
	return hs + ":00"
}

// eventsForWeekDay returns events that occur on this specific day in
// the Week view, filtered to those with a time component (timed
// events render in the hour grid; all-day events render in the strip
// via allDayEventsForDay).
func eventsForWeekDay(events []Event, year, month, day int) []Event {
	var out []Event
	for _, e := range events {
		if isMultiDayEvent(e) {
			continue
		}
		if e.Year != year || e.Month != month || e.Day != day {
			continue
		}
		// Timed = has StartHour set.
		if e.StartHour == nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

// allDayEventsForDay returns events on a specific day that have no
// time component — they render in the Week/Day view all-day strip.
func allDayEventsForDay(events []Event, year, month, day int) []Event {
	var out []Event
	for _, e := range events {
		if isMultiDayEvent(e) {
			continue
		}
		if e.Year != year || e.Month != month || e.Day != day {
			continue
		}
		if e.StartHour != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

// eventHourRange returns (startHour, endHour) for a single-day timed
// event, clipped to the calendar's HoursPerDay so events that exceed
// the day length don't render off-grid.
func eventHourRange(e Event, cal *Calendar) (startHour, endHour int) {
	if e.StartHour == nil {
		return 0, 1
	}
	startHour = *e.StartHour
	if e.EndHour != nil && (e.EndYear == nil || *e.EndYear == e.Year) &&
		(e.EndMonth == nil || *e.EndMonth == e.Month) &&
		(e.EndDay == nil || *e.EndDay == e.Day) {
		endHour = *e.EndHour
	} else {
		endHour = startHour + 1
	}
	maxH := 24
	if cal != nil && cal.HoursPerDay > 0 {
		maxH = cal.HoursPerDay
	}
	if endHour > maxH {
		endHour = maxH
	}
	if endHour <= startHour {
		endHour = startHour + 1
	}
	return startHour, endHour
}

// weekTimedEventStyle composes the grid-row inline style for a Week
// view timed event. Uses CSS Grid `grid-row: <start+1> / <end+1>`
// because grid lines are 1-indexed and the all-day strip occupies
// row 1.
func weekTimedEventStyle(e Event, cal *Calendar) string {
	startH, endH := eventHourRange(e, cal)
	// Grid rows: row 1 = all-day strip header; hour 0 starts at row 2.
	return "grid-row: " + itoaCal(startH+2) + " / " + itoaCal(endH+2) + ";"
}

// dayCellClasses returns the Tailwind classes for a Week-view day-
// column header based on today/rest-day markers.
func dayCellClasses(wd weekDay) string {
	switch {
	case wd.IsToday:
		return "bg-accent/10 ring-2 ring-accent"
	case wd.IsRestDay:
		return "bg-surface-2"
	}
	return ""
}

// weekDayLabel composes the column-header text for a Week-view
// column: weekday name + month-name-day (e.g. "Mon · Mirtul 5").
func weekDayLabel(wd weekDay, cal *Calendar) string {
	if wd.Day == 0 || cal == nil {
		return ""
	}
	monthName := ""
	if wd.Month >= 1 && wd.Month <= len(cal.Months) {
		monthName = cal.Months[wd.Month-1].Name
	}
	if wd.Weekday != "" {
		return wd.Weekday + " · " + monthName + " " + itoaCal(wd.Day)
	}
	return monthName + " " + itoaCal(wd.Day)
}

// weekHeading composes the Week view's top heading from the visible
// window's first + last days (e.g. "Mirtul 3 – 9, 1492 DR").
func weekHeading(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil {
		return "Week"
	}
	days := weekDays(data)
	if len(days) == 0 {
		return "Week"
	}
	first := days[0]
	last := days[len(days)-1]
	calName := func(m int) string {
		if m >= 1 && m <= len(data.ActiveCalendar.Months) {
			return data.ActiveCalendar.Months[m-1].Name
		}
		return ""
	}
	epoch := ""
	if data.ActiveCalendar.EpochName != nil && *data.ActiveCalendar.EpochName != "" {
		epoch = " " + *data.ActiveCalendar.EpochName
	}
	if first.Month == last.Month && first.Year == last.Year {
		return calName(first.Month) + " " + itoaCal(first.Day) + " – " + itoaCal(last.Day) + ", " + itoaCal(first.Year) + epoch
	}
	return calName(first.Month) + " " + itoaCal(first.Day) + " – " + calName(last.Month) + " " + itoaCal(last.Day) + ", " + itoaCal(first.Year) + epoch
}

// dayHeading composes the Day view's heading (e.g. "Mirtul 5, 1492 DR").
func dayHeading(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil {
		return "Day"
	}
	monthName := ""
	if data.Month >= 1 && data.Month <= len(data.ActiveCalendar.Months) {
		monthName = data.ActiveCalendar.Months[data.Month-1].Name
	}
	epoch := ""
	if data.ActiveCalendar.EpochName != nil && *data.ActiveCalendar.EpochName != "" {
		epoch = " " + *data.ActiveCalendar.EpochName
	}
	return monthName + " " + itoaCal(data.Day) + ", " + itoaCal(data.Year) + epoch
}

// weekColumnsStyle builds the CSS Grid template-columns for the
// Week-view header + all-day strip: hour-label column at left
// (~60px) + N equal day columns.
func weekColumnsStyle(dayCount int) string {
	if dayCount < 1 {
		dayCount = 7
	}
	return "grid-template-columns: 60px repeat(" + itoaCal(dayCount) + ", minmax(0, 1fr));"
}

// weekHourGridStyle builds the CSS Grid template for the scrolling
// hour grid: hour-label column + day columns × hour-per-day rows.
// Each row is 32px to keep the grid readable; total height scales
// with HoursPerDay.
func weekHourGridStyle(data CalendarV2ViewData, dayCount int) string {
	rows := 24
	if data.ActiveCalendar != nil && data.ActiveCalendar.HoursPerDay > 0 {
		rows = data.ActiveCalendar.HoursPerDay
	}
	if dayCount < 1 {
		dayCount = 7
	}
	return "grid-template-columns: 60px repeat(" + itoaCal(dayCount) + ", minmax(0, 1fr)); " +
		"grid-template-rows: 20px repeat(" + itoaCal(rows) + ", 32px);"
}

// dayHourGridStyle builds the CSS Grid template for the Day view's
// hour grid: hour-label column + single content column × hour rows.
func dayHourGridStyle(data CalendarV2ViewData) string {
	rows := 24
	if data.ActiveCalendar != nil && data.ActiveCalendar.HoursPerDay > 0 {
		rows = data.ActiveCalendar.HoursPerDay
	}
	return "grid-template-columns: 60px 1fr; grid-template-rows: repeat(" + itoaCal(rows) + ", 40px);"
}

// dayTimedEventStyle is the Day view variant of weekTimedEventStyle.
// Day view has no all-day-strip in the grid (rendered separately
// above), so hour 0 starts at row 1 (not row 2).
func dayTimedEventStyle(e Event, cal *Calendar) string {
	startH, endH := eventHourRange(e, cal)
	return "grid-row: " + itoaCal(startH+1) + " / " + itoaCal(endH+1) + ";"
}

// weekHourCellClasses returns the per-cell tint for a Week-view
// background hour slot — today's column gets a subtle accent tint;
// rest-day columns get the same surface-2 tint as Month rest days.
func weekHourCellClasses(wd weekDay) string {
	switch {
	case wd.IsToday:
		return "bg-accent/5"
	case wd.IsRestDay:
		return "bg-surface-2/30"
	}
	return ""
}

// intStr is a wrapper used by templ inline styles where a plain
// strconv.Itoa would be too much to import locally.
func intStr(n int) string { return itoaCal(n) }

// eventsForDay returns the single-day events that fall on (year,
// month, day). Multi-day events render via the ribbon layer and are
// filtered out here; ribbons render once at the top of the week row.
//
// An event is "single-day" when EndYear/EndMonth/EndDay are nil or
// when the end date equals the start date.
func eventsForDay(events []Event, year, month, day int) []Event {
	var out []Event
	for _, e := range events {
		if e.Year != year || e.Month != month || e.Day != day {
			continue
		}
		if isMultiDayEvent(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// isMultiDayEvent reports whether an event spans more than one day.
// Used by the rendering layer to route single-day vs ribbon paths.
func isMultiDayEvent(e Event) bool {
	if e.EndYear == nil && e.EndMonth == nil && e.EndDay == nil {
		return false
	}
	if e.EndYear != nil && *e.EndYear != e.Year {
		return true
	}
	if e.EndMonth != nil && *e.EndMonth != e.Month {
		return true
	}
	if e.EndDay != nil && *e.EndDay != e.Day {
		return true
	}
	return false
}

// eventToCardData projects a calendar.Event into the
// calendar_v2.EventCardData widget payload. The category color
// resolves from the calendar's EventCategories by matching Category
// slug; falls back to Event.Color override; defaults to "".
//
// Tier is hardcoded to Standard in PR 4 — PR 5 wires the campaign's
// tier definitions (PR #358 event_tier_definitions field). Visibility
// is "public" iff Event.Visibility == "everyone".
func eventToCardData(e Event, cal *Calendar) calwidget.EventCardData {
	return eventToCardDataWithTiers(e, cal, nil)
}

// eventToCardDataWithTiers extends the projection with campaign-aware
// tier resolution. Looks up e.Tier (the new field added in Wave 1.6
// Phase 1) against the supplied tier definitions; sets TierLabel +
// TierColor on the resulting EventCardData when resolved, falls back
// to platform default (Tier: TierStandard) when nil or unresolved.
// A reference to a deleted tier slug logs slog.Warn so operators
// can clean up but doesn't crash the render.
func eventToCardDataWithTiers(e Event, cal *Calendar, tiers []TierDefinitionAlias) calwidget.EventCardData {
	color := ""
	cat := ""
	if e.Color != nil && *e.Color != "" {
		color = *e.Color
	}
	if e.Category != nil && *e.Category != "" {
		cat = *e.Category
		if color == "" && cal != nil {
			for _, c := range cal.EventCategories {
				if c.Slug == *e.Category {
					color = c.Color
					cat = c.Name
					break
				}
			}
		}
	}
	desc := ""
	if e.DescriptionHTML != nil {
		desc = *e.DescriptionHTML
	}
	timeLabel := ""
	if e.StartHour != nil && e.StartMinute != nil {
		timeLabel = fmtTimeDigits(*e.StartHour, *e.StartMinute)
		if e.EndHour != nil && e.EndMinute != nil {
			timeLabel += " — " + fmtTimeDigits(*e.EndHour, *e.EndMinute)
		}
	}
	startLabel := ""
	if cal != nil && e.Month >= 1 && e.Month <= len(cal.Months) {
		startLabel = cal.Months[e.Month-1].Name + " " + itoaCal(e.Day)
	}

	tier := calwidget.TierStandard
	tierLabel := ""
	tierColor := ""
	if e.Tier != nil && *e.Tier != "" {
		found := false
		for _, td := range tiers {
			if td.Slug == *e.Tier {
				tier = mapProminenceToTier(td.Prominence)
				tierLabel = td.Name
				tierColor = td.Color
				found = true
				break
			}
		}
		if !found {
			slog.Warn("event references unknown tier slug; falling back to platform default",
				slog.String("event_id", e.ID),
				slog.String("tier_slug", *e.Tier),
			)
		}
	}

	return calwidget.EventCardData{
		ID:              e.ID,
		Name:            e.Name,
		CategoryColor:   color,
		CategoryName:    cat,
		Tier:            tier,
		TierLabel:       tierLabel,
		TierColor:       tierColor,
		IsPublic:        e.Visibility == "everyone",
		DescriptionHTML: desc,
		StartLabel:      startLabel,
		TimeLabel:       timeLabel,
	}
}

// TierDefinitionAlias is the calendar-plugin-side view of campaigns.
// TierDefinition. Kept local so the calendar plugin's helpers don't
// import campaigns directly (per Chronicle plugin-isolation
// convention). Handler-layer code copies from the campaigns service
// into this shape before calling the projection.
type TierDefinitionAlias struct {
	Slug       string
	Name       string
	Color      string
	Prominence int
}

// mapProminenceToTier collapses the 0-100 campaign prominence into
// the widget's three-level enum:
//   - 0..33  → TierMinor (subdued render)
//   - 34..66 → TierStandard (default)
//   - 67..100 → TierMajor (accent + elevation)
//
// The boundary numbers fall on natural thirds; future iterations can
// widen the bands if operator-defined prominence patterns warrant.
func mapProminenceToTier(prominence int) calwidget.Tier {
	if prominence >= 67 {
		return calwidget.TierMajor
	}
	if prominence <= 33 {
		return calwidget.TierMinor
	}
	return calwidget.TierStandard
}

// fmtTimeDigits formats an hour:minute pair as two-digit "HH:MM".
func fmtTimeDigits(h, m int) string {
	hs := itoaCal(h)
	if len(hs) == 1 {
		hs = "0" + hs
	}
	ms := itoaCal(m)
	if len(ms) == 1 {
		ms = "0" + ms
	}
	return hs + ":" + ms
}

// itoaCal is a tiny strconv.Itoa stand-in (avoids adding the import).
func itoaCal(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// --- Wave 1.7A §G mini-month helpers ---

// miniMonthDay carries minimum data for one mini-month cell.
type miniMonthDay struct {
	Day       int
	IsToday   bool
	IsSelected bool
}

// miniMonthHeading returns the sidebar heading text (e.g. "Mirtul 1492 DR").
func miniMonthHeading(data CalendarV2ViewData) string {
	return monthHeading(data)
}

// miniMonthGridStyle returns the CSS Grid template for the
// mini-month: N equal columns where N = calendar's weekday count
// (fantasy-week aware; matches monthGridStyle pattern).
func miniMonthGridStyle(data CalendarV2ViewData) string {
	return monthGridStyle(data)
}

// miniMonthWeekdayLabels returns abbreviated weekday labels for the
// mini-month header row — first character only, to keep cells tight.
func miniMonthWeekdayLabels(data CalendarV2ViewData) []string {
	headers := monthWeekdayHeaders(data)
	out := make([]string, len(headers))
	for i, h := range headers {
		if h.Name != "" {
			out[i] = string(h.Name[0])
		}
	}
	return out
}

// miniMonthDays builds the day cells for the mini-month. Same shape
// as the main Month view's monthDays but with IsSelected reflecting
// the page cursor (so navigating to a day highlights it in the
// sidebar).
func miniMonthDays(data CalendarV2ViewData) []miniMonthDay {
	if data.ActiveCalendar == nil || data.Month < 1 || data.Month > len(data.ActiveCalendar.Months) {
		return nil
	}
	cal := data.ActiveCalendar
	dim := cal.Months[data.Month-1].Days
	// Leading blanks so day 1 lands under its true weekday — same shared offset
	// as the main grid (C-CAL-V2-MONTH-GRID-ALIGN-FIX). Day==0 → the template
	// renders an empty filler cell.
	offset := v2MonthLeadOffset(data)
	out := make([]miniMonthDay, 0, dim+offset)
	for i := 0; i < offset; i++ {
		out = append(out, miniMonthDay{Day: 0})
	}
	for i := 0; i < dim; i++ {
		day := i + 1
		out = append(out, miniMonthDay{
			Day:        day,
			IsToday:    data.Year == cal.CurrentYear && data.Month == cal.CurrentMonth && day == cal.CurrentDay,
			IsSelected: day == data.Day,
		})
	}
	return out
}

// miniMonthDayClasses returns the per-cell tint for the mini-month.
// Today + selected combinations layer cleanly: today wins visually
// when both apply (selected adds a slight overlay).
func miniMonthDayClasses(d miniMonthDay) string {
	switch {
	case d.IsToday && d.IsSelected:
		return "bg-accent text-white ring-2 ring-accent"
	case d.IsToday:
		return "ring-2 ring-accent text-fg"
	case d.IsSelected:
		return "bg-accent/20 text-fg font-medium"
	}
	return "text-fg"
}

// miniMonthDayHref builds a same-month jump link. Preserves view +
// year/month; only the day cursor changes.
func miniMonthDayHref(data CalendarV2ViewData, d miniMonthDay) templ.SafeURL {
	if data.ActiveCalendar == nil {
		return templ.SafeURL("/campaigns/" + data.CampaignID + "/calendar/v2")
	}
	return templ.SafeURL(fmt.Sprintf("/campaigns/%s/calendar/v2/%s/%s?year=%d&month=%d&day=%d",
		data.CampaignID, data.ActiveCalendar.ID, data.View, data.Year, data.Month, d.Day))
}

// miniMonthDataAttr serializes the jump target as a data attribute
// for the JS click handler — matches the dispatch §B.3 convention
// for date-jump targeting per view type. Format: "YYYY-MM-DD" using
// fantasy-calendar year (not Gregorian).
func miniMonthDataAttr(data CalendarV2ViewData, d miniMonthDay) string {
	return fmt.Sprintf("%d-%02d-%02d", data.Year, data.Month, d.Day)
}

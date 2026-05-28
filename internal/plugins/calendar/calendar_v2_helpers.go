// calendar_v2_helpers.go — pure Go helpers consumed by calendar_v2.templ.
// Kept separate from the templ file so the helpers can be unit-tested
// independently and so the .templ file stays focused on rendering.

package calendar

import (
	"encoding/json"
	"fmt"

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
		// Rest-day tint: cycle weekdays per-day-of-year, mark cells
		// whose weekday has is_rest_day=1.
		if len(cal.Weekdays) > 0 {
			doy := v2DayOfYear(cal, data.Month, day)
			weekdayIdx := (doy - 1) % len(cal.Weekdays)
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
		return "bg-accent/10 ring-2 ring-accent today-pulse"
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
func monthCellEvents(data CalendarV2ViewData, day int) []Event {
	return eventsForDay(data.Events, data.Year, data.Month, day)
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
	return calwidget.EventCardData{
		ID:              e.ID,
		Name:            e.Name,
		CategoryColor:   color,
		CategoryName:    cat,
		Tier:            calwidget.TierStandard,
		IsPublic:        e.Visibility == "everyone",
		DescriptionHTML: desc,
		StartLabel:      startLabel,
		TimeLabel:       timeLabel,
	}
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

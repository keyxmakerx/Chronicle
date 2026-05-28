// calendar_v2_helpers.go — pure Go helpers consumed by calendar_v2.templ.
// Kept separate from the templ file so the helpers can be unit-tested
// independently and so the .templ file stays focused on rendering.

package calendar

import (
	"fmt"

	"github.com/a-h/templ"
)

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
		return "bg-accent/10 ring-2 ring-accent"
	case day.IsRestDay:
		return "bg-surface-2"
	case day.Filler:
		return "opacity-50"
	default:
		return ""
	}
}

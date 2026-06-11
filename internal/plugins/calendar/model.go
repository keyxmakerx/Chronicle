// Package calendar provides a custom fantasy calendar system for campaigns.
// Supports non-Gregorian months, named weekdays, moons with phase tracking,
// named seasons, and events linked to entities. Each campaign can have
// multiple calendars (one marked as default); the addon must be enabled
// per-campaign.
package calendar

import (
	"encoding/json"
	"fmt"
	"time"
)

// VisibilityRules defines per-user visibility overrides for calendar events.
// If AllowedUsers is set, only those users can see the item (whitelist).
// If DeniedUsers is set, those users cannot see the item (blacklist).
// AllowedUsers takes precedence: if set, DeniedUsers is ignored.
type VisibilityRules struct {
	AllowedUsers []string `json:"allowed_users,omitempty"`
	DeniedUsers  []string `json:"denied_users,omitempty"`
}

// UpdateEventVisibilityInput is the validated input for updating event visibility.
type UpdateEventVisibilityInput struct {
	Visibility      string  `json:"visibility"`
	VisibilityRules *string `json:"visibility_rules"`
}

// UpdateCalendarVisibilityInput is the validated input for updating a
// calendar's per-calendar visibility (C-CAL-DASHBOARD-W5b). Same shape as the
// event one — the calendar reuses the event visibility model + resolver.
type UpdateCalendarVisibilityInput struct {
	Visibility      string  `json:"visibility"`
	VisibilityRules *string `json:"visibility_rules"`
}

// CalendarEventDate is a lightweight (calendar, date, name) tuple from the W5d
// batch upcoming read — kept minimal so the cross-calendar query stays cheap.
type CalendarEventDate struct {
	CalendarID string
	Year       int
	Month      int
	Day        int
	Name       string
}

// CalendarUpcoming is a calendar's next upcoming event + a short agenda
// (C-CAL-DASHBOARD-W5d), computed from the batch read relative to that
// calendar's own current date. Next is nil when the calendar has none upcoming.
type CalendarUpcoming struct {
	Next   *CalendarEventDate
	Agenda []CalendarEventDate
}

// Calendar mode constants.
const (
	// ModeFantasy indicates a fully custom fantasy calendar.
	ModeFantasy = "fantasy"
	// ModeRealLife indicates a Gregorian calendar synced to real-world time.
	ModeRealLife = "reallife"
)

// Calendar is the top-level calendar definition for a campaign.
type Calendar struct {
	ID             string  `json:"id"`
	CampaignID     string  `json:"campaign_id"`
	Mode           string  `json:"mode"` // "fantasy" or "reallife"
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	EpochName      *string `json:"epoch_name,omitempty"`
	CurrentYear    int     `json:"current_year"`
	CurrentMonth   int     `json:"current_month"`
	CurrentDay     int     `json:"current_day"`
	HoursPerDay      int `json:"hours_per_day"`
	MinutesPerHour   int `json:"minutes_per_hour"`
	SecondsPerMinute int `json:"seconds_per_minute"`
	CurrentHour    int     `json:"current_hour"`
	CurrentMinute  int     `json:"current_minute"`
	LeapYearEvery  int     `json:"leap_year_every"`
	LeapYearOffset int     `json:"leap_year_offset"`
	SortOrder      int     `json:"sort_order"`
	IsDefault      bool    `json:"is_default"`
	// Persisted live mood-tint wash (migration 008 /
	// C-CAL-WORLDSTATE-SERVER-MODEL). Both nil = no mood set. D2 is a
	// page-load read, so plain nullable columns suffice.
	MoodTintColor     *string  `json:"mood_tint_color,omitempty"`
	MoodTintIntensity *float64 `json:"mood_tint_intensity,omitempty"`
	// Per-calendar visibility (C-CAL-DASHBOARD-W5a, migration 010). Mirrors the
	// event model: Visibility is "everyone" | "dm_only"; VisibilityRules is the
	// optional {allowed_users,denied_users} JSON allow/deny override. Default
	// "everyone" = visible to all members (the DB default; existing calendars
	// unaffected). Resolved by canUserView / filterCalendarsByUser.
	Visibility      string  `json:"visibility"`
	VisibilityRules *string `json:"visibility_rules,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Eager-loaded sub-resources (populated by service, not by every query).
	Months          []Month         `json:"months,omitempty"`
	Weekdays        []Weekday       `json:"weekdays,omitempty"`
	Moons           []Moon          `json:"moons,omitempty"`
	Seasons         []Season        `json:"seasons,omitempty"`
	Eras            []Era           `json:"eras,omitempty"`
	EventCategories []EventCategory `json:"event_categories,omitempty"`
	Cycles          []Cycle         `json:"cycles,omitempty"`
	Festivals       []Festival      `json:"festivals,omitempty"`
	// Weather is nil when no row exists in calendar_weather for this
	// calendar. Added in C-CAL-WCF-UI so the settings page can render
	// the current state without a second handler-side fetch.
	Weather *Weather `json:"weather,omitempty"`
}

// GetCampaignID returns the campaign ID this calendar belongs to.
// Implements middleware.CampaignScoped for IDOR protection.
func (c *Calendar) GetCampaignID() string {
	return c.CampaignID
}

// IsRealLife returns true if this calendar syncs to real-world time.
func (c *Calendar) IsRealLife() bool {
	return c.Mode == ModeRealLife
}

// IsLeapYear returns true if the given year is a leap year according to
// the calendar's leap year configuration. LeapYearEvery=0 means no leap years.
func (c *Calendar) IsLeapYear(year int) bool {
	if c.LeapYearEvery <= 0 {
		return false
	}
	return (year-c.LeapYearOffset)%c.LeapYearEvery == 0
}

// YearLength returns the total number of days in a year by summing all month
// lengths. Does not account for leap year — use YearLengthForYear for that.
func (c *Calendar) YearLength() int {
	total := 0
	for _, m := range c.Months {
		total += m.Days
	}
	return total
}

// YearLengthForYear returns the total days in a specific year, including
// leap year extra days if applicable.
func (c *Calendar) YearLengthForYear(year int) int {
	total := 0
	isLeap := c.IsLeapYear(year)
	for _, m := range c.Months {
		total += m.Days
		if isLeap {
			total += m.LeapYearDays
		}
	}
	return total
}

// MonthDays returns the number of days in a month for a given year,
// accounting for leap year extra days.
func (c *Calendar) MonthDays(monthIdx int, year int) int {
	if monthIdx < 0 || monthIdx >= len(c.Months) {
		return 0
	}
	days := c.Months[monthIdx].Days
	if c.IsLeapYear(year) {
		days += c.Months[monthIdx].LeapYearDays
	}
	return days
}

// WeekLength returns the number of days in a week (number of weekdays).
func (c *Calendar) WeekLength() int {
	return len(c.Weekdays)
}

// Recurrence type constants — mirror the sessions plugin's vocabulary verbatim
// (internal/plugins/sessions/model.go) so the two share semantics
// (C-CAL-EDITOR-EXPANSION PR2). Any other / empty recurrence_type renders ONCE
// at its stored date, so legacy rows are untouched.
const (
	RecurrenceWeekly   = "weekly"   // every week, on the base date's weekday
	RecurrenceBiWeekly = "biweekly" // every 2 weeks
	RecurrenceMonthly  = "monthly"  // same day-of-month each month
	RecurrenceCustom   = "custom"   // every N weeks (RecurrenceInterval)
)

// absDayIndex returns a calendar-absolute day number for (year, month, day)
// using the constant year length — identical to the weekday math in
// v2WeekdayIndexFor, so week alignment stays consistent across the app. month
// is 1-based.
func (c *Calendar) absDayIndex(year, month, day int) int {
	abs := year * c.YearLength()
	for i := 0; i < month-1 && i < len(c.Months); i++ {
		abs += c.Months[i].Days
	}
	return abs + day
}

// OccursOn reports whether the event lands on (year, month, day) for cal. The
// SINGLE recurrence-expansion predicate — every grid/list projection routes
// through it so there is one source of truth (C-CAL-EDITOR-EXPANSION PR2).
//
// Non-recurring events (or a legacy/empty/unknown recurrence_type) match only
// their stored date — the prior behavior, so existing rows are untouched. The
// four recurring types expand forward from the base date:
//   - weekly/biweekly/custom: every (interval × week) days, base-anchored, so
//     each instance shares the base weekday;
//   - monthly: the same day-of-month each month, skipped in months too short for
//     that day (leap-aware via MonthDays).
//
// Recurrence stops at the recurrence-end date (inclusive) and/or after
// RecurrenceMaxOccurrences. Multi-day events are not expanded here (the ribbon
// layer renders their span).
func (e Event) OccursOn(cal *Calendar, year, month, day int) bool {
	onBase := e.Year == year && e.Month == month && e.Day == day
	if !e.IsRecurring || e.RecurrenceType == nil || cal == nil {
		return onBase
	}
	switch *e.RecurrenceType {
	case RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly, RecurrenceCustom:
		// expanded below
	default:
		return onBase // legacy / unknown type → single occurrence
	}

	base := cal.absDayIndex(e.Year, e.Month, e.Day)
	target := cal.absDayIndex(year, month, day)
	if target < base {
		return false // recurrence only moves forward from the base date
	}
	if e.RecurrenceEndYear != nil && e.RecurrenceEndMonth != nil && e.RecurrenceEndDay != nil {
		if target > cal.absDayIndex(*e.RecurrenceEndYear, *e.RecurrenceEndMonth, *e.RecurrenceEndDay) {
			return false // past the recurrence-end date
		}
	}

	if *e.RecurrenceType == RecurrenceMonthly {
		if day != e.Day || day > cal.MonthDays(month-1, year) {
			return false
		}
		if e.RecurrenceMaxOccurrences != nil {
			if n := monthsBetween(cal, e.Year, e.Month, year, month); n < 0 || n >= *e.RecurrenceMaxOccurrences {
				return false
			}
		}
		return true
	}

	// Week-based (weekly / biweekly / custom).
	wl := cal.WeekLength()
	stride := wl * recurrenceWeeks(*e.RecurrenceType, e.RecurrenceInterval)
	if stride <= 0 {
		return onBase
	}
	diff := target - base
	if diff%stride != 0 {
		return false
	}
	if e.RecurrenceMaxOccurrences != nil && diff/stride >= *e.RecurrenceMaxOccurrences {
		return false // occurrence index (0-based) past the cap
	}
	return true
}

// recurrenceWeeks maps a week-based recurrence type to its week interval.
func recurrenceWeeks(rtype string, interval *int) int {
	switch rtype {
	case RecurrenceBiWeekly:
		return 2
	case RecurrenceCustom:
		if interval != nil && *interval > 0 {
			return *interval
		}
	}
	return 1 // weekly (and custom with no/invalid interval)
}

// monthsBetween returns the whole-month offset from (y1,m1) to (y2,m2) using the
// calendar's (constant) month count. Negative when (y2,m2) precedes (y1,m1).
func monthsBetween(cal *Calendar, y1, m1, y2, m2 int) int {
	mc := len(cal.Months)
	if mc == 0 {
		return 0
	}
	return (y2-y1)*mc + (m2 - m1)
}

// FormatCurrentTime returns the current time formatted as "HH:MM".
// Pads hours/minutes with leading zeros based on the max values
// (e.g. a 24-hour system uses 2 digits, a 100-hour system uses 3).
func (c *Calendar) FormatCurrentTime() string {
	return fmt.Sprintf("%02d:%02d", c.CurrentHour, c.CurrentMinute)
}

// CurrentSeason returns the season for the current date, or nil if none match.
func (c *Calendar) CurrentSeason() *Season {
	return c.SeasonForDate(c.CurrentMonth, c.CurrentDay)
}

// SeasonForDate returns the season containing the given month+day, or nil.
func (c *Calendar) SeasonForDate(month, day int) *Season {
	for i := range c.Seasons {
		s := &c.Seasons[i]
		if s.ContainsDate(month, day) {
			return s
		}
	}
	return nil
}

// CurrentEra returns the era containing the current year, or nil if none match.
func (c *Calendar) CurrentEra() *Era {
	return c.EraForYear(c.CurrentYear)
}

// EraForYear returns the era containing the given year, or nil if none match.
// An era with nil EndYear is considered ongoing (matches all years >= StartYear).
func (c *Calendar) EraForYear(year int) *Era {
	for i := range c.Eras {
		e := &c.Eras[i]
		if year >= e.StartYear && (e.EndYear == nil || year <= *e.EndYear) {
			return e
		}
	}
	return nil
}

// AbsoluteDay returns the total number of days from year 0 day 0 to the given
// date (year, month 1-indexed, day). Used for moon phase calculation.
func (c *Calendar) AbsoluteDay(year, month, day int) int {
	total := 0
	// Add full years.
	for y := 0; y < year; y++ {
		total += c.YearLengthForYear(y)
	}
	// Add full months in the current year.
	for i := 0; i < month-1 && i < len(c.Months); i++ {
		total += c.MonthDays(i, year)
	}
	total += day
	return total
}

// CurrentAbsoluteDay returns AbsoluteDay for the current date.
func (c *Calendar) CurrentAbsoluteDay() int {
	return c.AbsoluteDay(c.CurrentYear, c.CurrentMonth, c.CurrentDay)
}

// Month is a named period in the calendar with a configurable number of days.
type Month struct {
	ID            int    `json:"id"`
	CalendarID    string `json:"calendar_id"`
	Name          string `json:"name"`
	Days          int    `json:"days"`
	SortOrder     int    `json:"sort_order"`
	IsIntercalary bool   `json:"is_intercalary"`
	LeapYearDays  int    `json:"leap_year_days"`
}

// Weekday is a named day in the repeating weekly cycle.
type Weekday struct {
	ID         int    `json:"id"`
	CalendarID string `json:"calendar_id"`
	Name       string `json:"name"`
	SortOrder  int    `json:"sort_order"`
	IsRestDay  bool   `json:"is_rest_day"`
}

// Moon is a celestial body with a phase cycle used for moon phase display.
//
// BaseDesign / Tint / PhaseSource / Size / OrbitSpeed are the moon-library
// render params (migration 008 / C-CAL-WORLDSTATE-SERVER-MODEL). They mirror
// the showcase's MOON_DESIGNS parameters so a moon's appearance can be
// authored rather than hardcoded in JS. Existing moons read the column
// defaults ('moon-realistic-selene' / null / 'css-clip' / 1 / 1).
type Moon struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
	BaseDesign  string  `json:"base_design"`
	Tint        *string `json:"tint,omitempty"`
	PhaseSource string  `json:"phase_source"`
	Size        float64 `json:"size"`
	OrbitSpeed  float64 `json:"orbit_speed"`
}

// MoonPhase returns the phase (0.0–1.0) of this moon on a given absolute day
// number (days since year 0 day 0). 0=new, 0.25=first quarter, 0.5=full,
// 0.75=last quarter.
func (m *Moon) MoonPhase(absoluteDay int) float64 {
	if m.CycleDays <= 0 {
		return 0
	}
	raw := (float64(absoluteDay) + m.PhaseOffset) / m.CycleDays
	phase := raw - float64(int(raw))
	if phase < 0 {
		phase += 1
	}
	return phase
}

// MoonPhaseName returns a human-readable phase name.
func (m *Moon) MoonPhaseName(absoluteDay int) string {
	phase := m.MoonPhase(absoluteDay)
	switch {
	case phase < 0.125:
		return "New Moon"
	case phase < 0.25:
		return "Waxing Crescent"
	case phase < 0.375:
		return "First Quarter"
	case phase < 0.5:
		return "Waxing Gibbous"
	case phase < 0.625:
		return "Full Moon"
	case phase < 0.75:
		return "Waning Gibbous"
	case phase < 0.875:
		return "Last Quarter"
	default:
		return "Waning Crescent"
	}
}

// MoonPhaseIcon returns an icon identifier for the current phase.
func (m *Moon) MoonPhaseIcon(absoluteDay int) string {
	phase := m.MoonPhase(absoluteDay)
	switch {
	case phase < 0.125:
		return "circle-dot"
	case phase < 0.25:
		return "moon-waxing-crescent"
	case phase < 0.375:
		return "moon-first-quarter"
	case phase < 0.5:
		return "moon-waxing-gibbous"
	case phase < 0.625:
		return "moon"
	case phase < 0.75:
		return "moon-waning-gibbous"
	case phase < 0.875:
		return "moon-last-quarter"
	default:
		return "moon-waning-crescent"
	}
}

// Season is a named period spanning a range of month+day to month+day.
type Season struct {
	ID            int     `json:"id"`
	CalendarID    string  `json:"calendar_id"`
	Name          string  `json:"name"`
	StartMonth    int     `json:"start_month"`
	StartDay      int     `json:"start_day"`
	EndMonth      int     `json:"end_month"`
	EndDay        int     `json:"end_day"`
	Description   *string `json:"description,omitempty"`
	Color         string  `json:"color"`
	WeatherEffect *string `json:"weather_effect,omitempty"`
}

// ContainsDate returns true if the given month+day falls within this season.
// Handles wrap-around (e.g. Winter: month 11 day 1 → month 2 day 28).
func (s *Season) ContainsDate(month, day int) bool {
	startVal := s.StartMonth*100 + s.StartDay
	endVal := s.EndMonth*100 + s.EndDay
	dateVal := month*100 + day

	if startVal <= endVal {
		// Normal range (e.g. Spring: 3/1 → 5/31).
		return dateVal >= startVal && dateVal <= endVal
	}
	// Wrap-around (e.g. Winter: 11/1 → 2/28).
	return dateVal >= startVal || dateVal <= endVal
}

// Era is a named time period spanning a range of years (e.g. "First Age", "Age of Fire").
type Era struct {
	ID         int     `json:"id"`
	CalendarID string  `json:"calendar_id"`
	Name       string  `json:"name"`
	StartYear  int     `json:"start_year"`
	EndYear    *int    `json:"end_year,omitempty"` // nil = ongoing
	Description *string `json:"description,omitempty"`
	Color      string  `json:"color"`
	SortOrder  int     `json:"sort_order"`
}

// IsOngoing returns true if this era has no end year (still in progress).
func (e *Era) IsOngoing() bool {
	return e.EndYear == nil
}

// ContainsYear returns true if the given year falls within this era.
func (e *Era) ContainsYear(year int) bool {
	return year >= e.StartYear && (e.EndYear == nil || year <= *e.EndYear)
}

// Event is a calendar entry on a specific date, optionally linked to an entity.
// Description stores ProseMirror JSON for rich text editing; DescriptionHTML
// stores pre-rendered sanitized HTML for display (same pattern as entity entries).
type Event struct {
	ID              string    `json:"id"`
	CalendarID      string    `json:"calendar_id"`
	EntityID        *string   `json:"entity_id,omitempty"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	DescriptionHTML *string   `json:"description_html,omitempty"`
	Year            int       `json:"year"`
	Month          int       `json:"month"`
	Day            int       `json:"day"`
	StartHour      *int      `json:"start_hour,omitempty"`
	StartMinute    *int      `json:"start_minute,omitempty"`
	EndYear        *int      `json:"end_year,omitempty"`
	EndMonth       *int      `json:"end_month,omitempty"`
	EndDay         *int      `json:"end_day,omitempty"`
	EndHour        *int      `json:"end_hour,omitempty"`
	EndMinute      *int      `json:"end_minute,omitempty"`
	IsRecurring              bool    `json:"is_recurring"`
	RecurrenceType           *string `json:"recurrence_type,omitempty"`
	RecurrenceInterval       *int    `json:"recurrence_interval,omitempty"`
	RecurrenceEndYear        *int    `json:"recurrence_end_year,omitempty"`
	RecurrenceEndMonth       *int    `json:"recurrence_end_month,omitempty"`
	RecurrenceEndDay         *int    `json:"recurrence_end_day,omitempty"`
	RecurrenceMaxOccurrences *int    `json:"recurrence_max_occurrences,omitempty"`
	RecurrenceDayOfWeek      *int    `json:"recurrence_day_of_week,omitempty"`
	Visibility               string  `json:"visibility"`
	VisibilityRules          *string `json:"visibility_rules,omitempty"`
	Category                 *string `json:"category,omitempty"`
	// Tier references one of the campaign's event_tier_definitions
	// by slug (PR #358 migration 004 added the calendar_events.tier
	// column; Wave 1.6 closes the Go-side plumbing per PR #368
	// stop-and-flag #2). Nil means "use platform default" —
	// render-time fallback per V2 design §C1 three-tier model.
	Tier                     *string `json:"tier,omitempty"`
	Color                    *string `json:"color,omitempty"`
	Icon                     *string `json:"icon,omitempty"`
	AllDay                   bool    `json:"all_day"`
	CreatedBy                *string `json:"created_by,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`

	// Joined fields for display (populated by some queries).
	EntityName  string `json:"entity_name,omitempty"`
	EntityIcon  string `json:"entity_icon,omitempty"`
	EntityColor string `json:"entity_color,omitempty"`
}

// HasTime returns true if this event has a specific start time (not all-day).
func (e *Event) HasTime() bool {
	return e.StartHour != nil && e.StartMinute != nil
}

// FormatTime returns the event's start time as "HH:MM", or empty for all-day events.
func (e *Event) FormatTime() string {
	if !e.HasTime() {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", *e.StartHour, *e.StartMinute)
}

// FormatEndTime returns the event's end time as "HH:MM", or empty if not set.
func (e *Event) FormatEndTime() string {
	if e.EndHour == nil || e.EndMinute == nil {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", *e.EndHour, *e.EndMinute)
}

// FormatTimeRange returns "HH:MM - HH:MM" or just "HH:MM" if no end time.
func (e *Event) FormatTimeRange() string {
	start := e.FormatTime()
	if start == "" {
		return ""
	}
	end := e.FormatEndTime()
	if end == "" {
		return start
	}
	return start + " – " + end
}

// IsMultiDay returns true if this event spans more than one day.
func (e *Event) IsMultiDay() bool {
	return e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil
}

// ParseVisibilityRules parses the JSON visibility rules into a VisibilityRules struct.
// Returns nil if no rules are set.
func (e *Event) ParseVisibilityRules() *VisibilityRules {
	if e.VisibilityRules == nil || *e.VisibilityRules == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*e.VisibilityRules), &rules); err != nil {
		return nil
	}
	return &rules
}

// HasRichText returns true if this event has a rich text description (ProseMirror JSON
// with pre-rendered HTML), as opposed to a legacy plain text description.
func (e *Event) HasRichText() bool {
	return e.DescriptionHTML != nil && *e.DescriptionHTML != ""
}

// PlainDescription returns a plain text version of the description for tooltips.
// For rich text events, returns empty (tooltip should not show raw JSON).
// For legacy plain text events, returns the description as-is.
func (e *Event) PlainDescription() string {
	if e.Description == nil || *e.Description == "" {
		return ""
	}
	// If there's no HTML version, description is plain text (legacy).
	if !e.HasRichText() {
		return *e.Description
	}
	// Rich text event: description is ProseMirror JSON, not displayable as text.
	return ""
}

// --- Request DTOs ---

// CreateCalendarInput is the validated input for creating a calendar.
type CreateCalendarInput struct {
	Mode             string // "fantasy" or "reallife"
	Name             string
	Description      *string
	EpochName        *string
	CurrentYear      int
	HoursPerDay      int
	MinutesPerHour   int
	SecondsPerMinute int
	LeapYearEvery    int
	LeapYearOffset   int
}

// UpdateCalendarInput is the validated input for updating calendar settings.
// Mode is optional: empty means "leave unchanged"; non-empty must be one of
// the calendar Mode constants (ModeFantasy / ModeRealLife).
type UpdateCalendarInput struct {
	Name             string
	Description      *string
	EpochName        *string
	Mode             string
	CurrentYear      int
	CurrentMonth     int
	CurrentDay       int
	CurrentHour      int
	CurrentMinute    int
	HoursPerDay      int
	MinutesPerHour   int
	SecondsPerMinute int
	LeapYearEvery    int
	LeapYearOffset   int
}

// CreateEventInput is the validated input for creating a calendar event.
type CreateEventInput struct {
	Name                     string
	Description              *string
	DescriptionHTML          *string
	EntityID                 *string
	Year                     int
	Month                    int
	Day                      int
	StartHour                *int
	StartMinute              *int
	EndYear                  *int
	EndMonth                 *int
	EndDay                   *int
	EndHour                  *int
	EndMinute                *int
	IsRecurring              bool
	RecurrenceType           *string
	RecurrenceInterval       *int
	RecurrenceEndYear        *int
	RecurrenceEndMonth       *int
	RecurrenceEndDay         *int
	RecurrenceMaxOccurrences *int
	Visibility               string
	VisibilityRules          *string
	Category                 *string
	// Tier — campaign tier definition slug; nil = use platform default
	// at render. PR #358 schema column + Wave 1.6 Go-side plumbing.
	Tier                     *string
	Color                    *string
	Icon                     *string
	AllDay                   bool
	CreatedBy                string
}

// UpdateEventInput is the validated input for updating an event.
type UpdateEventInput struct {
	Name                     string
	Description              *string
	DescriptionHTML          *string
	EntityID                 *string
	Year                     int
	Month                    int
	Day                      int
	StartHour                *int
	StartMinute              *int
	EndYear                  *int
	EndMonth                 *int
	EndDay                   *int
	EndHour                  *int
	EndMinute                *int
	IsRecurring              bool
	RecurrenceType           *string
	RecurrenceInterval       *int
	RecurrenceEndYear        *int
	RecurrenceEndMonth       *int
	RecurrenceEndDay         *int
	RecurrenceMaxOccurrences *int
	Visibility               string
	VisibilityRules          *string
	Category                 *string
	// Tier — see CreateEventInput.Tier doc.
	Tier                     *string
	Color                    *string
	Icon                     *string
	AllDay                   bool
}

// MonthInput is the input for creating/updating a month.
type MonthInput struct {
	Name          string `json:"name"`
	Days          int    `json:"days"`
	SortOrder     int    `json:"sort_order"`
	IsIntercalary bool   `json:"is_intercalary"`
	LeapYearDays  int    `json:"leap_year_days"`
}

// WeekdayInput is the input for creating/updating a weekday.
type WeekdayInput struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	IsRestDay bool   `json:"is_rest_day"`
}

// MoonInput is the input for creating/updating a moon.
type MoonInput struct {
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
}

// EraInput is the input for creating/updating an era.
type EraInput struct {
	Name        string  `json:"name"`
	StartYear   int     `json:"start_year"`
	EndYear     *int    `json:"end_year"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
	SortOrder   int     `json:"sort_order"`
}

// EventCategory is a campaign-defined event category for calendar events.
// Categories have a slug (stored on events), display name, emoji icon, and color.
type EventCategory struct {
	ID         int    `json:"id"`
	CalendarID string `json:"calendar_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
	SortOrder  int    `json:"sort_order"`
}

// EventCategoryInput is the input for creating/updating an event category.
type EventCategoryInput struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// Weather represents the current weather state for a calendar.
// Set manually by the GM or synced from external tools (Calendaria).
type Weather struct {
	ID                     int      `json:"id"`
	CalendarID             string   `json:"calendar_id"`
	PresetID               *string  `json:"preset_id,omitempty"`
	PresetLabel            *string  `json:"preset_label,omitempty"`
	Icon                   *string  `json:"icon,omitempty"`
	Color                  *string  `json:"color,omitempty"`
	TemperatureCelsius     *float64 `json:"temperature_celsius,omitempty"`
	Wind                   *Wind    `json:"wind,omitempty"`
	Precipitation          *Precipitation `json:"precipitation,omitempty"`
	ZoneID                 *string  `json:"zone_id,omitempty"`
	ZoneName               *string  `json:"zone_name,omitempty"`
	Description            *string  `json:"description,omitempty"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// Wind describes wind speed and direction.
type Wind struct {
	SpeedKPH         *float64 `json:"speed_kph,omitempty"`
	SpeedTier        *string  `json:"speed_tier,omitempty"`
	Direction        *string  `json:"direction,omitempty"`
	DirectionDegrees *int     `json:"direction_degrees,omitempty"`
}

// Precipitation describes the type and intensity of precipitation.
type Precipitation struct {
	Type      *string  `json:"type,omitempty"`
	Intensity *float64 `json:"intensity,omitempty"`
}

// WeatherInput is the input for setting weather state.
type WeatherInput struct {
	PresetID           *string  `json:"preset_id"`
	PresetLabel        *string  `json:"preset_label"`
	Icon               *string  `json:"icon"`
	Color              *string  `json:"color"`
	TemperatureCelsius *float64 `json:"temperature_celsius"`
	WindSpeedKPH       *float64 `json:"wind_speed_kph"`
	WindSpeedTier      *string  `json:"wind_speed_tier"`
	WindDirection      *string  `json:"wind_direction"`
	WindDirectionDeg   *int     `json:"wind_direction_degrees"`
	PrecipitationType  *string  `json:"precipitation_type"`
	PrecipitationIntensity *float64 `json:"precipitation_intensity"`
	ZoneID             *string  `json:"zone_id"`
	ZoneName           *string  `json:"zone_name"`
	Description        *string  `json:"description"`
}

// WeatherZone is a per-calendar climate region definition (e.g.
// "temperate", "tropical", "arctic"). The zone's payload carries the
// active presets + per-season overrides as opaque JSON — the structural
// shape is owned by Calendaria/Foundry sync (presets array,
// season_overrides map) and Chronicle stores it verbatim with a small
// validation helper (see service.go validateWeatherZonePayload).
//
// Zones are calendar-scoped (V2 multi-cal): each calendar has its own
// zones. The active-zone reference lives on calendar_weather.zone_id
// (migration 003) + zone_name; the zone DEFINITIONS live in this
// table (migration 005). Per C-CAL-WEATHER-ZONES dispatch +
// cordinator/reports/chronicle/2026-05-28-c-cal-weather-zones.md
// §"Scope decision."
type WeatherZone struct {
	CalendarID string                 `json:"calendar_id"`
	ZoneID     string                 `json:"zone_id"`
	Name       string                 `json:"name"`
	Payload    map[string]any         `json:"payload"`
	CreatedAt  time.Time              `json:"created_at,omitempty"`
	UpdatedAt  time.Time              `json:"updated_at,omitempty"`
}

// WeatherZonesState bundles the per-calendar active-zone reference +
// the full zone definitions list — the canonical GET response from
// /api/v1/campaigns/:cid/calendar/weather/zones. ActiveZone is "" when
// no zone is currently active; Zones may be empty.
type WeatherZonesState struct {
	ActiveZone string        `json:"active_zone"`
	Zones      []WeatherZone `json:"zones"`
}

// Cycle is a periodic named cycle (zodiac, elemental, seasonal, etc.).
// Entries rotate based on cycle_length (number of years per full rotation).
type Cycle struct {
	ID          int          `json:"id"`
	CalendarID  string       `json:"calendar_id"`
	Name        string       `json:"name"`
	CycleLength int          `json:"cycle_length"`
	Type        string       `json:"type"` // "yearly", "monthly", etc.
	SortOrder   int          `json:"sort_order"`
	Entries     []CycleEntry `json:"entries,omitempty"`
}

// CycleEntry is a single entry within a cycle.
type CycleEntry struct {
	ID         int    `json:"id"`
	CycleID    int    `json:"cycle_id"`
	Name       string `json:"name"`
	Icon       *string `json:"icon,omitempty"`
	YearOffset int    `json:"year_offset"`
	SortOrder  int    `json:"sort_order"`
}

// CycleInput is the input for creating/updating a cycle with its entries.
type CycleInput struct {
	Name        string           `json:"name"`
	CycleLength int              `json:"cycle_length"`
	Type        string           `json:"type"`
	SortOrder   int              `json:"sort_order"`
	Entries     []CycleEntryInput `json:"entries"`
}

// CycleEntryInput is the input for a single cycle entry.
type CycleEntryInput struct {
	Name       string  `json:"name"`
	Icon       *string `json:"icon"`
	YearOffset int     `json:"year_offset"`
	SortOrder  int     `json:"sort_order"`
}

// Festival is a fixed calendar entry (holiday) that is part of the calendar
// structure rather than a recurring event. month+day specifies the date;
// after_month is used for intercalary festivals that fall between months.
type Festival struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	Month       *int    `json:"month,omitempty"`
	Day         *int    `json:"day,omitempty"`
	AfterMonth  *int    `json:"after_month,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	SortOrder   int     `json:"sort_order"`
}

// FestivalInput is the input for creating/updating a festival.
type FestivalInput struct {
	Name        string  `json:"name"`
	Month       *int    `json:"month"`
	Day         *int    `json:"day"`
	AfterMonth  *int    `json:"after_month"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Icon        *string `json:"icon"`
	SortOrder   int     `json:"sort_order"`
}

// DefaultEventCategories returns the default set of event categories seeded
// for new calendars. Provides a sensible starting point for TTRPG campaigns.
func DefaultEventCategories() []EventCategoryInput {
	return []EventCategoryInput{
		{Slug: "holiday", Name: "Holiday", Icon: "⭐", Color: "#f59e0b", SortOrder: 0},
		{Slug: "battle", Name: "Battle", Icon: "⚔", Color: "#ef4444", SortOrder: 1},
		{Slug: "quest", Name: "Quest", Icon: "❗", Color: "#8b5cf6", SortOrder: 2},
		{Slug: "birthday", Name: "Birthday", Icon: "🎂", Color: "#ec4899", SortOrder: 3},
		{Slug: "festival", Name: "Festival", Icon: "🎉", Color: "#10b981", SortOrder: 4},
		{Slug: "travel", Name: "Travel", Icon: "🚶", Color: "#3b82f6", SortOrder: 5},
	}
}

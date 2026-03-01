// Package calendar provides a custom fantasy calendar system for campaigns.
// Supports non-Gregorian months, named weekdays, moons with phase tracking,
// named seasons, and events linked to entities. Each campaign has at most
// one calendar; the addon must be enabled per-campaign.
package calendar

import (
	"fmt"
	"time"
)

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
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Eager-loaded sub-resources (populated by service, not by every query).
	Months   []Month   `json:"months,omitempty"`
	Weekdays []Weekday `json:"weekdays,omitempty"`
	Moons    []Moon    `json:"moons,omitempty"`
	Seasons  []Season  `json:"seasons,omitempty"`
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
}

// Moon is a celestial body with a phase cycle used for moon phase display.
type Moon struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
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

// Season is a named period spanning a range of month+day to month+day.
type Season struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	StartMonth  int     `json:"start_month"`
	StartDay    int     `json:"start_day"`
	EndMonth    int     `json:"end_month"`
	EndDay      int     `json:"end_day"`
	Description *string `json:"description,omitempty"`
	Color       string  `json:"color"`
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

// Event is a calendar entry on a specific date, optionally linked to an entity.
type Event struct {
	ID             string    `json:"id"`
	CalendarID     string    `json:"calendar_id"`
	EntityID       *string   `json:"entity_id,omitempty"`
	Name           string    `json:"name"`
	Description    *string   `json:"description,omitempty"`
	Year           int       `json:"year"`
	Month          int       `json:"month"`
	Day            int       `json:"day"`
	StartHour      *int      `json:"start_hour,omitempty"`
	StartMinute    *int      `json:"start_minute,omitempty"`
	EndYear        *int      `json:"end_year,omitempty"`
	EndMonth       *int      `json:"end_month,omitempty"`
	EndDay         *int      `json:"end_day,omitempty"`
	EndHour        *int      `json:"end_hour,omitempty"`
	EndMinute      *int      `json:"end_minute,omitempty"`
	IsRecurring    bool      `json:"is_recurring"`
	RecurrenceType *string   `json:"recurrence_type,omitempty"`
	Visibility     string    `json:"visibility"`
	Category       *string   `json:"category,omitempty"`
	CreatedBy      *string   `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

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
type UpdateCalendarInput struct {
	Name             string
	Description      *string
	EpochName        *string
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
	Name           string
	Description    *string
	EntityID       *string
	Year           int
	Month          int
	Day            int
	StartHour      *int
	StartMinute    *int
	EndYear        *int
	EndMonth       *int
	EndDay         *int
	EndHour        *int
	EndMinute      *int
	IsRecurring    bool
	RecurrenceType *string
	Visibility     string
	Category       *string
	CreatedBy      string
}

// UpdateEventInput is the validated input for updating an event.
type UpdateEventInput struct {
	Name           string
	Description    *string
	EntityID       *string
	Year           int
	Month          int
	Day            int
	StartHour      *int
	StartMinute    *int
	EndYear        *int
	EndMonth       *int
	EndDay         *int
	EndHour        *int
	EndMinute      *int
	IsRecurring    bool
	RecurrenceType *string
	Visibility     string
	Category       *string
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
}

// MoonInput is the input for creating/updating a moon.
type MoonInput struct {
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
}

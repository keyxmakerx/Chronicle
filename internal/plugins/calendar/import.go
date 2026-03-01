// Package calendar — import.go provides calendar import from three formats:
// Chronicle native JSON, Simple Calendar (Foundry VTT), and Calendaria (Foundry VTT).
//
// # Supported Formats
//
// ## Chronicle (chronicle-calendar-v1)
// Native format exported by Chronicle. Round-trips perfectly.
//
// ## Simple Calendar (Foundry VTT)
// The most popular Foundry VTT calendar module. Identified by top-level
// "calendar" key containing "months", "weekdays", "time", "leapYear", etc.
// Months use numberOfDays/numberOfLeapYearDays. Time uses hoursInDay/minutesInHour.
// Seasons have startingMonth/startingDay. Moons have cycleLength/cycleDayAdjust.
//
// ## Calendaria (Foundry VTT)
// A newer Foundry VTT calendar module. Identified by top-level "months" as an
// object (not array) with keyed entries, or by presence of "days.hoursPerDay".
// Months use days/leapDays. Seasons use dayStart/dayEnd (day-of-year numbers).
// Moons have cycleLength/referenceDate. Supports eras and festivals natively.
package calendar

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// ImportFormat identifies which JSON format was detected.
type ImportFormat string

const (
	FormatChronicle    ImportFormat = "chronicle"
	FormatSimpleCal    ImportFormat = "simple-calendar"
	FormatCalendaria   ImportFormat = "calendaria"
	FormatFantasyCal   ImportFormat = "fantasy-calendar"
	FormatUnknown      ImportFormat = "unknown"
)

// ImportResult holds the parsed calendar data ready to be applied.
type ImportResult struct {
	Format       ImportFormat     `json:"format"`
	CalendarName string           `json:"calendar_name"`
	Months       []MonthInput     `json:"months"`
	Weekdays     []WeekdayInput   `json:"weekdays"`
	Moons        []MoonInput      `json:"moons"`
	Seasons      []Season         `json:"seasons"`
	Eras         []EraInput       `json:"eras"`
	Settings     ImportedSettings `json:"settings"`
}

// ImportedSettings holds calendar-level settings extracted from the import.
type ImportedSettings struct {
	EpochName        *string `json:"epoch_name,omitempty"`
	CurrentYear      int     `json:"current_year"`
	HoursPerDay      int     `json:"hours_per_day"`
	MinutesPerHour   int     `json:"minutes_per_hour"`
	SecondsPerMinute int     `json:"seconds_per_minute"`
	LeapYearEvery    int     `json:"leap_year_every"`
	LeapYearOffset   int     `json:"leap_year_offset"`
}

// DetectAndParse auto-detects the format of raw JSON bytes and parses into
// an ImportResult. Returns an error if the format cannot be detected or parsed.
func DetectAndParse(data []byte) (*ImportResult, error) {
	format := detectFormat(data)
	switch format {
	case FormatChronicle:
		return parseChronicle(data)
	case FormatSimpleCal:
		return parseSimpleCalendar(data)
	case FormatCalendaria:
		return parseCalendaria(data)
	case FormatFantasyCal:
		return parseFantasyCalendar(data)
	default:
		return nil, fmt.Errorf("unrecognized calendar format: could not detect Chronicle, Simple Calendar, Calendaria, or Fantasy-Calendar JSON")
	}
}

// detectFormat inspects the raw JSON to determine which calendar format it is.
func detectFormat(data []byte) ImportFormat {
	// Try to unmarshal as a generic map to inspect top-level keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return FormatUnknown
	}

	// Chronicle native: has "format" key with value "chronicle-calendar-v1".
	if formatVal, ok := raw["format"]; ok {
		var f string
		if json.Unmarshal(formatVal, &f) == nil && f == "chronicle-calendar-v1" {
			return FormatChronicle
		}
	}

	// Simple Calendar v1: has top-level "calendar" key containing sub-objects.
	if _, ok := raw["calendar"]; ok {
		return FormatSimpleCal
	}

	// Simple Calendar v2 export: has "exportVersion" and "calendars" array.
	if _, ok := raw["exportVersion"]; ok {
		if _, hasCalendars := raw["calendars"]; hasCalendars {
			return FormatSimpleCal
		}
	}

	// Fantasy-Calendar.com: has "static_data" and "dynamic_data" top-level keys.
	if _, hasStatic := raw["static_data"]; hasStatic {
		if _, hasDynamic := raw["dynamic_data"]; hasDynamic {
			return FormatFantasyCal
		}
	}

	// Calendaria: has "days" key with "hoursPerDay" inside, or "months" as
	// an object with named keys (not an array).
	if daysRaw, ok := raw["days"]; ok {
		var daysObj map[string]json.RawMessage
		if json.Unmarshal(daysRaw, &daysObj) == nil {
			if _, hasHPD := daysObj["hoursPerDay"]; hasHPD {
				return FormatCalendaria
			}
		}
	}
	// Also check for Calendaria by "months" being an object (not array).
	if monthsRaw, ok := raw["months"]; ok {
		trimmed := strings.TrimSpace(string(monthsRaw))
		if len(trimmed) > 0 && trimmed[0] == '{' {
			return FormatCalendaria
		}
	}

	return FormatUnknown
}

// --- Chronicle Native Parser ---

// parseChronicle parses Chronicle's own export format.
func parseChronicle(data []byte) (*ImportResult, error) {
	var export ChronicleExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("parse chronicle JSON: %w", err)
	}

	result := &ImportResult{
		Format:       FormatChronicle,
		CalendarName: export.Calendar.Name,
		Settings: ImportedSettings{
			EpochName:        export.Calendar.EpochName,
			CurrentYear:      export.Calendar.CurrentYear,
			HoursPerDay:      export.Calendar.HoursPerDay,
			MinutesPerHour:   export.Calendar.MinutesPerHour,
			SecondsPerMinute: export.Calendar.SecondsPerMinute,
			LeapYearEvery:    export.Calendar.LeapYearEvery,
			LeapYearOffset:   export.Calendar.LeapYearOffset,
		},
	}

	// Copy months.
	for _, m := range export.Calendar.Months {
		result.Months = append(result.Months, MonthInput{
			Name:          m.Name,
			Days:          m.Days,
			SortOrder:     m.SortOrder,
			IsIntercalary: m.IsIntercalary,
			LeapYearDays:  m.LeapYearDays,
		})
	}

	// Copy weekdays.
	for _, w := range export.Calendar.Weekdays {
		result.Weekdays = append(result.Weekdays, WeekdayInput{
			Name:      w.Name,
			SortOrder: w.SortOrder,
		})
	}

	// Copy moons.
	for _, m := range export.Calendar.Moons {
		result.Moons = append(result.Moons, MoonInput{
			Name:        m.Name,
			CycleDays:   m.CycleDays,
			PhaseOffset: m.PhaseOffset,
			Color:       m.Color,
		})
	}

	// Copy seasons.
	for _, s := range export.Calendar.Seasons {
		result.Seasons = append(result.Seasons, Season{
			Name:          s.Name,
			StartMonth:    s.StartMonth,
			StartDay:      s.StartDay,
			EndMonth:      s.EndMonth,
			EndDay:        s.EndDay,
			Description:   s.Description,
			Color:         s.Color,
			WeatherEffect: s.WeatherEffect,
		})
	}

	// Copy eras.
	for _, e := range export.Calendar.Eras {
		result.Eras = append(result.Eras, EraInput{
			Name:        e.Name,
			StartYear:   e.StartYear,
			EndYear:     e.EndYear,
			Description: e.Description,
			Color:       e.Color,
			SortOrder:   e.SortOrder,
		})
	}

	return result, nil
}

// --- Simple Calendar Parser ---

// scData is the top-level Simple Calendar export structure.
type scData struct {
	Calendar scCalendar `json:"calendar"`
}

// scCalendar holds the Simple Calendar configuration. Supports both v2 field names
// and v1 legacy aliases (yearSettings, monthSettings, etc.) via custom UnmarshalJSON.
type scCalendar struct {
	Name           string           `json:"name"`
	CurrentDate    scCurrentDate    `json:"currentDate"`
	General        scGeneral        `json:"general"`
	LeapYear       scLeapYear       `json:"leapYear"`
	Months         []scMonth        `json:"months"`
	Moons          []scMoon         `json:"moons"`
	NoteCategories []scNoteCategory `json:"noteCategories"`
	Seasons        []scSeason       `json:"seasons"`
	Time           scTime           `json:"time"`
	Weekdays       []scWeekday      `json:"weekdays"`
	Year           scYear           `json:"year"`
}

// UnmarshalJSON handles Simple Calendar v1 legacy field names as aliases.
func (c *scCalendar) UnmarshalJSON(data []byte) error {
	// Alias type to avoid infinite recursion.
	type Alias scCalendar
	var v2 Alias
	if err := json.Unmarshal(data, &v2); err != nil {
		return err
	}
	*c = scCalendar(v2)

	// If v2 fields are empty, try v1 aliases.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if len(c.Months) == 0 {
		if v, ok := raw["monthSettings"]; ok {
			json.Unmarshal(v, &c.Months)
		}
	}
	if len(c.Weekdays) == 0 {
		if v, ok := raw["weekdaySettings"]; ok {
			json.Unmarshal(v, &c.Weekdays)
		}
	}
	if len(c.Seasons) == 0 {
		if v, ok := raw["seasonSettings"]; ok {
			json.Unmarshal(v, &c.Seasons)
		}
	}
	if len(c.Moons) == 0 {
		if v, ok := raw["moonSettings"]; ok {
			json.Unmarshal(v, &c.Moons)
		}
	}
	if c.Year.NumericRepresentation == 0 {
		if v, ok := raw["yearSettings"]; ok {
			json.Unmarshal(v, &c.Year)
		}
	}
	if c.Time.HoursInDay == 0 {
		if v, ok := raw["timeSettings"]; ok {
			json.Unmarshal(v, &c.Time)
		}
	}
	if c.LeapYear.Rule == "" {
		if v, ok := raw["leapYearSettings"]; ok {
			json.Unmarshal(v, &c.LeapYear)
		}
	}
	return nil
}

type scCurrentDate struct {
	Year    int `json:"year"`
	Month   int `json:"month"`   // 0-indexed
	Day     int `json:"day"`     // 0-indexed
	Seconds int `json:"seconds"` // seconds since midnight
}

type scGeneral struct {
	GameWorldTimeIntegration string `json:"gameWorldTimeIntegration"`
}

type scLeapYear struct {
	Rule      string `json:"rule"`      // "none", "gregorian", "custom"
	CustomMod int    `json:"customMod"` // interval for custom rule
}

type scMonth struct {
	Name                         string `json:"name"`
	Abbreviation                 string `json:"abbreviation"`
	NumericRepresentation        int    `json:"numericRepresentation"`
	NumericRepresentationOffset  int    `json:"numericRepresentationOffset"`
	NumberOfDays                 int    `json:"numberOfDays"`
	NumberOfLeapYearDays         int    `json:"numberOfLeapYearDays"`
	Intercalary                  bool   `json:"intercalary"`
	IntercalaryInclude           bool   `json:"intercalaryInclude"`
	StartingWeekday              *int   `json:"startingWeekday"`
	Description                  string `json:"description"`
}

type scWeekday struct {
	Name                  string `json:"name"`
	Abbreviation          string `json:"abbreviation"`
	NumericRepresentation int    `json:"numericRepresentation"`
	Restday               bool   `json:"restday"`
	Description           string `json:"description"`
}

type scSeason struct {
	Name          string `json:"name"`
	StartingMonth int    `json:"startingMonth"` // 0-indexed month
	StartingDay   int    `json:"startingDay"`   // 0-indexed day
	Color         string `json:"color"`
	Icon          string `json:"icon"`
	SunriseTime   int    `json:"sunriseTime"` // seconds since midnight
	SunsetTime    int    `json:"sunsetTime"`  // seconds since midnight
	Description   string `json:"description"`
}

type scMoon struct {
	Name           string         `json:"name"`
	CycleLength    float64        `json:"cycleLength"`
	CycleDayAdjust float64        `json:"cycleDayAdjust"`
	FirstNewMoon   scFirstNewMoon `json:"firstNewMoon"`
	Color          string         `json:"color"`
}

type scFirstNewMoon struct {
	Year      int    `json:"year"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	YearReset string `json:"yearReset"`
	YearX     int    `json:"yearX"`
}

type scTime struct {
	HoursInDay      int `json:"hoursInDay"`
	MinutesInHour   int `json:"minutesInHour"`
	SecondsInMinute int `json:"secondsInMinute"`
	GameTimeRatio   int `json:"gameTimeRatio"`
}

type scYear struct {
	NumericRepresentation int      `json:"numericRepresentation"`
	Prefix                string   `json:"prefix"`
	Postfix               string   `json:"postfix"`
	YearZero              int      `json:"yearZero"`
	FirstWeekday          int      `json:"firstWeekday"`
	YearNames             []string `json:"yearNames"`
	YearNamingRule        string   `json:"yearNamingRule"`
}

type scNoteCategory struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// parseSimpleCalendar converts a Simple Calendar JSON export into an ImportResult.
// Handles both v1 format (top-level "calendar" key) and v2 format ("calendars" array).
func parseSimpleCalendar(data []byte) (*ImportResult, error) {
	// Try v2 format first (has "calendars" array).
	var v2 struct {
		ExportVersion int          `json:"exportVersion"`
		Calendars     []scCalendar `json:"calendars"`
	}
	if err := json.Unmarshal(data, &v2); err == nil && len(v2.Calendars) > 0 {
		return parseSimpleCalendarInner(v2.Calendars[0])
	}

	// Fall back to v1 format (single "calendar" key).
	var sc scData
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse simple calendar JSON: %w", err)
	}

	return parseSimpleCalendarInner(sc.Calendar)
}

// parseSimpleCalendarInner does the actual conversion from a Simple Calendar
// configuration object to an ImportResult.
func parseSimpleCalendarInner(cal scCalendar) (*ImportResult, error) {
	result := &ImportResult{
		Format:       FormatSimpleCal,
		CalendarName: "Imported Calendar",
	}

	// Settings.
	result.Settings = ImportedSettings{
		CurrentYear:      cal.Year.NumericRepresentation,
		HoursPerDay:      cal.Time.HoursInDay,
		MinutesPerHour:   cal.Time.MinutesInHour,
		SecondsPerMinute: cal.Time.SecondsInMinute,
	}
	if result.Settings.HoursPerDay <= 0 {
		result.Settings.HoursPerDay = 24
	}
	if result.Settings.MinutesPerHour <= 0 {
		result.Settings.MinutesPerHour = 60
	}
	if result.Settings.SecondsPerMinute <= 0 {
		result.Settings.SecondsPerMinute = 60
	}

	// Epoch from year prefix/postfix.
	if cal.Year.Postfix != "" {
		ep := strings.TrimSpace(cal.Year.Postfix)
		result.Settings.EpochName = &ep
	} else if cal.Year.Prefix != "" {
		ep := strings.TrimSpace(cal.Year.Prefix)
		result.Settings.EpochName = &ep
	}

	// Leap year.
	switch cal.LeapYear.Rule {
	case "gregorian":
		result.Settings.LeapYearEvery = 4
	case "custom":
		if cal.LeapYear.CustomMod > 0 {
			result.Settings.LeapYearEvery = cal.LeapYear.CustomMod
		}
	}

	// Months — Simple Calendar uses 0-indexed arrays, sorted by numericRepresentation.
	for i, m := range cal.Months {
		leapExtra := 0
		if m.NumberOfLeapYearDays > m.NumberOfDays {
			leapExtra = m.NumberOfLeapYearDays - m.NumberOfDays
		}
		result.Months = append(result.Months, MonthInput{
			Name:          stripLocalizationKey(m.Name),
			Days:          m.NumberOfDays,
			SortOrder:     i,
			IsIntercalary: m.Intercalary,
			LeapYearDays:  leapExtra,
		})
	}

	// Weekdays.
	for i, w := range cal.Weekdays {
		result.Weekdays = append(result.Weekdays, WeekdayInput{
			Name:      stripLocalizationKey(w.Name),
			SortOrder: i,
		})
	}

	// Moons — cycleLength maps to CycleDays, cycleDayAdjust to PhaseOffset.
	for _, m := range cal.Moons {
		result.Moons = append(result.Moons, MoonInput{
			Name:        stripLocalizationKey(m.Name),
			CycleDays:   m.CycleLength,
			PhaseOffset: m.CycleDayAdjust,
			Color:       normalizeColor(m.Color),
		})
	}

	// Seasons — Simple Calendar uses 0-indexed month/day; Chronicle uses 1-indexed.
	// We need to compute end dates since SC only has start dates.
	for i, s := range cal.Seasons {
		startMonth := s.StartingMonth + 1 // convert 0-indexed to 1-indexed
		startDay := s.StartingDay + 1     // convert 0-indexed to 1-indexed

		// End date is the day before the next season's start.
		var endMonth, endDay int
		if i+1 < len(cal.Seasons) {
			next := cal.Seasons[i+1]
			endMonth, endDay = dayBefore(next.StartingMonth+1, next.StartingDay+1, cal.Months)
		} else {
			// Last season wraps to day before first season.
			first := cal.Seasons[0]
			endMonth, endDay = dayBefore(first.StartingMonth+1, first.StartingDay+1, cal.Months)
		}

		result.Seasons = append(result.Seasons, Season{
			Name:       stripLocalizationKey(s.Name),
			StartMonth: startMonth,
			StartDay:   startDay,
			EndMonth:   endMonth,
			EndDay:     endDay,
			Color:      normalizeColor(s.Color),
		})
	}

	return result, nil
}

// dayBefore returns the month+day that is one day before the given month+day.
// Uses the Simple Calendar months list for day counts. Both params are 1-indexed.
func dayBefore(month, day int, scMonths []scMonth) (int, int) {
	if day > 1 {
		return month, day - 1
	}
	// First day of month — go to last day of previous month.
	prevMonth := month - 1
	if prevMonth < 1 {
		prevMonth = len(scMonths)
	}
	prevDays := 30 // fallback
	if prevMonth-1 >= 0 && prevMonth-1 < len(scMonths) {
		prevDays = scMonths[prevMonth-1].NumberOfDays
	}
	return prevMonth, prevDays
}

// --- Calendaria Parser ---

// calData is the top-level Calendaria JSON structure. Calendaria uses object
// maps with named keys for months, weekdays, etc. rather than arrays.
// Some fields may be nested under a "values" sub-key.
type calData struct {
	ID             string                     `json:"id"`
	Name           string                     `json:"name"`
	Years          calYears                   `json:"years"`
	LeapYearConfig calLeapYear                `json:"leapYearConfig"`
	Months         map[string]calMonth        `json:"-"` // custom unmarshal
	Days           calDays                    `json:"days"`
	Seasons        map[string]calSeason       `json:"-"` // custom unmarshal
	Eras           map[string]calEra          `json:"-"` // custom unmarshal
	Moons          map[string]calMoon         `json:"-"` // custom unmarshal
	Festivals      map[string]calFestival     `json:"-"` // custom unmarshal
	Weeks          map[string]calWeek         `json:"weeks"`
	Metadata       map[string]json.RawMessage `json:"metadata"`
}

// UnmarshalJSON handles Calendaria's inconsistent nesting. Some files put
// data directly in "months": {...}, others nest it under "months": {"values": {...}}.
func (d *calData) UnmarshalJSON(data []byte) error {
	// Alias to avoid infinite recursion.
	type Alias struct {
		ID             string                     `json:"id"`
		Name           string                     `json:"name"`
		Years          calYears                   `json:"years"`
		LeapYearConfig calLeapYear                `json:"leapYearConfig"`
		Days           calDays                    `json:"days"`
		Weeks          map[string]calWeek         `json:"weeks"`
		Metadata       map[string]json.RawMessage `json:"metadata"`
	}
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	d.ID = alias.ID
	d.Name = alias.Name
	d.Years = alias.Years
	d.LeapYearConfig = alias.LeapYearConfig
	d.Days = alias.Days
	d.Weeks = alias.Weeks
	d.Metadata = alias.Metadata

	// Helper to unwrap potential {values: ...} nesting.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	d.Months = unmarshalValuedMap[calMonth](raw, "months")
	d.Seasons = unmarshalValuedMap[calSeason](raw, "seasons")
	d.Eras = unmarshalValuedMap[calEra](raw, "eras")
	d.Moons = unmarshalValuedMap[calMoon](raw, "moons")
	d.Festivals = unmarshalValuedMap[calFestival](raw, "festivals")

	return nil
}

// unmarshalValuedMap tries to unmarshal a JSON field as either a direct map or
// a map nested under a "values" sub-key (Calendaria's two conventions).
func unmarshalValuedMap[T any](raw map[string]json.RawMessage, key string) map[string]T {
	fieldRaw, ok := raw[key]
	if !ok {
		return nil
	}

	// Try direct map first.
	var direct map[string]T
	if err := json.Unmarshal(fieldRaw, &direct); err == nil && len(direct) > 0 {
		return direct
	}

	// Try {values: {...}} wrapper.
	var wrapper struct {
		Values map[string]T `json:"values"`
	}
	if err := json.Unmarshal(fieldRaw, &wrapper); err == nil && len(wrapper.Values) > 0 {
		return wrapper.Values
	}

	return nil
}

type calYears struct {
	YearZero     int         `json:"yearZero"`
	FirstWeekday int         `json:"firstWeekday"`
	LeapYear     *calLeapYr2 `json:"leapYear,omitempty"`
}

type calLeapYr2 struct {
	LeapStart    int `json:"leapStart"`
	LeapInterval int `json:"leapInterval"`
}

type calLeapYear struct {
	Rule  string `json:"rule"` // "none", "gregorian", "custom"
	Start int    `json:"start"`
}

type calMonth struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Ordinal      int    `json:"ordinal"`
	Days         int    `json:"days"`
	LeapDays     int    `json:"leapDays,omitempty"` // total days in leap year (not extra)
}

type calDays struct {
	Values           map[string]calWeekday `json:"values"`
	DaysPerYear      int                   `json:"daysPerYear"`
	HoursPerDay      int                   `json:"hoursPerDay"`
	MinutesPerHour   int                   `json:"minutesPerHour"`
	SecondsPerMinute int                   `json:"secondsPerMinute"`
}

type calWeekday struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Ordinal      int    `json:"ordinal"`
	IsRestDay    bool   `json:"isRestDay"`
}

type calSeason struct {
	Name         string `json:"name"`
	Icon         string `json:"icon"`
	Color        string `json:"color"`
	SeasonalType string `json:"seasonalType"`
	DayStart     int    `json:"dayStart"` // day-of-year (1-indexed)
	DayEnd       int    `json:"dayEnd"`   // day-of-year (1-indexed)
	Abbreviation string `json:"abbreviation"`
}

type calEra struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	StartYear    int    `json:"startYear"`
	EndYear      *int   `json:"endYear"` // null = ongoing
}

type calMoon struct {
	Name          string       `json:"name"`
	CycleLength   float64      `json:"cycleLength"`
	Color         string       `json:"color"`
	ReferenceDate calRefDate   `json:"referenceDate"`
}

type calRefDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type calFestival struct {
	Name        string  `json:"name"`
	Month       int     `json:"month"`
	Day         int     `json:"day"`
	Icon        string  `json:"icon"`
	Color       string  `json:"color"`
	Description string  `json:"description"`
}

type calWeek struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Ordinal      int    `json:"ordinal"`
	IsRestDay    bool   `json:"isRestDay"`
}

// parseCalendaria converts a Calendaria JSON file into an ImportResult.
func parseCalendaria(data []byte) (*ImportResult, error) {
	var cal calData
	if err := json.Unmarshal(data, &cal); err != nil {
		return nil, fmt.Errorf("parse calendaria JSON: %w", err)
	}

	result := &ImportResult{
		Format:       FormatCalendaria,
		CalendarName: stripLocalizationKey(cal.Name),
	}
	if result.CalendarName == "" {
		result.CalendarName = "Imported Calendar"
	}

	// Settings.
	result.Settings = ImportedSettings{
		CurrentYear:      cal.Years.YearZero,
		HoursPerDay:      cal.Days.HoursPerDay,
		MinutesPerHour:   cal.Days.MinutesPerHour,
		SecondsPerMinute: cal.Days.SecondsPerMinute,
	}
	if result.Settings.HoursPerDay <= 0 {
		result.Settings.HoursPerDay = 24
	}
	if result.Settings.MinutesPerHour <= 0 {
		result.Settings.MinutesPerHour = 60
	}
	if result.Settings.SecondsPerMinute <= 0 {
		result.Settings.SecondsPerMinute = 60
	}

	// Leap year — check both locations (leapYearConfig and years.leapYear).
	switch cal.LeapYearConfig.Rule {
	case "gregorian":
		result.Settings.LeapYearEvery = 4
	case "custom":
		// Custom rules may be specified in years.leapYear.
		if cal.Years.LeapYear != nil && cal.Years.LeapYear.LeapInterval > 0 {
			result.Settings.LeapYearEvery = cal.Years.LeapYear.LeapInterval
			result.Settings.LeapYearOffset = cal.Years.LeapYear.LeapStart
		}
	}

	// Months — Calendaria uses object map; sort by ordinal.
	type monthEntry struct {
		key string
		val calMonth
	}
	var monthList []monthEntry
	for k, m := range cal.Months {
		monthList = append(monthList, monthEntry{k, m})
	}
	sort.Slice(monthList, func(i, j int) bool {
		return monthList[i].val.Ordinal < monthList[j].val.Ordinal
	})

	for i, m := range monthList {
		leapExtra := 0
		if m.val.LeapDays > m.val.Days {
			leapExtra = m.val.LeapDays - m.val.Days
		}
		result.Months = append(result.Months, MonthInput{
			Name:          stripLocalizationKey(m.val.Name),
			Days:          m.val.Days,
			SortOrder:     i,
			IsIntercalary: false, // Calendaria doesn't flag intercalary months
			LeapYearDays:  leapExtra,
		})
	}

	// Weekdays — from days.values or weeks, sort by ordinal.
	weekdaySource := cal.Days.Values
	if len(weekdaySource) == 0 {
		// Some Calendaria files use "weeks" instead of "days.values".
		for k, w := range cal.Weeks {
			weekdaySource[k] = calWeekday{
				Name:         w.Name,
				Abbreviation: w.Abbreviation,
				Ordinal:      w.Ordinal,
				IsRestDay:    w.IsRestDay,
			}
		}
	}

	type weekdayEntry struct {
		key string
		val calWeekday
	}
	var wdList []weekdayEntry
	for k, w := range weekdaySource {
		wdList = append(wdList, weekdayEntry{k, w})
	}
	sort.Slice(wdList, func(i, j int) bool {
		return wdList[i].val.Ordinal < wdList[j].val.Ordinal
	})

	for i, w := range wdList {
		result.Weekdays = append(result.Weekdays, WeekdayInput{
			Name:      stripLocalizationKey(w.val.Name),
			SortOrder: i,
		})
	}

	// Moons.
	for _, m := range cal.Moons {
		result.Moons = append(result.Moons, MoonInput{
			Name:        stripLocalizationKey(m.Name),
			CycleDays:   m.CycleLength,
			PhaseOffset: 0, // Calendaria uses referenceDate instead of offset
			Color:       normalizeColor(m.Color),
		})
	}

	// Seasons — Calendaria uses day-of-year ranges; convert to month+day.
	type seasonEntry struct {
		key string
		val calSeason
	}
	var seasonList []seasonEntry
	for k, s := range cal.Seasons {
		seasonList = append(seasonList, seasonEntry{k, s})
	}
	sort.Slice(seasonList, func(i, j int) bool {
		return seasonList[i].val.DayStart < seasonList[j].val.DayStart
	})

	// Build cumulative day-of-year → month+day lookup from months.
	for _, s := range seasonList {
		startMonth, startDay := dayOfYearToMonthDay(s.val.DayStart, result.Months)
		endMonth, endDay := dayOfYearToMonthDay(s.val.DayEnd, result.Months)

		result.Seasons = append(result.Seasons, Season{
			Name:       stripLocalizationKey(s.val.Name),
			StartMonth: startMonth,
			StartDay:   startDay,
			EndMonth:   endMonth,
			EndDay:     endDay,
			Color:      normalizeColor(s.val.Color),
		})
	}

	// Eras.
	type eraEntry struct {
		key string
		val calEra
	}
	var eraList []eraEntry
	for k, e := range cal.Eras {
		eraList = append(eraList, eraEntry{k, e})
	}
	sort.Slice(eraList, func(i, j int) bool {
		return eraList[i].val.StartYear < eraList[j].val.StartYear
	})

	for i, e := range eraList {
		abbr := stripLocalizationKey(e.val.Abbreviation)
		var desc *string
		if abbr != "" {
			desc = &abbr
		}
		result.Eras = append(result.Eras, EraInput{
			Name:        stripLocalizationKey(e.val.Name),
			StartYear:   e.val.StartYear,
			EndYear:     e.val.EndYear,
			Description: desc,
			Color:       "#6366f1", // default since Calendaria doesn't have era colors
			SortOrder:   i,
		})
	}

	return result, nil
}

// dayOfYearToMonthDay converts a 1-based day-of-year number to a 1-based
// month index and day-of-month, using the parsed month list.
func dayOfYearToMonthDay(dayOfYear int, months []MonthInput) (int, int) {
	if dayOfYear <= 0 {
		return 1, 1
	}
	cumulative := 0
	for _, m := range months {
		if dayOfYear <= cumulative+m.Days {
			return m.SortOrder + 1, dayOfYear - cumulative
		}
		cumulative += m.Days
	}
	// Past end of year — clamp to last day of last month.
	if len(months) > 0 {
		last := months[len(months)-1]
		return last.SortOrder + 1, last.Days
	}
	return 1, 1
}

// --- Fantasy-Calendar.com Parser ---

// fcData is the top-level Fantasy-Calendar.com export structure.
type fcData struct {
	Name        string        `json:"name"`
	StaticData  fcStaticData  `json:"static_data"`
	DynamicData fcDynamicData `json:"dynamic_data"`
}

type fcStaticData struct {
	YearData fcYearData `json:"year_data"`
	Moons    []fcMoon   `json:"moons"`
	Clock    fcClock    `json:"clock"`
	Seasons  fcSeasons  `json:"seasons"`
	Eras     []fcEra    `json:"eras"`
}

type fcYearData struct {
	FirstDay   int           `json:"first_day"`
	Overflow   bool          `json:"overflow"`
	GlobalWeek []string      `json:"global_week"`
	Timespans  []fcTimespan  `json:"timespans"`
	LeapDays   []fcLeapDay   `json:"leap_days"`
}

type fcTimespan struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "month" or "intercalary"
	Length   int    `json:"length"`
	Interval int    `json:"interval"`
	Offset   int    `json:"offset"`
}

type fcLeapDay struct {
	Name        string `json:"name"`
	Intercalary bool   `json:"intercalary"`
	Timespan    int    `json:"timespan"` // month index
	Day         int    `json:"day"`
	Interval    string `json:"interval"` // e.g. "1" or complex
}

type fcMoon struct {
	Name        string  `json:"name"`
	Cycle       float64 `json:"cycle"`
	Shift       float64 `json:"shift"`
	Granularity int     `json:"granularity"`
	Color       string  `json:"color"`
	Hidden      bool    `json:"hidden"`
}

type fcClock struct {
	Enabled bool `json:"enabled"`
	Hours   int  `json:"hours"`
	Minutes int  `json:"minutes"`
}

type fcSeasons struct {
	Data []fcSeason `json:"data"`
}

type fcSeason struct {
	Name  string     `json:"name"`
	Color [2]string  `json:"color"` // [start_color, end_color]
	Time  fcDaylight `json:"time"`
}

type fcDaylight struct {
	Sunrise fcHourMin `json:"sunrise"`
	Sunset  fcHourMin `json:"sunset"`
}

type fcHourMin struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

type fcEra struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Date        fcDate `json:"date"`
}

type fcDate struct {
	Year     int `json:"year"`
	Timespan int `json:"timespan"`
	Day      int `json:"day"`
}

type fcDynamicData struct {
	Year     int `json:"year"`
	Timespan int `json:"timespan"` // current month index
	Day      int `json:"day"`
	Hour     int `json:"hour"`
	Minute   int `json:"minute"`
}

// parseFantasyCalendar converts a Fantasy-Calendar.com JSON export into an ImportResult.
func parseFantasyCalendar(data []byte) (*ImportResult, error) {
	var fc fcData
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parse fantasy-calendar JSON: %w", err)
	}

	result := &ImportResult{
		Format:       FormatFantasyCal,
		CalendarName: fc.Name,
	}
	if result.CalendarName == "" {
		result.CalendarName = "Imported Calendar"
	}

	// Settings.
	result.Settings = ImportedSettings{
		CurrentYear:      fc.DynamicData.Year,
		HoursPerDay:      fc.StaticData.Clock.Hours,
		MinutesPerHour:   fc.StaticData.Clock.Minutes,
		SecondsPerMinute: 60, // Fantasy-Calendar doesn't track seconds
	}
	if result.Settings.HoursPerDay <= 0 {
		result.Settings.HoursPerDay = 24
	}
	if result.Settings.MinutesPerHour <= 0 {
		result.Settings.MinutesPerHour = 60
	}

	// Months — timespans array. Intercalary timespans become intercalary months.
	for i, ts := range fc.StaticData.YearData.Timespans {
		result.Months = append(result.Months, MonthInput{
			Name:          ts.Name,
			Days:          ts.Length,
			SortOrder:     i,
			IsIntercalary: ts.Type == "intercalary",
		})
	}

	// Check for leap days — add to the month they belong to.
	for _, ld := range fc.StaticData.YearData.LeapDays {
		if ld.Timespan >= 0 && ld.Timespan < len(result.Months) {
			result.Months[ld.Timespan].LeapYearDays++
		}
	}

	// Weekdays.
	for i, name := range fc.StaticData.YearData.GlobalWeek {
		result.Weekdays = append(result.Weekdays, WeekdayInput{
			Name:      name,
			SortOrder: i,
		})
	}

	// Moons.
	for _, m := range fc.StaticData.Moons {
		if m.Hidden {
			continue
		}
		result.Moons = append(result.Moons, MoonInput{
			Name:        m.Name,
			CycleDays:   m.Cycle,
			PhaseOffset: m.Shift,
			Color:       normalizeColor(m.Color),
		})
	}

	// Seasons — Fantasy-Calendar doesn't always have day ranges in the export,
	// so we distribute seasons evenly across the year if needed.
	yearDays := 0
	for _, m := range result.Months {
		yearDays += m.Days
	}

	if len(fc.StaticData.Seasons.Data) > 0 && yearDays > 0 {
		nSeasons := len(fc.StaticData.Seasons.Data)
		daysPerSeason := yearDays / nSeasons
		remainder := yearDays % nSeasons

		dayCounter := 1
		for i, s := range fc.StaticData.Seasons.Data {
			length := daysPerSeason
			if i < remainder {
				length++
			}

			startMonth, startDay := dayOfYearToMonthDay(dayCounter, result.Months)
			endMonth, endDay := dayOfYearToMonthDay(dayCounter+length-1, result.Months)

			color := "#808080"
			if len(s.Color) >= 1 && s.Color[0] != "" {
				color = normalizeColor(s.Color[0])
			}

			result.Seasons = append(result.Seasons, Season{
				Name:       s.Name,
				StartMonth: startMonth,
				StartDay:   startDay,
				EndMonth:   endMonth,
				EndDay:     endDay,
				Color:      color,
			})

			dayCounter += length
		}
	}

	// Eras.
	for i, e := range fc.StaticData.Eras {
		var desc *string
		if e.Description != "" {
			desc = &e.Description
		}
		result.Eras = append(result.Eras, EraInput{
			Name:        e.Name,
			StartYear:   e.Date.Year,
			Description: desc,
			Color:       "#6366f1",
			SortOrder:   i,
		})
	}

	return result, nil
}

// --- Helpers ---

// stripLocalizationKey removes Foundry VTT localization prefixes from names.
// e.g. "CALENDARIA.Calendar.Gregorian.Month.January" → "January"
// Strings without dots are returned unchanged.
func stripLocalizationKey(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ".") {
		return s
	}
	parts := strings.Split(s, ".")
	return parts[len(parts)-1]
}

// normalizeColor ensures a color string is a valid hex color.
// Returns the color as-is if already valid, or a default gray.
func normalizeColor(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return "#808080"
	}
	if c[0] != '#' {
		c = "#" + c
	}
	return c
}

// roundFloat rounds a float to n decimal places.
func roundFloat(f float64, n int) float64 {
	pow := math.Pow(10, float64(n))
	return math.Round(f*pow) / pow
}

// unused but kept for potential future use with moon phase offsets.
var _ = roundFloat

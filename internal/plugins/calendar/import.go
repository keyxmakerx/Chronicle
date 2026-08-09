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
// Months use days/leapDays. Moons have cycleLength/referenceDate. Supports eras
// and festivals natively.
//
// Calendaria authors SEASONS IN TWO SHAPES and both are read: a day-of-year span
// (dayStart/dayEnd counted from the start of the year) and a MONTH RANGE
// (monthStart/monthEnd naming whole months, dayStart/dayEnd narrowing the first
// and last). Which shape a file is in, and whether its month indices are 0- or
// 1-based, is detected per file — see calendariaSeasonMonthBase, and do not
// replace that detection with a constant: the real exports disagree.
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
	// Real-time tracking (C-REAL-CALENDAR-P3 0c). Only the Chronicle native
	// format carries these — external formats (Simple Calendar / Calendaria /
	// Fantasy-Calendar) are all fantasy calendars and leave TracksRealTime=false
	// with a nil zone. ApplyImport applies them through the SAME validation as
	// the enable flow (reallife + loadable zone + 24h), so a bad payload is a
	// named validation error rather than a silently-stranded flag. This is what
	// makes the export.go round-trip claim actually true.
	TracksRealTime bool    `json:"tracks_real_time,omitempty"`
	RealTimeZone   *string `json:"real_time_zone,omitempty"`
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
			// Real-time round-trip (0c): carry the flag + anchor zone so a
			// re-import restores wall-clock authority instead of dropping it.
			TracksRealTime: export.Calendar.TracksRealTime,
			RealTimeZone:   export.Calendar.RealTimeZone,
		},
	}

	// Copy months.
	for _, m := range export.Calendar.Months {
		result.Months = append(result.Months, MonthInput(m))
	}

	// Copy weekdays.
	for _, w := range export.Calendar.Weekdays {
		result.Weekdays = append(result.Weekdays, WeekdayInput(w))
	}

	// Copy moons.
	for _, m := range export.Calendar.Moons {
		result.Moons = append(result.Moons, MoonInput(m))
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
		result.Eras = append(result.Eras, EraInput(e))
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
	// Try v1 legacy field aliases. Errors are non-fatal — malformed v1
	// fields are silently skipped since the v2 fields take precedence.
	if len(c.Months) == 0 {
		if v, ok := raw["monthSettings"]; ok {
			_ = json.Unmarshal(v, &c.Months)
		}
	}
	if len(c.Weekdays) == 0 {
		if v, ok := raw["weekdaySettings"]; ok {
			_ = json.Unmarshal(v, &c.Weekdays)
		}
	}
	if len(c.Seasons) == 0 {
		if v, ok := raw["seasonSettings"]; ok {
			_ = json.Unmarshal(v, &c.Seasons)
		}
	}
	if len(c.Moons) == 0 {
		if v, ok := raw["moonSettings"]; ok {
			_ = json.Unmarshal(v, &c.Moons)
		}
	}
	if c.Year.NumericRepresentation == 0 {
		if v, ok := raw["yearSettings"]; ok {
			_ = json.Unmarshal(v, &c.Year)
		}
	}
	if c.Time.HoursInDay == 0 {
		if v, ok := raw["timeSettings"]; ok {
			_ = json.Unmarshal(v, &c.Time)
		}
	}
	if c.LeapYear.Rule == "" {
		if v, ok := raw["leapYearSettings"]; ok {
			_ = json.Unmarshal(v, &c.LeapYear)
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
	// The file names itself and this parser was the only one dropping it (the
	// Calendaria and Fantasy-Calendar parsers both carry cal.Name through), so
	// every Simple Calendar import arrived on Review as the placeholder and the
	// author had to retype it. stripLocalizationKey, not a bare TrimSpace,
	// because Simple Calendar ships localization-key names ("FSC.Date.January")
	// and every other name field in this parser is read through it. The
	// placeholder stays as the empty-name fallback.
	if n := stripLocalizationKey(cal.Name); n != "" {
		result.CalendarName = n
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
	Ordinal      int    `json:"ordinal"`  // author-declared rank; the only total order when dayStart ties
	DayStart     int    `json:"dayStart"` // day-of-year (1-indexed), or day-of-MONTH in the month-range shape
	DayEnd       int    `json:"dayEnd"`   // day-of-year (1-indexed), or day-of-MONTH in the month-range shape
	Abbreviation string `json:"abbreviation"`

	// Calendaria authors seasons in ONE OF TWO SHAPES and the day fields mean
	// different things in each — see calendariaSeasonMonthBase. These two are
	// POINTERS because "absent" is the discriminator: a file that declares no
	// monthStart anywhere is the day-of-year shape, and 0 is a legitimate
	// monthStart in the other one, so a value-typed int cannot tell them apart.
	MonthStart *int `json:"monthStart"`
	MonthEnd   *int `json:"monthEnd"`
}

// calendariaSeasonMonthBase decides which shape a Calendaria file's seasons are
// authored in, and — when it is the month-range shape — whether its month
// indices are 0-based or 1-based.
//
// THE TWO SHAPES. Calendaria seasons come as either a day-of-year span
// (dayStart/dayEnd counted from the start of the year, no month fields) or a
// MONTH RANGE (monthStart/monthEnd naming whole months, with dayStart/dayEnd
// narrowing the first and last of them). parseCalendaria only ever implemented
// the first, so every month-range file collapsed: presets/elven.json declares
// three seasons that all carry dayStart 0 / dayEnd 45 and differ ONLY in
// monthStart/monthEnd, so the shipped Elven preset imported three IDENTICAL
// ranges — Aevel 1 → Aevel 45, three times over — instead of Aevel→Lethra,
// Vanyr→Serel and Thalor→Myrren. Seven of its eight months belonged to no
// season at all.
//
// THE BASE IS DETECTED, NOT DECREED, because the real exports disagree and a
// hard-coded "+1 because monthStart is 0-based" would silently shift half of
// them by a month. The two reference files in cordinator/references/calendars
// are the evidence:
//
//   - forbidden-lands.json — 8 months, seasons at monthStart 0/2/4/6 with
//     monthEnd 1/3/5/7. A 1-based reading has no month 0, so it is 0-based, and
//     the four seasons tile all eight months exactly.
//   - calendar-of-therin.json — 15 months, seasons at monthStart 1/4/7/10/13
//     with monthEnd 3/6/9/12/0. A 0-based reading leaves the FIRST month in no
//     season and pushes the last past the end, so it is 1-based, and the five
//     seasons then tile all fifteen months exactly.
//
// The discriminator that separates them is therefore the smallest monthStart in
// the FILE: a 0-based export addresses its first month as 0, a 1-based one as 1.
// It is file-global rather than per-season because a base is a property of the
// exporter, not of one row. monthEnd is deliberately NOT consulted — therin's
// last season carries monthEnd 0 as "runs to the end of the year", which would
// wrongly read as evidence of 0-basing.
//
// Returns declared=false when no season names a month at all; that file is the
// day-of-year shape and keeps dayOfYearToMonthDay byte-for-byte.
func calendariaSeasonMonthBase(seasons []calSeason) (declared bool, base int) {
	smallest := 0
	for _, s := range seasons {
		if s.MonthStart == nil {
			continue
		}
		if !declared || *s.MonthStart < smallest {
			smallest = *s.MonthStart
		}
		declared = true
	}
	if !declared {
		return false, 0
	}
	if smallest <= 0 {
		return true, 0 // addresses the first month as 0
	}
	return true, 1 // addresses the first month as 1
}

// calendariaSeasonRange converts one month-range season into Chronicle's
// (startMonth, startDay, endMonth, endDay), all 1-based.
//
// The month indices are rebased by `base` and then CLAMPED into the months the
// file actually declares, because a season may not name a month that does not
// exist. Two conventions are honoured inside that clamp:
//
//   - a monthEnd that normalises BELOW the first month means "to the end of the
//     year" (calendar-of-therin's Greylight: monthEnd 0 on a 1-based file, the
//     season that runs from month 13 to the end). On a 0-based file the same
//     literal 0 normalises to month 1 and is an ordinary index, so the two
//     readings never collide.
//   - dayStart/dayEnd are days WITHIN the first/last month here, not days of the
//     year. dayStart 0 means "from the first day" (Calendaria writes 0 for an
//     unset start exactly as it does in the day-of-year shape, where
//     dayOfYearToMonthDay already maps it to 1/1), and a dayEnd that is unset or
//     longer than the closing month runs to that month's last day.
//
// It is faithful to the file rather than tidy: forbidden-lands.json writes
// dayEnd 45 for every season, including ones closing on a 46-day month, so
// Spring ends on day 45 of a 46-day month and day 46 belongs to no season. That
// is what the payload says. Inventing "…and always to the end of the month"
// would be a nicer calendar than the one the author exported.
func calendariaSeasonRange(s calSeason, base int, months []MonthInput) (startMonth, startDay, endMonth, endDay int) {
	n := len(months)
	if n == 0 {
		return 1, 1, 1, 1
	}

	startMonth = 1
	if s.MonthStart != nil {
		startMonth = *s.MonthStart - base + 1
	}
	endMonth = n
	if s.MonthEnd != nil {
		endMonth = *s.MonthEnd - base + 1
	}
	if endMonth < 1 {
		endMonth = n // "to the end of the year"
	}
	startMonth = clampInt(startMonth, 1, n)
	endMonth = clampInt(endMonth, 1, n)

	startDay = clampInt(s.DayStart, 1, months[startMonth-1].Days)
	endDay = months[endMonth-1].Days
	if s.DayEnd >= 1 && s.DayEnd < endDay {
		endDay = s.DayEnd
	}
	return startMonth, startDay, endMonth, endDay
}

// clampInt confines v to [lo, hi]. lo wins when the bounds are inverted, which
// only happens for a calendar with no months — a case its callers reject first.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
		if monthList[i].val.Ordinal != monthList[j].val.Ordinal {
			return monthList[i].val.Ordinal < monthList[j].val.Ordinal
		}
		// Map iteration is randomised, so an ordinal tie must fall back to the
		// authored key or two parses of the same bytes disagree on SortOrder.
		return monthList[i].key < monthList[j].key
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
		// W1 (R4 crash-guard): cal.Days.Values is a nil map when "days.values"
		// is absent — writing into it ("assignment to entry in nil map") panics
		// the import. Allocate before the fallback copy.
		weekdaySource = make(map[string]calWeekday, len(cal.Weeks))
		for k, w := range cal.Weeks {
			weekdaySource[k] = calWeekday(w)
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
		if wdList[i].val.Ordinal != wdList[j].val.Ordinal {
			return wdList[i].val.Ordinal < wdList[j].val.Ordinal
		}
		return wdList[i].key < wdList[j].key // total order over a randomised map
	})

	for i, w := range wdList {
		result.Weekdays = append(result.Weekdays, WeekdayInput{
			Name:      stripLocalizationKey(w.val.Name),
			SortOrder: i,
		})
	}

	// Moons — Calendaria stores them in an object map, and Go's map iteration is
	// randomised, so ranging straight over cal.Moons made two parses of the SAME
	// bytes emit the moons in a different order. Nothing in the payload ranks
	// moons (unlike months / weekdays / eras, calMoon carries no ordinal), so the
	// authored map key is the only deterministic rank the file gives us; keys are
	// unique within a map, so this comparator is total.
	type moonEntry struct {
		key string
		val calMoon
	}
	var moonList []moonEntry
	for k, m := range cal.Moons {
		moonList = append(moonList, moonEntry{k, m})
	}
	sort.Slice(moonList, func(i, j int) bool {
		return moonList[i].key < moonList[j].key
	})

	for _, m := range moonList {
		result.Moons = append(result.Moons, MoonInput{
			Name:        stripLocalizationKey(m.val.Name),
			CycleDays:   m.val.CycleLength,
			PhaseOffset: 0, // Calendaria uses referenceDate instead of offset
			Color:       normalizeColor(m.val.Color),
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
	// Calendaria authors an explicit `ordinal` on seasons exactly as it does on
	// months and weekdays, and it is the only field that ranks them when their
	// day-of-year ranges tie — presets/elven.json ties all three seasons at
	// dayStart 0. Sorting on DayStart alone therefore left the order to Go's
	// randomised map iteration. The map key is the final tiebreak so the
	// comparator is total and two parses of the same bytes always agree.
	//
	// MONTHSTART IS PART OF THE KEY, and it has to be: a month-range file with
	// no ordinals ties on BOTH remaining fields, and calendar-of-therin.json is
	// exactly that — five seasons, no ordinal, no dayStart, ranked only by the
	// months they name. Without this the determinism the tie-break bought would
	// hold (the key is still total) while the ORDER would be alphabetical
	// nonsense. Ordinal still outranks it, so elven's authored 1/2/3 decides
	// there and its order is unchanged.
	seasonMonthKey := func(s calSeason) int {
		if s.MonthStart == nil {
			return 0
		}
		return *s.MonthStart
	}
	sort.Slice(seasonList, func(i, j int) bool {
		a, b := seasonList[i], seasonList[j]
		if a.val.Ordinal != b.val.Ordinal {
			return a.val.Ordinal < b.val.Ordinal
		}
		if ka, kb := seasonMonthKey(a.val), seasonMonthKey(b.val); ka != kb {
			return ka < kb
		}
		if a.val.DayStart != b.val.DayStart {
			return a.val.DayStart < b.val.DayStart
		}
		return a.key < b.key
	})

	// Which of the two season shapes is this file in? See
	// calendariaSeasonMonthBase — the answer is file-global, so it is resolved
	// once here rather than re-guessed per season.
	seasonVals := make([]calSeason, 0, len(seasonList))
	for _, s := range seasonList {
		seasonVals = append(seasonVals, s.val)
	}
	monthRanged, monthBase := calendariaSeasonMonthBase(seasonVals)

	for _, s := range seasonList {
		var startMonth, startDay, endMonth, endDay int
		if monthRanged {
			startMonth, startDay, endMonth, endDay = calendariaSeasonRange(s.val, monthBase, result.Months)
		} else {
			// The day-of-year shape: cumulative day-of-year → month+day.
			startMonth, startDay = dayOfYearToMonthDay(s.val.DayStart, result.Months)
			endMonth, endDay = dayOfYearToMonthDay(s.val.DayEnd, result.Months)
		}

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
		if eraList[i].val.StartYear != eraList[j].val.StartYear {
			return eraList[i].val.StartYear < eraList[j].val.StartYear
		}
		return eraList[i].key < eraList[j].key // total order over a randomised map
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

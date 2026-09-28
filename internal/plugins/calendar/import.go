// Package calendar — import.go imports calendars from three formats,
// auto-detected from the JSON shape:
//
//   - Chronicle native (chronicle-calendar-v1): round-trips perfectly.
//   - Simple Calendar (Foundry VTT): top-level "calendar" key with
//     months/weekdays/time/leapYear; numberOfDays/numberOfLeapYearDays,
//     hoursInDay/minutesInHour, startingMonth/startingDay,
//     cycleLength/cycleDayAdjust.
//   - Calendaria (Foundry VTT): top-level "months" as an object, or
//     "days.hoursPerDay" present; days/leapDays, cycleLength/referenceDate,
//     eras and festivals.
//
// Calendaria seasons appear in two shapes (day-of-year span, or a month
// range with 0- or 1-based indices), and which shape/base a file uses is
// detected per file in calendariaSeasonMonthBase — do not replace that
// detection with a constant, real exports disagree.
package calendar

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ImportFormat identifies which JSON format was detected.
type ImportFormat string

const (
	FormatChronicle  ImportFormat = "chronicle"
	FormatSimpleCal  ImportFormat = "simple-calendar"
	FormatCalendaria ImportFormat = "calendaria"
	FormatFantasyCal ImportFormat = "fantasy-calendar"
	FormatUnknown    ImportFormat = "unknown"
	// FormatRealWorld and FormatBuilt never reach DetectAndParse — they mark
	// an ImportResult the wizard built server-side (reallife.go) or from the
	// browser's "Build your own" step (parseWizardImportJSON), never a file
	// on disk. Informational only (ImportResult.Format has no other reader),
	// kept distinct from the upload-format constants above for clarity.
	FormatRealWorld ImportFormat = "reallife-gregorian"
	FormatBuilt     ImportFormat = "built"
)

// ImportResult holds the parsed calendar data ready to be applied.
type ImportResult struct {
	Format       ImportFormat   `json:"format"`
	CalendarName string         `json:"calendar_name"`
	Months       []MonthInput   `json:"months"`
	Weekdays     []WeekdayInput `json:"weekdays"`
	Moons        []MoonInput    `json:"moons"`
	Seasons      []Season       `json:"seasons"`
	Eras         []EraInput     `json:"eras"`
	// Cycles and Festivals are Chronicle-native sub-resources (#771):
	// parseChronicle is the only parser that populates Cycles (no external
	// format models a repeating named cycle); Calendaria also carries
	// festivals of its own, read into Festivals alongside Chronicle's.
	Cycles    []CycleInput     `json:"cycles,omitempty"`
	Festivals []FestivalInput  `json:"festivals,omitempty"`
	Settings  ImportedSettings `json:"settings"`
	// Events carries a Chronicle-native export's own events forward through a
	// re-import (#779): parseChronicle is the only parser that populates it —
	// Simple Calendar, Calendaria and Fantasy-Calendar files never had
	// Chronicle events to bring over in the first place. Shaped exactly like
	// ExportEvent (kind by SLUG, never EntityID/KindID/VisibilityRules/
	// RecurrenceDayOfWeek — see ExportEvent's own doc comment on why a
	// portable event excludes those), so CreateCalendarFromImport resolves
	// the kind slug against the TARGET campaign rather than trusting a
	// numeric id that means nothing there.
	Events []ExportEvent `json:"events,omitempty"`
	// Today is what the import file itself determined for the created
	// calendar's current ("today") date — never a fallback this package
	// invented on the file's behalf. Month/Day are nil when the format
	// carries no day-level current date at all (Calendaria only ever
	// specifies a year); CreateCalendarFromImport then REQUIRES the caller
	// to supply an explicit override for whichever of the two is nil rather
	// than silently defaulting either to 1 (#741: "an import never silently
	// resets the calendar's current date").
	Today ImportedToday `json:"today"`
	// Warnings are user-facing notes about data this import clamped or
	// filled in rather than failing the whole import over (#741: "warn,
	// never refuse, when a structural oddity is found") — an out-of-range
	// season date, a non-positive month length, a blank name, an event whose
	// kind slug doesn't exist in the target campaign. Populated at parse
	// time by clampCalendarStructure for Calendaria/Simple Calendar (real
	// exports in the wild carry values Chronicle's own schema can't store
	// as-is; Chronicle's own export and Fantasy-Calendar's computed ranges
	// never produce these shapes) and appended to at apply time by
	// CreateCalendarFromImport (an unresolved event kind slug).
	Warnings []string `json:"warnings,omitempty"`
}

// ImportedToday is the day-level "today" ImportResult carries forward — see
// ImportResult.Today's doc comment for the rule it exists to enforce.
type ImportedToday struct {
	Year  int  `json:"year"`
	Month *int `json:"month,omitempty"`
	Day   *int `json:"day,omitempty"`
}

// ImportedSettings holds calendar-level settings extracted from the import.
type ImportedSettings struct {
	// Mode is only ever set by parseChronicle (from the export's own Mode):
	// every external format (Simple Calendar, Calendaria, Fantasy-Calendar)
	// describes a custom fantasy calendar with no Gregorian/real-time
	// concept, so their parsers leave this "" and CreateCalendarFromImport's
	// CreateCalendar call defaults an empty Mode to ModeFantasy — the same
	// default CreateCalendar already applies to a manual create with no
	// mode specified.
	Mode             string  `json:"mode,omitempty"`
	EpochName        *string `json:"epoch_name,omitempty"`
	CurrentYear      int     `json:"current_year"`
	HoursPerDay      int     `json:"hours_per_day"`
	MinutesPerHour   int     `json:"minutes_per_hour"`
	SecondsPerMinute int     `json:"seconds_per_minute"`
	LeapYearEvery    int     `json:"leap_year_every"`
	LeapYearOffset   int     `json:"leap_year_offset"`
	// Only the Chronicle native format carries these — external formats are
	// all fantasy calendars and leave TracksRealTime=false with a nil zone.
	// ApplyImport validates them through the same rules as the enable flow.
	TracksRealTime bool    `json:"tracks_real_time,omitempty"`
	RealTimeZone   *string `json:"real_time_zone,omitempty"`
	// Also Chronicle-native only: external formats have no concept of a
	// hemisphere, forecast toggle or week-start rule.
	Hemisphere         *string `json:"hemisphere,omitempty"`
	ForecastsEnabled   bool    `json:"forecasts_enabled,omitempty"`
	MonthStartsNewWeek bool    `json:"month_starts_new_week,omitempty"`
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
			Mode:             export.Calendar.Mode,
			EpochName:        export.Calendar.EpochName,
			CurrentYear:      export.Calendar.CurrentYear,
			HoursPerDay:      export.Calendar.HoursPerDay,
			MinutesPerHour:   export.Calendar.MinutesPerHour,
			SecondsPerMinute: export.Calendar.SecondsPerMinute,
			LeapYearEvery:    export.Calendar.LeapYearEvery,
			LeapYearOffset:   export.Calendar.LeapYearOffset,
			// Carries the real-time flag, anchor zone, hemisphere, forecast
			// toggle and week-start rule forward, so a re-import restores them
			// instead of silently dropping back to their defaults.
			TracksRealTime:     export.Calendar.TracksRealTime,
			RealTimeZone:       export.Calendar.RealTimeZone,
			Hemisphere:         export.Calendar.Hemisphere,
			ForecastsEnabled:   export.Calendar.ForecastsEnabled,
			MonthStartsNewWeek: export.Calendar.MonthStartsNewWeek,
		},
		// export.Events is already the top-level "events" array (ExportEvent's
		// own json tag lives on ChronicleExport, not nested under "calendar") —
		// json.Unmarshal above populated it directly; #779's bug was only ever
		// that ImportResult had nowhere to carry it onward from here.
		Events: export.Events,
		Today: ImportedToday{
			Year:  export.Calendar.CurrentYear,
			Month: importIntPtr(export.Calendar.CurrentMonth),
			Day:   importIntPtr(export.Calendar.CurrentDay),
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

	// Copy moons. Field-by-field, not a type conversion: ExportMoon carries
	// no ID (a re-import always inserts fresh moons), while MoonInput does
	// (for SetMoons' upsert), so the two shapes are not interchangeable.
	for _, m := range export.Calendar.Moons {
		result.Moons = append(result.Moons, MoonInput{
			Name:        m.Name,
			CycleDays:   m.CycleDays,
			PhaseOffset: m.PhaseOffset,
			// normalizeColor: a hand-edited or malicious "chronicle-calendar-v1"
			// upload is not guaranteed to round-trip a valid color the way a
			// real Chronicle export does — calendar_moons.color is VARCHAR(7),
			// so an unnormalized value can crash the import under strict SQL
			// mode instead of merely looking wrong.
			Color:             normalizeColor(m.Color),
			HiddenFromPlayers: m.HiddenFromPlayers,
		})
	}

	// Copy seasons.
	for _, s := range export.Calendar.Seasons {
		result.Seasons = append(result.Seasons, Season{
			Name:        s.Name,
			StartMonth:  s.StartMonth,
			StartDay:    s.StartDay,
			EndMonth:    s.EndMonth,
			EndDay:      s.EndDay,
			Description: s.Description,
			// normalizeColor: same reasoning as the moon color above —
			// calendar_seasons.color is VARCHAR(7) too.
			Color:         normalizeColor(s.Color),
			WeatherEffect: s.WeatherEffect,
		})
	}

	// Copy eras. A pre-V5 export has no start_month/start_day keys at all,
	// which unmarshal to the Go zero value 0; normalizeEraStart treats that
	// the same as any other era missing day-level bounds.
	for _, e := range export.Calendar.Eras {
		result.Eras = append(result.Eras, normalizeEraStart(EraInput(e)))
	}

	// Copy cycles (with their entries) and festivals (#771): the export
	// already writes both (see BuildExport), but ImportResult had nowhere to
	// carry them onward from here, so a Chronicle export/import round trip
	// silently dropped them — the header's "round-trips perfectly" claim.
	for _, c := range export.Calendar.Cycles {
		ci := CycleInput{Name: c.Name, CycleLength: c.CycleLength, Type: c.Type, SortOrder: c.SortOrder}
		for _, e := range c.Entries {
			ci.Entries = append(ci.Entries, CycleEntryInput{
				Name: e.Name, Icon: e.Icon, YearOffset: e.YearOffset, SortOrder: e.SortOrder,
			})
		}
		result.Cycles = append(result.Cycles, ci)
	}
	for _, f := range export.Calendar.Festivals {
		result.Festivals = append(result.Festivals, FestivalInput{
			Name: f.Name, Month: f.Month, Day: f.Day, AfterMonth: f.AfterMonth,
			Description: f.Description, Color: normalizeOptionalColor(f.Color),
			Icon: f.Icon, SortOrder: f.SortOrder,
		})
	}

	// clampCalendarStructure's bounds apply here too: detectFormat trusts
	// any file that merely claims "format":"chronicle-calendar-v1", so a
	// hand-edited or malicious upload can carry this path's structure
	// without ever having gone through a real Chronicle export.
	if err := clampCalendarStructure(result); err != nil {
		return nil, err
	}
	return result, nil
}

// normalizeEraStart clamps a missing start month or day (0, from a format
// with no day-level era bounds) up to 1, so the era begins on a real day
// instead of one that does not exist.
func normalizeEraStart(e EraInput) EraInput {
	if e.StartMonth < 1 {
		e.StartMonth = 1
	}
	if e.StartDay < 1 {
		e.StartDay = 1
	}
	// Every era-producing parser (Chronicle, Calendaria, Fantasy-Calendar)
	// funnels through here, so this is also where every era's color gets
	// clamped to something calendar_eras.color can hold, regardless of
	// format.
	e.Color = normalizeColor(e.Color)
	return e
}

// --- Simple Calendar Parser ---

// scData is the top-level Simple Calendar export structure.
type scData struct {
	Calendar scCalendar `json:"calendar"`
}

// scCalendar holds the Simple Calendar configuration. Supports both v2 field names
// and v1 legacy aliases (yearSettings, monthSettings, etc.) via custom UnmarshalJSON.
type scCalendar struct {
	Name string `json:"name"`
	// CurrentDate is a pointer, not a value, so a file that omits it
	// entirely (a template/definitions-only export with no live campaign
	// date) unmarshals to nil rather than the zero value {Month:0, Day:0} —
	// which, being valid 0-indexed values, is indistinguishable from "no
	// current date" if collapsed to a value type. See its use in
	// parseSimpleCalendarInner.
	CurrentDate    *scCurrentDate   `json:"currentDate"`
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
	Name                        string `json:"name"`
	Abbreviation                string `json:"abbreviation"`
	NumericRepresentation       int    `json:"numericRepresentation"`
	NumericRepresentationOffset int    `json:"numericRepresentationOffset"`
	NumberOfDays                int    `json:"numberOfDays"`
	NumberOfLeapYearDays        int    `json:"numberOfLeapYearDays"`
	Intercalary                 bool   `json:"intercalary"`
	IntercalaryInclude          bool   `json:"intercalaryInclude"`
	StartingWeekday             *int   `json:"startingWeekday"`
	Description                 string `json:"description"`
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
	// Use stripLocalizationKey, not a bare TrimSpace: Simple Calendar ships
	// localization-key names ("FSC.Date.January"), and every other name field
	// in this parser is read through it.
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

	// Simple Calendar's currentDate is 0-indexed, like its months/seasons —
	// see scCurrentDate's own field comments. A file with no currentDate at
	// all leaves Month/Day nil (#741: never a disguised default) — only the
	// year, which Simple Calendar always carries via yearSettings/year, is
	// populated unconditionally.
	result.Today = ImportedToday{Year: cal.Year.NumericRepresentation}
	if cal.CurrentDate != nil {
		result.Today.Month = importIntPtr(cal.CurrentDate.Month + 1)
		result.Today.Day = importIntPtr(cal.CurrentDate.Day + 1)
	}

	if err := clampCalendarStructure(result); err != nil {
		return nil, err
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
// authored in — a day-of-year span (dayStart/dayEnd, no month fields) or a
// month range (monthStart/monthEnd naming whole months) — and, for the
// month-range shape, whether its month indices are 0-based or 1-based.
//
// The base must be detected, not decreed: real exports disagree, so a
// hard-coded offset would silently shift half of them by a month. The
// discriminator is the smallest monthStart in the file (a 0-based export
// addresses its first month as 0, a 1-based one as 1); it is file-global
// because the base is a property of the exporter, not of one row. monthEnd is
// deliberately not consulted: a "runs to the end of the year" season can
// legitimately carry monthEnd 0, which would misread as evidence of 0-basing.
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
// Month indices are rebased by `base` and then clamped into the months the
// file actually declares. Two conventions apply inside that clamp:
//
//   - a monthEnd that normalises below the first month means "to the end of
//     the year"; on a 0-based file the same literal 0 normalises to month 1
//     and is an ordinary index, so the two readings never collide.
//   - dayStart/dayEnd are days within the first/last month, not days of the
//     year; dayStart 0 means "from the first day", and a dayEnd that is unset
//     or longer than the closing month runs to that month's last day.
//
// It is faithful to the file rather than tidy: an unset or short dayEnd is
// taken literally even when it leaves days at the end of a month unassigned
// to any season, rather than inventing "runs to the end of the month".
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
	Name          string     `json:"name"`
	CycleLength   float64    `json:"cycleLength"`
	Color         string     `json:"color"`
	ReferenceDate calRefDate `json:"referenceDate"`
}

type calRefDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type calFestival struct {
	Name        string `json:"name"`
	Month       int    `json:"month"`
	Day         int    `json:"day"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	Description string `json:"description"`
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
	// Sort key: ordinal, then monthStart (month-range files may tie on
	// ordinal and dayStart both), then DayStart, then map key as a final
	// tiebreak — the comparator must be total so two parses of the same bytes
	// always agree (map iteration order is not stable).
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
		result.Eras = append(result.Eras, normalizeEraStart(EraInput{
			Name:        stripLocalizationKey(e.val.Name),
			StartYear:   e.val.StartYear,
			EndYear:     e.val.EndYear,
			Description: desc,
			Color:       "#6366f1", // default since Calendaria doesn't have era colors
			SortOrder:   i,
		}))
	}

	// Festivals (#771): cal.Festivals was already parsed above but never
	// read into the result, so every Calendaria festival was silently
	// dropped on import. Object map, so sort by (month, day) then the
	// authored key for a total order over Go's randomised map iteration —
	// the same reasoning eraList/moonList/seasonList use above.
	type festivalEntry struct {
		key string
		val calFestival
	}
	var festivalList []festivalEntry
	for k, f := range cal.Festivals {
		festivalList = append(festivalList, festivalEntry{k, f})
	}
	sort.Slice(festivalList, func(i, j int) bool {
		a, b := festivalList[i].val, festivalList[j].val
		if a.Month != b.Month {
			return a.Month < b.Month
		}
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return festivalList[i].key < festivalList[j].key
	})
	for i, f := range festivalList {
		result.Festivals = append(result.Festivals, FestivalInput{
			Name:        stripLocalizationKey(f.val.Name),
			Month:       importIntPtr(f.val.Month),
			Day:         importIntPtr(f.val.Day),
			Description: nonEmptyPtr(f.val.Description),
			Color:       normalizeOptionalColor(nonEmptyPtr(f.val.Color)),
			Icon:        nonEmptyPtr(f.val.Icon),
			SortOrder:   i,
		})
	}

	// Calendaria's file gives no day-level current date at all — only a
	// year (Years.YearZero, already used for Settings.CurrentYear above).
	// Month/Day stay nil so CreateCalendarFromImport requires the caller to
	// confirm them explicitly instead of guessing day 1 (#741).
	result.Today = ImportedToday{Year: result.Settings.CurrentYear}

	if err := clampCalendarStructure(result); err != nil {
		return nil, err
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
	FirstDay   int          `json:"first_day"`
	Overflow   bool         `json:"overflow"`
	GlobalWeek []string     `json:"global_week"`
	Timespans  []fcTimespan `json:"timespans"`
	LeapDays   []fcLeapDay  `json:"leap_days"`
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

	// Today — dynamic_data is Fantasy-Calendar's live current-date state for
	// THIS specific calendar export, not an optional/defaults-only section,
	// so year/month/day are populated unconditionally from it, the same way
	// parseChronicle treats its own always-present current_month/
	// current_day. Timespan (the current month) and Day are both
	// 0-indexed, like LeapDays' Timespan index above.
	result.Today = ImportedToday{
		Year:  fc.DynamicData.Year,
		Month: importIntPtr(fc.DynamicData.Timespan + 1),
		Day:   importIntPtr(fc.DynamicData.Day + 1),
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
		result.Eras = append(result.Eras, normalizeEraStart(EraInput{
			Name:        e.Name,
			StartYear:   e.Date.Year,
			Description: desc,
			Color:       "#6366f1",
			SortOrder:   i,
		}))
	}

	if err := clampCalendarStructure(result); err != nil {
		return nil, err
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

// normalizeColor makes an import-supplied color value safe to store: it
// must fit calendar_moons.color/calendar_seasons.color's VARCHAR(7) — the
// narrowest color column any import format writes to — under strict SQL
// mode, where an over-length value errors the whole transaction instead of
// silently truncating (a crash-the-import risk, not just a cosmetic one).
// Every caller across every format (including Chronicle's own re-import,
// which is untrusted the moment it's a file someone uploaded, not
// necessarily one Chronicle itself produced) routes through this rather
// than writing a source value directly.
//
// A valid #rgb/#rrggbb (with or without the leading '#') is accepted and
// #rgb is expanded to #rrggbb, so every result is exactly 7 characters.
// Anything else — a CSS name like "steelblue", an rgba() string, garbage —
// falls back to a default gray rather than being truncated or rejected.
func normalizeColor(c string) string {
	const fallback = "#808080"
	c = strings.TrimSpace(c)
	if c == "" {
		return fallback
	}
	if c[0] != '#' {
		c = "#" + c
	}
	if !hexColorPattern.MatchString(c) {
		return fallback
	}
	if len(c) == 4 { // "#" + 3 hex digits: expand to the 6-digit form.
		c = fmt.Sprintf("#%c%c%c%c%c%c", c[1], c[1], c[2], c[2], c[3], c[3])
	}
	return c
}

// normalizeOptionalColor applies normalizeColor to an optional color pointer,
// leaving a truly-absent color (nil) as nil instead of forcing the gray
// fallback normalizeColor gives an empty string — unlike a moon or season, a
// festival's color is a genuinely optional field (the UI falls back to its
// own default), not one that always has a value to sanitize.
func normalizeOptionalColor(c *string) *string {
	if c == nil {
		return nil
	}
	v := normalizeColor(*c)
	return &v
}

// roundFloat rounds a float to n decimal places.
func roundFloat(f float64, n int) float64 {
	pow := math.Pow(10, float64(n))
	return math.Round(f*pow) / pow
}

// unused but kept for potential future use with moon phase offsets.
var _ = roundFloat

// importIntPtr returns a pointer to v — used to populate
// ImportedToday.Month/Day from a plain int without a throwaway local
// variable at every call site. Named distinctly from the repository
// integration tests' own intPtr test helper (same package, different file).
func importIntPtr(v int) *int { return &v }

// Upper bounds an imported calendar's structure is held to. Without them, a
// single crafted (or just badly-behaved) file can hand this package a month
// with billions of days, or tens of thousands of rows to write in one
// request — clampCalendarStructure enforces every one of these for every
// import format (see its own doc comment); buildMonthGrid/countEventsByDay
// in view_helpers.go re-clamp maxCalendarMonthDays independently, so a
// calendar_months row that reached this bound some other way (not
// necessarily an import) still renders safely.
//
// Sized well past any real calendar this importer targets: the shipped
// Harptos preset — the Forgotten Realms' actual calendar, festivals and
// all — comes to 17 months, 10 weekdays, 4 moons, 4 seasons and 1 era, and
// the Gregorian/Golarion shapes it also covers are smaller still; a large
// Fantasy-Calendar or Calendaria back-catalog runs to a few thousand
// events, not tens of thousands.
const (
	maxCalendarMonths       = 200  // real calendars top out near 20
	maxCalendarMonthDays    = 3660 // ten real years of days in one "month" is already absurd
	maxCalendarWeekdays     = 100
	maxCalendarEras         = 500
	maxCalendarSeasons      = 200
	maxCalendarMoons        = 100
	maxCalendarImportEvents = 20000

	// calendar_months/weekdays/moons/seasons.name are all VARCHAR(100).
	// calendar_eras.name is the wider VARCHAR(200) a hand-made era already
	// enforces via apperror.MaxNameLength (validateEraShape) — reused below
	// rather than a second constant carrying the same number.
	maxCalendarShortNameLength = 100
	// calendar_seasons.weather_effect VARCHAR(200).
	maxCalendarWeatherEffectLength = 200
	// calendar_festivals.icon / calendar_cycle_entries.icon VARCHAR(50).
	maxCalendarIconLength = 50

	maxCalendarCycles       = 100 // real calendars use at most a handful
	maxCalendarCycleEntries = 500 // per cycle — a century-long zodiac is already generous
	maxCalendarFestivals    = 500
)

// checkCalendarImportLimits refuses an import whose structure is too large
// to plausibly be a real calendar (see the maxCalendar* constants above).
// Counts are refused rather than truncated: there's no sane way to drop "the
// extra 4,000 months" that leaves a usable calendar behind, so the whole
// file is rejected with a message the wizard's preview step shows directly,
// before anything is parsed further or written.
func checkCalendarImportLimits(result *ImportResult) error {
	if n := len(result.Months); n > maxCalendarMonths {
		return fmt.Errorf("this calendar has %d months; the maximum is %d", n, maxCalendarMonths)
	}
	if n := len(result.Weekdays); n > maxCalendarWeekdays {
		return fmt.Errorf("this calendar has %d weekdays; the maximum is %d", n, maxCalendarWeekdays)
	}
	if n := len(result.Eras); n > maxCalendarEras {
		return fmt.Errorf("this calendar has %d eras; the maximum is %d", n, maxCalendarEras)
	}
	if n := len(result.Seasons); n > maxCalendarSeasons {
		return fmt.Errorf("this calendar has %d seasons; the maximum is %d", n, maxCalendarSeasons)
	}
	if n := len(result.Moons); n > maxCalendarMoons {
		return fmt.Errorf("this calendar has %d moons; the maximum is %d", n, maxCalendarMoons)
	}
	if n := len(result.Events); n > maxCalendarImportEvents {
		return fmt.Errorf("this calendar has %d events; the maximum is %d", n, maxCalendarImportEvents)
	}
	if n := len(result.Cycles); n > maxCalendarCycles {
		return fmt.Errorf("this calendar has %d cycles; the maximum is %d", n, maxCalendarCycles)
	}
	for _, c := range result.Cycles {
		if n := len(c.Entries); n > maxCalendarCycleEntries {
			return fmt.Errorf("cycle %q has %d entries; the maximum is %d", c.Name, n, maxCalendarCycleEntries)
		}
	}
	if n := len(result.Festivals); n > maxCalendarFestivals {
		return fmt.Errorf("this calendar has %d festivals; the maximum is %d", n, maxCalendarFestivals)
	}
	return nil
}

// truncateImportText shortens s to at most maxLen bytes without splitting a
// multi-byte UTF-8 rune in two — every name/description/weather-effect
// length clamp below routes its truncation through this rather than a bare
// s[:n], which can produce a byte sequence MariaDB's utf8mb4 columns refuse
// to store under strict SQL mode.
func truncateImportText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return strings.ToValidUTF8(s[:maxLen], "")
}

// clampCalendarStructure holds an imported calendar to the bounds above and
// clamps the out-of-range or blank values a real Simple Calendar / Calendaria
// export can carry — a season date outside its month's day range, a
// non-positive month length, a blank name — to something calendars' own
// schema can store, appending a warning for each clamp instead of failing
// the import outright (#741: "warn, never refuse, when a structural oddity
// is found"). The one exception is the counts checked by
// checkCalendarImportLimits: there's no reasonable value to clamp "too many
// months" down to, so those are refused instead — see its own doc comment.
// The fix belongs here, upstream of CalendarRepository.ApplyImport: that
// transaction stays atomic and rejects only genuine data-integrity
// violations (see "ApplyImport rolls back every write when a later step
// fails" in repository_integration_test.go), never a merely-odd-but-storable
// shape.
//
// Called by every format's parser (parseChronicle included — detectFormat
// trusts any file that merely claims the right "format" value, so a
// hand-edited or malicious upload can reach this path without ever having
// gone through a real Chronicle export) and, for the wizard's create step,
// a second time over browser-submitted data — see WizardCreate — so it must
// be safe to run twice over its own output; every clamp below is already
// idempotent (an in-range value is left untouched, so a second pass adds no
// further warnings).
func clampCalendarStructure(result *ImportResult) error {
	if err := checkCalendarImportLimits(result); err != nil {
		return err
	}

	for i := range result.Months {
		m := &result.Months[i]
		if strings.TrimSpace(m.Name) == "" {
			m.Name = fmt.Sprintf("Month %d", i+1)
			result.Warnings = append(result.Warnings, fmt.Sprintf("month %d had no name; named %q", i+1, m.Name))
		}
		if trunc := truncateImportText(m.Name, maxCalendarShortNameLength); trunc != m.Name {
			m.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("month %d's name was too long; shortened", i+1))
		}
		if m.Days <= 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"month %q had a non-positive length (%d days); clamped to 1 day", m.Name, m.Days))
			m.Days = 1
		} else if m.Days > maxCalendarMonthDays {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"month %q had a length of %d days, over the %d-day maximum; clamped", m.Name, m.Days, maxCalendarMonthDays))
			m.Days = maxCalendarMonthDays
		}
	}

	for i := range result.Weekdays {
		w := &result.Weekdays[i]
		if strings.TrimSpace(w.Name) == "" {
			w.Name = fmt.Sprintf("Day %d", i+1)
			result.Warnings = append(result.Warnings, fmt.Sprintf("weekday %d had no name; named %q", i+1, w.Name))
		}
		if trunc := truncateImportText(w.Name, maxCalendarShortNameLength); trunc != w.Name {
			w.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("weekday %d's name was too long; shortened", i+1))
		}
	}

	for i := range result.Moons {
		mo := &result.Moons[i]
		mo.Color = normalizeColor(mo.Color)
		if trunc := truncateImportText(mo.Name, maxCalendarShortNameLength); trunc != mo.Name {
			mo.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("moon %d's name was too long; shortened", i+1))
		}
	}

	for i := range result.Eras {
		e := &result.Eras[i]
		e.Color = normalizeColor(e.Color)
		if strings.TrimSpace(e.Name) == "" {
			e.Name = fmt.Sprintf("Era %d", i+1)
			result.Warnings = append(result.Warnings, fmt.Sprintf("era %d had no name; named %q", i+1, e.Name))
		}
		if trunc := truncateImportText(e.Name, apperror.MaxNameLength); trunc != e.Name {
			e.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("era %d's name was too long; shortened", i+1))
		}
		if e.Description != nil {
			if trunc := truncateImportText(*e.Description, apperror.MaxDescriptionLength); trunc != *e.Description {
				e.Description = &trunc
				result.Warnings = append(result.Warnings, fmt.Sprintf("era %d's description was too long; shortened", i+1))
			}
		}
	}

	for i := range result.Cycles {
		c := &result.Cycles[i]
		if strings.TrimSpace(c.Name) == "" {
			c.Name = fmt.Sprintf("Cycle %d", i+1)
			result.Warnings = append(result.Warnings, fmt.Sprintf("cycle %d had no name; named %q", i+1, c.Name))
		}
		if trunc := truncateImportText(c.Name, maxCalendarShortNameLength); trunc != c.Name {
			c.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("cycle %d's name was too long; shortened", i+1))
		}
		for j := range c.Entries {
			e := &c.Entries[j]
			if trunc := truncateImportText(e.Name, maxCalendarShortNameLength); trunc != e.Name {
				e.Name = trunc
				result.Warnings = append(result.Warnings, fmt.Sprintf("cycle %q entry %d's name was too long; shortened", c.Name, j+1))
			}
			if e.Icon != nil {
				if trunc := truncateImportText(*e.Icon, maxCalendarIconLength); trunc != *e.Icon {
					e.Icon = &trunc
					result.Warnings = append(result.Warnings, fmt.Sprintf("cycle %q entry %d's icon was too long; shortened", c.Name, j+1))
				}
			}
		}
	}

	for i := range result.Festivals {
		f := &result.Festivals[i]
		if strings.TrimSpace(f.Name) == "" {
			f.Name = fmt.Sprintf("Festival %d", i+1)
			result.Warnings = append(result.Warnings, fmt.Sprintf("festival %d had no name; named %q", i+1, f.Name))
		}
		if trunc := truncateImportText(f.Name, apperror.MaxNameLength); trunc != f.Name {
			f.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("festival %d's name was too long; shortened", i+1))
		}
		if f.Description != nil {
			if trunc := truncateImportText(*f.Description, apperror.MaxDescriptionLength); trunc != *f.Description {
				f.Description = &trunc
				result.Warnings = append(result.Warnings, fmt.Sprintf("festival %d's description was too long; shortened", i+1))
			}
		}
		f.Color = normalizeOptionalColor(f.Color)
		if f.Icon != nil {
			if trunc := truncateImportText(*f.Icon, maxCalendarIconLength); trunc != *f.Icon {
				f.Icon = &trunc
				result.Warnings = append(result.Warnings, fmt.Sprintf("festival %d's icon was too long; shortened", i+1))
			}
		}
	}

	// Colors are normalized here as well as in each parser, so the wizard's
	// create step, which re-reads browser-submitted data, gets the same check.
	for i := range result.Seasons {
		result.Seasons[i].Color = normalizeColor(result.Seasons[i].Color)
	}

	n := len(result.Months)
	if n == 0 {
		// No months to clamp a season into — nothing more this pass can do
		// (a months-less calendar is its own, separately-surfaced problem).
		return nil
	}
	for i := range result.Seasons {
		s := &result.Seasons[i]
		if strings.TrimSpace(s.Name) == "" {
			s.Name = fmt.Sprintf("Season %d", i+1)
			result.Warnings = append(result.Warnings, fmt.Sprintf("season %d had no name; named %q", i+1, s.Name))
		}
		if trunc := truncateImportText(s.Name, maxCalendarShortNameLength); trunc != s.Name {
			s.Name = trunc
			result.Warnings = append(result.Warnings, fmt.Sprintf("season %d's name was too long; shortened", i+1))
		}
		if s.Description != nil {
			if trunc := truncateImportText(*s.Description, apperror.MaxDescriptionLength); trunc != *s.Description {
				s.Description = &trunc
				result.Warnings = append(result.Warnings, fmt.Sprintf("season %d's description was too long; shortened", i+1))
			}
		}
		if s.WeatherEffect != nil {
			if trunc := truncateImportText(*s.WeatherEffect, maxCalendarWeatherEffectLength); trunc != *s.WeatherEffect {
				s.WeatherEffect = &trunc
				result.Warnings = append(result.Warnings, fmt.Sprintf("season %d's weather effect was too long; shortened", i+1))
			}
		}

		clampedStartMonth := clampInt(s.StartMonth, 1, n)
		clampedEndMonth := clampInt(s.EndMonth, 1, n)
		if clampedStartMonth != s.StartMonth || clampedEndMonth != s.EndMonth {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"season %q referenced a month outside this calendar's %d months; clamped", s.Name, n))
		}
		s.StartMonth, s.EndMonth = clampedStartMonth, clampedEndMonth

		clampedStartDay := clampInt(s.StartDay, 1, result.Months[s.StartMonth-1].Days)
		clampedEndDay := clampInt(s.EndDay, 1, result.Months[s.EndMonth-1].Days)
		if clampedStartDay != s.StartDay || clampedEndDay != s.EndDay {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"season %q had a day outside its month's range; clamped", s.Name))
		}
		s.StartDay, s.EndDay = clampedStartDay, clampedEndDay
	}
	return nil
}

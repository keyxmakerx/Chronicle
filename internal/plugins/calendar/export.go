// Package calendar — export.go provides JSON export of calendar configurations.
// Exports include all sub-resources (months, weekdays, moons, seasons, eras)
// in Chronicle's native format. Events are optionally included.
package calendar

// ChronicleExport is the top-level JSON envelope for calendar export.
// This is Chronicle's native format — a superset of what can be imported
// from external sources (Simple Calendar, Calendaria).
type ChronicleExport struct {
	Format  string         `json:"format"`  // "chronicle-calendar-v1"
	Version int            `json:"version"` // schema version (1)
	Calendar ExportCalendar `json:"calendar"`
	Events  []ExportEvent  `json:"events,omitempty"` // optional
}

// ExportCalendar holds the calendar configuration for export.
type ExportCalendar struct {
	Name             string           `json:"name"`
	Description      *string          `json:"description,omitempty"`
	Mode             string           `json:"mode"` // "fantasy" or "reallife"
	EpochName        *string          `json:"epoch_name,omitempty"`
	CurrentYear      int              `json:"current_year"`
	CurrentMonth     int              `json:"current_month"`
	CurrentDay       int              `json:"current_day"`
	CurrentHour      int              `json:"current_hour"`
	CurrentMinute    int              `json:"current_minute"`
	HoursPerDay      int              `json:"hours_per_day"`
	MinutesPerHour   int              `json:"minutes_per_hour"`
	SecondsPerMinute int              `json:"seconds_per_minute"`
	LeapYearEvery    int              `json:"leap_year_every"`
	LeapYearOffset   int              `json:"leap_year_offset"`
	Months           []ExportMonth    `json:"months"`
	Weekdays         []ExportWeekday  `json:"weekdays"`
	Moons            []ExportMoon     `json:"moons,omitempty"`
	Seasons          []ExportSeason   `json:"seasons,omitempty"`
	Eras             []ExportEra      `json:"eras,omitempty"`
}

// ExportMonth is a month definition for export.
type ExportMonth struct {
	Name          string `json:"name"`
	Days          int    `json:"days"`
	SortOrder     int    `json:"sort_order"`
	IsIntercalary bool   `json:"is_intercalary"`
	LeapYearDays  int    `json:"leap_year_days"`
}

// ExportWeekday is a weekday definition for export.
type ExportWeekday struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// ExportMoon is a moon definition for export.
type ExportMoon struct {
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
}

// ExportSeason is a season definition for export.
type ExportSeason struct {
	Name          string  `json:"name"`
	StartMonth    int     `json:"start_month"`
	StartDay      int     `json:"start_day"`
	EndMonth      int     `json:"end_month"`
	EndDay        int     `json:"end_day"`
	Description   *string `json:"description,omitempty"`
	Color         string  `json:"color"`
	WeatherEffect *string `json:"weather_effect,omitempty"`
}

// ExportEra is an era definition for export.
type ExportEra struct {
	Name        string  `json:"name"`
	StartYear   int     `json:"start_year"`
	EndYear     *int    `json:"end_year,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       string  `json:"color"`
	SortOrder   int     `json:"sort_order"`
}

// ExportEvent is a calendar event for export.
type ExportEvent struct {
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	Year           int     `json:"year"`
	Month          int     `json:"month"`
	Day            int     `json:"day"`
	StartHour      *int    `json:"start_hour,omitempty"`
	StartMinute    *int    `json:"start_minute,omitempty"`
	EndYear        *int    `json:"end_year,omitempty"`
	EndMonth       *int    `json:"end_month,omitempty"`
	EndDay         *int    `json:"end_day,omitempty"`
	EndHour        *int    `json:"end_hour,omitempty"`
	EndMinute      *int    `json:"end_minute,omitempty"`
	IsRecurring    bool    `json:"is_recurring"`
	RecurrenceType *string `json:"recurrence_type,omitempty"`
	Visibility     string  `json:"visibility"`
	Category       *string `json:"category,omitempty"`
}

// BuildExport creates a ChronicleExport from a fully-loaded Calendar and
// optional events. The calendar must have sub-resources eager-loaded.
func BuildExport(cal *Calendar, events []Event, includeEvents bool) *ChronicleExport {
	export := &ChronicleExport{
		Format:  "chronicle-calendar-v1",
		Version: 1,
		Calendar: ExportCalendar{
			Name:             cal.Name,
			Description:      cal.Description,
			Mode:             cal.Mode,
			EpochName:        cal.EpochName,
			CurrentYear:      cal.CurrentYear,
			CurrentMonth:     cal.CurrentMonth,
			CurrentDay:       cal.CurrentDay,
			CurrentHour:      cal.CurrentHour,
			CurrentMinute:    cal.CurrentMinute,
			HoursPerDay:      cal.HoursPerDay,
			MinutesPerHour:   cal.MinutesPerHour,
			SecondsPerMinute: cal.SecondsPerMinute,
			LeapYearEvery:    cal.LeapYearEvery,
			LeapYearOffset:   cal.LeapYearOffset,
		},
	}

	// Months.
	for _, m := range cal.Months {
		export.Calendar.Months = append(export.Calendar.Months, ExportMonth{
			Name:          m.Name,
			Days:          m.Days,
			SortOrder:     m.SortOrder,
			IsIntercalary: m.IsIntercalary,
			LeapYearDays:  m.LeapYearDays,
		})
	}

	// Weekdays.
	for _, w := range cal.Weekdays {
		export.Calendar.Weekdays = append(export.Calendar.Weekdays, ExportWeekday{
			Name:      w.Name,
			SortOrder: w.SortOrder,
		})
	}

	// Moons.
	for _, m := range cal.Moons {
		export.Calendar.Moons = append(export.Calendar.Moons, ExportMoon{
			Name:        m.Name,
			CycleDays:   m.CycleDays,
			PhaseOffset: m.PhaseOffset,
			Color:       m.Color,
		})
	}

	// Seasons.
	for _, s := range cal.Seasons {
		export.Calendar.Seasons = append(export.Calendar.Seasons, ExportSeason{
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

	// Eras.
	for _, e := range cal.Eras {
		export.Calendar.Eras = append(export.Calendar.Eras, ExportEra{
			Name:        e.Name,
			StartYear:   e.StartYear,
			EndYear:     e.EndYear,
			Description: e.Description,
			Color:       e.Color,
			SortOrder:   e.SortOrder,
		})
	}

	// Events (optional).
	if includeEvents && len(events) > 0 {
		for _, evt := range events {
			export.Events = append(export.Events, ExportEvent{
				Name:           evt.Name,
				Description:    evt.Description,
				Year:           evt.Year,
				Month:          evt.Month,
				Day:            evt.Day,
				StartHour:      evt.StartHour,
				StartMinute:    evt.StartMinute,
				EndYear:        evt.EndYear,
				EndMonth:       evt.EndMonth,
				EndDay:         evt.EndDay,
				EndHour:        evt.EndHour,
				EndMinute:      evt.EndMinute,
				IsRecurring:    evt.IsRecurring,
				RecurrenceType: evt.RecurrenceType,
				Visibility:     evt.Visibility,
				Category:       evt.Category,
			})
		}
	}

	return export
}

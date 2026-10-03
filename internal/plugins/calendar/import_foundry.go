package calendar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// foundryImportWireVersion is the only create payload shape the Foundry
// module sends today (its IMPORT_WIRE_VERSION). A newer module sending a
// shape this server doesn't know is refused rather than half-read.
const foundryImportWireVersion = 1

// foundryImportPayload is what the Foundry module POSTs to create a calendar
// from a Calendaria calendar (transformCalendariaCalendar in the module).
// It is not a Calendaria file: the module has already flattened Calendaria's
// keyed maps into ordered lists and made the current date 1-based, but
// season month indices, moon cycles and leap-day counts are passed through
// as Calendaria holds them.
type foundryImportPayload struct {
	SchemaVersion int             `json:"schema_version"`
	Source        string          `json:"source"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	CurrentYear   int             `json:"current_year"`
	CurrentMonth  int             `json:"current_month"`
	CurrentDay    int             `json:"current_day"`
	CurrentHour   int             `json:"current_hour"`
	CurrentMinute int             `json:"current_minute"`
	Months        []foundryMonth  `json:"months"`
	Weekdays      []foundryDay    `json:"weekdays"`
	Seasons       []foundrySeason `json:"seasons"`
	Moons         []foundryMoon   `json:"moons"`
	Eras          []foundryEra    `json:"eras"`
}

type foundryMonth struct {
	Name        string `json:"name"`
	Days        int    `json:"days"`
	Intercalary bool   `json:"intercalary"`
	// LeapExtraDays is Calendaria's leapDays passed through, which is the
	// month's total length in a leap year despite the wire name.
	LeapExtraDays *int `json:"leap_extra_days"`
}

type foundryDay struct {
	Name    string `json:"name"`
	RestDay bool   `json:"rest_day"`
}

type foundrySeason struct {
	Name       string `json:"name"`
	Color      string `json:"color"`
	MonthStart *int   `json:"month_start"`
	MonthEnd   *int   `json:"month_end"`
	DayStart   *int   `json:"day_start"`
	DayEnd     *int   `json:"day_end"`
}

type foundryMoon struct {
	Name           string  `json:"name"`
	Color          string  `json:"color"`
	CycleLength    float64 `json:"cycle_length"`
	CycleDayAdjust float64 `json:"cycle_day_adjust"`
	PhaseMode      string  `json:"phase_mode"`
	ReferenceDate  struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"reference_date"`
}

type foundryEra struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	StartYear    *int   `json:"start_year"`
	EndYear      *int   `json:"end_year"`
}

// ParseFoundryImport turns the Foundry module's create-calendar payload into
// an ImportResult, reusing the Calendaria file parser's own season and moon
// arithmetic so a calendar arrives the same whether it came as a file or
// straight from Foundry. The payload states a full current date, so Today is
// always complete.
func ParseFoundryImport(data []byte) (*ImportResult, error) {
	var p foundryImportPayload
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("read the calendar sent from Foundry: %w", err)
	}
	if p.SchemaVersion != foundryImportWireVersion {
		return nil, fmt.Errorf("this Chronicle understands Foundry calendar imports of version %d, not %d; update Chronicle or the Chronicle Sync module so they match", foundryImportWireVersion, p.SchemaVersion)
	}
	if p.Source != "calendaria" {
		return nil, fmt.Errorf("only Calendaria calendars can be imported from Foundry")
	}
	if len(p.Months) == 0 {
		return nil, fmt.Errorf("the calendar sent from Foundry has no months")
	}

	ir := &ImportResult{
		Format:       FormatCalendaria,
		CalendarName: foundryLabel(p.Name),
		Settings: ImportedSettings{
			CurrentYear:      p.CurrentYear,
			HoursPerDay:      24,
			MinutesPerHour:   60,
			SecondsPerMinute: 60,
		},
	}
	if ir.CalendarName == "" {
		ir.CalendarName = "Imported Calendar"
	}
	if d := strings.TrimSpace(p.Description); d != "" {
		d = truncateImportText(d, apperror.MaxDescriptionLength)
		ir.Settings.Description = &d
	}

	hasLeapDays := false
	for i, m := range p.Months {
		extra := 0
		if m.LeapExtraDays != nil && *m.LeapExtraDays > m.Days {
			extra = *m.LeapExtraDays - m.Days
			hasLeapDays = true
		}
		ir.Months = append(ir.Months, MonthInput{
			Name:          foundryLabel(m.Name),
			Days:          m.Days,
			SortOrder:     i,
			IsIntercalary: m.Intercalary,
			LeapYearDays:  extra,
		})
	}
	// The module sends neither the leap-year rule nor the length of a day,
	// so both start at Chronicle's defaults and the owner is told to check.
	ir.Warnings = append(ir.Warnings, "Foundry doesn't send the length of a day, so this calendar uses 24 hours of 60 minutes; change it in the calendar's settings if yours differs.")
	if hasLeapDays {
		ir.Warnings = append(ir.Warnings, "Some months have leap days, but Foundry doesn't send the leap-year rule, so they never apply yet; set how often leap years come in the calendar's settings.")
	}

	for i, d := range p.Weekdays {
		ir.Weekdays = append(ir.Weekdays, WeekdayInput{
			Name:      foundryLabel(d.Name),
			SortOrder: i,
			IsRestDay: d.RestDay,
		})
	}

	seasons := make([]calSeason, 0, len(p.Seasons))
	for _, s := range p.Seasons {
		seasons = append(seasons, calSeason{
			Name:       s.Name,
			Color:      s.Color,
			DayStart:   intOrZero(s.DayStart),
			DayEnd:     intOrZero(s.DayEnd),
			MonthStart: s.MonthStart,
			MonthEnd:   s.MonthEnd,
		})
	}
	monthRanged, monthBase := calendariaSeasonMonthBase(seasons)
	for _, s := range seasons {
		var startMonth, startDay, endMonth, endDay int
		if monthRanged {
			startMonth, startDay, endMonth, endDay = calendariaSeasonRange(s, monthBase, ir.Months)
		} else {
			startMonth, startDay = dayOfYearToMonthDay(s.DayStart, ir.Months)
			endMonth, endDay = dayOfYearToMonthDay(s.DayEnd, ir.Months)
		}
		ir.Seasons = append(ir.Seasons, Season{
			Name:       foundryLabel(s.Name),
			StartMonth: startMonth,
			StartDay:   startDay,
			EndMonth:   endMonth,
			EndDay:     endDay,
			Color:      normalizeColor(s.Color),
		})
	}

	randomized := 0
	for _, m := range p.Moons {
		if m.PhaseMode == "randomized" {
			randomized++
		}
		ir.Moons = append(ir.Moons, MoonInput{
			Name:      foundryLabel(m.Name),
			CycleDays: m.CycleLength,
			PhaseOffset: moonPhaseOffsetFromReference(ir.Months, 0, 0,
				m.CycleLength, m.CycleDayAdjust,
				m.ReferenceDate.Year, m.ReferenceDate.Month, m.ReferenceDate.Day),
			Color: normalizeColor(m.Color),
		})
	}
	if randomized > 0 {
		ir.Warnings = append(ir.Warnings, fmt.Sprintf("%d moon(s) use random phases in Calendaria; Chronicle gives them a steady cycle instead.", randomized))
	}

	for i, e := range p.Eras {
		var desc *string
		if abbr := foundryLabel(e.Abbreviation); abbr != "" {
			desc = &abbr
		}
		ir.Eras = append(ir.Eras, normalizeEraStart(EraInput{
			Name:        foundryLabel(e.Name),
			StartYear:   intOrZero(e.StartYear),
			EndYear:     e.EndYear,
			Description: desc,
			Color:       "#6366f1", // Calendaria has no era colours; same default as a file import
			SortOrder:   i,
		}))
	}

	month, day := p.CurrentMonth, p.CurrentDay
	ir.Today = ImportedToday{
		Year:   p.CurrentYear,
		Month:  &month,
		Day:    &day,
		Hour:   p.CurrentHour,
		Minute: p.CurrentMinute,
	}

	if err := clampCalendarStructure(ir); err != nil {
		return nil, err
	}
	return ir, nil
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// foundryLabel turns an unlocalized Calendaria key ("CALENDARIA.Month.Hammer")
// into its last segment and leaves real text alone. Only a single token whose
// first segment is all capitals counts as a key, so a name or abbreviation
// with its own full stops ("D.R.", "A. B") arrives intact.
func foundryLabel(s string) string {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " \t") {
		return s
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || parts[len(parts)-1] == "" || parts[0] == "" || strings.ToUpper(parts[0]) != parts[0] || strings.ToLower(parts[0]) == parts[0] {
		return s
	}
	return parts[len(parts)-1]
}

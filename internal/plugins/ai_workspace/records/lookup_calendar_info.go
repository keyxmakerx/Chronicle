package records

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// lookupWhatCalendar is the `what:` of the calendar lookup.
const (
	lookupWhatCalendar = "calendar"
)

// WeatherSettingsAPI reads a calendar's climate and the owner's own kinds
// of weather (calendar.CalendarService).
type WeatherSettingsAPI interface {
	GetWeatherSettings(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*calendar.WeatherSettings, error)
}

// Everything below is read from the calendar and the server's own lists
// (climates, built-in weathers, sky effects), never typed out here, so
// what an owner or a later build adds shows up without a change.

// nextDay is the day after d on cal.
func nextDay(cal *calendar.Calendar, d calDate) calDate {
	if d.D < cal.MonthDays(d.M-1, d.Y) {
		return calDate{d.Y, d.M, d.D + 1}
	}
	if d.M < len(cal.Months) {
		return calDate{d.Y, d.M + 1, 1}
	}
	return calDate{d.Y + 1, 1, 1}
}

// nextPhases finds a moon's next new and full moon from today: the day in
// the coming cycle whose phase is nearest each turning point.
func nextPhases(cal *calendar.Calendar, m calendar.Moon, today calDate) (newMoon, fullMoon calDate) {
	span := int(math.Ceil(m.CycleDays)) + 1
	if m.CycleDays <= 0 || span > 400 {
		return today, today
	}
	bestNew, bestFull := 2.0, 2.0
	d := today
	for i := 0; i < span; i++ {
		p := m.MoonPhase(cal.AbsoluteDay(d.Y, d.M, d.D))
		if dn := math.Min(p, 1-p); dn < bestNew {
			bestNew, newMoon = dn, d
		}
		if df := math.Abs(p - 0.5); df < bestFull {
			bestFull, fullMoon = df, d
		}
		d = nextDay(cal, d)
	}
	return newMoon, fullMoon
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// calendarInfo answers `what: calendar`: the calendar's whole structure,
// today, the moons' phases, and its climate, as the Actor may see them.
func (run *lookupRun) calendarInfo() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Calendar"}
	cal, err := run.calendarOrErr()
	if err != nil {
		return ans, err
	}
	ans.Chip = "Calendar · " + cal.Name
	var b strings.Builder
	fmt.Fprintf(&b, "## Calendar: %s\n\n", cal.Name)
	label := ""
	if cal.EpochName != nil && *cal.EpochName != "" {
		label = " " + *cal.EpochName
		fmt.Fprintf(&b, "Year label: %s\n", *cal.EpochName)
	}
	today := calDate{cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay}
	if len(cal.Months) > 0 {
		line := "Today: " + dateText(cal, today) + label
		if n := len(cal.Weekdays); n > 0 {
			if i := cal.WeekdayIndex(today.Y, today.M, today.D); i >= 0 && i < n {
				line += " (" + cal.Weekdays[i].Name + ")"
			}
		}
		if s := cal.SeasonForDate(today.M, today.D); s != nil {
			line += ", " + s.Name
		}
		if e := cal.EraForDate(today.Y, today.M, today.D); e != nil {
			line += ", in the era " + e.Name
		}
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "A day has %d hours.", cal.HoursPerDay)
	if cal.LeapYearEvery > 0 {
		fmt.Fprintf(&b, " A leap year comes every %d years (offset %d).", cal.LeapYearEvery, cal.LeapYearOffset)
	} else {
		b.WriteString(" No leap years.")
	}
	if cal.UsesRealTime() {
		b.WriteString(" This calendar follows the real-world date, so its date and months can't be changed.")
	}
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "### Months (%d, %d days a year)\n\n", len(cal.Months), cal.YearLength())
	for i, m := range cal.Months {
		line := fmt.Sprintf("%d. %s: %d days", i+1, m.Name, m.Days)
		if m.LeapYearDays > 0 {
			line += fmt.Sprintf(" (+%d in a leap year)", m.LeapYearDays)
		}
		if m.IsIntercalary {
			line += ", festival days outside the weeks"
		}
		if s := cal.SeasonForDate(i+1, 1); s != nil {
			line += ", starts in " + s.Name
		}
		b.WriteString(line + "\n")
	}
	if len(cal.Weekdays) > 0 {
		names := make([]string, len(cal.Weekdays))
		for i, w := range cal.Weekdays {
			names[i] = w.Name
			if w.IsRestDay {
				names[i] += " (rest day)"
			}
		}
		fmt.Fprintf(&b, "\n### Week (%d days)\n\n%s\n", len(names), strings.Join(names, ", "))
		if cal.MonthStartsNewWeek {
			b.WriteString("Every month starts a new week.\n")
		}
	}
	if len(cal.Seasons) > 0 {
		b.WriteString("\n### Seasons\n\n")
		for _, s := range cal.Seasons {
			fmt.Fprintf(&b, "- %s: %s %d to %s %d\n", s.Name, cal.MonthName(s.StartMonth), s.StartDay, cal.MonthName(s.EndMonth), s.EndDay)
		}
	}
	if len(cal.Moons) > 0 {
		b.WriteString("\n### Moons\n\n")
		for _, m := range cal.Moons {
			line := fmt.Sprintf("- %s: a %s-day cycle, phase offset %s, colour %s", m.Name, num(m.CycleDays), num(m.PhaseOffset), m.Color)
			if len(cal.Months) > 0 {
				nm, fm := nextPhases(cal, m, today)
				line += fmt.Sprintf("; today %s; next new moon %s, next full moon %s",
					strings.ToLower(m.MoonPhaseName(cal.AbsoluteDay(today.Y, today.M, today.D))), dateText(cal, nm), dateText(cal, fm))
			}
			if m.HiddenFromPlayers {
				line += " (hidden from players)"
			}
			b.WriteString(line + "\n")
		}
	}
	// Eras and event kinds are calendar structure only Directors see; the
	// service has already left them out for anyone else.
	if len(cal.Eras) > 0 {
		b.WriteString("\n### Eras\n\n")
		for _, e := range cal.Eras {
			line := fmt.Sprintf("- %s: from %s", e.Name, dateText(cal, calDate{e.StartYear, e.StartMonth, e.StartDay}))
			if e.EndYear != nil {
				if e.EndMonth != nil && e.EndDay != nil {
					line += " to " + dateText(cal, calDate{*e.EndYear, *e.EndMonth, *e.EndDay})
				} else {
					line += fmt.Sprintf(" to the end of %d", *e.EndYear)
				}
			} else {
				line += ", ongoing"
			}
			b.WriteString(line + "\n")
		}
	}
	if len(cal.Festivals) > 0 {
		b.WriteString("\n### Festivals\n\n")
		for _, f := range cal.Festivals {
			switch {
			case f.Month != nil && f.Day != nil:
				fmt.Fprintf(&b, "- %s: %s %d\n", f.Name, cal.MonthName(*f.Month), *f.Day)
			case f.AfterMonth != nil:
				fmt.Fprintf(&b, "- %s: after %s\n", f.Name, cal.MonthName(*f.AfterMonth))
			default:
				fmt.Fprintf(&b, "- %s\n", f.Name)
			}
		}
	}
	if len(cal.Cycles) > 0 {
		b.WriteString("\n### Cycles\n\n")
		for _, c := range cal.Cycles {
			names := make([]string, len(c.Entries))
			for i, e := range c.Entries {
				names[i] = e.Name
			}
			fmt.Fprintf(&b, "- %s (%s, every %d): %s\n", c.Name, c.Type, c.CycleLength, strings.Join(names, ", "))
		}
	}
	if len(cal.EventKinds) > 0 {
		names := make([]string, len(cal.EventKinds))
		for i, k := range cal.EventKinds {
			names[i] = k.Name
		}
		fmt.Fprintf(&b, "\n### Event kinds\n\n%s\n", strings.Join(names, ", "))
	}
	b.WriteString(run.climateText(cal))
	ans.Text = b.String()
	return ans, nil
}

// climateText is the calendar's weather settings, or "" when they can't be
// read; the rest of the calendar answer stands without them.
func (run *lookupRun) climateText(cal *calendar.Calendar) string {
	if run.l.Weather == nil {
		return ""
	}
	ws, err := run.l.Weather.GetWeatherSettings(run.ctx, cal.ID, run.campaignID, run.a.Viewer())
	if err != nil || ws == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n### Weather\n\n")
	fmt.Fprintf(&b, "Climate: %s (`%s`). How long weather lasts: %s on a 0 to 1 scale.\n", climateLabel(ws.Climate), ws.Climate, num(ws.Continuity))
	if ws.ForecastsEnabled {
		fmt.Fprintf(&b, "Players see a forecast %d days ahead.\n", ws.ForecastDays)
	} else {
		b.WriteString("Players see no forecast.\n")
	}
	if len(ws.Kinds) > 0 {
		names := make([]string, len(ws.Kinds))
		for i, k := range ws.Kinds {
			names[i] = k.Name
		}
		fmt.Fprintf(&b, "The owner's own kinds of weather: %s. Ask `what: weather-kinds` for details.\n", strings.Join(names, ", "))
	}
	return b.String()
}

func climateLabel(id string) string {
	for _, c := range calendar.WeatherClimates {
		if c.ID == id {
			return c.Name
		}
	}
	return id
}

// weatherKinds answers `what: weather-kinds`: every climate, built-in
// weather and sky effect, plus the owner's own kinds of weather.
func (run *lookupRun) weatherKinds() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Weather kinds"}
	var b strings.Builder
	b.WriteString("## Weather kinds\n\n### Climates\n\n")
	for _, c := range calendar.WeatherClimates {
		line := fmt.Sprintf("- `%s`: %s", c.ID, c.Name)
		if c.Magic {
			line += " (magical)"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n### Built-in weather (use the label as `label` on `kind: weather`)\n\n")
	labels := []string{}
	for _, p := range calendar.WeatherPresets() {
		labels = append(labels, p.Label)
	}
	b.WriteString(strings.Join(labels, ", ") + "\n")
	b.WriteString("\n### Sky effects the calendar animates\n\n")
	effects := []string{}
	for _, e := range calendar.WeatherEffects() {
		effects = append(effects, fmt.Sprintf("%s (`%s`)", e.Label, e.ID))
	}
	b.WriteString(strings.Join(effects, ", ") + "\n")

	if run.l.Cal != nil && run.l.Weather != nil {
		if cal, err := run.calendarOrErr(); err == nil {
			if ws, err := run.l.Weather.GetWeatherSettings(run.ctx, cal.ID, run.campaignID, run.a.Viewer()); err == nil && ws != nil && len(ws.Kinds) > 0 {
				b.WriteString("\n### The owner's own kinds of weather\n\n")
				for _, k := range ws.Kinds {
					line := fmt.Sprintf("- %s: like %s", k.Name, k.Like)
					if k.Look != nil && k.Look.Effect != "" {
						line += ", drawn as " + k.Look.Effect
					}
					if k.Magic {
						line += ", magical"
					}
					b.WriteString(line + "\n")
				}
			}
		}
	}
	ans.Text = b.String()
	return ans, nil
}

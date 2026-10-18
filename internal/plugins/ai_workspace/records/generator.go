package records

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// Chronicle's generators (static/js/widgets/chronicle_gen.js) run only in
// the browser. A generator row is planned here (what to run, over which
// days, with which seed), run by the review screen when the operator
// presses Import, and its output comes back with the commit. That output
// is treated like any other pasted input: it is re-validated against the
// plan's scope and written through the same services, so a tampered
// payload can do no more than the operator could by hand.

// Output caps per generator row.
const (
	maxGenWeatherDays = 400
	maxGenEvents      = 300
	maxGenNames       = 30 // the names engine's own per-kind maximum
)

// GeneratorKind runs one of Chronicle's generators. Weather, when set, gives
// a weather run the calendar's own settings, as the calendar's weather sheet
// does; without it a run uses the generator's defaults.
type GeneratorKind struct {
	Cal     CalendarAPI
	Tables  TableKind
	Weather WeatherSettingsAPI
	// Rolls runs the `rolls` generator; without it that generator is refused.
	Rolls EventRollAPI
}

// Weather tuning a row may set, with the generator's own bounds.
var genWeatherNudges = []struct {
	key      string
	min, max float64
}{
	{"continuity", 0, 1},
	{"warmer", -15, 15},
	{"wetter", -1, 1},
	{"windier", -1, 1},
}

func climateIDs() string {
	ids := make([]string, len(calendar.WeatherClimates))
	for i, c := range calendar.WeatherClimates {
		ids[i] = c.ID
	}
	return strings.Join(ids, ", ")
}

func knownClimate(id string) bool {
	for _, c := range calendar.WeatherClimates {
		if c.ID == id {
			return true
		}
	}
	return false
}

func (GeneratorKind) Name() string  { return "generator" }
func (GeneratorKind) Label() string { return "Generator" }
func (GeneratorKind) Doc() string {
	return "Asks Chronicle's own generators to make something, instead of writing it yourself. Use `action: create`. Keys by `generator`:\n" +
		"- `weather`: weather for days of the default calendar, from `year`/`month`/`day` to `end_year`/`end_month`/`end_day`. It follows the calendar's own seasons, climate, how long its weather lasts, and the owner's own kinds of weather. Optional, for this run only: `climate` (" + climateIDs() + "), `continuity` (0 changes every day, 1 long spells), `warmer` (-15 to 15 degrees C), `wetter` and `windier` (-1 to 1). Days set by hand or locked are kept.\n" +
		"- `events`: festivals and other calendar events for one `year`.\n" +
		"- `sky`: sky events (eclipses, meteor showers, comets) for one `year`.\n" +
		"- `names`: `count` names (up to 30) of `names` kind `people` or `places`, in a `theme` (pastoral, nautical, dwarven, elven, desert, imperial, fey, grim), added to the rolling table `table`.\n" +
		"- `rolls` (owner or co-DM): rolls every night of one `year` for the `event` (by name) that has a `roll:`, on Chronicle's own server roller. Locked nights keep their result (`keep_locked: true`, the only choice), and a night that has passed keeps a result it already has. `only_unrolled: true` rolls just the nights with no result yet.\n\n" +
		"```\n---\nkind: generator\nname: Spring weather\ngenerator: weather\nyear: 1492\nmonth: 3\nday: 1\nend_year: 1492\nend_month: 3\nend_day: 30\nclimate: temperate\n---\n```"
}

// genPlan is what the browser runs, and what Apply checks the output against.
type genPlan struct {
	Generator string          `json:"generator"`
	Calendar  json.RawMessage `json:"calendar,omitempty"`
	Recipe    map[string]any  `json:"recipe"`
	Scope     map[string]any  `json:"scope,omitempty"`
	Seed      string          `json:"seed"`
	NamesKind string          `json:"namesKind,omitempty"`
	// Kinds is the owner's own kinds of weather, which the run may make.
	Kinds []calendar.WeatherKind `json:"kinds,omitempty"`
}

type ymd struct{ Year, Month, Day int }

func (d ymd) before(o ymd) bool {
	if d.Year != o.Year {
		return d.Year < o.Year
	}
	if d.Month != o.Month {
		return d.Month < o.Month
	}
	return d.Day < o.Day
}

// countDays counts the days from..to inclusive on the calendar, stopping
// once it passes limit so a far-off end year costs nothing. The plan's
// range limit is this count because Apply refuses more output than the cap.
func countDays(cal *calendar.Calendar, from, to ymd, limit int) int {
	n := 0
	for y, m := from.Year, from.Month; !to.before(ymd{y, m, 1}); {
		first, last := 1, cal.MonthDays(m-1, y)
		if y == from.Year && m == from.Month {
			first = from.Day
		}
		if y == to.Year && m == to.Month {
			last = to.Day
		}
		n += last - first + 1
		if n > limit {
			return n
		}
		if m++; m > len(cal.Months) {
			m, y = 1, y+1
		}
	}
	return n
}

func (k GeneratorKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (genPlan, *calendar.Calendar, string, error) {
	g := strings.ToLower(r.Str("generator"))
	p := genPlan{Generator: g, Seed: r.Str("seed")}
	if p.Seed == "" {
		// The seed is fixed at review time, so what runs on Import is the
		// same run every time the page is submitted.
		var b [6]byte
		_, _ = rand.Read(b[:])
		p.Seed = hex.EncodeToString(b[:])
	}
	if r.Action != ActionCreate {
		return p, nil, "", apperror.NewBadRequest("generators only add things; use action: create")
	}
	switch g {
	case "names":
		if !a.CanAuthorDmOnly() {
			return p, nil, "", apperror.NewBadRequest("only the owner or a co-DM can change rolling tables")
		}
		n, ok, err := r.Int("count")
		if err != nil || !ok || n < 1 || n > maxGenNames {
			return p, nil, "", badRequestf("count must be 1 to %d", maxGenNames)
		}
		kind := r.Str("names")
		if kind == "" {
			kind = "people"
		}
		if kind != "people" && kind != "places" {
			return p, nil, "", apperror.NewBadRequest("names must be people or places")
		}
		if r.Str("table") == "" {
			return p, nil, "", apperror.NewBadRequest("say which rolling table gets the names (`table`)")
		}
		makes := map[string]any{"months": 0, "weekdays": 0, "festivals": 0, "eras": 0, "moons": 0, "seasons": 0, "places": 0, "people": 0}
		makes[kind] = n
		details := map[string]any{}
		if t := r.Str("theme"); t != "" {
			details["theme"] = t
		}
		p.Recipe = map[string]any{"generator": "names", "makes": makes, "details": details}
		p.NamesKind = kind
		return p, nil, fmt.Sprintf("%d %s names into the table %s", n, kind, quote(r.Str("table"))), nil
	case "weather", "events", "sky":
	default:
		return p, nil, "", apperror.NewBadRequest("generator must be weather, events, sky, names or rolls")
	}
	if g == "weather" && !a.CanAuthorDmOnly() {
		return p, nil, "", apperror.NewBadRequest("only the owner or a co-DM can set weather")
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return p, nil, "", err
	}
	cj, err := json.Marshal(cal)
	if err != nil {
		return p, nil, "", err
	}
	p.Calendar = cj
	p.Recipe = map[string]any{"generator": g}
	if g == "weather" {
		y, m, d, ok, err := readDate(cal, r, "")
		if err != nil || !ok {
			return p, nil, "", apperror.NewBadRequest("weather needs year, month and day to start from")
		}
		ey, em, ed, eok, err := readDate(cal, r, "end_")
		if err != nil {
			return p, nil, "", err
		}
		if !eok {
			ey, em, ed = y, m, cal.MonthDays(m-1, y)
		}
		from, to := ymd{y, m, d}, ymd{ey, em, ed}
		if to.before(from) {
			return p, nil, "", apperror.NewBadRequest("the end date is before the start date")
		}
		if n := countDays(cal, from, to, maxGenWeatherDays); n > maxGenWeatherDays {
			return p, nil, "", badRequestf("generate at most %d days of weather at a time", maxGenWeatherDays)
		}
		details, climate, err := k.weatherDetails(ctx, campaignID, a, cal, r, &p)
		if err != nil {
			return p, nil, "", err
		}
		p.Recipe["details"] = details
		p.Scope = map[string]any{"range": map[string]any{
			"from": map[string]int{"year": y, "month": m, "day": d},
			"to":   map[string]int{"year": ey, "month": em, "day": ed},
		}}
		return p, cal, "weather from " + dateLabel(cal, y, m, d) + " to " + dateLabel(cal, ey, em, ed) + climate + ", kept off days set by hand or locked", nil
	}
	y, ok, err := r.Int("year")
	if err != nil || !ok {
		return p, nil, "", badRequestf("%s needs a year", g)
	}
	p.Scope = map[string]any{"year": y}
	what := "festivals and events"
	if g == "sky" {
		what = "sky events"
	}
	return p, cal, fmt.Sprintf("%s for the year %d on %s", what, y, cal.Name), nil
}

// weatherDetails is a weather run's recipe details: the row's own values,
// else the calendar's settings. climate is the summary's wording for them.
func (k GeneratorKind) weatherDetails(ctx context.Context, campaignID string, a Actor, cal *calendar.Calendar, r Record, p *genPlan) (map[string]any, string, error) {
	details := map[string]any{}
	var ws *calendar.WeatherSettings
	if k.Weather != nil {
		// Unreadable settings leave the generator's defaults, as on the
		// calendar's weather sheet.
		ws, _ = k.Weather.GetWeatherSettings(ctx, cal.ID, campaignID, a.Viewer())
	}
	from := " (the calendar's own)"
	if ws != nil {
		if knownClimate(ws.Climate) {
			details["climate"] = ws.Climate
		}
		if ws.Continuity >= 0 && ws.Continuity <= 1 {
			details["continuity"] = ws.Continuity
		}
		p.Kinds = ws.Kinds
	}
	if c := strings.ToLower(r.Str("climate")); c != "" {
		if !knownClimate(c) {
			return nil, "", badRequestf("climate: %q is not one of %s", c, climateIDs())
		}
		details["climate"] = c
		from = ""
	}
	for _, n := range genWeatherNudges {
		v, ok, err := r.Float(n.key)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			continue
		}
		if v < n.min || v > n.max {
			return nil, "", badRequestf("%s must be from %v to %v", n.key, n.min, n.max)
		}
		details[n.key] = v
	}
	c, ok := details["climate"].(string)
	if !ok {
		return details, "", nil
	}
	return details, " in the " + climateLabel(c) + " climate" + from, nil
}

// rollsRow is a checked rolls row. It runs on the server at Import, so it
// hands the browser nothing to run.
type rollsRow struct {
	cal   *calendar.Calendar
	ev    *calendar.Event // nil until the event exists
	year  int
	only  bool
	what  string
	noRol bool // the event exists but has no roll yet
}

// planRolls checks a rolls row. The event may be made, or given its roll,
// by an earlier row of the same paste, so at review a missing event or roll
// is a warning and Apply looks again.
func (k GeneratorKind) planRolls(ctx context.Context, campaignID string, a Actor, r Record) (rollsRow, error) {
	var row rollsRow
	if r.Action != ActionCreate {
		return row, apperror.NewBadRequest("generators only add things; use action: create")
	}
	if !a.CanAuthorDmOnly() {
		return row, apperror.NewBadRequest("only the owner or a co-DM can roll an event's nights")
	}
	if k.Rolls == nil {
		return row, apperror.NewBadRequest("event rolls aren't available on this server")
	}
	if v, ok := r.Bool("keep_locked"); ok && !v {
		return row, apperror.NewBadRequest("locked nights always keep their result; unlock them first (kind: roll_night)")
	}
	if r.Str("event") == "" {
		return row, apperror.NewBadRequest("say which event to roll (`event`)")
	}
	y, ok, err := r.Int("year")
	if err != nil || !ok {
		return row, apperror.NewBadRequest("rolls needs a year")
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return row, err
	}
	row.cal, row.year = cal, y
	row.only, _ = r.Bool("only_unrolled")
	row.what = "every night"
	if row.only {
		row.what = "every night with no result yet"
	}
	ev, err := rollEvent(ctx, k.Cal, cal, campaignID, a, r.Str("event"))
	if err != nil {
		return row, nil
	}
	row.ev = ev
	got, err := k.Rolls.GetEventRoll(ctx, ev.ID, cal.ID, campaignID, y, a.Viewer())
	if err != nil {
		return row, err
	}
	row.noRol = got.Config == nil
	return row, nil
}

func (k GeneratorKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if strings.EqualFold(r.Str("generator"), "rolls") {
		row, err := k.planRolls(ctx, campaignID, a, r)
		if err != nil {
			return Plan{Error: planError(err)}
		}
		p := Plan{Summary: fmt.Sprintf("rolls %s of %s in %d, keeping locked nights.", row.what, r.Str("event"), row.year)}
		if row.ev == nil {
			p.Warnings = append(p.Warnings, "No single event called "+quote(r.Str("event"))+" yet; an earlier block must add it with a roll")
		} else if row.noRol {
			p.Warnings = append(p.Warnings, quote(row.ev.Name)+" has no roll yet; an earlier block must give it `roll:`")
		}
		return p
	}
	p, _, summary, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	cj, err := json.Marshal(p)
	if err != nil {
		return Plan{Error: "could not prepare the generator"}
	}
	return Plan{Summary: summary + ". Runs when you press Import.", Client: string(cj)}
}

// genEvent is the subset of a generated event that is saved.
type genEvent struct {
	Name        string  `json:"name"`
	Year        int     `json:"year"`
	Month       int     `json:"month"`
	Day         int     `json:"day"`
	EndYear     *int    `json:"end_year"`
	EndMonth    *int    `json:"end_month"`
	EndDay      *int    `json:"end_day"`
	Description *string `json:"description"`
	Visibility  string  `json:"visibility"`
	Color       *string `json:"color"`
	Icon        *string `json:"icon"`
	AllDay      bool    `json:"all_day"`
	StartHour   *int    `json:"start_hour"`
	StartMinute *int    `json:"start_minute"`
	Recurring   bool    `json:"is_recurring"`
	RecurType   *string `json:"recurrence_type"`
}

// genDay is one generated weather day: a date plus WeatherInput fields.
type genDay struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
	calendar.WeatherInput
}

func (k GeneratorKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if strings.EqualFold(r.Str("generator"), "rolls") {
		row, err := k.planRolls(ctx, campaignID, a, r)
		if err != nil {
			return err
		}
		if row.ev == nil {
			_, err := rollEvent(ctx, k.Cal, row.cal, campaignID, a, r.Str("event"))
			return err
		}
		_, err = k.Rolls.RollEventNights(ctx, row.ev.ID, row.cal.ID, campaignID,
			calendar.RollNightsInput{Year: row.year, OnlyUnrolled: row.only}, a.Viewer())
		return err
	}
	p, cal, _, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(r.Generated) == "" {
		return apperror.NewBadRequest("the generator did not run in your browser; try Import again")
	}
	switch p.Generator {
	case "names":
		var names []string
		if err := json.Unmarshal([]byte(r.Generated), &names); err != nil {
			return apperror.NewBadRequest("the generator's names could not be read")
		}
		if len(names) == 0 || len(names) > maxGenNames {
			return badRequestf("the generator returned %d names", len(names))
		}
		es := make([]Entry, 0, len(names))
		for _, n := range names {
			if n = strings.TrimSpace(n); n != "" {
				es = append(es, Entry{Name: n, Weight: 1})
			}
		}
		return k.Tables.appendEntries(ctx, campaignID, a, r.Str("table"), es)
	case "weather":
		var days []genDay
		if err := json.Unmarshal([]byte(r.Generated), &days); err != nil {
			return apperror.NewBadRequest("the generated weather could not be read")
		}
		if len(days) == 0 || len(days) > maxGenWeatherDays {
			return badRequestf("the generator returned %d days", len(days))
		}
		rng := p.Scope["range"].(map[string]any)
		from := rng["from"].(map[string]int)
		to := rng["to"].(map[string]int)
		lo, hi := ymd{from["year"], from["month"], from["day"]}, ymd{to["year"], to["month"], to["day"]}
		in := make([]calendar.DayWeatherInput, 0, len(days))
		for _, d := range days {
			at := ymd{d.Year, d.Month, d.Day}
			if at.before(lo) || hi.before(at) {
				return apperror.NewBadRequest("the generator returned a day outside the range")
			}
			in = append(in, calendar.DayWeatherInput{Year: d.Year, Month: d.Month, Day: d.Day, Source: calendar.WeatherSourceGenerated, WeatherInput: d.WeatherInput})
		}
		return k.Cal.SetDayWeather(ctx, cal.ID, campaignID, in)
	default:
		var evs []genEvent
		if err := json.Unmarshal([]byte(r.Generated), &evs); err != nil {
			return apperror.NewBadRequest("the generated events could not be read")
		}
		if len(evs) > maxGenEvents {
			return badRequestf("the generator returned %d events", len(evs))
		}
		year := p.Scope["year"].(int)
		// A rerun after a partial failure must not add the events that
		// already went in, so same-name same-day events are skipped.
		existing, err := k.Cal.ListEventsForCalendar(ctx, campaignID, cal.ID, a.Role)
		if err != nil {
			return apperror.NewBadRequest("could not read the calendar's events")
		}
		have := make(map[string]bool, len(existing))
		for _, ex := range existing {
			have[eventKey(ex.Name, ex.Year, ex.Month, ex.Day)] = true
		}
		var failed, added, skipped int
		for _, e := range evs {
			if e.Year != year || strings.TrimSpace(e.Name) == "" {
				failed++
				continue
			}
			if have[eventKey(e.Name, e.Year, e.Month, e.Day)] {
				skipped++
				continue
			}
			if e.Visibility != "dm_only" {
				e.Visibility = "everyone"
			}
			var desc *string
			if e.Description != nil {
				if desc, err = bodyHTML(*e.Description, nil); err != nil {
					failed++
					continue
				}
			}
			_, err := k.Cal.CreateEvent(ctx, cal.ID, campaignID, calendar.CreateEventInput{
				Name: e.Name, Year: e.Year, Month: e.Month, Day: e.Day,
				EndYear: e.EndYear, EndMonth: e.EndMonth, EndDay: e.EndDay,
				DescriptionHTML: desc, Visibility: e.Visibility, Color: e.Color, Icon: e.Icon, AllDay: e.AllDay,
				StartHour: e.StartHour, StartMinute: e.StartMinute, IsRecurring: e.Recurring, RecurrenceType: e.RecurType,
				CreatedBy: a.UserID, CanAuthorDmOnly: a.CanAuthorDmOnly(), Author: a.Viewer(),
			})
			if err != nil {
				failed++
				continue
			}
			added++
		}
		if failed > 0 {
			return badRequestf("%d of %d generated events could not be added (%d were added, %d were already there). Running it again only adds the missing ones", failed, len(evs), added, skipped)
		}
		return nil
	}
}

func (GeneratorKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

func eventKey(name string, y, m, d int) string {
	return fmt.Sprintf("%s|%d-%d-%d", strings.ToLower(strings.TrimSpace(name)), y, m, d)
}

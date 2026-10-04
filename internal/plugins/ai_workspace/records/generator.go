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

// GeneratorKind runs one of Chronicle's generators.
type GeneratorKind struct {
	Cal    CalendarAPI
	Tables TableKind
}

func (GeneratorKind) Name() string  { return "generator" }
func (GeneratorKind) Label() string { return "Generator" }
func (GeneratorKind) Doc() string {
	return "Asks Chronicle's own generators to make something, instead of writing it yourself. Use `action: create`. Keys by `generator`:\n" +
		"- `weather`: weather for days of the default calendar, from `year`/`month`/`day` to `end_year`/`end_month`/`end_day`; optional `climate` (temperate, cold-coast, desert, tropical, highland, mediterranean, tundra, fey-wilds, ashlands, gloomfen). Days set by hand or locked are kept.\n" +
		"- `events`: festivals and other calendar events for one `year`.\n" +
		"- `sky`: sky events (eclipses, meteor showers, comets) for one `year`.\n" +
		"- `names`: `count` names (up to 30) of `names` kind `people` or `places`, in a `theme` (pastoral, nautical, dwarven, elven, desert, imperial, fey, grim), added to the rolling table `table`.\n\n" +
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
		return p, nil, "", apperror.NewBadRequest("generator must be weather, events, sky or names")
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
		if to.Year-from.Year > 1 {
			return p, nil, "", apperror.NewBadRequest("generate at most a year of weather at a time")
		}
		if c := r.Str("climate"); c != "" {
			p.Recipe["details"] = map[string]any{"climate": c}
		}
		p.Scope = map[string]any{"range": map[string]any{
			"from": map[string]int{"year": y, "month": m, "day": d},
			"to":   map[string]int{"year": ey, "month": em, "day": ed},
		}}
		return p, cal, "weather from " + dateLabel(cal, y, m, d) + " to " + dateLabel(cal, ey, em, ed) + ", kept off days set by hand or locked", nil
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

func (k GeneratorKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _, summary, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: err.Error()}
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
				es = append(es, Entry{Name: n})
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
		var failed int
		for _, e := range evs {
			if e.Year != year || strings.TrimSpace(e.Name) == "" {
				failed++
				continue
			}
			if e.Visibility != "dm_only" {
				e.Visibility = "everyone"
			}
			var desc *string
			if e.Description != nil {
				if desc, err = bodyHTML(*e.Description); err != nil {
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
			}
		}
		if failed > 0 {
			return badRequestf("%d of %d generated events could not be added", failed, len(evs))
		}
		return nil
	}
}

func (GeneratorKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

package calendarai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// Weather by place. A page whose Climate field is set keeps its own weather,
// and the pages inside it share it; the party's moves say whose weather the
// party has each day. A place's climate is a page field, so the AI sets it
// the way it edits any page; this file is the rest: the lookup, the party's
// move, and the generator's place key below.

// KindPartyPlace is the `kind:` that moves the party.
const KindPartyPlace = "party-place"

// whatPlaceWeather is the lookup of places' climates and weather.
const whatPlaceWeather = "place-weather"

// PartyPlaceKind says where the party is from a day on. Their weather, and
// all players see of the weather, is that place's from then until the next
// move. Directors only, like any weather write.
type PartyPlaceKind struct {
	Cal    CalendarAPI
	Places calendar.PlaceWeatherService
	Pages  PageResolver
}

var _ aiplug.Briefed = PartyPlaceKind{}

func (PartyPlaceKind) Name() string  { return KindPartyPlace }
func (PartyPlaceKind) Label() string { return "Where the party is" }
func (PartyPlaceKind) Brief() string {
	return "where the party is from a day on, which sets the weather players see"
}
func (PartyPlaceKind) Doc() string {
	return "Where the party is from a day on, on the default calendar. Their weather, and all players see of the weather, is that place's until the next move: the weather of the nearest page at or above it with a Climate set, or the world's. Keys: `place` (a page name, or `everywhere else` for the world's weather); optional `year`, `month`, `day` (default today). `action: delete` takes back the move on that day. A place's climate itself is the page's Climate field: change it as you change any page.\n\n```\n---\nkind: party-place\nplace: Saltreach\nyear: 1492\nmonth: Hammer\nday: 8\n---\n```"
}

// partyPlaceKeys is every key the party-place block reads.
var partyPlaceKeys = keySet("place", "year", "month", "day")

// partyMove is a checked party-place row.
type partyMove struct {
	cal     *calendar.Calendar
	day     calendar.DayDate
	placeID string
	what    string
}

// everywhereElse reports a place that means "the world's weather".
func everywhereElse(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "everywhere else", "none", "world", "the world":
		return true
	}
	return false
}

func (k PartyPlaceKind) check(ctx context.Context, campaignID string, a Actor, r Record) (partyMove, error) {
	var mv partyMove
	if !a.CanAuthorDmOnly() {
		return mv, apperror.NewBadRequest("only the owner or a co-DM can move the party")
	}
	if k.Places == nil {
		return mv, apperror.NewBadRequest("weather by place isn't available on this server")
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return mv, err
	}
	mv.cal = cal
	y, m, d, dated, err := readDate(cal, r, "")
	if err != nil {
		return mv, err
	}
	if !dated {
		y, m, d = cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay
	}
	mv.day = calendar.DayDate{Year: y, Month: m, Day: d}
	when := dateLabel(cal, y, m, d)
	if r.Action == ActionDelete {
		mv.what = "takes back the party's move on " + when
		return mv, nil
	}
	name := strings.TrimSpace(r.Str("place"))
	if name == "" {
		return mv, apperror.NewBadRequest("say where the party is (`place`), or `everywhere else` for the world's weather")
	}
	if everywhereElse(name) {
		mv.what = "the party is somewhere with the world's weather from " + when
		return mv, nil
	}
	if k.Pages == nil {
		return mv, apperror.NewBadRequest("pages can't be looked up on this server")
	}
	id, err := k.Pages.OpenablePage(ctx, campaignID, name, a)
	if err != nil {
		return mv, err
	}
	if id == "" {
		return mv, badRequestf("place: no page called %s", quote(name))
	}
	mv.placeID = id
	mv.what = "the party is at " + name + " from " + when
	return mv, nil
}

func (k PartyPlaceKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	mv, err := k.check(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	return Plan{Summary: mv.what, Warnings: unknownKeyWarnings(r, partyPlaceKeys, KindPartyPlace)}
}

func (k PartyPlaceKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	mv, err := k.check(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		return k.Places.ClearPartyPlace(ctx, mv.cal.ID, campaignID, mv.day)
	}
	return k.Places.SetPartyPlace(ctx, mv.cal.ID, campaignID, mv.day, mv.placeID)
}

// Export is empty: the moves are read with `what: place-weather`.
func (PartyPlaceKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// placeWeatherLookup is the place-weather lookup's entry in All.
func (l *Lookups) placeWeatherLookup() aiplug.Lookup {
	return calLookup{l, aiplug.LookupInfo{
		What: whatPlaceWeather, Aliases: []string{"weather-by-place", "party-place"},
		Keys:  "`from` and `to`, or `year` and `month`",
		Brief: "where the party is; for the Director, each place's climate and weather by day",
		Tool: aiplug.Tool{Name: "place_weather", Title: "Weather by place", Did: "Looked up the weather by place",
			Desc: "Where the party is and its climate; for the Director, every place's climate, the party's moves and each place's weather in a range of dates.", Args: dateArgs},
	}, "Weather by place", (*lookupRun).placeWeather}
}

// placeWeather answers `what: place-weather`. A player is told only where
// the party is (a page they may open, by name) and its climate: their day
// weather is already the party's, through `what: weather`.
func (run *lookupRun) placeWeather(r Record) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Weather by place"}
	cal, err := run.calendarOrErr()
	if err != nil {
		return ans, err
	}
	if run.l.Places == nil {
		ans.Text = "## Weather by place\n\nThis server keeps one weather for the whole world.\n"
		return ans, nil
	}
	var b strings.Builder
	b.WriteString("## Weather by place\n\n")
	if now := run.partyNowText(cal); now != "" {
		b.WriteString(now)
	} else {
		b.WriteString("The party has not been placed anywhere, so it has the world's weather.\n")
	}
	if !run.director() {
		b.WriteString("\nThe party's weather by day is `what: weather`.\n")
		ans.Text = b.String()
		return ans, nil
	}
	from, to, err := dateRange(cal, r)
	if err != nil {
		return ans, err
	}
	ms, err := months(cal, from, to)
	if err != nil {
		return ans, err
	}
	years := map[int]*calendar.PlaceWeatherYear{}
	for _, m := range ms {
		if years[m.Y] != nil {
			continue
		}
		pw, err := run.l.Places.PlaceWeatherYear(run.ctx, cal.ID, run.campaignID, m.Y)
		if err != nil {
			return ans, err
		}
		years[m.Y] = pw
	}
	first := years[ms[0].Y]
	fmt.Fprintf(&b, "\nWorld climate: %s. A place with no Climate of its own, and no page above it with one, keeps it.\n", first.World.Name)
	if len(first.Places) == 0 {
		b.WriteString("\nNo page has a Climate of its own, so the whole world shares one weather. A place's climate is its page's Climate field.\n")
		ans.Text = b.String()
		return ans, nil
	}
	b.WriteString("\n### Places with a climate of their own (pages inside each share it)\n\n")
	for _, p := range first.Places {
		fmt.Fprintf(&b, "- %s: %s (`%s`)\n", p.Name, p.ClimateName, p.Climate)
	}
	if len(first.Party) > 0 {
		b.WriteString("\n### The party's moves (each holds until the next)\n\n")
		for _, mk := range first.Party {
			where := "everywhere else (the world's weather)"
			if mk.PlaceID != "" {
				where = mk.PlaceName
				if where == "" {
					where = "a page that no longer exists"
				}
			}
			fmt.Fprintf(&b, "- %s: %s\n", dateText(cal, calDate{mk.Year, mk.Month, mk.Day}), where)
		}
	}
	fmt.Fprintf(&b, "\n### Each place's weather, %s\n", rangeLabel(cal, from, to))
	n := 0
	for _, p := range first.Places {
		var lines []string
		for _, y := range sortedYears(years) {
			for _, d := range years[y].Days[p.ID] {
				at := calDate{d.Year, d.Month, d.Day}
				if at.Before(from) || to.Before(at) {
					continue
				}
				lines = append(lines, "- "+dateText(cal, at)+": "+weatherText(d))
			}
		}
		n += len(lines)
		fmt.Fprintf(&b, "\n#### %s\n\n", p.Name)
		if len(lines) == 0 {
			b.WriteString("No day in this range has weather yet.\n")
		}
		aiplug.WriteLines(&b, lines)
	}
	ans.Chip = fmt.Sprintf("Weather by place · %d places · %d days", len(first.Places), n)
	ans.Text = b.String()
	return ans, nil
}

func sortedYears(m map[int]*calendar.PlaceWeatherYear) []int {
	out := make([]int, 0, len(m))
	for y := range m {
		out = append(out, y)
	}
	sort.Ints(out)
	return out
}

// partyNowText is where the party is today, naming only pages the Actor may
// open; with no page it may name, only the party's climate, so a hidden
// place is never confirmed. "" when the party was never placed or it can't
// be read; the rest of an answer stands without it.
func (run *lookupRun) partyNowText(cal *calendar.Calendar) string {
	if run.l.Places == nil {
		return ""
	}
	now, err := run.l.Places.PartyNow(run.ctx, cal.ID, run.campaignID, run.a.Viewer())
	if err != nil || now == nil || now.Since == nil {
		return ""
	}
	since := " since " + dateText(cal, calDate{now.Since.Year, now.Since.Month, now.Since.Day})
	switch {
	case now.PlaceName != "" && now.RegionName != "" && now.RegionName != now.PlaceName:
		return fmt.Sprintf("Today the party is at %s%s, which shares %s's weather; its climate is %s.\n", now.PlaceName, since, now.RegionName, now.ClimateName)
	case now.PlaceName != "":
		return fmt.Sprintf("Today the party is at %s%s; its climate is %s.\n", now.PlaceName, since, now.ClimateName)
	case now.RegionName != "":
		return fmt.Sprintf("Today the party is in %s%s; its climate is %s.\n", now.RegionName, since, now.ClimateName)
	}
	return fmt.Sprintf("Today the party's climate is %s%s.\n", now.ClimateName, since)
}

// placeZone is a generator row's `place`: the page with a climate of its
// own that the run makes weather for, as the generator's zone.
func (k GeneratorKind) placeZone(ctx context.Context, campaignID string, cal *calendar.Calendar, year int, name string) (*calendar.ClimatePlace, error) {
	if k.Places == nil {
		return nil, apperror.NewBadRequest("weather by place isn't available on this server")
	}
	pw, err := k.Places.PlaceWeatherYear(ctx, cal.ID, campaignID, year)
	if err != nil {
		return nil, err
	}
	for i := range pw.Places {
		if sameName(pw.Places[i].Name, name) {
			return &pw.Places[i], nil
		}
	}
	return nil, badRequestf("place: %s has no Climate of its own; set one on its page first (a page inside a place with a climate shares that place's weather)", quote(name))
}

package calendarai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// fakePlaces is the calendar's weather by place as the AI reaches it. now
// answers PartyNow per viewer the way the service does: names only for a
// Director, or the pages a player may open (here: Saltreach only).
type fakePlaces struct {
	calendar.PlaceWeatherService
	year     calendar.PlaceWeatherYear
	moves    []calendar.PartyMark
	cleared  []calendar.DayDate
	written  map[string][]calendar.DayWeatherInput
	partyAt  string // the page the party is at today
	yearAsks int
}

func (f *fakePlaces) PlaceWeatherYear(context.Context, string, string, int) (*calendar.PlaceWeatherYear, error) {
	f.yearAsks++
	y := f.year
	return &y, nil
}
func (f *fakePlaces) SetPartyPlace(_ context.Context, _, _ string, d calendar.DayDate, placeID string) error {
	f.moves = append(f.moves, calendar.PartyMark{Year: d.Year, Month: d.Month, Day: d.Day, PlaceID: placeID})
	return nil
}
func (f *fakePlaces) ClearPartyPlace(_ context.Context, _, _ string, d calendar.DayDate) error {
	f.cleared = append(f.cleared, d)
	return nil
}
func (f *fakePlaces) SetPlaceDayWeather(_ context.Context, _, _, placeID string, days []calendar.DayWeatherInput) error {
	if f.written == nil {
		f.written = map[string][]calendar.DayWeatherInput{}
	}
	f.written[placeID] = append(f.written[placeID], days...)
	return nil
}
func (f *fakePlaces) PartyNow(_ context.Context, _, _ string, v permissions.Viewer) (*calendar.PartyNow, error) {
	since := &calendar.DayDate{Year: 1492, Month: 1, Day: 2}
	switch f.partyAt {
	case "p-salt":
		return &calendar.PartyNow{PlaceName: "Saltreach", RegionName: "Saltreach", ClimateName: "Cold coast", Since: since}, nil
	case "p-hollow":
		if v.SkipsPerUserRules() {
			return &calendar.PartyNow{PlaceName: "The Hidden Hollow", RegionName: "The Hidden Hollow", ClimateName: "Desert", Since: since}, nil
		}
		return &calendar.PartyNow{ClimateName: "Desert", Since: since}, nil
	}
	return &calendar.PartyNow{ClimateName: "Temperate"}, nil
}

func label(s string) *string { return &s }

func placesWorld(partyAt string) *fakePlaces {
	return &fakePlaces{
		partyAt: partyAt,
		year: calendar.PlaceWeatherYear{
			World: calendar.ClimateRef{ID: "temperate", Name: "Temperate"},
			Places: []calendar.ClimatePlace{
				{ID: "p-salt", Name: "Saltreach", Climate: "cold-coast", ClimateName: "Cold coast"},
				{ID: "p-hollow", Name: "The Hidden Hollow", Climate: "desert", ClimateName: "Desert"},
			},
			Party: []calendar.PartyMarkView{{PartyMark: calendar.PartyMark{Year: 1492, Month: 1, Day: 2, PlaceID: "p-hollow"}, PlaceName: "The Hidden Hollow", RegionID: "p-hollow"}},
			Days: map[string][]calendar.DayWeather{
				"p-salt":   {{Year: 1492, Month: 1, Day: 3, PresetLabel: label("Sea fog")}},
				"p-hollow": {{Year: 1492, Month: 1, Day: 3, PresetLabel: label("Sandstorm")}},
			},
		},
	}
}

func placeLookups(pl *fakePlaces) *Lookups {
	c := newCal()
	c.cal.CurrentYear, c.cal.CurrentMonth, c.cal.CurrentDay = 1492, 1, 5
	return &Lookups{Cal: c, Places: pl}
}

func TestPlaceWeatherLookup_DirectorSeesEveryPlace(t *testing.T) {
	ans := placeLookups(placesWorld("p-hollow")).Answer(context.Background(), camp, owner,
		[]Record{lookup(whatPlaceWeather, map[string]any{"year": 1492, "month": "Hammer"})})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"Today the party is at The Hidden Hollow since Hammer 2 1492", "World climate: Temperate",
		"- Saltreach: Cold coast (`cold-coast`)", "- The Hidden Hollow: Desert", "The party's moves",
		"Sea fog", "Sandstorm",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
}

// A player is told where the party is only by a page they may open, and
// never any place's weather: theirs is already the party's, by `weather`.
func TestPlaceWeatherLookup_PlayerSeesOnlyTheParty(t *testing.T) {
	tests := []struct {
		name, partyAt string
		want          []string
	}{
		{"at a page they may open", "p-salt", []string{"Today the party is at Saltreach", "Cold coast"}},
		{"at a hidden place", "p-hollow", []string{"Today the party's climate is Desert"}},
		{"never placed", "", []string{"has not been placed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := placesWorld(tt.partyAt)
			ans := placeLookups(pl).Answer(context.Background(), camp, player, []Record{lookup("weather-by-place", nil)})[0]
			if ans.Error != "" {
				t.Fatal(ans.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans.Text)
				}
			}
			for _, leak := range []string{"Hidden Hollow", "Sandstorm", "Sea fog", "moves"} {
				if strings.Contains(ans.Text, leak) {
					t.Errorf("player answer leaks %q:\n%s", leak, ans.Text)
				}
			}
			if pl.yearAsks != 0 {
				t.Error("a player's answer read every place's weather")
			}
		})
	}
}

func TestCalendarLookup_SaysWhereThePartyIs(t *testing.T) {
	ans := placeLookups(placesWorld("p-salt")).Answer(context.Background(), camp, player, []Record{lookup(WhatCalendar, nil)})[0]
	if !strings.Contains(ans.Text, "Today the party is at Saltreach") || !strings.Contains(ans.Text, "place-weather") {
		t.Errorf("calendar answer lacks the party:\n%s", ans.Text)
	}
	none := placeLookups(placesWorld("")).Answer(context.Background(), camp, player, []Record{lookup(WhatCalendar, nil)})[0]
	if strings.Contains(none.Text, "party") {
		t.Errorf("a party never placed still gets a line:\n%s", none.Text)
	}
}

func TestPartyPlaceKind(t *testing.T) {
	pages := fakePageResolver{
		pages:  map[string]*fakePage{"gull-rock": {ID: "p-gull", Name: "Gull Rock"}, "secret-cove": {ID: "p-cove", Name: "Secret Cove"}},
		hidden: map[string]bool{"p-cove": true},
	}
	tests := []struct {
		name      string
		actor     Actor
		action    string
		fields    map[string]any
		wantErr   string
		wantMove  *calendar.PartyMark
		wantClear bool
	}{
		{"a page on a day", owner, ActionCreate, map[string]any{"place": "Gull Rock", "year": 1492, "month": "Hammer", "day": 8}, "",
			&calendar.PartyMark{Year: 1492, Month: 1, Day: 8, PlaceID: "p-gull"}, false},
		{"today when no day is given", owner, ActionCreate, map[string]any{"place": "Gull Rock"}, "",
			&calendar.PartyMark{Year: 1492, Month: 1, Day: 5, PlaceID: "p-gull"}, false},
		{"everywhere else is the world's weather", owner, ActionCreate, map[string]any{"place": "everywhere else"}, "",
			&calendar.PartyMark{Year: 1492, Month: 1, Day: 5}, false},
		{"take back a move", owner, ActionDelete, map[string]any{"year": 1492, "month": 1, "day": 8}, "", nil, true},
		{"players can't move the party", player, ActionCreate, map[string]any{"place": "Gull Rock"}, "only the owner or a co-DM", nil, false},
		{"no place named", owner, ActionCreate, map[string]any{}, "say where the party is", nil, false},
		{"a page that doesn't exist", owner, ActionCreate, map[string]any{"place": "Atlantis"}, "no page called", nil, false},
		{"a page the actor may not open reads as missing", owner, ActionCreate, map[string]any{"place": "Secret Cove"}, "no page called", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCal()
			c.cal.CurrentYear, c.cal.CurrentMonth, c.cal.CurrentDay = 1492, 1, 5
			pl := placesWorld("")
			k := PartyPlaceKind{Cal: c, Places: pl, Pages: pages}
			r := rec(KindPartyPlace, tt.action, "", tt.fields, "")
			p := k.Plan(context.Background(), camp, tt.actor, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q, want %q", p.Error, tt.wantErr)
				}
				if err := k.Apply(context.Background(), camp, tt.actor, r); err == nil || len(pl.moves) != 0 {
					t.Fatal("apply wrote a refused move")
				}
				return
			}
			if p.Error != "" || p.Summary == "" {
				t.Fatalf("plan %+v", p)
			}
			if err := k.Apply(context.Background(), camp, tt.actor, r); err != nil {
				t.Fatalf("apply: %v", err)
			}
			if tt.wantMove != nil && (len(pl.moves) != 1 || pl.moves[0] != *tt.wantMove) {
				t.Fatalf("moves %+v, want %+v", pl.moves, *tt.wantMove)
			}
			if tt.wantClear && (len(pl.cleared) != 1 || pl.cleared[0] != (calendar.DayDate{Year: 1492, Month: 1, Day: 8})) {
				t.Fatalf("cleared %+v", pl.cleared)
			}
		})
	}
}

func TestPartyPlaceKind_WarnsOnUnknownKeys(t *testing.T) {
	c := newCal()
	k := PartyPlaceKind{Cal: c, Places: placesWorld(""), Pages: fakePageResolver{}}
	p := k.Plan(context.Background(), camp, owner, rec(KindPartyPlace, ActionCreate, "", map[string]any{"place": "everywhere else", "climate": "desert"}, ""))
	if p.Error != "" || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "climate") {
		t.Fatalf("plan %+v", p)
	}
}

// A place's weather comes from the same generator as the world's: the plan
// hands the browser the place as its one zone with the place's own climate,
// and the result is stored for that place, generated, never manual.
func TestGeneratorKind_PlaceWeather(t *testing.T) {
	base := func(extra map[string]any) map[string]any {
		f := map[string]any{"generator": "weather", "year": 1492, "month": "Hammer", "day": 1, "end_year": 1492, "end_month": 1, "end_day": 3}
		for k, v := range extra {
			f[k] = v
		}
		return f
	}
	t.Run("a place with its own climate", func(t *testing.T) {
		c, pl := newCal(), placesWorld("")
		k := GeneratorKind{Cal: c, Places: pl}
		r := rec("generator", ActionCreate, "", base(map[string]any{"place": "saltreach"}), "")
		p := k.Plan(context.Background(), camp, owner, r)
		if p.Error != "" {
			t.Fatal(p.Error)
		}
		var plan genPlan
		if err := json.Unmarshal([]byte(p.Client), &plan); err != nil {
			t.Fatalf("client plan: %v", err)
		}
		if len(plan.Zones) != 1 || plan.Zones[0] != (genZone{ID: "p-salt", Name: "Saltreach", Climate: "cold-coast"}) {
			t.Fatalf("zones %+v", plan.Zones)
		}
		if plan.Recipe["details"].(map[string]any)["climate"] != "cold-coast" {
			t.Fatalf("recipe %+v", plan.Recipe)
		}
		if !strings.Contains(p.Summary, "for Saltreach") || !strings.Contains(p.Summary, "Cold coast climate (set on its page)") {
			t.Errorf("summary %q", p.Summary)
		}
		r.Generated = `[{"year":1492,"month":1,"day":2,"preset_label":"Sea fog"}]`
		if err := k.Apply(context.Background(), camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(c.weather) != 0 || len(pl.written["p-salt"]) != 1 || pl.written["p-salt"][0].Source != calendar.WeatherSourceGenerated {
			t.Fatalf("world %+v, place %+v", c.weather, pl.written)
		}
	})
	for _, tt := range []struct {
		name, want string
		fields     map[string]any
		places     *fakePlaces
	}{
		{"a page with no climate of its own", "has no Climate of its own", map[string]any{"place": "Gull Rock"}, placesWorld("")},
		{"a climate besides the page's", "follows the Climate on its page", map[string]any{"place": "Saltreach", "climate": "desert"}, placesWorld("")},
		{"a server without weather by place", "isn't available", map[string]any{"place": "Saltreach"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			k := GeneratorKind{Cal: newCal()}
			if tt.places != nil {
				k.Places = tt.places
			}
			p := k.Plan(context.Background(), camp, owner, rec("generator", ActionCreate, "", base(tt.fields), ""))
			if !strings.Contains(p.Error, tt.want) {
				t.Fatalf("plan error %q, want %q", p.Error, tt.want)
			}
		})
	}
}

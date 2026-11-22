package calendar

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakePlaceRepo is an in-memory PlaceWeatherRepository.
type fakePlaceRepo struct {
	days  map[string][]DayWeather // by place
	marks []PartyMark
	set   map[string][]DayWeatherInput
	asked [][]string // place ids each ListPlaceDays was asked for
}

func (f *fakePlaceRepo) ListPlaceDays(_ context.Context, _ string, ids []string, year, month int) (map[string][]DayWeather, error) {
	f.asked = append(f.asked, ids)
	out := map[string][]DayWeather{}
	for _, id := range ids {
		for _, d := range f.days[id] {
			if d.Year == year && (month == 0 || d.Month == month) {
				out[id] = append(out[id], d)
			}
		}
	}
	return out, nil
}
func (f *fakePlaceRepo) SetPlaceDays(_ context.Context, _, placeID string, days []DayWeatherInput) error {
	if f.set == nil {
		f.set = map[string][]DayWeatherInput{}
	}
	f.set[placeID] = append(f.set[placeID], days...)
	return nil
}
func (f *fakePlaceRepo) ClearPlaceDays(context.Context, string, string, []DayDate) error { return nil }
func (f *fakePlaceRepo) LockPlaceDays(context.Context, string, string, []DayDate, bool) (int, error) {
	return 0, nil
}
func (f *fakePlaceRepo) ListPartyMarks(context.Context, string) ([]PartyMark, error) {
	return f.marks, nil
}
func (f *fakePlaceRepo) SetPartyMark(_ context.Context, _ string, m PartyMark) error {
	f.marks = append(f.marks, m)
	return nil
}
func (f *fakePlaceRepo) ClearPartyMark(context.Context, string, DayDate) error { return nil }

// fakeDirectory is a campaign's pages as the calendar sees them: each
// page's region (the page holding the climate that covers it) and which
// pages a player may open.
type fakeDirectory struct {
	places   []ClimatePlace          // pages with a climate of their own
	regionOf map[string]ClimatePlace // page id -> its region
	names    map[string]string       // every page, by id
	open     map[string]bool         // pages a player may open
}

func (d fakeDirectory) ClimatePlaces(context.Context, string) ([]ClimatePlace, error) {
	return d.places, nil
}
func (d fakeDirectory) RegionOf(_ context.Context, _ string, id string) (*ClimatePlace, error) {
	r, ok := d.regionOf[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}
func (d fakeDirectory) Viewable(_ context.Context, _ string, ids []string, v permissions.Viewer) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = v.SkipsPerUserRules() || d.open[id]
	}
	return out
}
func (d fakeDirectory) PlaceName(_ context.Context, _ string, id string) (string, bool) {
	n, ok := d.names[id]
	return n, ok
}

var (
	saltreach = ClimatePlace{ID: "salt", Name: "Saltreach", Climate: "cold-coast"}
	hollow    = ClimatePlace{ID: "hollow", Name: "The Hidden Hollow", Climate: "desert"}
)

// placeFixture is dayWeatherFixture's calendar (today is year 5, month 2,
// day 10) with per-place weather wired: Saltreach (open to players) holds
// the town of Gull Rock, and the Hidden Hollow is a page players can't open.
func placeFixture(world []DayWeather, place *fakePlaceRepo) CalendarService {
	weather := &fakeWeatherRepo{listDaysFn: func(context.Context, string, int, int) ([]DayWeather, error) {
		return world, nil
	}}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp", CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}, nil
		},
		getDefaultFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal", CampaignID: "camp", CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, weather)
	dir := fakeDirectory{
		places:   []ClimatePlace{saltreach, hollow},
		regionOf: map[string]ClimatePlace{"salt": saltreach, "gull": saltreach, "hollow": hollow},
		names:    map[string]string{"salt": "Saltreach", "gull": "Gull Rock", "hollow": "The Hidden Hollow", "plain": "Open Plain"},
		open:     map[string]bool{"salt": true, "gull": true},
	}
	svc.(*calendarService).SetPlaceWeather(place, dir)
	return svc
}

func label(s string) *string { return &s }

func wx(y, m, d int, lbl string) DayWeather {
	return DayWeather{Year: y, Month: m, Day: d, PresetLabel: label(lbl)}
}

var (
	pwPlayer   = permissions.RequestViewer(int(permissions.RolePlayer), "u1")
	pwDirector = permissions.RequestViewer(int(permissions.RoleOwner), "u1")
)

func labels(days []DayWeather) string {
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = *d.PresetLabel
		if d.Place != nil {
			parts[i] += "@" + *d.Place
		}
	}
	return strings.Join(parts, ",")
}

func TestRegionAt(t *testing.T) {
	marks := []regionMark{{DayDate{5, 2, 3}, "salt"}, {DayDate{5, 2, 8}, ""}, {DayDate{6, 1, 1}, "hollow"}}
	tests := []struct {
		name    string
		y, m, d int
		want    string
	}{
		{"before the first move: the world", 5, 2, 2, ""},
		{"on the move's own day", 5, 2, 3, "salt"},
		{"between moves", 5, 2, 7, "salt"},
		{"moved back to everywhere else", 5, 2, 8, ""},
		{"a later year", 6, 1, 1, "hollow"},
		{"long after the last move", 9, 1, 1, "hollow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := regionAt(marks, tt.y, tt.m, tt.d); got != tt.want {
				t.Errorf("regionAt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPartyWeather(t *testing.T) {
	world := []DayWeather{wx(5, 2, 1, "w1"), wx(5, 2, 2, "w2"), wx(5, 2, 3, "w3"), wx(5, 2, 4, "w4")}
	place := map[string][]DayWeather{"salt": {wx(5, 2, 2, "s2"), wx(5, 2, 5, "s5")}}
	tests := []struct {
		name  string
		marks []regionMark
		want  string
	}{
		{"no moves: the world's weather", nil, "w1,w2,w3,w4"},
		{"in a region: its days, and nothing on a day it has none", []regionMark{{DayDate{5, 2, 2}, "salt"}}, "w1,s2,s5"},
		{"back to everywhere else", []regionMark{{DayDate{5, 2, 2}, "salt"}, {DayDate{5, 2, 4}, ""}}, "w1,s2,w4"},
		{"a move to a page with no climate above it is the world", []regionMark{{DayDate{5, 2, 1}, ""}}, "w1,w2,w3,w4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labels(partyWeather(world, tt.marks, place, nil)); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestListDayWeather_PlayersSeeOnlyThePartysWeather(t *testing.T) {
	zone := "salt"
	world := []DayWeather{wx(5, 2, 4, "world4"), wx(5, 2, 5, "world5"), wx(5, 2, 9, "world9"), wx(5, 2, 10, "world10"), wx(5, 2, 12, "world12")}
	locked := true
	salt5 := wx(5, 2, 5, "salt5")
	salt5.ZoneID, salt5.ZoneName, salt5.Locked = &zone, label("Saltreach"), &locked
	place := &fakePlaceRepo{
		days: map[string][]DayWeather{
			"salt":   {salt5, wx(5, 2, 6, "salt6"), wx(5, 2, 13, "salt13")},
			"hollow": {wx(5, 2, 9, "hollow9"), wx(5, 2, 10, "hollow10")},
		},
		marks: []PartyMark{
			{Year: 5, Month: 2, Day: 5, PlaceID: "gull"},   // a town inside Saltreach
			{Year: 5, Month: 2, Day: 9, PlaceID: "hollow"}, // a place players can't open
			{Year: 5, Month: 2, Day: 12, PlaceID: "salt"},  // the Director's plan, after today
			{Year: 5, Month: 2, Day: 15, PlaceID: "plain"}, // a page with no climate: the world
		},
	}
	svc := placeFixture(world, place)
	got, err := svc.ListDayWeather(context.Background(), "cal", "camp", 5, 0, pwPlayer)
	if err != nil {
		t.Fatalf("ListDayWeather: %v", err)
	}
	// Day 4 is the world's; 5 and 6 Saltreach's, named; 9 and 10 the hidden
	// place's weather with no name; nothing after today, so the planned move
	// back to Saltreach leaks neither its days nor the world's.
	if want := "world4,salt5@Saltreach,salt6@Saltreach,hollow9,hollow10"; labels(got) != want {
		t.Fatalf("player got %s, want %s", labels(got), want)
	}
	for _, d := range got {
		if d.ZoneID != nil || d.ZoneName != nil || d.Locked != nil {
			t.Errorf("player day %d carries the generator's zone or the lock: %+v", d.Day, d)
		}
	}
	// The Director's own day weather stays the world's: that is what they paint.
	dir, err := svc.ListDayWeather(context.Background(), "cal", "camp", 5, 0, pwDirector)
	if err != nil || labels(dir) != "world4,world5,world9,world10,world12" {
		t.Fatalf("director got %s, %v", labels(dir), err)
	}
}

func TestListPartyDayWeather_DirectorSeesThePartysWeatherAhead(t *testing.T) {
	world := []DayWeather{wx(5, 2, 4, "world4"), wx(5, 2, 12, "world12")}
	place := &fakePlaceRepo{
		days:  map[string][]DayWeather{"salt": {wx(5, 2, 13, "salt13")}},
		marks: []PartyMark{{Year: 5, Month: 2, Day: 12, PlaceID: "salt"}},
	}
	got, err := placeFixture(world, place).(PlaceWeatherService).ListPartyDayWeather(context.Background(), "cal", "camp", 5, 0, pwDirector)
	if err != nil || labels(got) != "world4,salt13@Saltreach" {
		t.Fatalf("got %s, %v", labels(got), err)
	}
}

func TestListDayWeather_NoPlacesIsUnchanged(t *testing.T) {
	world := []DayWeather{wx(5, 2, 4, "world4"), wx(5, 2, 9, "world9")}
	place := &fakePlaceRepo{}
	got, err := placeFixture(world, place).ListDayWeather(context.Background(), "cal", "camp", 5, 0, pwPlayer)
	if err != nil || labels(got) != "world4,world9" {
		t.Fatalf("got %s, %v", labels(got), err)
	}
	if len(place.asked) != 0 {
		t.Errorf("read place weather with no party moves: %v", place.asked)
	}
}

func TestPlaceToday_PlayerOnlyWhereThePartyIs(t *testing.T) {
	place := &fakePlaceRepo{
		days: map[string][]DayWeather{
			"salt":   {wx(5, 2, 10, "salt10")},
			"hollow": {wx(5, 2, 10, "hollow10")},
		},
		marks: []PartyMark{{Year: 5, Month: 2, Day: 5, PlaceID: "gull"}},
	}
	pw := placeFixture(nil, place).(PlaceWeatherService)
	tests := []struct {
		name   string
		region string
		v      permissions.Viewer
		want   string // reading label, "" for no answer
		here   bool
	}{
		{"player, the party's region", "salt", pwPlayer, "salt10", true},
		{"player, another region", "hollow", pwPlayer, "", false},
		{"director, another region", "hollow", pwDirector, "hollow10", false},
		{"director, the party's region", "salt", pwDirector, "salt10", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pw.PlaceToday(context.Background(), "camp", tt.region, tt.v)
			if err != nil {
				t.Fatalf("PlaceToday: %v", err)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("got %+v, want nothing", got)
				}
				return
			}
			if got == nil || got.Reading == nil || *got.Reading.PresetLabel != tt.want || got.PartyHere != tt.here || got.Date != "Two 10" {
				t.Fatalf("got %+v, want %s here=%v", got, tt.want, tt.here)
			}
		})
	}
}

func TestPartyNow_NamesOnlyPagesThePlayerMayOpen(t *testing.T) {
	tests := []struct {
		name              string
		mark              string
		v                 permissions.Viewer
		place, region, cl string
	}{
		{"a town players may open", "gull", pwPlayer, "Gull Rock", "Saltreach", "Cold coast"},
		{"a hidden place", "hollow", pwPlayer, "", "", "Desert"},
		{"the director sees the hidden place", "hollow", pwDirector, "The Hidden Hollow", "The Hidden Hollow", "Desert"},
		{"everywhere else", "", pwPlayer, "", "", "Temperate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			place := &fakePlaceRepo{marks: []PartyMark{{Year: 5, Month: 2, Day: 1, PlaceID: tt.mark}, {Year: 5, Month: 2, Day: 20, PlaceID: "salt"}}}
			got, err := placeFixture(nil, place).(PlaceWeatherService).PartyNow(context.Background(), "cal", "camp", tt.v)
			if err != nil {
				t.Fatalf("PartyNow: %v", err)
			}
			if got.PlaceName != tt.place || got.RegionName != tt.region || got.ClimateName != tt.cl || got.Since == nil || got.Since.Day != 1 {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestPlaceWrites_Validation(t *testing.T) {
	ctx := context.Background()
	rain := []DayWeatherInput{{Year: 5, Month: 2, Day: 3, Source: WeatherSourceManual, WeatherInput: WeatherInput{PresetLabel: label("Rain")}}}
	tests := []struct {
		name    string
		run     func(PlaceWeatherService) error
		wantErr int // 0 for success, else the HTTP code
	}{
		{"weather for a place with its own climate", func(s PlaceWeatherService) error {
			return s.SetPlaceDayWeather(ctx, "cal", "camp", "salt", rain)
		}, 0},
		{"weather for a town inside it is refused: it shares Saltreach's", func(s PlaceWeatherService) error {
			return s.SetPlaceDayWeather(ctx, "cal", "camp", "gull", rain)
		}, 400},
		{"weather for a page with no climate is refused", func(s PlaceWeatherService) error {
			return s.SetPlaceDayWeather(ctx, "cal", "camp", "plain", rain)
		}, 400},
		{"another campaign's calendar is not found", func(s PlaceWeatherService) error {
			return s.SetPlaceDayWeather(ctx, "cal", "other", "salt", rain)
		}, 404},
		{"a day the calendar does not have", func(s PlaceWeatherService) error {
			return s.SetPlaceDayWeather(ctx, "cal", "camp", "salt", []DayWeatherInput{{Year: 5, Month: 2, Day: 25}})
		}, 400},
		{"the party can move to any page", func(s PlaceWeatherService) error {
			return s.SetPartyPlace(ctx, "cal", "camp", DayDate{5, 2, 3}, "plain")
		}, 0},
		{"or everywhere else", func(s PlaceWeatherService) error {
			return s.SetPartyPlace(ctx, "cal", "camp", DayDate{5, 2, 3}, "")
		}, 0},
		{"not to a page that isn't in the campaign", func(s PlaceWeatherService) error {
			return s.SetPartyPlace(ctx, "cal", "camp", DayDate{5, 2, 3}, "nowhere")
		}, 400},
		{"not on a day the calendar does not have", func(s PlaceWeatherService) error {
			return s.SetPartyPlace(ctx, "cal", "camp", DayDate{5, 3, 1}, "salt")
		}, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(placeFixture(nil, &fakePlaceRepo{}).(PlaceWeatherService))
			if tt.wantErr == 0 {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tt.wantErr {
				t.Fatalf("err = %v, want %d", err, tt.wantErr)
			}
		})
	}
}

func TestPlaceWeather_OffWithoutWiring(t *testing.T) {
	svc := newTestCalendarService(nil, nil, nil, nil).(PlaceWeatherService)
	err := svc.SetPartyPlace(context.Background(), "cal", "camp", DayDate{5, 2, 3}, "salt")
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != 400 {
		t.Fatalf("err = %v, want a 400 saying it is not available", err)
	}
}

func TestPlaceWeatherYear_ListsPlacesMovesAndDays(t *testing.T) {
	place := &fakePlaceRepo{
		days:  map[string][]DayWeather{"salt": {wx(5, 2, 3, "salt3")}},
		marks: []PartyMark{{Year: 5, Month: 2, Day: 1, PlaceID: "gull"}, {Year: 5, Month: 2, Day: 9, PlaceID: "plain"}},
	}
	got, err := placeFixture(nil, place).(PlaceWeatherService).PlaceWeatherYear(context.Background(), "cal", "camp", 5)
	if err != nil {
		t.Fatalf("PlaceWeatherYear: %v", err)
	}
	if got.World.ID != DefaultWeatherClimate || len(got.Places) != 2 || got.Places[0].Name != "Saltreach" || got.Places[0].ClimateName != "Cold coast" {
		t.Fatalf("world/places = %+v %+v", got.World, got.Places)
	}
	if len(got.Party) != 2 || got.Party[0].PlaceName != "Gull Rock" || got.Party[0].RegionID != "salt" || got.Party[1].RegionID != "" {
		t.Fatalf("party = %+v", got.Party)
	}
	if len(got.Days["salt"]) != 1 {
		t.Fatalf("days = %+v", got.Days)
	}
}

func TestBuildDayUpserts_KeysAndRules(t *testing.T) {
	manual, generated := buildDayUpserts("calendar_place_weather_days", "calendar_id", "place_id", "year", "month", "day")
	for _, sql := range []string{manual, generated} {
		if !strings.Contains(sql, "INSERT INTO calendar_place_weather_days (calendar_id, place_id, year, month, day,") {
			t.Errorf("keys missing: %s", sql)
		}
	}
	if !strings.Contains(generated, "locked") || !strings.Contains(generated, "source") {
		t.Errorf("a generated write must respect manual and locked days: %s", generated)
	}
}

// The route fake's per-place half, so the gates below reach a handler that
// answers rather than "not available here".
func (f *fakeCalendarSvc) PlaceWeatherYear(context.Context, string, string, int) (*PlaceWeatherYear, error) {
	return &PlaceWeatherYear{Places: []ClimatePlace{}, Party: []PartyMarkView{}, Days: map[string][]DayWeather{}}, nil
}
func (f *fakeCalendarSvc) SetPlaceDayWeather(context.Context, string, string, string, []DayWeatherInput) error {
	return nil
}
func (f *fakeCalendarSvc) ClearPlaceDayWeather(context.Context, string, string, string, []DayDate) error {
	return nil
}
func (f *fakeCalendarSvc) LockPlaceDayWeather(context.Context, string, string, string, []DayDate, bool) (int, error) {
	return 0, nil
}
func (f *fakeCalendarSvc) SetPartyPlace(context.Context, string, string, DayDate, string) error {
	return nil
}
func (f *fakeCalendarSvc) ClearPartyPlace(context.Context, string, string, DayDate) error { return nil }
func (f *fakeCalendarSvc) ListPartyDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error) {
	return f.ListDayWeather(ctx, calendarID, campaignID, year, month, v)
}
func (f *fakeCalendarSvc) PlaceToday(context.Context, string, string, permissions.Viewer) (*PlaceToday, error) {
	return nil, nil
}
func (f *fakeCalendarSvc) WorldClimate(context.Context, string, permissions.Viewer) (ClimateRef, bool) {
	return ClimateRef{}, false
}
func (f *fakeCalendarSvc) PartyNow(context.Context, string, string, permissions.Viewer) (*PartyNow, error) {
	return &PartyNow{}, nil
}

// Every per-place route is the Director's: players read the party's weather
// through the ordinary day-weather route, which the service narrows.
func TestRouteGates_PlaceWeatherIsTheDirectors(t *testing.T) {
	routes := []struct {
		name, method, path, body string
		wantOK                   int
	}{
		{"read weather by place", http.MethodGet, "/campaigns/camp-1/calendars/cal-1/weather/places?year=1", "", http.StatusOK},
		{"set a place's weather", http.MethodPut, "/campaigns/camp-1/calendars/cal-1/weather/places/p-1/days", `{"days":[]}`, http.StatusNoContent},
		{"clear a place's weather", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/weather/places/p-1/days/clear", `{"days":[]}`, http.StatusNoContent},
		{"lock a place's weather", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/weather/places/p-1/days/lock", `{"days":[],"locked":true}`, http.StatusOK},
		{"move the party", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/weather/party", `{"year":1,"month":1,"day":1,"place_id":"p-1"}`, http.StatusNoContent},
		{"take back a move", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/weather/party/clear", `{"year":1,"month":1,"day":1}`, http.StatusNoContent},
	}
	callers := []struct {
		name    string
		role    campaigns.Role
		granted bool
		want    bool
	}{
		{"Owner", campaigns.RoleOwner, false, true},
		{"co-Director", campaigns.RoleScribe, true, true},
		{"Scribe", campaigns.RoleScribe, false, false},
		{"Player", campaigns.RolePlayer, false, false},
	}
	const userID = "u-caller"
	for _, rt := range routes {
		for _, c := range callers {
			t.Run(rt.name+"/"+c.name, func(t *testing.T) {
				settings := ""
				if c.granted {
					settings = `{"dm_grant_ids":["` + userID + `"]}`
				}
				e, _ := newAccessTestRouterWithSettings(false, true, map[string]campaigns.Role{userID: c.role}, settings)
				rec := doRequestWithBody(e, rt.method, rt.path, userID, rt.body)
				want := http.StatusForbidden
				if c.want {
					want = rt.wantOK
				}
				if rec.Code != want {
					t.Errorf("got %d, want %d: %s", rec.Code, want, rec.Body.String())
				}
			})
		}
	}
}

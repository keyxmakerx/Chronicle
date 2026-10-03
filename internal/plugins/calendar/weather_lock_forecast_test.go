package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var (
	owner  = permissions.RequestViewer(int(permissions.RoleOwner), "u-owner")
	player = permissions.RequestViewer(int(permissions.RolePlayer), "u-player")
)

// doRequestForm posts an HTMX form submission as userID, like the settings
// page's own preview and save requests.
func doRequestForm(e http.Handler, path, userID string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func strp(s string) *string     { return &s }
func f64p(f float64) *float64   { return &f }
func boolp(b bool) *bool        { return &b }
func badRequest(err error) bool { return apperror.SafeCode(err) == http.StatusBadRequest }

// forecastFixture is a two-month calendar (30 and 20 days) in campaign
// "camp" with forecasts on, today at (year 5, month 2, day 10). Callers
// adjust the calendar through mut.
func forecastFixture(repo *fakeWeatherRepo, mut func(*Calendar)) (CalendarService, *[]*Calendar) {
	var updated []*Calendar
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			cal := &Calendar{ID: id, CampaignID: "camp", Name: "Reckoning", Mode: ModeFantasy, ForecastsEnabled: true,
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
				CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}
			if mut != nil {
				mut(cal)
			}
			return cal, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}}, nil
		},
		updateFn: func(_ context.Context, cal *Calendar) error {
			updated = append(updated, cal)
			return nil
		},
	}
	return newTestCalendarService(calRepo, nil, nil, repo), &updated
}

// readingsOn answers ListDays with a rain reading (and a temperature) on
// each given (year, month, day), as a repository filtered to the month would.
func readingsOn(days ...DayDate) *fakeWeatherRepo {
	return &fakeWeatherRepo{listDaysFn: func(_ context.Context, _ string, year, month int) ([]DayWeather, error) {
		var out []DayWeather
		for _, d := range days {
			if d.Year == year && d.Month == month {
				out = append(out, DayWeather{Year: d.Year, Month: d.Month, Day: d.Day, Icon: strp("rain"),
					PresetID: strp("fire-rain"), PresetLabel: strp("Fire rain"), Color: strp("#ff5500"),
					Description: strp("secret ember storm"), TemperatureCelsius: f64p(14), Source: WeatherSourceManual})
			}
		}
		return out, nil
	}}
}

func TestLockDayWeather(t *testing.T) {
	tooMany := make([]DayDate, maxDayWeatherBatch+1)
	for i := range tooMany {
		tooMany[i] = DayDate{5, 1, 1}
	}
	tests := []struct {
		name     string
		campaign string
		dates    []DayDate
		locked   bool
		repoN    int
		wantErr  func(error) bool
		wantCall bool
	}{
		{"locks existing days and reports the count", "camp", []DayDate{{5, 1, 3}, {5, 2, 4}}, true, 2, nil, true},
		{"unlocks", "camp", []DayDate{{5, 1, 3}}, false, 1, nil, true},
		{"an empty list changes nothing", "camp", nil, true, 0, nil, true},
		{"more than the batch limit", "camp", tooMany, true, 0, badRequest, false},
		{"a month that is not the calendar's", "camp", []DayDate{{5, 3, 1}}, true, 0, badRequest, false},
		{"a day past the month's end", "camp", []DayDate{{5, 2, 21}}, true, 0, badRequest, false},
		{"another campaign's calendar is not found", "other", []DayDate{{5, 1, 1}}, true, 0,
			func(err error) bool { return apperror.SafeCode(err) == http.StatusNotFound }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotDates []DayDate
			var gotLocked *bool
			repo := &fakeWeatherRepo{lockDaysFn: func(_ context.Context, _ string, dates []DayDate, locked bool) (int, error) {
				gotDates, gotLocked = dates, &locked
				return tt.repoN, nil
			}}
			svc, _ := forecastFixture(repo, nil)
			n, err := svc.LockDayWeather(context.Background(), "cal", tt.campaign, tt.dates, tt.locked)
			if tt.wantErr != nil {
				if err == nil || !tt.wantErr(err) {
					t.Fatalf("got %v, want the expected refusal", err)
				}
				if gotLocked != nil {
					t.Fatal("nothing may be written on a refusal")
				}
				return
			}
			if err != nil || n != tt.repoN {
				t.Fatalf("LockDayWeather = %d, %v; want %d", n, err, tt.repoN)
			}
			if tt.wantCall && (gotLocked == nil || *gotLocked != tt.locked || !reflect.DeepEqual(gotDates, tt.dates)) {
				t.Errorf("repo got %v locked=%v", gotDates, gotLocked)
			}
		})
	}
}

func TestLockDayWeatherAPI(t *testing.T) {
	const path = "/campaigns/camp-1/calendars/cal-1/weather/days/lock"
	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantBody   string
		wantLocked *bool
	}{
		{"lock", `{"days":[{"year":5,"month":1,"day":2}],"locked":true}`, http.StatusOK, `{"changed":1}`, boolp(true)},
		{"unlock", `{"days":[{"year":5,"month":1,"day":2}],"locked":false}`, http.StatusOK, `{"changed":1}`, boolp(false)},
		{"locked is required, so a missing field can never unlock", `{"days":[{"year":5,"month":1,"day":2}]}`, http.StatusBadRequest, "", nil},
		{"not json", `nope`, http.StatusBadRequest, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, svc := newAccessTestRouter(false, true, map[string]campaigns.Role{"u-owner": campaigns.RoleOwner})
			rec := doRequestWithBody(e, http.MethodPost, path, "u-owner", tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Errorf("body %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if (svc.lockedTo == nil) != (tt.wantLocked == nil) || (tt.wantLocked != nil && *svc.lockedTo != *tt.wantLocked) {
				t.Errorf("service saw locked=%v, want %v", svc.lockedTo, tt.wantLocked)
			}
		})
	}
}

// Players and anonymous visitors read the forecast; it is not a Director
// route, so a Player passes where the lock route refuses them.
func TestForecastRoute_PlayersCanRead(t *testing.T) {
	const path = "/campaigns/camp-1/calendars/cal-1/weather/forecast"
	tests := []struct {
		name   string
		public bool
		roles  map[string]campaigns.Role
		user   string
		want   int
	}{
		{"player", false, map[string]campaigns.Role{"u-p": campaigns.RolePlayer}, "u-p", http.StatusOK},
		{"owner", false, map[string]campaigns.Role{"u-o": campaigns.RoleOwner}, "u-o", http.StatusOK},
		{"non-member of a private campaign", false, map[string]campaigns.Role{}, "u-x", http.StatusForbidden},
		{"anonymous visitor of a public campaign", true, nil, "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newAccessTestRouter(tt.public, true, tt.roles)
			rec := doRequest(e, http.MethodGet, path, tt.user)
			if rec.Code != tt.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tt.want)
			}
			if tt.want == http.StatusOK && strings.TrimSpace(rec.Body.String()) != "[]" {
				t.Errorf("body %q, want []", rec.Body.String())
			}
		})
	}
}

func TestListDayWeather_LockedIsDirectorOnly(t *testing.T) {
	stored := func() []DayWeather {
		return []DayWeather{
			{Year: 5, Month: 2, Day: 9, Locked: boolp(true)},
			{Year: 5, Month: 2, Day: 10, Locked: boolp(false)},
		}
	}
	repo := &fakeWeatherRepo{listDaysFn: func(context.Context, string, int, int) ([]DayWeather, error) { return stored(), nil }}
	svc := dayWeatherFixture(repo)

	tests := []struct {
		name        string
		viewer      permissions.Viewer
		wantLocked  []bool
		wantKeyInJS bool
	}{
		{"owner sees lock state", owner, []bool{true, false}, true},
		{"player never learns a day is locked", player, nil, false},
		{"anonymous visitor never learns it either", permissions.RequestViewer(int(permissions.RolePlayer), ""), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.ListDayWeather(context.Background(), "cal", "camp", 5, 0, tt.viewer)
			if err != nil || len(got) != 2 {
				t.Fatalf("ListDayWeather = %+v, %v", got, err)
			}
			raw, _ := json.Marshal(got)
			if has := strings.Contains(string(raw), `"locked"`); has != tt.wantKeyInJS {
				t.Errorf(`"locked" present=%v in %s, want %v`, has, raw, tt.wantKeyInJS)
			}
			for i, want := range tt.wantLocked {
				if got[i].Locked == nil || *got[i].Locked != want {
					t.Errorf("day %d locked = %v, want %v", i, got[i].Locked, want)
				}
			}
		})
	}
}

// The SQL itself is exercised against MariaDB by the repository integration
// test; this pins the two rules that must not drift apart in the statements.
func TestDayUpserts_LockRules(t *testing.T) {
	if !strings.Contains(upsertManualDaySQL, "locked = FALSE") {
		t.Error("a manual write must clear the lock")
	}
	if strings.Contains(upsertGeneratedDaySQL, "locked = ") && !strings.Contains(upsertGeneratedDaySQL, "OR locked") {
		t.Error("a generated write must not assign locked")
	}
	if !strings.Contains(upsertGeneratedDaySQL, "source = 'manual' OR locked") {
		t.Error("a generated write must keep a manual or locked day")
	}
	if strings.Contains(upsertGeneratedDaySQL, "locked = VALUES") || strings.Contains(upsertGeneratedDaySQL, "locked = FALSE") {
		t.Error("a generated write must leave the lock as stored")
	}
}

func TestForecastConfidence(t *testing.T) {
	tests := []struct {
		lead int
		want float64
	}{
		{1, 0.97}, {2, 0.82}, {3, 0.69}, {4, 0.58}, {5, 0.49}, {6, 0.41}, {7, 0.35}, {10, 0.21}, {11, 0.18}, {100, 0.18},
	}
	for _, tt := range tests {
		got := forecastConfidence(tt.lead)
		if r := float64(int(got*100+0.5)) / 100; r != tt.want {
			t.Errorf("lead %d: confidence %.4f rounds to %.2f, want %.2f", tt.lead, got, r, tt.want)
		}
	}
	for l := 2; l <= 12; l++ {
		if forecastConfidence(l) > forecastConfidence(l-1) {
			t.Errorf("confidence rose between lead %d and %d", l-1, l)
		}
	}
}

func TestForecastWords(t *testing.T) {
	tests := []struct {
		category string
		conf     float64
		chance   int
		want     string
	}{
		{fcDry, 0.97, 0, "Dry and bright"},
		{fcDry, 0.8, 0, "Dry and bright"},
		{fcDry, 0.79, 0, "Probably dry"},
		{fcDry, 0.55, 10, "Probably dry"},
		{fcDry, 0.54, 10, "Mostly dry"},
		{fcDry, 0.35, 10, "Mostly dry"},
		{fcCloud, 0.9, 0, "Grey and dry"},
		{fcCloud, 0.6, 0, "Probably grey"},
		{fcCloud, 0.4, 0, "Cloudy, maybe"},
		{fcWet, 0.9, 90, "Rain"},
		{fcWet, 0.6, 60, "Rain likely"},
		{fcWet, 0.4, 40, "Chance of rain"},
		{fcSnow, 0.9, 90, "Snow"},
		{fcStorm, 0.6, 60, "Storms likely"},
		{fcFog, 0.4, 20, "Chance of fog"},
		{fcWet, 0.34, 50, "Unsettled"},
		{fcDry, 0.2, 60, "Unsettled"},
		{fcWet, 0.34, 49, "Hard to call"},
		{fcDry, 0.18, 30, "Hard to call"},
		{"nonsense", 0.9, 0, "Unsettled"},
	}
	for _, tt := range tests {
		if got := forecastWords(tt.category, tt.conf, tt.chance); got != tt.want {
			t.Errorf("forecastWords(%s, %v, %d) = %q, want %q", tt.category, tt.conf, tt.chance, got, tt.want)
		}
	}
}

func TestBuildForecastEntry(t *testing.T) {
	day := DayDate{5, 2, 11}
	reading := func(icon string, temp *float64) DayWeather {
		return DayWeather{Icon: &icon, TemperatureCelsius: temp, PresetLabel: strp("Fire rain"), Color: strp("#ff5500")}
	}

	t.Run("deterministic", func(t *testing.T) {
		a := buildForecastEntry("cal-1", day, 3, reading("storm", f64p(8)))
		b := buildForecastEntry("cal-1", day, 3, reading("storm", f64p(8)))
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("same inputs gave %+v then %+v", a, b)
		}
	})

	t.Run("a different day, lead or calendar draws independently", func(t *testing.T) {
		seeds := map[int64]bool{}
		for _, s := range []int64{
			forecastSeed("cal-1", day, 3), forecastSeed("cal-1", day, 4),
			forecastSeed("cal-1", DayDate{5, 2, 12}, 3), forecastSeed("cal-2", day, 3),
		} {
			seeds[s] = true
		}
		if len(seeds) != 4 {
			t.Errorf("seeds collided: %v", seeds)
		}
	})

	t.Run("each icon forecasts only itself or a listed neighbour, and sends only a glyph", func(t *testing.T) {
		for icon, truth := range forecastCategoryByIcon {
			allowed := map[string]bool{forecastIconByCategory[truth]: true}
			for _, n := range forecastNeighbours[truth] {
				allowed[forecastIconByCategory[n]] = true
			}
			for lead := 1; lead <= 10; lead++ {
				for i := 0; i < 40; i++ {
					e := buildForecastEntry("cal-"+string(rune('a'+i)), day, lead, reading(icon, nil))
					if !allowed[e.Icon] {
						t.Fatalf("icon %q lead %d forecast %q, not itself or a neighbour", icon, lead, e.Icon)
					}
				}
			}
		}
	})

	t.Run("an unknown or missing icon reads as cloud", func(t *testing.T) {
		for _, d := range []DayWeather{{Icon: strp("tornado")}, {}} {
			ok := false
			for i := 0; i < 40 && !ok; i++ {
				e := buildForecastEntry("cal-"+string(rune('a'+i)), day, 1, d)
				ok = e.Icon == "cloud"
			}
			if !ok {
				t.Errorf("%+v never forecast cloud at lead 1", d)
			}
		}
	})

	t.Run("tomorrow is mostly right and the far end is often wrong", func(t *testing.T) {
		right := func(lead int) int {
			n := 0
			for i := 0; i < 400; i++ {
				if buildForecastEntry("cal-"+string(rune(i)), day, lead, reading("rain", nil)).Icon == "rain" {
					n++
				}
			}
			return n
		}
		if near, far := right(1), right(10); near < 370 || far > 140 || far < 40 {
			t.Errorf("rain called right %d/400 at lead 1 and %d/400 at lead 10", near, far)
		}
	})

	t.Run("temperature is null without a reading and a widening band with one", func(t *testing.T) {
		e := buildForecastEntry("cal-1", day, 2, reading("clear", nil))
		if e.TempLow != nil || e.TempHigh != nil {
			t.Errorf("no temperature in, got %v..%v", e.TempLow, e.TempHigh)
		}
		raw, _ := json.Marshal(e)
		if !strings.Contains(string(raw), `"temp_low":null`) || !strings.Contains(string(raw), `"temp_high":null`) {
			t.Errorf("null temperatures must be explicit: %s", raw)
		}
		width := func(lead int) int {
			w := 0
			for i := 0; i < 50; i++ {
				e := buildForecastEntry("cal-"+string(rune('a'+i)), day, lead, reading("clear", f64p(10)))
				if *e.TempLow > *e.TempHigh {
					t.Fatalf("low %d above high %d", *e.TempLow, *e.TempHigh)
				}
				w += *e.TempHigh - *e.TempLow
			}
			return w
		}
		if width(10) <= width(1) {
			t.Errorf("band at lead 10 (%d) should be wider than at lead 1 (%d)", width(10), width(1))
		}
	})

	t.Run("precipitation chance is in tens, between 0 and 100", func(t *testing.T) {
		for _, icon := range []string{"clear", "cloud", "rain", "snow", "storm", "fog"} {
			for lead := 1; lead <= 10; lead++ {
				for i := 0; i < 20; i++ {
					p := buildForecastEntry("cal-"+string(rune('a'+i)), day, lead, reading(icon, nil)).PrecipChance
					if p < 0 || p > 100 || p%10 != 0 {
						t.Fatalf("%s lead %d chance %d", icon, lead, p)
					}
				}
			}
		}
	})

	t.Run("a confident wet forecast carries a high chance and a dry one a low chance", func(t *testing.T) {
		wet := buildForecastEntry("cal-q", day, 1, reading("rain", nil))
		if wet.Icon == "rain" && wet.PrecipChance != 100 {
			t.Errorf("rain at 0.97 chance = %d, want 100", wet.PrecipChance)
		}
		dry := buildForecastEntry("cal-q", day, 1, reading("clear", nil))
		if dry.Icon == "clear" && dry.PrecipChance != 0 {
			t.Errorf("clear at 0.97 chance = %d, want 0", dry.PrecipChance)
		}
	})
}

func TestCalendarAddDays(t *testing.T) {
	cal := &Calendar{Months: []Month{{Days: 30}, {Days: 20}}}
	empty := &Calendar{Months: []Month{{Days: 0}, {Days: 0}}}
	tests := []struct {
		name   string
		cal    *Calendar
		from   DayDate
		n      int
		want   DayDate
		wantOK bool
	}{
		{"within a month", cal, DayDate{5, 1, 3}, 4, DayDate{5, 1, 7}, true},
		{"across a month end", cal, DayDate{5, 1, 29}, 3, DayDate{5, 2, 2}, true},
		{"to the last day", cal, DayDate{5, 1, 29}, 1, DayDate{5, 1, 30}, true},
		{"across a year end", cal, DayDate{5, 2, 19}, 2, DayDate{6, 1, 1}, true},
		{"ten days over a year end", cal, DayDate{5, 2, 15}, 10, DayDate{6, 1, 5}, true},
		{"zero days stays put", cal, DayDate{5, 2, 10}, 0, DayDate{5, 2, 10}, true},
		{"a calendar with no days gives up instead of looping", empty, DayDate{5, 1, 1}, 1, DayDate{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.cal.addDays(tt.from, tt.n)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("addDays = %v, %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}

	t.Run("a leap year's extra days are counted", func(t *testing.T) {
		leap := &Calendar{LeapYearEvery: 4, LeapYearOffset: 0, Months: []Month{{Days: 28, LeapYearDays: 1}, {Days: 10}}}
		// Year 4 is a leap year: month 1 has 29 days.
		if got, _ := leap.addDays(DayDate{4, 1, 28}, 1); got != (DayDate{4, 1, 29}) {
			t.Errorf("leap year day 29 = %v", got)
		}
		if got, _ := leap.addDays(DayDate{5, 1, 28}, 1); got != (DayDate{5, 2, 1}) {
			t.Errorf("common year rolls at 28 = %v", got)
		}
	})
}

func TestListWeatherForecast(t *testing.T) {
	ctx := context.Background()
	v := func(t *testing.T, svc CalendarService) []ForecastEntry {
		t.Helper()
		got, err := svc.ListWeatherForecast(ctx, "cal-1", "camp", player)
		if err != nil {
			t.Fatalf("ListWeatherForecast: %v", err)
		}
		if got == nil {
			t.Fatal("must be an empty list, never nil (JSON null)")
		}
		return got
	}
	leads := func(es []ForecastEntry) []int {
		out := []int{}
		for _, e := range es {
			out = append(out, e.Lead)
		}
		return out
	}
	dates := func(es []ForecastEntry) []DayDate {
		out := []DayDate{}
		for _, e := range es {
			out = append(out, DayDate{e.Year, e.Month, e.Day})
		}
		return out
	}
	allDays := readingsOn(DayDate{5, 2, 11}, DayDate{5, 2, 12}, DayDate{5, 2, 13}, DayDate{5, 2, 14}, DayDate{5, 2, 15},
		DayDate{5, 2, 16}, DayDate{5, 2, 17}, DayDate{5, 2, 18}, DayDate{5, 2, 19}, DayDate{5, 2, 20}, DayDate{6, 1, 1})

	t.Run("off gives an empty list and reads nothing", func(t *testing.T) {
		repo := &fakeWeatherRepo{listDaysFn: func(context.Context, string, int, int) ([]DayWeather, error) {
			t.Fatal("a forecast that is off must not read the readings")
			return nil, nil
		}}
		svc, _ := forecastFixture(repo, func(c *Calendar) { c.ForecastsEnabled = false })
		if got := v(t, svc); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("no current date gives an empty list", func(t *testing.T) {
		for name, mut := range map[string]func(*Calendar){
			"zero date":         func(c *Calendar) { c.CurrentYear, c.CurrentMonth, c.CurrentDay = 0, 0, 0 },
			"month off the end": func(c *Calendar) { c.CurrentMonth = 9 },
			"day off the end":   func(c *Calendar) { c.CurrentDay = 99 },
		} {
			svc, _ := forecastFixture(allDays, mut)
			if got := v(t, svc); len(got) != 0 {
				t.Errorf("%s: got %+v", name, got)
			}
		}
	})

	t.Run("defaults to five days when no setting was stored", func(t *testing.T) {
		svc, _ := forecastFixture(allDays, nil)
		got := v(t, svc)
		if !reflect.DeepEqual(leads(got), []int{1, 2, 3, 4, 5}) {
			t.Fatalf("leads = %v", leads(got))
		}
		if got[0].Day != 11 || got[4].Day != 15 {
			t.Errorf("dates = %v", dates(got))
		}
	})

	t.Run("the stored reach sets how many days", func(t *testing.T) {
		for _, n := range []int{1, 3, 10} {
			repo := *allDays
			repo.getSettingsFn = func(context.Context, string) (*WeatherSettings, error) {
				return &WeatherSettings{ForecastDays: n}, nil
			}
			svc, _ := forecastFixture(&repo, nil)
			if got := v(t, svc); len(got) != n {
				t.Errorf("reach %d gave %d entries", n, len(got))
			}
		}
	})

	t.Run("an out-of-range stored reach falls back to five", func(t *testing.T) {
		repo := *allDays
		repo.getSettingsFn = func(context.Context, string) (*WeatherSettings, error) {
			return &WeatherSettings{ForecastDays: 99}, nil
		}
		svc, _ := forecastFixture(&repo, nil)
		if got := v(t, svc); len(got) != 5 {
			t.Errorf("got %d entries", len(got))
		}
	})

	t.Run("days without a stored reading are skipped and keep their lead", func(t *testing.T) {
		svc, _ := forecastFixture(readingsOn(DayDate{5, 2, 12}, DayDate{5, 2, 14}), nil)
		if got := v(t, svc); !reflect.DeepEqual(leads(got), []int{2, 4}) {
			t.Fatalf("leads = %v, want [2 4]", leads(got))
		}
	})

	t.Run("nothing stored ahead is an empty list", func(t *testing.T) {
		svc, _ := forecastFixture(readingsOn(DayDate{5, 2, 9}, DayDate{5, 2, 10}), nil)
		if got := v(t, svc); len(got) != 0 {
			t.Fatalf("got %+v; today and the past are not forecast", got)
		}
	})

	t.Run("the forecast crosses a month end and a year end", func(t *testing.T) {
		svc, _ := forecastFixture(readingsOn(DayDate{5, 2, 20}, DayDate{6, 1, 1}, DayDate{6, 1, 2}), func(c *Calendar) { c.CurrentDay = 19 })
		got := v(t, svc)
		want := []DayDate{{5, 2, 20}, {6, 1, 1}, {6, 1, 2}}
		if !reflect.DeepEqual(dates(got), want) || !reflect.DeepEqual(leads(got), []int{1, 2, 3}) {
			t.Fatalf("dates = %v leads = %v, want %v", dates(got), leads(got), want)
		}
		svc, _ = forecastFixture(readingsOn(DayDate{5, 2, 1}), func(c *Calendar) { c.CurrentMonth, c.CurrentDay = 1, 29 })
		if got := v(t, svc); len(got) != 1 || got[0].Month != 2 || got[0].Day != 1 || got[0].Lead != 2 {
			t.Fatalf("month rollover = %+v", got)
		}
	})

	t.Run("a month is read once however many forecast days land in it", func(t *testing.T) {
		reads := 0
		repo := *allDays
		inner := repo.listDaysFn
		repo.listDaysFn = func(ctx context.Context, id string, y, m int) ([]DayWeather, error) {
			reads++
			return inner(ctx, id, y, m)
		}
		svc, _ := forecastFixture(&repo, nil)
		v(t, svc)
		if reads != 1 {
			t.Errorf("read %d times, want 1", reads)
		}
	})

	t.Run("deterministic, and the same for a Director and a player", func(t *testing.T) {
		svc, _ := forecastFixture(allDays, nil)
		a, err1 := svc.ListWeatherForecast(ctx, "cal-1", "camp", player)
		b, err2 := svc.ListWeatherForecast(ctx, "cal-1", "camp", owner)
		c, err3 := svc.ListWeatherForecast(ctx, "cal-1", "camp", player)
		if err1 != nil || err2 != nil || err3 != nil || !reflect.DeepEqual(a, b) || !reflect.DeepEqual(a, c) {
			t.Fatalf("forecasts differ:\n%+v\n%+v\n%+v", a, b, c)
		}
	})

	t.Run("entries carry the confidence curve and never the real reading", func(t *testing.T) {
		repo := *allDays
		repo.getSettingsFn = func(context.Context, string) (*WeatherSettings, error) {
			return &WeatherSettings{ForecastDays: 10}, nil
		}
		svc, _ := forecastFixture(&repo, nil)
		got := v(t, svc)
		wantConf := map[int]float64{1: 0.97, 2: 0.82, 3: 0.69, 4: 0.58, 5: 0.49, 6: 0.41, 7: 0.35, 8: 0.29, 9: 0.25, 10: 0.21}
		for _, e := range got {
			if e.Confidence != wantConf[e.Lead] {
				t.Errorf("lead %d confidence %v, want %v", e.Lead, e.Confidence, wantConf[e.Lead])
			}
		}
		raw, _ := json.Marshal(got)
		for _, leak := range []string{"Fire rain", "fire-rain", "#ff5500", "secret", "ember", "preset", "color", "description", "wind", "label"} {
			if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(leak)) {
				t.Errorf("forecast leaks %q: %s", leak, raw)
			}
		}
		var keys map[string]any
		var arr []map[string]any
		if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
			t.Fatalf("not an array of objects: %s", raw)
		}
		keys = arr[0]
		for _, k := range []string{"year", "month", "day", "lead", "confidence", "icon", "words", "temp_low", "temp_high", "precip_chance"} {
			if _, ok := keys[k]; !ok {
				t.Errorf("entry missing key %q: %s", k, raw)
			}
		}
		if len(keys) != 10 {
			t.Errorf("entry has %d keys, want exactly 10: %s", len(keys), raw)
		}
	})

	t.Run("a calendar the viewer cannot see answers not found", func(t *testing.T) {
		svc, _ := forecastFixture(allDays, func(c *Calendar) { c.Visibility = "dm_only" })
		if _, err := svc.ListWeatherForecast(ctx, "cal-1", "camp", player); err == nil || apperror.SafeCode(err) != http.StatusNotFound {
			t.Fatalf("want not found, got %v", err)
		}
		if _, err := svc.ListWeatherForecast(ctx, "cal-1", "camp", owner); err != nil {
			t.Fatalf("the owner can see it: %v", err)
		}
	})

	t.Run("another campaign's calendar is not found", func(t *testing.T) {
		svc, _ := forecastFixture(allDays, nil)
		if _, err := svc.ListWeatherForecast(ctx, "cal-1", "other", player); err == nil || apperror.SafeCode(err) != http.StatusNotFound {
			t.Fatalf("want not found, got %v", err)
		}
	})

	t.Run("a repository failure is returned", func(t *testing.T) {
		boom := errors.New("boom")
		svc, _ := forecastFixture(&fakeWeatherRepo{listDaysFn: func(context.Context, string, int, int) ([]DayWeather, error) { return nil, boom }}, nil)
		if _, err := svc.ListWeatherForecast(ctx, "cal-1", "camp", player); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestGetWeatherSettings_ReportsForecastSwitchFromCalendar(t *testing.T) {
	for _, on := range []bool{true, false} {
		svc, _ := forecastFixture(&fakeWeatherRepo{}, func(c *Calendar) { c.ForecastsEnabled = on })
		got, err := svc.GetWeatherSettings(context.Background(), "cal-1", "camp", owner)
		if err != nil || got.ForecastsEnabled != on || got.ForecastDays != DefaultForecastDays {
			t.Errorf("enabled=%v: got %+v, %v", on, got, err)
		}
		raw, _ := json.Marshal(got)
		if !strings.Contains(string(raw), `"forecast_days":5`) || !strings.Contains(string(raw), `"forecasts_enabled":`) {
			t.Errorf("settings JSON lacks the forecast keys: %s", raw)
		}
	}
}

// Saving the switch goes through the calendar's own partial update: only
// forecasts_enabled changes, and an unchanged switch writes nothing.
func TestSetWeatherSettings_WritesForecastSwitchToCalendar(t *testing.T) {
	tests := []struct {
		name       string
		stored     bool
		in         bool
		wantUpdate bool
	}{
		{"turned on", false, true, true},
		{"turned off", true, false, true},
		{"unchanged on", true, true, false},
		{"unchanged off", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var savedDays int
			repo := &fakeWeatherRepo{setSettingsFn: func(_ context.Context, _ string, s WeatherSettings) error {
				savedDays = s.ForecastDays
				return nil
			}}
			svc, updated := forecastFixture(repo, func(c *Calendar) {
				c.ForecastsEnabled = tt.stored
				c.Description = strp("kept")
			})
			err := svc.SetWeatherSettings(context.Background(), "cal-1", "camp",
				WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: 7, ForecastsEnabled: tt.in})
			if err != nil {
				t.Fatalf("SetWeatherSettings: %v", err)
			}
			if savedDays != 7 {
				t.Errorf("forecast days stored = %d, want 7", savedDays)
			}
			if !tt.wantUpdate {
				if len(*updated) != 0 {
					t.Fatalf("calendar was rewritten: %+v", *updated)
				}
				return
			}
			if len(*updated) != 1 {
				t.Fatalf("calendar updated %d times, want once", len(*updated))
			}
			cal := (*updated)[0]
			if cal.ForecastsEnabled != tt.in || cal.Name != "Reckoning" || cal.Description == nil || *cal.Description != "kept" ||
				cal.CurrentYear != 5 || cal.CurrentMonth != 2 || cal.CurrentDay != 10 {
				t.Errorf("update changed more than the switch: %+v", cal)
			}
		})
	}
}

// TestStructureForm_Forecast covers the settings page's forecast controls:
// they render from the stored values, the preview carries them and notes what
// changed, and the save hands them to the service.
func TestStructureForm_Forecast(t *testing.T) {
	const preview = "/campaigns/camp-1/calendars/cal-1/structure/preview"
	const save = "/campaigns/camp-1/calendars/cal-1/structure"
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}

	post := func(t *testing.T, path string, stored WeatherSettings, extra url.Values) (*fakeCalendarSvc, string, string) {
		t.Helper()
		e, svc := newAccessTestRouter(false, true, roles)
		svc.weather = &stored
		form := url.Values{"import_json": {buildImportJSON(t, 3)}, "fingerprint": {"fp"}}
		for k, v := range extra {
			form[k] = v
		}
		rec := doRequestForm(e, path, "u-owner", form)
		return svc, rec.Body.String(), rec.Header().Get("HX-Redirect")
	}
	base := WeatherSettings{Climate: "temperate", Continuity: 0.55, ForecastDays: 5}
	on := base
	on.ForecastsEnabled = true

	t.Run("page renders the switch and reach from storage", func(t *testing.T) {
		tests := []struct {
			name    string
			stored  WeatherSettings
			want    []string
			notWant []string
		}{
			{"off, five days", base, []string{`name="weather_forecast_sent"`, `name="weather_forecast" value="on"`, `value="5" selected`,
				"Show players a forecast", "Days ahead", "Players see a guess that can be wrong. You always see the real weather.", `@change="clearPreview()"`},
				[]string{`value="on" checked`}},
			{"on, three days", WeatherSettings{Climate: "temperate", Continuity: 0.55, ForecastDays: 3, ForecastsEnabled: true},
				[]string{`checked`, `value="3" selected`, `<option value="10"`, `<option value="1"`}, []string{`<option value="11"`, `<option value="0"`}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				e, svc := newAccessTestRouter(false, true, roles)
				svc.weather = &tt.stored
				body := doRequest(e, http.MethodGet, save, "u-owner").Body.String()
				for _, w := range tt.want {
					if !strings.Contains(body, w) {
						t.Errorf("page missing %q", w)
					}
				}
				for _, w := range tt.notWant {
					if strings.Contains(body, w) {
						t.Errorf("page has %q", w)
					}
				}
			})
		}
	})

	t.Run("preview notes", func(t *testing.T) {
		tests := []struct {
			name      string
			stored    WeatherSettings
			form      url.Values
			wantNote  string
			noWeather bool
			wantHid   []string
			notHid    []string
		}{
			{"turned on with its reach", base, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {"5"}},
				"Weather: players now see a 5-day forecast.", false,
				[]string{`name="weather_forecast_sent" value="1"`, `name="weather_forecast" value="on"`, `name="weather_forecast_days" value="5"`}, nil},
			{"turned on and the reach changed together", base, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {"2"}},
				"Weather: players now see a 2-day forecast.", false, nil, nil},
			{"turned off by an unchecked box", on, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast_days": {"5"}},
				"Weather: players no longer see a forecast.", false,
				[]string{`name="weather_forecast_sent" value="1"`, `name="weather_forecast_days" value="5"`}, []string{`name="weather_forecast" value="on"`}},
			{"reach changed while on", on, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {"3"}},
				"Weather: the forecast now covers 3 days.", false, nil, nil},
			{"reach of one day is singular", on, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {"1"}},
				"Weather: the forecast now covers 1 day.", false, nil, nil},
			{"reach changed while off says nothing", base, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast_days": {"8"}},
				"", true, []string{`name="weather_forecast_days" value="8"`}, []string{`name="weather_forecast" value="on"`}},
			{"unchanged on says nothing", on, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {"5"}},
				"", true, nil, nil},
			{"unchanged off says nothing", base, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast_days": {"5"}},
				"", true, nil, nil},
			{"a reach alone keeps the stored switch", on, url.Values{"weather_forecast_days": {"4"}},
				"Weather: the forecast now covers 4 days.", false, []string{`name="weather_forecast" value="on"`}, nil},
			{"the marker is what makes an absent box mean off, so without it the switch stays", on, url.Values{"weather_continuity": {"0.55"}},
				"", true, []string{`name="weather_forecast" value="on"`}, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, body, _ := post(t, preview, tt.stored, tt.form)
				if strings.Contains(body, `class="calv5-err"`) {
					t.Fatalf("unexpected error:\n%s", body)
				}
				if tt.wantNote != "" && !strings.Contains(body, tt.wantNote) {
					t.Errorf("missing note %q:\n%s", tt.wantNote, body)
				}
				if tt.noWeather && strings.Contains(body, "Weather:") {
					t.Errorf("unexpected weather note:\n%s", body)
				}
				for _, h := range tt.wantHid {
					if !strings.Contains(body, h) {
						t.Errorf("preview missing hidden field %q", h)
					}
				}
				for _, h := range tt.notHid {
					if strings.Contains(body, h) {
						t.Errorf("preview has hidden field %q", h)
					}
				}
			})
		}
	})

	t.Run("a form with no weather fields carries no forecast", func(t *testing.T) {
		_, body, _ := post(t, preview, on, nil)
		if strings.Contains(body, "weather_forecast") || strings.Contains(body, "Weather:") {
			t.Errorf("a preview with no weather fields must not carry any:\n%s", body)
		}
	})

	t.Run("invalid reach is an error in the preview slot and refuses the save", func(t *testing.T) {
		for _, bad := range []string{"0", "11", "-3", "abc", "2.5"} {
			form := url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {bad}}
			_, body, _ := post(t, preview, base, form)
			if !strings.Contains(body, `class="calv5-err"`) || !strings.Contains(body, "1 to 10 days") || strings.Contains(body, "Before you save") {
				t.Errorf("reach %q: want an error only, got %s", bad, body)
			}
			svc, _, redirect := post(t, save, base, form)
			if redirect != "" || svc.savedWeather != nil {
				t.Errorf("reach %q: redirect %q, saved %+v", bad, redirect, svc.savedWeather)
			}
		}
	})

	t.Run("save hands the switch and reach to the service", func(t *testing.T) {
		tests := []struct {
			name       string
			stored     WeatherSettings
			form       url.Values
			wantOn     bool
			wantReach  int
			wantClimat string
		}{
			{"on", base, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast": {"on"}, "weather_forecast_days": {"7"}}, true, 7, "temperate"},
			{"off", on, url.Values{"weather_forecast_sent": {"1"}, "weather_forecast_days": {"5"}}, false, 5, "temperate"},
			{"reach only keeps the switch", on, url.Values{"weather_forecast_days": {"2"}}, true, 2, "temperate"},
			{"other weather fields keep the stored forecast", on, url.Values{"weather_climate": {"desert"}}, true, 5, "desert"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, body, redirect := post(t, save, tt.stored, tt.form)
				if redirect == "" || svc.savedWeather == nil {
					t.Fatalf("want a save, got redirect %q saved %+v: %s", redirect, svc.savedWeather, body)
				}
				got := svc.savedWeather
				if got.ForecastsEnabled != tt.wantOn || got.ForecastDays != tt.wantReach || got.Climate != tt.wantClimat {
					t.Errorf("saved %+v, want on=%v reach=%d climate=%s", got, tt.wantOn, tt.wantReach, tt.wantClimat)
				}
			})
		}
	})

	t.Run("save without weather fields writes no weather", func(t *testing.T) {
		svc, _, redirect := post(t, save, on, nil)
		if redirect == "" || svc.savedWeather != nil {
			t.Errorf("redirect %q, saved %+v", redirect, svc.savedWeather)
		}
	})
}

func TestValidateWeatherSettings_ForecastDays(t *testing.T) {
	for _, tt := range []struct {
		days int
		ok   bool
	}{{-1, false}, {0, false}, {1, true}, {5, true}, {10, true}, {11, false}} {
		err := validateWeatherSettings(WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: tt.days})
		if (err == nil) != tt.ok || (err != nil && !badRequest(err)) {
			t.Errorf("days %d: err %v, want ok=%v", tt.days, err, tt.ok)
		}
	}
}

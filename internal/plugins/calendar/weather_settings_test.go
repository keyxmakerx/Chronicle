package calendar

import (
	"context"
	"errors"
	"math"
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

func TestSetWeatherSettings(t *testing.T) {
	tests := []struct {
		name     string
		in       WeatherSettings
		wantErr  bool
		wantCont float64
	}{
		{"unknown climate", WeatherSettings{Climate: "swamp", Continuity: 0.5, ForecastDays: 5}, true, 0},
		{"empty climate", WeatherSettings{Continuity: 0.5, ForecastDays: 5}, true, 0},
		{"continuity below zero", WeatherSettings{Climate: "desert", Continuity: -0.1, ForecastDays: 5}, true, 0},
		{"continuity above one", WeatherSettings{Climate: "desert", Continuity: 1.1, ForecastDays: 5}, true, 0},
		{"continuity NaN", WeatherSettings{Climate: "desert", Continuity: math.NaN(), ForecastDays: 5}, true, 0},
		{"forecast days below one", WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: 0}, true, 0},
		{"forecast days negative", WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: -1}, true, 0},
		{"forecast days above ten", WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: 11}, true, 0},
		{"forecast days one", WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: 1}, false, 0.5},
		{"forecast days ten", WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: 10}, false, 0.5},
		{"lower bound", WeatherSettings{Climate: "tundra", Continuity: 0, ForecastDays: 5}, false, 0},
		{"upper bound", WeatherSettings{Climate: "gloomfen", Continuity: 1, ForecastDays: 5}, false, 1},
		{"rounds to two decimals", WeatherSettings{Climate: "ashlands", Continuity: 0.456, ForecastDays: 5}, false, 0.46},
		{"rounds down", WeatherSettings{Climate: "highland", Continuity: 0.504, ForecastDays: 5}, false, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved *WeatherSettings
			repo := &fakeWeatherRepo{setSettingsFn: func(_ context.Context, _ string, s WeatherSettings) error {
				saved = &s
				return nil
			}}
			err := dayWeatherFixture(repo).SetWeatherSettings(context.Background(), "cal", "camp", tt.in)
			if tt.wantErr {
				var ae *apperror.AppError
				if err == nil || !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
					t.Fatalf("want a bad request, got %v", err)
				}
				if saved != nil {
					t.Fatalf("nothing should be stored on a refusal, got %+v", saved)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetWeatherSettings: %v", err)
			}
			if saved == nil || saved.Climate != tt.in.Climate || saved.Continuity != tt.wantCont {
				t.Fatalf("stored %+v, want climate %q continuity %v", saved, tt.in.Climate, tt.wantCont)
			}
		})
	}
}

func TestSetWeatherSettings_WrongCampaignIsNotFound(t *testing.T) {
	err := dayWeatherFixture(&fakeWeatherRepo{}).SetWeatherSettings(context.Background(), "cal", "other",
		WeatherSettings{Climate: "desert", Continuity: 0.5, ForecastDays: 5})
	if err == nil || apperror.SafeCode(err) != http.StatusNotFound {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestGetWeatherSettings(t *testing.T) {
	tests := []struct {
		name   string
		stored *WeatherSettings
		want   WeatherSettings
	}{
		{"unset gives the defaults", nil, WeatherSettings{Climate: "temperate", Continuity: 0.55, Kinds: []WeatherKind{}, ForecastDays: 5}},
		{"stored is returned", &WeatherSettings{Climate: "desert", Continuity: 0.8, ForecastDays: 3}, WeatherSettings{Climate: "desert", Continuity: 0.8, Kinds: []WeatherKind{}, ForecastDays: 3}},
		{"an out-of-range stored forecast reach reads as the default", &WeatherSettings{Climate: "desert", Continuity: 0.8, ForecastDays: 40}, WeatherSettings{Climate: "desert", Continuity: 0.8, Kinds: []WeatherKind{}, ForecastDays: 5}},
		{"a retired climate id falls back but keeps the forecast reach", &WeatherSettings{Climate: "gone", Continuity: 0.8, ForecastDays: 7}, WeatherSettings{Climate: "temperate", Continuity: 0.55, Kinds: []WeatherKind{}, ForecastDays: 7}},
		{"a stored kind that no longer validates is dropped", &WeatherSettings{Climate: "desert", Continuity: 0.8, ForecastDays: 5, Kinds: []WeatherKind{fireRain, {ID: "rain", Name: "Rain", Icon: "rain", Color: "#112233", Like: "rain"}}}, WeatherSettings{Climate: "desert", Continuity: 0.8, ForecastDays: 5, Kinds: []WeatherKind{fireRainClean}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeWeatherRepo{getSettingsFn: func(context.Context, string) (*WeatherSettings, error) { return tt.stored, nil }}
			got, err := dayWeatherFixture(repo).GetWeatherSettings(context.Background(), "cal", "camp", permissions.RequestViewer(int(permissions.RoleOwner), "u1"))
			if err != nil || got == nil || !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

// TestStructureForm_WeatherCarry covers the settings page's weather fields:
// the editor renders them, the preview carries them as hidden inputs with an
// "Also" note only when they differ from stored, and the save stores them.
func TestStructureForm_WeatherCarry(t *testing.T) {
	const preview = "/campaigns/camp-1/calendars/cal-1/structure/preview"
	const save = "/campaigns/camp-1/calendars/cal-1/structure"
	owner := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}

	post := func(t *testing.T, path string, extra url.Values) (*fakeCalendarSvc, string, int, string) {
		t.Helper()
		e, svc := newAccessTestRouter(false, true, owner)
		form := url.Values{"import_json": {buildImportJSON(t, 3)}, "fingerprint": {"fp"}}
		for k, v := range extra {
			form[k] = v
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "u-owner"})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return svc, rec.Body.String(), rec.Code, rec.Header().Get("HX-Redirect")
	}

	t.Run("page renders both controls with the stored values selected", func(t *testing.T) {
		e, svc := newAccessTestRouter(false, true, owner)
		svc.weather = &WeatherSettings{Climate: "ashlands", Continuity: 0.8, ForecastDays: 5}
		body := doRequest(e, http.MethodGet, save, "u-owner").Body.String()
		for _, want := range []string{`name="weather_climate"`, `name="weather_continuity"`, `value="ashlands" selected`,
			`value="0.8"`, `label="Natural"`, `label="Magic"`, "How long weather lasts", `@change="clearPreview()"`} {
			if !strings.Contains(body, want) {
				t.Errorf("page missing %q", want)
			}
		}
	})

	t.Run("preview carries changed values and notes them", func(t *testing.T) {
		_, body, code, _ := post(t, preview, url.Values{"weather_climate": {"ashlands"}, "weather_continuity": {"0.9"}})
		if code != http.StatusOK {
			t.Fatalf("got %d: %s", code, body)
		}
		for _, want := range []string{`name="weather_climate" value="ashlands"`, `name="weather_continuity" value="0.9"`,
			"Weather: the climate changes to Ashlands.", "Weather: weather lasts longer."} {
			if !strings.Contains(body, want) {
				t.Errorf("preview missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("preview of unchanged values carries them but adds no note", func(t *testing.T) {
		_, body, _, _ := post(t, preview, url.Values{"weather_climate": {"temperate"}, "weather_continuity": {"0.55"}})
		if strings.Contains(body, "Weather:") || !strings.Contains(body, `name="weather_climate" value="temperate"`) {
			t.Errorf("unexpected preview:\n%s", body)
		}
	})

	t.Run("preview without the fields keeps stored settings", func(t *testing.T) {
		_, body, _, _ := post(t, preview, nil)
		if strings.Contains(body, `name="weather_climate"`) || strings.Contains(body, "Weather:") {
			t.Errorf("a preview with no weather fields must not carry any:\n%s", body)
		}
	})

	t.Run("a lower continuity is noted", func(t *testing.T) {
		_, body, _, _ := post(t, preview, url.Values{"weather_continuity": {"0.1"}})
		if !strings.Contains(body, "Weather: weather changes more often.") {
			t.Errorf("preview missing the shorter-spell note:\n%s", body)
		}
	})

	for _, bad := range []url.Values{
		{"weather_climate": {"swamp"}},
		{"weather_continuity": {"1.5"}},
		{"weather_continuity": {"abc"}},
		{"weather_continuity": {"NaN"}},
	} {
		t.Run("invalid "+bad.Encode()+" shows an error in the preview slot", func(t *testing.T) {
			_, body, code, _ := post(t, preview, bad)
			if code != http.StatusOK || !strings.Contains(body, `class="calv5-err"`) || strings.Contains(body, "Before you save") {
				t.Errorf("want an error only, got %d: %s", code, body)
			}
		})
	}

	t.Run("save stores the weather and redirects", func(t *testing.T) {
		svc, body, code, redirect := post(t, save, url.Values{"weather_climate": {"fey-wilds"}, "weather_continuity": {"0.35"}})
		if redirect == "" {
			t.Fatalf("want a redirect, got %d: %s", code, body)
		}
		if svc.savedWeather == nil || svc.savedWeather.Climate != "fey-wilds" || svc.savedWeather.Continuity != 0.35 {
			t.Errorf("saved %+v", svc.savedWeather)
		}
	})

	t.Run("save without the fields writes no weather", func(t *testing.T) {
		svc, _, _, redirect := post(t, save, nil)
		if redirect == "" || svc.savedWeather != nil {
			t.Errorf("redirect %q, saved %+v", redirect, svc.savedWeather)
		}
	})

	t.Run("save with an invalid climate refuses before anything is written", func(t *testing.T) {
		svc, body, _, redirect := post(t, save, url.Values{"weather_climate": {"swamp"}})
		if redirect != "" || svc.savedWeather != nil || !strings.Contains(body, `class="calv5-err"`) {
			t.Errorf("redirect %q, saved %+v, body %s", redirect, svc.savedWeather, body)
		}
	})
}

// TestStructureForm_WeatherKinds covers the own-weather editor: the page
// prefills it, the preview validates and notes each change by kind, the save
// stores the kinds, and an absent field keeps what is stored.
func TestStructureForm_WeatherKinds(t *testing.T) {
	const preview = "/campaigns/camp-1/calendars/cal-1/structure/preview"
	const save = "/campaigns/camp-1/calendars/cal-1/structure"
	owner := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	fireJSON := encodeWeatherKinds([]WeatherKind{fireRainClean})

	post := func(t *testing.T, path string, stored []WeatherKind, extra url.Values) (*fakeCalendarSvc, string, string) {
		t.Helper()
		e, svc := newAccessTestRouter(false, true, owner)
		svc.weather = &WeatherSettings{Climate: "temperate", Continuity: 0.55, ForecastDays: 5, Kinds: stored}
		form := url.Values{"import_json": {buildImportJSON(t, 3)}, "fingerprint": {"fp"}}
		for k, v := range extra {
			form[k] = v
		}
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "u-owner"})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return svc, rec.Body.String(), rec.Header().Get("HX-Redirect")
	}

	t.Run("page prefills the editor from the stored kinds", func(t *testing.T) {
		e, svc := newAccessTestRouter(false, true, owner)
		svc.weather = &WeatherSettings{Climate: "temperate", Continuity: 0.55, ForecastDays: 5, Kinds: []WeatherKind{fireRainClean}}
		body := doRequest(e, http.MethodGet, save, "u-owner").Body.String()
		for _, want := range []string{`name="weather_kinds"`, "Your own weather", "+ Add weather", "Behaves like",
			"its temperature, wind and wetness", "Same as the weather it's like", `value="thunderstorm"`, `value="rain-acid"`,
			"What the day says", "How long it lingers", "Sky effect", "Fleeting", `aria-pressed`, "<legend>Winter</legend>",
			"&#34;id&#34;:&#34;fire-rain&#34;"} {
			if !strings.Contains(body, want) {
				t.Errorf("page missing %q", want)
			}
		}
	})

	t.Run("preview carries the kinds and notes an added one", func(t *testing.T) {
		_, body, _ := post(t, preview, nil, url.Values{"weather_kinds": {fireJSON}})
		if !strings.Contains(body, "Weather: Fire rain added.") || !strings.Contains(body, `name="weather_kinds"`) ||
			!strings.Contains(body, "&#34;id&#34;:&#34;fire-rain&#34;") {
			t.Errorf("preview:\n%s", body)
		}
	})

	t.Run("preview notes a removed and a changed kind", func(t *testing.T) {
		_, body, _ := post(t, preview, []WeatherKind{fireRainClean}, url.Values{"weather_kinds": {"[]"}})
		if !strings.Contains(body, "Weather: Fire rain removed.") {
			t.Errorf("want a removed note:\n%s", body)
		}
		changed := encodeWeatherKinds([]WeatherKind{kindWith(func(k *WeatherKind) { k.Name = "Fire rain!" })})
		_, body, _ = post(t, preview, []WeatherKind{fireRainClean}, url.Values{"weather_kinds": {changed}})
		if !strings.Contains(body, "Weather: Fire rain! changed.") {
			t.Errorf("want a changed note:\n%s", body)
		}
	})

	t.Run("unchanged kinds add no note", func(t *testing.T) {
		_, body, _ := post(t, preview, []WeatherKind{fireRainClean}, url.Values{"weather_kinds": {fireJSON}})
		if strings.Contains(body, "Weather:") {
			t.Errorf("unexpected note:\n%s", body)
		}
	})

	t.Run("absent field keeps the stored kinds", func(t *testing.T) {
		svc, body, redirect := post(t, save, []WeatherKind{fireRainClean}, url.Values{"weather_continuity": {"0.3"}})
		if redirect == "" || svc.savedWeather == nil || len(svc.savedWeather.Kinds) != 1 {
			t.Errorf("redirect %q, saved %+v, body %s", redirect, svc.savedWeather, body)
		}
	})

	t.Run("an empty list clears them", func(t *testing.T) {
		svc, _, redirect := post(t, save, []WeatherKind{fireRainClean}, url.Values{"weather_kinds": {"[]"}})
		if redirect == "" || svc.savedWeather == nil || len(svc.savedWeather.Kinds) != 0 {
			t.Errorf("redirect %q, saved %+v", redirect, svc.savedWeather)
		}
	})

	t.Run("save stores the cleaned kinds", func(t *testing.T) {
		raw := encodeWeatherKinds([]WeatherKind{fireRain})
		svc, _, redirect := post(t, save, nil, url.Values{"weather_kinds": {raw}})
		if redirect == "" || svc.savedWeather == nil || !reflect.DeepEqual(svc.savedWeather.Kinds, []WeatherKind{fireRainClean}) {
			t.Errorf("redirect %q, saved %+v", redirect, svc.savedWeather)
		}
	})

	badBuiltin := encodeWeatherKinds([]WeatherKind{kindWith(func(k *WeatherKind) { k.ID, k.Name = "rain", "Rain" })})
	for name, raw := range map[string]string{
		"built-in name":  badBuiltin,
		"unknown field":  `[{"id":"a","name":"A","icon":"rain","color":"#000000","like":"rain","bogus":1}]`,
		"not json":       "nope",
		"missing colour": `[{"id":"a","name":"A","icon":"rain","like":"rain"}]`,
	} {
		t.Run("invalid "+name+" is an error in the preview slot", func(t *testing.T) {
			_, body, _ := post(t, preview, nil, url.Values{"weather_kinds": {raw}})
			if !strings.Contains(body, `class="calv5-err"`) || strings.Contains(body, "Before you save") {
				t.Errorf("want an error only:\n%s", body)
			}
		})
		t.Run("invalid "+name+" refuses the save before anything is written", func(t *testing.T) {
			svc, _, redirect := post(t, save, nil, url.Values{"weather_kinds": {raw}})
			if redirect != "" || svc.savedWeather != nil {
				t.Errorf("redirect %q, saved %+v", redirect, svc.savedWeather)
			}
		})
	}

	t.Run("the built-in clash reads in the owner's words", func(t *testing.T) {
		_, body, _ := post(t, preview, nil, url.Values{"weather_kinds": {badBuiltin}})
		if !strings.Contains(body, "Rain has the same name as the built-in weather Rain; give yours a different name.") {
			t.Errorf("message missing:\n%s", body)
		}
	})
}

func TestGetWeatherSettingsAPI_KindsDefaultToEmptyList(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{"u-owner": campaigns.RoleOwner})
	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1/weather/settings", "u-owner")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kinds":[]`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestWeatherClimates_ValidClimate(t *testing.T) {
	if len(WeatherClimates) != 10 {
		t.Fatalf("got %d climates, want 10", len(WeatherClimates))
	}
	if !validClimate(DefaultWeatherClimate) {
		t.Error("the default climate must be listed")
	}
	if validClimate("") || validClimate("swamp") {
		t.Error("unknown ids must be refused")
	}
}

// The settings page's readout must use the Generate sheet's words at the
// same thresholds (continuityWords in calendar_weather_sheet.js).
func TestContinuityWords(t *testing.T) {
	tests := []struct {
		v    float64
		want string
	}{
		{0, "Changes daily"}, {0.24, "Changes daily"}, {0.25, "Changeable"},
		{0.5, "Some spells"}, {0.55, "Some spells"}, {0.75, "Long spells"}, {1, "Long spells"},
	}
	for _, tt := range tests {
		if got := continuityWords(tt.v); got != tt.want {
			t.Errorf("continuityWords(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

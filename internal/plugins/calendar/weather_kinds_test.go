package calendar

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fireRain is a valid kind with only the required parts, as the editor
// sends it before cleaning; fireRainClean is what validation returns.
var (
	fireRain = WeatherKind{
		ID: "fire-rain", Name: "  Fire rain ", Icon: "rain", Color: "#ff5500", Like: "thunderstorm",
		Seasons: WeatherKindSeasons{Summer: "often", Winter: "off"},
		Lasts:   "normal",
	}
	fireRainClean = WeatherKind{
		ID: "fire-rain", Name: "Fire rain", Icon: "rain", Color: "#ff5500", Like: "thunderstorm",
		Seasons: WeatherKindSeasons{Winter: "off", Spring: "normal", Summer: "often", Autumn: "normal"},
	}
)

func kindWith(mut func(*WeatherKind)) WeatherKind {
	k := fireRainClean
	mut(&k)
	return k
}

func TestValidateWeatherKinds(t *testing.T) {
	tests := []struct {
		name    string
		in      []WeatherKind
		want    []WeatherKind
		wantErr string // substring of the owner-readable message; empty = valid
	}{
		{name: "none is an empty list", in: nil, want: []WeatherKind{}},
		{name: "cleans name, fills seasons, drops the default lasts", in: []WeatherKind{fireRain}, want: []WeatherKind{fireRainClean}},
		{name: "everything optional kept", in: []WeatherKind{kindWith(func(k *WeatherKind) {
			k.Lasts = "stable"
			k.Words = []string{" Embers fall ", "The air shimmers"}
			k.Magic = true
			k.Look = &WeatherKindLook{Effect: "embers"}
		})}, want: []WeatherKind{kindWith(func(k *WeatherKind) {
			k.Lasts = "stable"
			k.Words = []string{"Embers fall", "The air shimmers"}
			k.Magic = true
			k.Look = &WeatherKindLook{Effect: "embers"}
		})}},
		{name: "an empty effect is none", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Look = &WeatherKindLook{} })}, want: []WeatherKind{fireRainClean}},
		{name: "name at the limit", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Name = strings.Repeat("a", 40) })}, want: []WeatherKind{kindWith(func(k *WeatherKind) { k.Name = strings.Repeat("a", 40) })}},
		{name: "name too long", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Name = strings.Repeat("a", 41) })}, wantErr: "41 characters"},
		{name: "blank name", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Name = "   " })}, wantErr: "Give your weather a name"},
		{name: "empty id", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.ID = "" })}, wantErr: "at least one letter or number"},
		{name: "id with capitals", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.ID = "Fire" })}, wantErr: "lowercase letters"},
		{name: "id with double dash", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.ID = "a--b" })}, wantErr: "lowercase letters"},
		{name: "id too long", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.ID = strings.Repeat("a", 41) })}, wantErr: "up to 40"},
		{name: "built-in id", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.ID, k.Name = "acid-rain", "Acid rain" })}, wantErr: "Acid rain has the same name as the built-in weather Acid Rain; give yours a different name."},
		{name: "duplicate id", in: []WeatherKind{fireRainClean, kindWith(func(k *WeatherKind) { k.Name = "Other" })}, wantErr: "already called"},
		{name: "unknown icon", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Icon = "sun" })}, wantErr: "needs an icon"},
		{name: "bad colour", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Color = "red" })}, wantErr: "colour"},
		{name: "short hex colour", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Color = "#f50" })}, wantErr: "colour"},
		{name: "unknown like", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Like = "lava" })}, wantErr: "behaves like"},
		{name: "missing like", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Like = "" })}, wantErr: "behaves like"},
		{name: "bad season level", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Seasons.Spring = "sometimes" })}, wantErr: "off, rare, normal or often"},
		{name: "off in every season", in: []WeatherKind{kindWith(func(k *WeatherKind) {
			k.Seasons = WeatherKindSeasons{Winter: "off", Spring: "off", Summer: "off", Autumn: "off"}
		})}, wantErr: "off in every season"},
		{name: "bad lasts", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Lasts = "forever" })}, wantErr: "fleeting, normal or stable"},
		{name: "eight lines is fine", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{"1", "2", "3", "4", "5", "6", "7", "8"} })}, want: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{"1", "2", "3", "4", "5", "6", "7", "8"} })}},
		{name: "nine lines", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} })}, wantErr: "keep it to 8"},
		{name: "empty line", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{"ok", " "} })}, wantErr: "Line 2"},
		{name: "line at the limit", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{strings.Repeat("a", 34)} })}, want: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{strings.Repeat("a", 34)} })}},
		{name: "line too long", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Words = []string{strings.Repeat("a", 35)} })}, wantErr: "35 characters"},
		{name: "unknown effect", in: []WeatherKind{kindWith(func(k *WeatherKind) { k.Look = &WeatherKindLook{Effect: "lava"} })}, wantErr: "sky effect"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateWeatherKinds(tt.in)
			if tt.wantErr != "" {
				if err == nil || apperror.SafeCode(err) != http.StatusBadRequest || !strings.Contains(apperror.UserMessage(err, ""), tt.wantErr) {
					t.Fatalf("want a bad request containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("at most 24 kinds", func(t *testing.T) {
		var many []WeatherKind
		for i := 0; i < 25; i++ {
			many = append(many, kindWith(func(k *WeatherKind) { k.ID = "k" + strings.Repeat("a", i) }))
		}
		if _, err := validateWeatherKinds(many[:24]); err != nil {
			t.Fatalf("24 should be fine: %v", err)
		}
		if _, err := validateWeatherKinds(many); err == nil || !strings.Contains(apperror.UserMessage(err, ""), "at most 24") {
			t.Fatalf("25 should be refused, got %v", err)
		}
	})
}

func TestParseWeatherKinds(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{"empty is none", "", 0, false},
		{"null is none", "null", 0, false},
		{"empty array", "[]", 0, false},
		{"one kind", `[{"id":"a","name":"A","icon":"rain","color":"#000000","like":"rain","seasons":{"winter":"off"}}]`, 1, false},
		{"unknown field", `[{"id":"a","name":"A","climates":["tundra"]}]`, 0, true},
		{"unknown look field", `[{"id":"a","look":{"effect":"rain","tint":"#fff"}}]`, 0, true},
		{"unknown season", `[{"id":"a","seasons":{"wet":"off"}}]`, 0, true},
		{"not an array", `{"id":"a"}`, 0, true},
		{"trailing data", `[] []`, 0, true},
		{"garbage", `nope`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWeatherKinds(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if apperror.SafeCode(err) != http.StatusBadRequest {
					t.Errorf("want a bad request, got %v", err)
				}
				return
			}
			if got == nil || len(got) != tt.wantLen {
				t.Errorf("got %#v, want %d kinds (non-nil)", got, tt.wantLen)
			}
		})
	}
}

func TestEncodeWeatherKinds(t *testing.T) {
	if got := encodeWeatherKinds(nil); got != "[]" {
		t.Errorf("nil encodes as %q, want []", got)
	}
	want := `[{"id":"fire-rain","name":"Fire rain","icon":"rain","color":"#ff5500","like":"thunderstorm","seasons":{"winter":"off","spring":"normal","summer":"often","autumn":"normal"}}]`
	if got := encodeWeatherKinds([]WeatherKind{fireRainClean}); got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	back, err := parseWeatherKinds(want)
	if err != nil || !reflect.DeepEqual(back, []WeatherKind{fireRainClean}) {
		t.Errorf("round trip = %+v, %v", back, err)
	}
}

func TestSlugWeatherKindID(t *testing.T) {
	tests := map[string]string{
		"Fire rain":      "fire-rain",
		"  Mana  Storm!": "mana-storm",
		"A--B":           "a-b",
		"!!!":            "",
		"Acid rain 2":    "acid-rain-2",
	}
	for in, want := range tests {
		if got := slugWeatherKindID(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWeatherKindNotes(t *testing.T) {
	changed := kindWith(func(k *WeatherKind) { k.Magic = true })
	other := kindWith(func(k *WeatherKind) { k.ID, k.Name = "acid-fog", "Acid fog" })
	tests := []struct {
		name      string
		cur, next []WeatherKind
		want      []string
	}{
		{"no change", []WeatherKind{fireRainClean}, []WeatherKind{fireRainClean}, nil},
		{"added", nil, []WeatherKind{fireRainClean}, []string{"Weather: Fire rain added."}},
		{"removed", []WeatherKind{fireRainClean}, nil, []string{"Weather: Fire rain removed."}},
		{"changed", []WeatherKind{fireRainClean}, []WeatherKind{changed}, []string{"Weather: Fire rain changed."}},
		{"one note per kind", []WeatherKind{fireRainClean, other}, []WeatherKind{changed}, []string{"Weather: Fire rain changed.", "Weather: Acid fog removed."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weatherKindNotes(tt.cur, tt.next); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWeatherTables(t *testing.T) {
	if len(weatherPresets) != 42 || weatherPresets[0].ID != "clear" {
		t.Errorf("presets: %d entries, first %v", len(weatherPresets), weatherPresets[0])
	}
	if len(weatherEffects) != 41 || weatherEffects[0].ID != "clear" {
		t.Errorf("effects: %d entries, first %v", len(weatherEffects), weatherEffects[0])
	}
	seen := map[string]bool{}
	for _, p := range weatherPresets {
		if seen[p.ID] {
			t.Errorf("duplicate preset %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestWeatherSettings_JSONKindsNeverNull(t *testing.T) {
	got, err := (WeatherSettings{Climate: "desert", Continuity: 0.5}).MarshalJSON()
	if err != nil || !strings.Contains(string(got), `"kinds":[]`) {
		t.Fatalf("got %s, %v", got, err)
	}
}

// Lengths are counted as the JS generator counts them: an emoji is two.
func TestJSLength(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{{"Fire rain", 9}, {"Selûne", 6}, {"🔥", 2}, {"", 0}}
	for _, tt := range tests {
		if got := jsLength(tt.in); got != tt.want {
			t.Errorf("jsLength(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

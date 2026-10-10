// Package calendar - weather_kinds.go validates the owner's own kinds of
// weather (calendar_weather_settings.kinds). The rules mirror
// validateWeatherKind in static/js/widgets/chronicle_gen.js, which is the
// generator that consumes these kinds, so a kind saved here is one the
// generator accepts. A JS test pins weatherPresets and weatherEffects to the
// generator's tables; keep each entry on its own line in that exact form.
package calendar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// weatherPreset is a built-in weather a kind can borrow its temperature,
// wind and wetness from.
type weatherPreset struct {
	ID    string
	Label string
}

// weatherEffect is a sky effect a kind can ask the renderer to draw.
type weatherEffect struct {
	ID    string
	Label string
}

// weatherPresets is the generator's built-in weathers in its order.
var weatherPresets = []weatherPreset{
	{ID: "clear", Label: "Clear"},
	{ID: "partly-cloudy", Label: "Partly Cloudy"},
	{ID: "cloudy", Label: "Cloudy"},
	{ID: "overcast", Label: "Overcast"},
	{ID: "drizzle", Label: "Drizzle"},
	{ID: "rain", Label: "Rain"},
	{ID: "fog", Label: "Fog"},
	{ID: "mist", Label: "Mist"},
	{ID: "windy", Label: "Windy"},
	{ID: "sunshower", Label: "Sunshower"},
	{ID: "snow", Label: "Snow"},
	{ID: "sleet", Label: "Sleet"},
	{ID: "heat-wave", Label: "Heat Wave"},
	{ID: "thunderstorm", Label: "Thunderstorm"},
	{ID: "blizzard", Label: "Blizzard"},
	{ID: "hail", Label: "Hail"},
	{ID: "tornado", Label: "Tornado"},
	{ID: "hurricane", Label: "Hurricane"},
	{ID: "ice-storm", Label: "Ice Storm"},
	{ID: "monsoon", Label: "Monsoon"},
	{ID: "ashfall", Label: "Ashfall"},
	{ID: "sandstorm", Label: "Sandstorm"},
	{ID: "luminous-sky", Label: "Luminous Sky"},
	{ID: "sakura-bloom", Label: "Sakura Bloom"},
	{ID: "autumn-leaves", Label: "Autumn Leaves"},
	{ID: "rolling-fog", Label: "Rolling Fog"},
	{ID: "wildfire-smoke", Label: "Wildfire Smoke"},
	{ID: "dust-devil", Label: "Dust Devil"},
	{ID: "black-sun", Label: "Black Sun"},
	{ID: "ley-surge", Label: "Ley Surge"},
	{ID: "aether-haze", Label: "Aether Haze"},
	{ID: "nullfront", Label: "Nullfront"},
	{ID: "permafrost-surge", Label: "Permafrost Surge"},
	{ID: "gravewind", Label: "Gravewind"},
	{ID: "veilfall", Label: "Veilfall"},
	{ID: "arcane-winds", Label: "Arcane Winds"},
	{ID: "acid-rain", Label: "Acid Rain"},
	{ID: "blood-rain", Label: "Blood Rain"},
	{ID: "meteor-shower", Label: "Meteor Shower"},
	{ID: "spore-cloud", Label: "Spore Cloud"},
	{ID: "divine-light", Label: "Divine Light"},
	{ID: "plague-miasma", Label: "Plague Miasma"},
}

// weatherEffects is the generator's sky effects in its order.
var weatherEffects = []weatherEffect{
	{ID: "clear", Label: "Clear skies"},
	{ID: "clouds-light", Label: "Light clouds"},
	{ID: "clouds-heavy", Label: "Heavy clouds"},
	{ID: "clouds-overcast", Label: "Overcast"},
	{ID: "rain", Label: "Rain"},
	{ID: "rain-heavy", Label: "Heavy rain"},
	{ID: "snow", Label: "Snow"},
	{ID: "snow-heavy", Label: "Heavy snow"},
	{ID: "fog", Label: "Fog"},
	{ID: "lightning", Label: "Lightning"},
	{ID: "sand", Label: "Blowing sand"},
	{ID: "ashfall", Label: "Ashfall"},
	{ID: "embers", Label: "Drifting embers"},
	{ID: "ice", Label: "Ice"},
	{ID: "hail", Label: "Hail"},
	{ID: "tornado", Label: "Tornado"},
	{ID: "hurricane", Label: "Hurricane"},
	{ID: "nullstatic", Label: "Null static"},
	{ID: "gust", Label: "Gusting wind"},
	{ID: "aurora", Label: "Aurora"},
	{ID: "aether", Label: "Aether haze"},
	{ID: "void", Label: "Void"},
	{ID: "spectral", Label: "Spectral wind"},
	{ID: "arcane", Label: "Arcane energy"},
	{ID: "arcane-wind", Label: "Arcane wind"},
	{ID: "veil", Label: "The veil"},
	{ID: "petals", Label: "Falling petals"},
	{ID: "sleet", Label: "Sleet"},
	{ID: "haze", Label: "Heat haze"},
	{ID: "leaves", Label: "Falling leaves"},
	{ID: "smoke", Label: "Smoke"},
	{ID: "rain-acid", Label: "Acid rain"},
	{ID: "rain-blood", Label: "Blood rain"},
	{ID: "meteors", Label: "Meteors"},
	{ID: "spores", Label: "Spores"},
	{ID: "divine", Label: "Divine light"},
	{ID: "miasma", Label: "Miasma"},
	{ID: "ley-surge", Label: "Ley surge"},
	{ID: "motes", Label: "Drifting motes"},
	{ID: "sparks", Label: "Sparks"},
	{ID: "sigils", Label: "Floating sigils"},
}

const (
	// maxWeatherKinds bounds the list so the settings row and the preview stay
	// small; 24 is far more than a campaign writes by hand.
	maxWeatherKinds = 24
	// maxWeatherKindName, maxWeatherKindID and maxWeatherWordLen match the
	// generator's limits (a name and id fit a card, a line fits a day cell).
	maxWeatherKindName = 40
	maxWeatherKindID   = 40
	maxWeatherWordLen  = 34
	maxWeatherWords    = 8
)

var (
	weatherKindIDRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	weatherKindColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

	weatherKindIcons  = []string{"clear", "cloud", "rain", "snow", "storm", "fog"}
	weatherKindLevels = []string{"off", "rare", "normal", "often"}
	weatherKindLasts  = []string{"fleeting", "normal", "stable"}
)

// WeatherKind is one of the owner's own kinds of weather, in exactly the
// shape the generator accepts as its `kinds` option. Optional parts are
// omitted when they hold their default, so a stored kind stays minimal.
type WeatherKind struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Icon    string             `json:"icon"`
	Color   string             `json:"color"`
	Like    string             `json:"like"`
	Seasons WeatherKindSeasons `json:"seasons"`
	Lasts   string             `json:"lasts,omitempty"`
	Words   []string           `json:"words,omitempty"`
	Magic   bool               `json:"magic,omitempty"`
	Look    *WeatherKindLook   `json:"look,omitempty"`
}

// WeatherKindSeasons is how often a kind comes in each season: off, rare,
// normal or often. A missing level reads as normal.
type WeatherKindSeasons struct {
	Winter string `json:"winter"`
	Spring string `json:"spring"`
	Summer string `json:"summer"`
	Autumn string `json:"autumn"`
}

// WeatherKindLook is the sky effect drawn for a kind; without one the sky
// uses the effect of the built-in weather the kind is like.
type WeatherKindLook struct {
	Effect string `json:"effect"`
}

// levels returns pointers to the four season levels so a caller can fill a
// missing one in place.
func (s *WeatherKindSeasons) levels() [4]*string {
	return [4]*string{&s.Winter, &s.Spring, &s.Summer, &s.Autumn}
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// weatherPresetLabel returns the built-in weather's label for id.
func weatherPresetLabel(id string) (string, bool) {
	for _, p := range weatherPresets {
		if p.ID == id {
			return p.Label, true
		}
	}
	return "", false
}

func weatherEffectKnown(id string) bool {
	for _, e := range weatherEffects {
		if e.ID == id {
			return true
		}
	}
	return false
}

// slugWeatherKindID is the id the editor derives from a name; it is here so
// tests can pin the Go and editor rules to one definition.
func slugWeatherKindID(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	return b.String()
}

// validateWeatherKind checks one kind and returns it cleaned: name trimmed,
// season levels filled in, defaults omitted. taken holds the ids of the
// kinds already accepted, so a repeat is refused. Every message is written
// for the owner who typed the kind.
func validateWeatherKind(k WeatherKind, taken map[string]bool) (WeatherKind, error) {
	bad := func(format string, args ...any) (WeatherKind, error) {
		return WeatherKind{}, apperror.NewBadRequest(fmt.Sprintf(format, args...))
	}
	k.Name = strings.TrimSpace(k.Name)
	label, it := "This weather", "this weather"
	if k.Name != "" {
		label = "“" + k.Name + "”"
		it = label
	}
	if k.Name == "" {
		return bad("Give your weather a name, like \"Mana storm\".")
	}
	if n := jsLength(k.Name); n > maxWeatherKindName {
		return bad("%s is %d characters long; keep weather names to %d or fewer.", label, n, maxWeatherKindName)
	}
	switch {
	case k.ID == "":
		return bad("%s needs a name with at least one letter or number.", label)
	case len(k.ID) > maxWeatherKindID || !weatherKindIDRe.MatchString(k.ID):
		return bad("%s can't be saved: its id must be lowercase letters, numbers and single dashes, up to %d characters.", label, maxWeatherKindID)
	}
	if builtin, ok := weatherPresetLabel(k.ID); ok {
		return bad("%s has the same name as the built-in weather %s; give yours a different name.", k.Name, builtin)
	}
	if taken[k.ID] {
		return bad("Another of your weathers is already called %s; give each its own name.", label)
	}
	if !containsString(weatherKindIcons, k.Icon) {
		return bad("%s needs an icon: clear, cloud, rain, snow, storm or fog.", label)
	}
	if !weatherKindColorRe.MatchString(k.Color) {
		return bad("%s needs a colour written like #7a5cff.", label)
	}
	if _, ok := weatherPresetLabel(k.Like); !ok {
		return bad("Choose which built-in weather %s behaves like.", it)
	}
	allOff := true
	for _, lv := range k.Seasons.levels() {
		if *lv == "" {
			*lv = "normal"
		}
		if !containsString(weatherKindLevels, *lv) {
			return bad("How often %s comes can be off, rare, normal or often in each season.", it)
		}
		allOff = allOff && *lv == "off"
	}
	if allOff {
		return bad("%s is off in every season, so it would never come. Switch a season on.", label)
	}
	if k.Lasts == "normal" {
		k.Lasts = ""
	}
	if k.Lasts != "" && !containsString(weatherKindLasts, k.Lasts) {
		return bad("How long %s lingers can be fleeting, normal or stable.", it)
	}
	var words []string
	for i, w := range k.Words {
		w = strings.TrimSpace(w)
		if w == "" {
			return bad("Line %d of what the day says for %s is empty; remove it or write something.", i+1, it)
		}
		if n := jsLength(w); n > maxWeatherWordLen {
			return bad("\u201c%s\u201d is %d characters; keep each line of what the day says to %d so it fits in a day.", w, n, maxWeatherWordLen)
		}
		words = append(words, w)
	}
	if len(words) > maxWeatherWords {
		return bad("%s has %d lines of what the day says; keep it to %d.", label, len(words), maxWeatherWords)
	}
	k.Words = words
	if k.Look != nil {
		if k.Look.Effect == "" {
			k.Look = nil
		} else if !weatherEffectKnown(k.Look.Effect) {
			return bad("%s's sky effect isn't one the sky can draw; choose one from the list.", label)
		}
	}
	return k, nil
}

// validateWeatherKinds checks the whole list and returns it cleaned, never
// nil. The first problem stops it, named after the kind it is in.
func validateWeatherKinds(kinds []WeatherKind) ([]WeatherKind, error) {
	if len(kinds) > maxWeatherKinds {
		return nil, apperror.NewBadRequest(fmt.Sprintf("You can have at most %d of your own weathers.", maxWeatherKinds))
	}
	out := make([]WeatherKind, 0, len(kinds))
	taken := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		clean, err := validateWeatherKind(k, taken)
		if err != nil {
			return nil, err
		}
		taken[clean.ID] = true
		out = append(out, clean)
	}
	return out, nil
}

// keepValidWeatherKinds is the lenient read for stored kinds: one that no
// longer validates (a built-in it clashes with was added later) is dropped
// so it cannot break the page or the generator.
func keepValidWeatherKinds(kinds []WeatherKind) []WeatherKind {
	out := make([]WeatherKind, 0, len(kinds))
	taken := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		if len(out) == maxWeatherKinds {
			break
		}
		clean, err := validateWeatherKind(k, taken)
		if err != nil {
			continue
		}
		taken[clean.ID] = true
		out = append(out, clean)
	}
	return out
}

// parseWeatherKinds decodes the editor's JSON strictly: an unknown field or
// trailing data is refused rather than silently dropped, so a typo cannot
// quietly lose a setting. An empty string or "null" is no kinds.
func parseWeatherKinds(raw string) ([]WeatherKind, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []WeatherKind{}, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var kinds []WeatherKind
	if err := dec.Decode(&kinds); err != nil {
		return nil, apperror.NewBadRequest("your own weather couldn't be read; reload the page and try again")
	}
	if dec.More() {
		return nil, apperror.NewBadRequest("your own weather couldn't be read; reload the page and try again")
	}
	return kinds, nil
}

// encodeWeatherKinds is the stored and carried form: a JSON array, "[]"
// when there are none.
func encodeWeatherKinds(kinds []WeatherKind) string {
	if kinds == nil {
		return "[]"
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(kinds); err != nil {
		return "[]"
	}
	return strings.TrimSpace(buf.String())
}

// weatherKindNotes describes what changed between the stored kinds and the
// submitted ones, one note per kind by id: added, removed or changed.
func weatherKindNotes(cur, next []WeatherKind) []string {
	byID := func(list []WeatherKind) map[string]WeatherKind {
		m := make(map[string]WeatherKind, len(list))
		for _, k := range list {
			m[k.ID] = k
		}
		return m
	}
	was, now := byID(cur), byID(next)
	var notes []string
	for _, k := range next {
		old, ok := was[k.ID]
		switch {
		case !ok:
			notes = append(notes, fmt.Sprintf("Weather: %s added.", k.Name))
		case !reflect.DeepEqual(old, k):
			notes = append(notes, fmt.Sprintf("Weather: %s changed.", k.Name))
		}
	}
	for _, k := range cur {
		if _, ok := now[k.ID]; !ok {
			notes = append(notes, fmt.Sprintf("Weather: %s removed.", k.Name))
		}
	}
	return notes
}

// jsLength counts a string the way the generator's JS `.length` does (UTF-16
// units), so a name the server accepts is never one the generator refuses.
func jsLength(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// WeatherChoice is one built-in weather or sky effect, as AI Import lists
// them. Read from the generator's own tables, so a new entry shows up there
// without a change elsewhere.
type WeatherChoice struct{ ID, Label string }

// WeatherPresets lists the generator's built-in weathers in its order.
func WeatherPresets() []WeatherChoice {
	out := make([]WeatherChoice, len(weatherPresets))
	for i, p := range weatherPresets {
		out[i] = WeatherChoice{ID: p.ID, Label: p.Label}
	}
	return out
}

// WeatherEffects lists the sky effects the renderer can draw, in its order.
func WeatherEffects() []WeatherChoice {
	out := make([]WeatherChoice, len(weatherEffects))
	for i, e := range weatherEffects {
		out[i] = WeatherChoice{ID: e.ID, Label: e.Label}
	}
	return out
}

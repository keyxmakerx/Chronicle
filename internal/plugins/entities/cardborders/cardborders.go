// Package cardborders is Chronicle's library of card borders for the hero
// creator's stage look. A pick-list entry names one with the properties
// card_border (a key below) and card_colour (#rrggbb). It is a leaf package so
// the creator page, the plan builder and the checks on written entries all
// read one list, and so none of them has to import another's plugin.
package cardborders

import (
	"encoding/json"
	"regexp"
	"strings"
)

// The two entry properties that name a border and its colour.
const (
	PropBorder = "card_border"
	PropColour = "card_colour"
)

// Border is one entry of the library. The picture fields are file names under
// StaticDir; Peak and Gain are the colouring script's numbers and are nil
// where the border has no part that takes colour. Glow is set where the lit
// parts were drawn glowing, and is how strongly. Ground is a picture laid on
// the card's dark field behind the words.
type Border struct {
	Key    string
	Name   string
	Colour string // the colour used when an entry names none, or a bad one

	Img        string
	Mask       string
	Over       string
	Corner     string
	CornerMask string
	Ground     string
	Dye        bool
	Peak       *float64
	Gain       *float64
	Glow       *float64
}

// StaticDir is where the pictures are served, under the entities plugin's
// static path.
const StaticDir = "/static/plugins/entities/img/borders/"

func num(v float64) *float64 { return &v }

var library = []Border{
	{Key: "carved-stone", Name: "Carved stone", Colour: "#e0a030", Img: "earth.webp", Corner: "carved-stone-corner.webp", CornerMask: "carved-stone-corner-mask.webp", Peak: num(0.93), Gain: num(1.3)},
	{Key: "earth", Name: "Earth", Colour: "#b08a5a", Img: "earth.webp"},
	{Key: "fire", Name: "Fire", Colour: "#d9622b", Img: "fire.webp", Mask: "fire-mask.webp", Peak: num(1), Gain: num(1.3)},
	{Key: "ice", Name: "Ice", Colour: "#8fd8ff", Img: "ice.webp"},
	{Key: "storm", Name: "Storm", Colour: "#6fa8dc", Img: "storm.webp", Mask: "storm-mask.webp", Peak: num(1), Gain: num(1.3)},
	{Key: "glass", Name: "Glass", Colour: "#58c4d8", Img: "glass.webp"},
	{Key: "glass-smoke", Name: "Smoke glass", Colour: "#9aa3ad", Img: "glass-smoke.webp"},
	{Key: "glass-sea", Name: "Sea glass", Colour: "#4fb8a8", Img: "glass-sea.webp"},
	{Key: "glass-rose", Name: "Rose glass", Colour: "#e58fb0", Img: "glass-rose.webp"},
	{Key: "glass-midnight", Name: "Midnight glass", Colour: "#a78bfa", Img: "glass-midnight.webp"},
	{Key: "rune-metal", Name: "Rune metal", Colour: "#3fc8d4", Img: "rune-metal.webp", Mask: "rune-metal-mask.webp", Peak: num(0.96), Gain: num(1.3)},
	{Key: "great-rune", Name: "Great rune", Colour: "#b48cf2", Img: "great-rune.webp", Mask: "great-rune-mask.webp", Peak: num(0), Gain: num(0.85)},
	{Key: "mechanical", Name: "Boiler works", Colour: "#6fa8dc", Img: "mechanical.webp", Mask: "mechanical-mask.webp", Corner: "mechanical-corners.webp", CornerMask: "mechanical-corners-mask.webp", Ground: "mechanical-steam.webp", Peak: num(0), Gain: num(1), Glow: num(1.4)},
	{Key: "thorn-glass", Name: "Thorn glass", Colour: "#c4243a", Img: "thorn-glass.webp", Mask: "thorn-glass-mask.webp", Peak: num(0.92), Gain: num(1.3)},
	{Key: "scales", Name: "Scales", Colour: "#2fb5a4", Img: "scales.webp", Over: "scales-boss.webp", Dye: true},
	{Key: "vine", Name: "Vine and wood", Colour: "#6fae4f", Img: "vine.webp", Mask: "vine-mask.webp", Peak: num(0.74), Gain: num(1.3)},
}

// All returns every border in the order the library lists them. The slice is
// a copy; changing it changes nothing.
func All() []Border { return append([]Border(nil), library...) }

// Get looks a border up by its key.
func Get(key string) (Border, bool) {
	for _, b := range library {
		if b.Key == key {
			return b, true
		}
	}
	return Border{}, false
}

// Keys returns the keys in library order.
func Keys() []string {
	out := make([]string, len(library))
	for i, b := range library {
		out[i] = b.Key
	}
	return out
}

// Files lists every picture file the library names, each once.
func Files() []string {
	seen := map[string]bool{}
	var out []string
	for _, b := range library {
		for _, f := range []string{b.Img, b.Mask, b.Over, b.Corner, b.CornerMask, b.Ground} {
			if f != "" && !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}

var colourPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidColour reports whether s is a colour an entry may name: a # and six
// hex digits.
func ValidColour(s string) bool { return colourPattern.MatchString(s) }

// Look is the border and colour an entry names.
type Look struct {
	Border string
	Colour string
}

// LookFrom reads the two properties off an entry. An entry whose card_border
// is not a library key has no look. A card_colour that is missing, not text
// or not six hex digits becomes the border's own default colour; a good one
// is lower-cased.
func LookFrom(props map[string]any) (Look, bool) {
	key, _ := props[PropBorder].(string)
	b, ok := Get(key)
	if !ok {
		return Look{}, false
	}
	col, _ := props[PropColour].(string)
	if !ValidColour(col) {
		return Look{Border: b.Key, Colour: b.Colour}, true
	}
	return Look{Border: b.Key, Colour: strings.ToLower(col)}, true
}

// LibraryJSON is the library as the colouring script reads it, keyed by
// border key, with every picture made into an address by url (so it can carry
// a content hash) and absent parts left out. Names and default colours stay
// on the server.
func LibraryJSON(url func(path string) string) string {
	type part struct {
		Img        string   `json:"img"`
		Mask       string   `json:"mask,omitempty"`
		Over       string   `json:"over,omitempty"`
		Corner     string   `json:"corner,omitempty"`
		CornerMask string   `json:"cornerMask,omitempty"`
		Ground     string   `json:"ground,omitempty"`
		Dye        bool     `json:"dye,omitempty"`
		Peak       *float64 `json:"peak,omitempty"`
		Gain       *float64 `json:"gain,omitempty"`
		Glow       *float64 `json:"glow,omitempty"`
	}
	at := func(f string) string {
		if f == "" {
			return ""
		}
		return url(StaticDir + f)
	}
	out := make(map[string]part, len(library))
	for _, b := range library {
		out[b.Key] = part{
			Img: at(b.Img), Mask: at(b.Mask), Over: at(b.Over), Corner: at(b.Corner), CornerMask: at(b.CornerMask), Ground: at(b.Ground),
			Dye: b.Dye, Peak: b.Peak, Gain: b.Gain, Glow: b.Glow,
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

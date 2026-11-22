package decks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Settings are a deck's rules. They are stored as a re-marshal of this typed
// struct, so an unknown key in a request never reaches the database.
type Settings struct {
	// Draws is who draws: "directors" or "players".
	Draws string `json:"draws"`
	// Knows is what a player who can see the deck may know of it.
	Knows string `json:"knows"`
	// OthersSee is what other players see of a card dealt to one hero.
	OthersSee string `json:"othersSee"`
	// Arrives is how a card dealt to a hero lands: "down" or "up".
	Arrives string `json:"arrives"`
	// Fate is the deck's default after a card is played.
	Fate string `json:"fate"`
	// Empty is what a draw from an empty pile does.
	Empty string `json:"empty"`
	// Discard lets players discard from their own hand.
	Discard bool `json:"discard"`
	// Notice is how a dealt card announces itself. It is stored and checked
	// only; nothing reads it yet.
	Notice string `json:"notice"`
}

// SettingsInput is Settings with every key optional: nil keeps what is there.
type SettingsInput struct {
	Draws     *string `json:"draws"`
	Knows     *string `json:"knows"`
	OthersSee *string `json:"othersSee"`
	Arrives   *string `json:"arrives"`
	Fate      *string `json:"fate"`
	Empty     *string `json:"empty"`
	Discard   *bool   `json:"discard"`
	Notice    *string `json:"notice"`
}

// Allowed values, one list each so the check and the message agree.
var (
	drawsValues     = []string{"directors", "players"}
	knowsValues     = []string{"nothing", "count", "kinds", "faces"}
	othersSeeValues = []string{"nothing", "back", "face"}
	arrivesValues   = []string{"down", "up"}
	fateValues      = []string{FateBack, FateUsed, FateGone, FatePlay}
	emptyValues     = []string{"reshuffle", "stop"}
	noticeValues    = []string{"slide", "count", "quiet"}
	kindValues      = []string{KindHero, KindParty, KindEvent}

	// The look lists. Each is one list: a later step adds a value here and
	// to nothing else. The first value is the default (defaultLook).
	faceValues    = []string{"plate", "window", "full"}
	shapeValues   = []string{"tall", "standard", "small"}
	borderValues  = []string{"double", "line", "kit", "metal", "none"}
	turnValues    = []string{"flip", "slow", "toss", "slam", "sweep", "burn", "ink"}
	patternValues = []string{"lattice", "stripes", "dots", "stars", "plain"}
)

// Palette is the one list of colour keys. No hex colour is ever stored: the
// stylesheet resolves a key for light and dark.
var Palette = []string{"violet", "indigo", "blue", "cyan", "teal", "moss", "plum", "rose", "umber", "slate"}

var kindKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

func oneOf(field string, v string, allowed []string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return errInvalid(fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowed, ", ")))
}

func validKind(kind string) error { return oneOf("kind", kind, kindValues) }

// defaultSettings is what a deck of this kind starts with.
func defaultSettings(kind string) Settings {
	s := Settings{
		Draws: "directors", Knows: "count", OthersSee: "back",
		Arrives: "up", Fate: FateUsed, Empty: "stop", Notice: "count",
	}
	if kind == KindHero {
		s.Arrives = "down"
		s.Notice = "slide"
	}
	return s
}

// applySettings merges in over cur and checks every value. It never mutates
// cur, and a bad value refuses the whole change.
func applySettings(cur Settings, in *SettingsInput) (Settings, error) {
	out := cur
	if in != nil {
		set := func(field string, dst *string, src *string, allowed []string) error {
			if src == nil {
				return nil
			}
			if err := oneOf(field, *src, allowed); err != nil {
				return err
			}
			*dst = *src
			return nil
		}
		for _, f := range []struct {
			field   string
			dst     *string
			src     *string
			allowed []string
		}{
			{"draws", &out.Draws, in.Draws, drawsValues},
			{"knows", &out.Knows, in.Knows, knowsValues},
			{"othersSee", &out.OthersSee, in.OthersSee, othersSeeValues},
			{"arrives", &out.Arrives, in.Arrives, arrivesValues},
			{"fate", &out.Fate, in.Fate, fateValues},
			{"empty", &out.Empty, in.Empty, emptyValues},
			{"notice", &out.Notice, in.Notice, noticeValues},
		} {
			if err := set(f.field, f.dst, f.src, f.allowed); err != nil {
				return Settings{}, err
			}
		}
		if in.Discard != nil {
			out.Discard = *in.Discard
		}
	}
	return out, nil
}

// readSettings decodes stored settings. A row that no longer parses, or holds
// a value outside the lists, falls back to the kind's default for that key
// rather than failing a read: the stored text is always a re-marshal of a
// checked struct, so this only covers a hand-edited row.
func readSettings(kind, raw string) Settings {
	def := defaultSettings(kind)
	var got Settings
	if raw == "" || json.Unmarshal([]byte(raw), &got) != nil {
		return def
	}
	fix := func(v *string, def string, allowed []string) {
		if oneOf("", *v, allowed) != nil {
			*v = def
		}
	}
	fix(&got.Draws, def.Draws, drawsValues)
	fix(&got.Knows, def.Knows, knowsValues)
	fix(&got.OthersSee, def.OthersSee, othersSeeValues)
	fix(&got.Arrives, def.Arrives, arrivesValues)
	fix(&got.Fate, def.Fate, fateValues)
	fix(&got.Empty, def.Empty, emptyValues)
	fix(&got.Notice, def.Notice, noticeValues)
	return got
}

// Look is how a deck's cards are drawn. Every value comes from a fixed list.
type Look struct {
	Face   string    `json:"face"`
	Shape  string    `json:"shape"`
	Border string    `json:"border"`
	Turn   string    `json:"turn"`
	Back   Back      `json:"back"`
	Kinds  []KindDef `json:"kinds"`
}

// Back is the card back.
type Back struct {
	Pattern string `json:"pattern"`
	// Colour is a palette key, or "" for the campaign's accent.
	Colour string `json:"colour"`
	// Picture is "" for now.
	Picture string `json:"picture"`
	Words   string `json:"words"`
}

// KindDef names a card kind and gives it a palette colour.
type KindDef struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Colour string `json:"colour"`
}

// LookInput is Look with every key optional: nil keeps what is there. Kinds
// nil keeps the list and an empty list clears it.
type LookInput struct {
	Face   *string    `json:"face"`
	Shape  *string    `json:"shape"`
	Border *string    `json:"border"`
	Turn   *string    `json:"turn"`
	Back   *BackInput `json:"back"`
	Kinds  []KindDef  `json:"kinds"`
}

// UnmarshalJSON refuses a key the look does not have, naming it, so a typo or
// a value from a later version is a plain error and never silently dropped.
func (l *LookInput) UnmarshalJSON(b []byte) error {
	type plain LookInput
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p plain
	if err := dec.Decode(&p); err != nil {
		return lookDecodeError(err)
	}
	*l = LookInput(p)
	return nil
}

// lookDecodeError turns a decoder failure into a message naming the field.
func lookDecodeError(err error) error {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, "json: unknown field "); ok {
		return errInvalid("the look has no key " + rest)
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) && te.Field != "" {
		return errInvalid(fmt.Sprintf("the look's %q has the wrong kind of value", te.Field))
	}
	return errInvalid("the look is not valid")
}

// BackInput is Back with every key optional.
type BackInput struct {
	Pattern *string `json:"pattern"`
	Colour  *string `json:"colour"`
	Picture *string `json:"picture"`
	Words   *string `json:"words"`
}

func defaultLook() Look {
	return Look{Face: "plate", Shape: "tall", Border: "double", Turn: "flip",
		Back: Back{Pattern: "lattice"}, Kinds: []KindDef{}}
}

func validColour(field, v string, allowEmpty bool) error {
	if v == "" && allowEmpty {
		return nil
	}
	return oneOf(field, v, Palette)
}

// plainText trims s and checks it is single-line text of at most max
// characters, so stored text is always safe to show with textContent.
func plainText(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) {
		return "", errInvalid(field + " is not valid text")
	}
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", errInvalid(field + " contains a character that is not allowed")
		}
	}
	return s, nil
}

// longText is plainText for the words on a card and a Director's note: line
// breaks are kept, other control characters are not.
func longText(field, s string, max int) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if !utf8.ValidString(s) {
		return "", errInvalid(field + " is not valid text")
	}
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", errInvalid(field + " contains a character that is not allowed")
		}
	}
	return s, nil
}

// applyLook merges in over cur and checks every value.
func applyLook(cur Look, in *LookInput) (Look, error) {
	out := cur
	out.Kinds = append([]KindDef{}, cur.Kinds...)
	if in == nil {
		return out, nil
	}
	for _, f := range []struct {
		field   string
		dst     *string
		src     *string
		allowed []string
	}{
		{"face", &out.Face, in.Face, faceValues},
		{"shape", &out.Shape, in.Shape, shapeValues},
		{"border", &out.Border, in.Border, borderValues},
		{"turn", &out.Turn, in.Turn, turnValues},
	} {
		if f.src == nil {
			continue
		}
		if err := oneOf(f.field, *f.src, f.allowed); err != nil {
			return Look{}, err
		}
		*f.dst = *f.src
	}
	if b := in.Back; b != nil {
		if b.Pattern != nil {
			if err := oneOf("back pattern", *b.Pattern, patternValues); err != nil {
				return Look{}, err
			}
			out.Back.Pattern = *b.Pattern
		}
		if b.Colour != nil {
			if err := validColour("back colour", *b.Colour, true); err != nil {
				return Look{}, err
			}
			out.Back.Colour = *b.Colour
		}
		if b.Picture != nil {
			// TODO(#2567): card backs take a picture once the workshop can upload one.
			if *b.Picture != "" {
				return Look{}, errInvalid("a card back can't have a picture yet")
			}
			out.Back.Picture = ""
		}
		if b.Words != nil {
			w, err := plainText("back words", *b.Words, MaxBackWordsChars)
			if err != nil {
				return Look{}, err
			}
			out.Back.Words = w
		}
	}
	if in.Kinds != nil {
		kinds, err := checkKinds(in.Kinds)
		if err != nil {
			return Look{}, err
		}
		out.Kinds = kinds
	}
	return out, nil
}

func checkKinds(in []KindDef) ([]KindDef, error) {
	if len(in) > MaxKinds {
		return nil, errInvalid(fmt.Sprintf("a deck can have at most %d card kinds", MaxKinds))
	}
	seen := map[string]bool{}
	out := make([]KindDef, 0, len(in))
	for _, k := range in {
		if !kindKeyPattern.MatchString(k.Key) {
			return nil, errInvalid("a card kind's key starts with a lowercase letter and uses only lowercase letters, digits and hyphens (at most 32)")
		}
		if seen[k.Key] {
			return nil, errInvalid("two card kinds share the key " + k.Key)
		}
		seen[k.Key] = true
		name, err := plainText("a card kind's name", k.Name, MaxKindNameChars)
		if err != nil {
			return nil, err
		}
		if name == "" {
			return nil, errInvalid("a card kind needs a name")
		}
		if err := validColour("a card kind's colour", k.Colour, false); err != nil {
			return nil, err
		}
		out = append(out, KindDef{Key: k.Key, Name: name, Colour: k.Colour})
	}
	return out, nil
}

// readLook decodes a stored look. It never fails a read: each field is
// checked on its own and falls back to that field's default, and the kinds
// are checked one at a time, dropping only the bad one.
func readLook(raw string) Look {
	def := defaultLook()
	var got Look
	if raw == "" || json.Unmarshal([]byte(raw), &got) != nil {
		// A document that does not parse may still have fields that do;
		// read what can be read key by key.
		got = salvageLook(raw)
	}
	fix := func(v *string, d string, allowed []string) {
		if oneOf("", *v, allowed) != nil {
			*v = d
		}
	}
	fix(&got.Face, def.Face, faceValues)
	fix(&got.Shape, def.Shape, shapeValues)
	fix(&got.Border, def.Border, borderValues)
	fix(&got.Turn, def.Turn, turnValues)
	fix(&got.Back.Pattern, def.Back.Pattern, patternValues)
	if got.Back.Colour != "" && oneOf("", got.Back.Colour, Palette) != nil {
		got.Back.Colour = ""
	}
	got.Back.Picture = ""
	if w, err := plainText("", got.Back.Words, MaxBackWordsChars); err == nil {
		got.Back.Words = w
	} else {
		got.Back.Words = ""
	}
	kinds := make([]KindDef, 0, len(got.Kinds))
	seen := map[string]bool{}
	for _, k := range got.Kinds {
		one, err := checkKinds([]KindDef{k})
		if err != nil || seen[k.Key] || len(kinds) >= MaxKinds {
			continue
		}
		seen[k.Key] = true
		kinds = append(kinds, one[0])
	}
	got.Kinds = kinds
	return got
}

// salvageLook reads the top-level keys of a look one at a time, so a single
// key of the wrong type does not cost the others.
func salvageLook(raw string) Look {
	var keys map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &keys) != nil {
		return Look{}
	}
	var l Look
	for k, v := range keys {
		switch k {
		case "face":
			_ = json.Unmarshal(v, &l.Face)
		case "shape":
			_ = json.Unmarshal(v, &l.Shape)
		case "border":
			_ = json.Unmarshal(v, &l.Border)
		case "turn":
			_ = json.Unmarshal(v, &l.Turn)
		case "back":
			_ = json.Unmarshal(v, &l.Back)
		case "kinds":
			var items []json.RawMessage
			if json.Unmarshal(v, &items) == nil {
				for _, it := range items {
					var kd KindDef
					if json.Unmarshal(it, &kd) == nil {
						l.Kinds = append(l.Kinds, kd)
					}
				}
			}
		}
	}
	return l
}

func hasKind(l Look, key string) bool {
	for _, k := range l.Kinds {
		if k.Key == key {
			return true
		}
	}
	return false
}

// validToken reports whether s is a short safe id: letters, digits, _ and -.
func validToken(s string) bool {
	if s == "" || len(s) > maxIDChars {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

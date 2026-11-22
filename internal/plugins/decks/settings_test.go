package decks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultSettingsPerKind(t *testing.T) {
	tests := []struct {
		kind string
		want Settings
	}{
		{KindHero, Settings{Draws: "directors", Knows: "count", OthersSee: "back", Arrives: "down", Fate: "used", Empty: "stop", Notice: "slide"}},
		{KindParty, Settings{Draws: "directors", Knows: "count", OthersSee: "back", Arrives: "up", Fate: "used", Empty: "stop", Notice: "count"}},
		{KindEvent, Settings{Draws: "directors", Knows: "count", OthersSee: "back", Arrives: "up", Fate: "used", Empty: "stop", Notice: "count"}},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			if got := defaultSettings(tt.kind); got != tt.want {
				t.Errorf("defaults = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestApplySettings(t *testing.T) {
	tests := []struct {
		name    string
		in      SettingsInput
		wantErr string
		check   func(Settings) bool
	}{
		{name: "empty keeps everything", check: func(s Settings) bool { return s == defaultSettings(KindHero) }},
		{name: "every key replaces", in: SettingsInput{Draws: ptr("players"), Knows: ptr("faces"), OthersSee: ptr("face"), Arrives: ptr("up"),
			Fate: ptr("play"), Empty: ptr("reshuffle"), Discard: ptr(true), Notice: ptr("quiet")},
			check: func(s Settings) bool {
				return s == Settings{Draws: "players", Knows: "faces", OthersSee: "face", Arrives: "up", Fate: "play", Empty: "reshuffle", Discard: true, Notice: "quiet"}
			}},
		{name: "bad draws", in: SettingsInput{Draws: ptr("everyone")}, wantErr: "draws must be one of: directors, players"},
		{name: "bad knows", in: SettingsInput{Knows: ptr("all")}, wantErr: "knows must be one of: nothing, count, kinds, faces"},
		{name: "bad othersSee", in: SettingsInput{OthersSee: ptr("front")}, wantErr: "othersSee must be one of: nothing, back, face"},
		{name: "bad arrives", in: SettingsInput{Arrives: ptr("sideways")}, wantErr: "arrives must be one of: down, up"},
		{name: "bad fate", in: SettingsInput{Fate: ptr("burn")}, wantErr: "fate must be one of: back, used, gone, play"},
		{name: "bad empty", in: SettingsInput{Empty: ptr("explode")}, wantErr: "empty must be one of: reshuffle, stop"},
		{name: "bad notice", in: SettingsInput{Notice: ptr("shout")}, wantErr: "notice must be one of: slide, count, quiet"},
		{name: "empty string is not a value", in: SettingsInput{Draws: ptr("")}, wantErr: "draws must be one of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applySettings(defaultSettings(KindHero), &tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !tt.check(got) {
				t.Errorf("got %+v, %v", got, err)
			}
		})
	}
}

func TestSettingsDropUnknownKeysAndStoreTheTypedStruct(t *testing.T) {
	var in SettingsInput
	if err := json.Unmarshal([]byte(`{"draws":"players","secret":"x","knows":"nothing"}`), &in); err != nil {
		t.Fatal(err)
	}
	got, err := applySettings(defaultSettings(KindEvent), &in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "secret") {
		t.Errorf("unknown key stored: %s", raw)
	}
	if back := readSettings(KindEvent, string(raw)); back != got {
		t.Errorf("round trip = %+v, want %+v", back, got)
	}
	// A hand-edited row never breaks a read.
	if fixed := readSettings(KindHero, `{"draws":"nobody","fate":"gone"}`); fixed.Draws != "directors" || fixed.Fate != "gone" || fixed.Arrives != "down" {
		t.Errorf("fallback = %+v", fixed)
	}
	if fixed := readSettings(KindHero, `not json`); fixed != defaultSettings(KindHero) {
		t.Errorf("unparseable row = %+v", fixed)
	}
}

func TestApplyLook(t *testing.T) {
	long := strings.Repeat("x", MaxBackWordsChars+1)
	manyKinds := make([]KindDef, MaxKinds+1)
	for i := range manyKinds {
		manyKinds[i] = KindDef{Key: "k" + itoa(i), Name: "K", Colour: "blue"}
	}
	tests := []struct {
		name    string
		in      LookInput
		wantErr string
		check   func(Look) bool
	}{
		{name: "defaults", check: func(l Look) bool {
			return l.Shape == "tall" && l.Border == "double" && l.Turn == "flip" && l.Window == "arch" && l.Name == "below" &&
				l.Lettering == "plain" && l.Corner == "roman" && l.Marker == "gem" && l.Finish == "matte" &&
				l.Back == Back{Pattern: "lattice", Sign: "star"} && l.Kinds != nil && len(l.Kinds) == 0
		}},
		{name: "every list value is accepted", in: LookInput{Shape: ptr("small"), Border: ptr("brass"), Turn: ptr("forge"), Window: ptr("full"),
			Name: ptr("over"), Lettering: ptr("hand"), Corner: ptr("none"), Marker: ptr("line"), Finish: ptr("foil")},
			check: func(l Look) bool {
				return l.Shape == "small" && l.Border == "brass" && l.Turn == "forge" && l.Window == "full" && l.Name == "over" &&
					l.Lettering == "hand" && l.Corner == "none" && l.Marker == "line" && l.Finish == "foil"
			}},
		{name: "back colour from the palette", in: LookInput{Back: &BackInput{Pattern: ptr("iron"), Sign: ptr("crown"), Colour: ptr("moss"), Words: ptr(" Fate ")}},
			check: func(l Look) bool {
				return l.Back == Back{Pattern: "iron", Sign: "crown", Colour: "moss", Words: "Fate"}
			}},
		{name: "back colour may be cleared", in: LookInput{Back: &BackInput{Colour: ptr("")}}, check: func(l Look) bool { return l.Back.Colour == "" }},
		{name: "kinds replace and take the first free signs", in: LookInput{Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"},
			{Key: "bane", Name: "Bane", Colour: "rose", Sign: "round"}, {Key: "odd", Name: "Odd", Colour: "slate"}}},
			check: func(l Look) bool {
				return len(l.Kinds) == 3 && l.Kinds[0].Sign == "diamond" && l.Kinds[1].Sign == "round" && l.Kinds[2].Sign == "point"
			}},
		{name: "old shape long is refused", in: LookInput{Shape: ptr("long")}, wantErr: "shape must be one of: tall, standard, small"},
		{name: "old turn slide is refused", in: LookInput{Turn: ptr("slide")}, wantErr: "turn must be one of: flip, slow, toss, slam, sweep, burn, ink, seal, bolt, forge"},
		{name: "bad border", in: LookInput{Border: ptr("gold")}, wantErr: "border must be one of: double, line, kit, metal, brass, steel, bound, stone, none"},
		{name: "bad window", in: LookInput{Window: ptr("oval")}, wantErr: "window must be one of: arch, square, pointed, round, full"},
		{name: "bad name", in: LookInput{Name: ptr("beside")}, wantErr: "name must be one of: below, over"},
		{name: "bad lettering", in: LookInput{Lettering: ptr("gothic")}, wantErr: "lettering must be one of: plain, carved, classic, condensed, hand"},
		{name: "bad corner", in: LookInput{Corner: ptr("letter")}, wantErr: "corner must be one of: roman, number, suit, pips, none"},
		{name: "bad marker", in: LookInput{Marker: ptr("flag")}, wantErr: "marker must be one of: gem, banner, tab, ribbon, line"},
		{name: "bad finish", in: LookInput{Finish: ptr("gloss")}, wantErr: "finish must be one of: matte, glow, sheen, foil"},
		{name: "retired pattern dots is refused", in: LookInput{Back: &BackInput{Pattern: ptr("dots")}}, wantErr: "back pattern must be one of: lattice, rays, waves, stars, check, plain, iron"},
		{name: "retired pattern stripes is refused", in: LookInput{Back: &BackInput{Pattern: ptr("stripes")}}, wantErr: "back pattern must be one of"},
		{name: "bad back sign", in: LookInput{Back: &BackInput{Sign: ptr("tree")}}, wantErr: "back sign must be one of: star, sun, moon, eye, crown, anchor, flame, sword, none"},
		{name: "hex colour refused", in: LookInput{Back: &BackInput{Colour: ptr("#ff0000")}}, wantErr: "back colour must be one of"},
		{name: "back picture refused for now", in: LookInput{Back: &BackInput{Picture: ptr("a.png")}}, wantErr: "can't have a picture yet"},
		{name: "back words too long", in: LookInput{Back: &BackInput{Words: &long}}, wantErr: "too long"},
		{name: "too many kinds", in: LookInput{Kinds: manyKinds}, wantErr: "at most 12"},
		{name: "kind key shape", in: LookInput{Kinds: []KindDef{{Key: "Boon", Name: "B", Colour: "blue"}}}, wantErr: "key starts with a lowercase letter"},
		{name: "kind key too long", in: LookInput{Kinds: []KindDef{{Key: "a" + strings.Repeat("b", 32), Name: "B", Colour: "blue"}}}, wantErr: "key starts with a lowercase letter"},
		{name: "kind keys unique", in: LookInput{Kinds: []KindDef{{Key: "a", Name: "A", Colour: "blue"}, {Key: "a", Name: "B", Colour: "blue"}}}, wantErr: "share the key a"},
		{name: "kind name needed", in: LookInput{Kinds: []KindDef{{Key: "a", Name: " ", Colour: "blue"}}}, wantErr: "needs a name"},
		{name: "kind name length", in: LookInput{Kinds: []KindDef{{Key: "a", Name: strings.Repeat("n", 25), Colour: "blue"}}}, wantErr: "too long"},
		{name: "kind colour needed", in: LookInput{Kinds: []KindDef{{Key: "a", Name: "A"}}}, wantErr: "colour must be one of"},
		{name: "kind sign unknown", in: LookInput{Kinds: []KindDef{{Key: "a", Name: "A", Colour: "blue", Sign: "star-of-david"}}},
			wantErr: "a card kind's sign must be one of: round, diamond, point, square, star, cross, crescent, drop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyLook(defaultLook(), &tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !tt.check(got) {
				t.Errorf("got %+v, %v", got, err)
			}
		})
	}
}

// With more kinds than signs a sign is reused, and a Director's own choices
// are never moved by the fill.
func TestKindSignsFillInOrderAndNeverMoveAChosenOne(t *testing.T) {
	var kinds []KindDef
	for i := 0; i < MaxKinds; i++ {
		kinds = append(kinds, KindDef{Key: "k" + itoa(i), Name: "K", Colour: "blue"})
	}
	kinds[3].Sign = "drop"
	got, err := checkKinds(kinds)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"round", "diamond", "point", "drop", "square", "star", "cross", "crescent", "round", "diamond", "point", "square"}
	for i, k := range got {
		if k.Sign != want[i] {
			t.Errorf("kind %d sign = %s, want %s", i, k.Sign, want[i])
		}
	}
}

func TestLookStoredValuesFallBackAndPaletteIsFixed(t *testing.T) {
	if want := []string{"violet", "indigo", "blue", "cyan", "teal", "moss", "plum", "rose", "umber", "slate"}; !reflect.DeepEqual(Palette, want) {
		t.Errorf("palette = %v", Palette)
	}
	// A stored value from an older list is replaced by the default, never sent.
	got := readLook(`{"face":"plate","shape":"long","border":"gold","turn":"slide","back":{"pattern":"dots","colour":"#fff"},"kinds":[{"key":"Bad","name":"x","colour":"red"}]}`)
	if got.Shape != "tall" || got.Border != "double" || got.Turn != "flip" || got.Back.Pattern != "lattice" || got.Back.Colour != "" || len(got.Kinds) != 0 {
		t.Errorf("fallback = %+v", got)
	}
	if got := readLook("{"); !reflect.DeepEqual(got, defaultLook()) {
		t.Errorf("unparseable look = %+v", got)
	}
	// The retired face key is read without error and does not come back out.
	raw, _ := json.Marshal(readLook(`{"face":"window","shape":"small"}`))
	if strings.Contains(string(raw), `"face"`) || !strings.Contains(string(raw), `"shape":"small"`) {
		t.Errorf("a stored face key survived a read: %s", raw)
	}
}

// The lists and their defaults are pinned so a change to either is deliberate.
func TestLookListsAndDefaultsArePinned(t *testing.T) {
	lists := map[string][]string{
		"shape":     {"tall", "standard", "small"},
		"border":    {"double", "line", "kit", "metal", "brass", "steel", "bound", "stone", "none"},
		"turn":      {"flip", "slow", "toss", "slam", "sweep", "burn", "ink", "seal", "bolt", "forge"},
		"window":    {"arch", "square", "pointed", "round", "full"},
		"name":      {"below", "over"},
		"lettering": {"plain", "carved", "classic", "condensed", "hand"},
		"corner":    {"roman", "number", "suit", "pips", "none"},
		"marker":    {"gem", "banner", "tab", "ribbon", "line"},
		"finish":    {"matte", "glow", "sheen", "foil"},
		"pattern":   {"lattice", "rays", "waves", "stars", "check", "plain", "iron"},
		"backSign":  {"star", "sun", "moon", "eye", "crown", "anchor", "flame", "sword", "none"},
		"sign":      {"round", "diamond", "point", "square", "star", "cross", "crescent", "drop"},
	}
	got := map[string][]string{"shape": shapeValues, "border": borderValues, "turn": turnValues, "window": windowValues,
		"name": nameValues, "lettering": letteringValues, "corner": cornerValues, "marker": markerValues, "finish": finishValues,
		"pattern": patternValues, "backSign": backSignValues, "sign": signValues}
	for k, want := range lists {
		if !reflect.DeepEqual(got[k], want) {
			t.Errorf("%s list = %v, want %v", k, got[k], want)
		}
	}
	def := defaultLook()
	wantDef := Look{Shape: "tall", Border: "double", Turn: "flip", Window: "arch", Name: "below", Lettering: "plain",
		Corner: "roman", Marker: "gem", Finish: "matte", Back: Back{Pattern: "lattice", Sign: "star"}, Kinds: []KindDef{}}
	if !reflect.DeepEqual(def, wantDef) {
		t.Errorf("defaults = %+v, want %+v", def, wantDef)
	}
	// An unknown value in a stored look falls back to the default, never fails the read.
	got2 := readLook(`{"shape":"x","border":"x","turn":"x","window":"x","name":"x","lettering":"x","corner":"x","marker":"x","finish":"x","back":{"pattern":"x","sign":"x"}}`)
	if !reflect.DeepEqual(got2, def) {
		t.Errorf("stored fallback = %+v, want %+v", got2, def)
	}
}

func TestLookInputRefusesUnknownKeysNamingThem(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"known keys", `{"window":"arch","back":{"pattern":"iron","sign":"sun"},"kinds":[{"key":"a","name":"A","colour":"blue","sign":"drop"}]}`, ""},
		{"the retired face key", `{"face":"plate"}`, `no key "face"`},
		{"unknown top-level key", `{"fase":"plate"}`, `no key "fase"`},
		{"unknown back key", `{"back":{"patern":"iron"}}`, `no key "patern"`},
		{"unknown kind key", `{"kinds":[{"key":"a","name":"A","colour":"blue","icon":"x"}]}`, `no key "icon"`},
		{"wrong kind of value", `{"shape":5}`, `"shape" has the wrong kind of value`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var in LookInput
			err := json.Unmarshal([]byte(tt.body), &in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			// Through a deck update the refusal keeps its message.
			var up UpdateDeckInput
			if err := json.Unmarshal([]byte(`{"version":1,"look":`+tt.body+`}`), &up); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("through UpdateDeckInput: %v", err)
			}
		})
	}
}

func TestReadLookChecksEachFieldAndKindOnItsOwn(t *testing.T) {
	longWords := strings.Repeat("w", MaxBackWordsChars+1)
	raw := `{"face":"window","shape":"x","border":"metal","turn":"ink","finish":"foil",` +
		`"back":{"pattern":"dots","colour":"nope","words":"` + longWords + `"},` +
		`"kinds":[{"key":"ok","name":"Fine","colour":"teal"},{"key":"Bad","name":"x","colour":"teal"},` +
		`{"key":"ok","name":"Again","colour":"teal"},{"key":"c","name":"","colour":"teal"},{"key":"d","name":"D","colour":"red"},` +
		`{"key":"e","name":"Fine too","colour":"plum","sign":"flag"}]}`
	got := readLook(raw)
	if got.Border != "metal" || got.Turn != "ink" || got.Finish != "foil" {
		t.Errorf("good fields were lost: %+v", got)
	}
	if got.Shape != "tall" || got.Back.Pattern != "lattice" || got.Back.Colour != "" || got.Back.Words != "" {
		t.Errorf("bad fields did not fall back: %+v", got)
	}
	// A kind with a retired colour or sign keeps its place with only that field
	// defaulted; a kind with no usable key or name is dropped.
	if len(got.Kinds) != 3 || got.Kinds[0].Key != "ok" || got.Kinds[1].Key != "d" || got.Kinds[2].Key != "e" {
		t.Fatalf("kinds = %+v", got.Kinds)
	}
	if got.Kinds[1].Colour != "violet" || got.Kinds[1].Name != "D" || got.Kinds[2].Colour != "plum" {
		t.Errorf("only the retired colour should be defaulted: %+v", got.Kinds)
	}
	seen := map[string]bool{}
	for _, k := range got.Kinds {
		if oneOf("", k.Sign, signValues) != nil || seen[k.Sign] {
			t.Errorf("kind %s has sign %q, want a distinct valid one", k.Key, k.Sign)
		}
		seen[k.Sign] = true
	}
	if got := readLook(`{"back":{"words":"a` + "\x01" + `b"}}`); got.Back.Words != "" {
		t.Errorf("a control character in the back's words survived a read: %q", got.Back.Words)
	}
	// A key of the wrong type costs only itself.
	if got := readLook(`{"shape":7,"turn":"ink","kinds":"nope"}`); got.Turn != "ink" || got.Shape != "tall" || len(got.Kinds) != 0 {
		t.Errorf("salvage = %+v", got)
	}
	var many []string
	for i := 0; i < MaxKinds+3; i++ {
		many = append(many, `{"key":"k`+itoa(i)+`","name":"K","colour":"blue"}`)
	}
	if got := readLook(`{"kinds":[` + strings.Join(many, ",") + `]}`); len(got.Kinds) != MaxKinds {
		t.Errorf("kept %d kinds, want %d", len(got.Kinds), MaxKinds)
	}
}

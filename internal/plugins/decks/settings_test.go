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
			return l.Face == "plate" && l.Shape == "tall" && l.Border == "double" && l.Turn == "flip" &&
				l.Back == Back{Pattern: "lattice"} && l.Kinds != nil && len(l.Kinds) == 0
		}},
		{name: "every list value is accepted", in: LookInput{Face: ptr("window"), Shape: ptr("small"), Border: ptr("metal"), Turn: ptr("sweep")},
			check: func(l Look) bool {
				return l.Face == "window" && l.Shape == "small" && l.Border == "metal" && l.Turn == "sweep"
			}},
		{name: "every turn is accepted", in: LookInput{Turn: ptr("ink")}, check: func(l Look) bool { return l.Turn == "ink" }},
		{name: "toss slam burn", in: LookInput{Turn: ptr("burn")}, check: func(l Look) bool { return l.Turn == "burn" }},
		{name: "full face", in: LookInput{Face: ptr("full")}, check: func(l Look) bool { return l.Face == "full" }},
		{name: "standard shape and kit border", in: LookInput{Shape: ptr("standard"), Border: ptr("kit")}, check: func(l Look) bool { return l.Shape == "standard" && l.Border == "kit" }},
		{name: "back colour from the palette", in: LookInput{Back: &BackInput{Pattern: ptr("stars"), Colour: ptr("moss"), Words: ptr(" Fate ")}},
			check: func(l Look) bool { return l.Back == Back{Pattern: "stars", Colour: "moss", Words: "Fate"} }},
		{name: "back colour may be cleared", in: LookInput{Back: &BackInput{Colour: ptr("")}}, check: func(l Look) bool { return l.Back.Colour == "" }},
		{name: "kinds replace", in: LookInput{Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}}},
			check: func(l Look) bool { return len(l.Kinds) == 1 && l.Kinds[0].Key == "boon" }},
		{name: "old shape long is refused", in: LookInput{Shape: ptr("long")}, wantErr: "shape must be one of: tall, standard, small"},
		{name: "old turn slide is refused", in: LookInput{Turn: ptr("slide")}, wantErr: "turn must be one of: flip, slow, toss, slam, sweep, burn, ink"},
		{name: "old turn turnup is refused", in: LookInput{Turn: ptr("turnup")}, wantErr: "turn must be one of: flip, slow, toss, slam, sweep, burn, ink"},
		{name: "bad face", in: LookInput{Face: ptr("cover")}, wantErr: "face must be one of: plate, window, full"},
		{name: "bad border", in: LookInput{Border: ptr("gold")}, wantErr: "border must be one of: double, line, kit, metal, none"},
		{name: "bad pattern", in: LookInput{Back: &BackInput{Pattern: ptr("checks")}}, wantErr: "back pattern must be one of"},
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

func TestLookStoredValuesFallBackAndPaletteIsFixed(t *testing.T) {
	if want := []string{"violet", "indigo", "blue", "cyan", "teal", "moss", "plum", "rose", "umber", "slate"}; !reflect.DeepEqual(Palette, want) {
		t.Errorf("palette = %v", Palette)
	}
	// A stored value from an older list is replaced by the default, never sent.
	got := readLook(`{"face":"plate","shape":"long","border":"gold","turn":"slide","back":{"pattern":"lattice","colour":"#fff"},"kinds":[{"key":"Bad","name":"x","colour":"red"}]}`)
	if got.Shape != "tall" || got.Border != "double" || got.Turn != "flip" || got.Back.Colour != "" || len(got.Kinds) != 0 {
		t.Errorf("fallback = %+v", got)
	}
	if got := readLook("{"); !reflect.DeepEqual(got, defaultLook()) {
		t.Errorf("unparseable look = %+v", got)
	}
}

// The lists and their defaults are pinned so a change to either is deliberate.
func TestLookListsAndDefaultsArePinned(t *testing.T) {
	lists := map[string][]string{
		"face":   {"plate", "window", "full"},
		"shape":  {"tall", "standard", "small"},
		"border": {"double", "line", "kit", "metal", "none"},
		"turn":   {"flip", "slow", "toss", "slam", "sweep", "burn", "ink"},
	}
	got := map[string][]string{"face": faceValues, "shape": shapeValues, "border": borderValues, "turn": turnValues}
	for k, want := range lists {
		if !reflect.DeepEqual(got[k], want) {
			t.Errorf("%s list = %v, want %v", k, got[k], want)
		}
	}
	def := defaultLook()
	if def.Face != "plate" || def.Shape != "tall" || def.Border != "double" || def.Turn != "flip" || def.Back.Pattern != "lattice" {
		t.Errorf("defaults = %+v", def)
	}
	// Each default is the first value of its list.
	if def.Face != faceValues[0] || def.Shape != shapeValues[0] || def.Border != borderValues[0] || def.Turn != turnValues[0] {
		t.Error("a default is not the first value of its list")
	}
	// An unknown value in a stored look falls back to the default, never fails the read.
	got2 := readLook(`{"face":"x","shape":"x","border":"x","turn":"x"}`)
	if got2.Face != def.Face || got2.Shape != def.Shape || got2.Border != def.Border || got2.Turn != def.Turn {
		t.Errorf("stored fallback = %+v", got2)
	}
}

func TestLookInputRefusesUnknownKeysNamingThem(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{"known keys", `{"face":"plate","back":{"pattern":"dots"},"kinds":[{"key":"a","name":"A","colour":"blue"}]}`, ""},
		{"unknown top-level key", `{"fase":"plate"}`, `no key "fase"`},
		{"unknown back key", `{"back":{"patern":"dots"}}`, `no key "patern"`},
		{"unknown kind key", `{"kinds":[{"key":"a","name":"A","colour":"blue","icon":"x"}]}`, `no key "icon"`},
		{"wrong kind of value", `{"face":5}`, `"face" has the wrong kind of value`},
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
	raw := `{"face":"window","shape":"x","border":"metal","turn":"ink",` +
		`"back":{"pattern":"dots","colour":"nope","words":"` + longWords + `"},` +
		`"kinds":[{"key":"ok","name":"Fine","colour":"teal"},{"key":"Bad","name":"x","colour":"teal"},` +
		`{"key":"ok","name":"Again","colour":"teal"},{"key":"c","name":"","colour":"teal"},{"key":"d","name":"D","colour":"red"},` +
		`{"key":"e","name":"Fine too","colour":"plum"}]}`
	got := readLook(raw)
	if got.Face != "window" || got.Border != "metal" || got.Turn != "ink" || got.Back.Pattern != "dots" {
		t.Errorf("good fields were lost: %+v", got)
	}
	if got.Shape != "tall" || got.Back.Colour != "" || got.Back.Words != "" {
		t.Errorf("bad fields did not fall back: %+v", got)
	}
	if len(got.Kinds) != 2 || got.Kinds[0].Key != "ok" || got.Kinds[1].Key != "e" {
		t.Errorf("only the bad kinds should be dropped: %+v", got.Kinds)
	}
	if got := readLook(`{"back":{"words":"a` + "\x01" + `b"}}`); got.Back.Words != "" {
		t.Errorf("a control character in the back's words survived a read: %q", got.Back.Words)
	}
	// A key of the wrong type costs only itself.
	if got := readLook(`{"face":7,"turn":"ink","kinds":"nope"}`); got.Turn != "ink" || got.Face != "plate" || len(got.Kinds) != 0 {
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

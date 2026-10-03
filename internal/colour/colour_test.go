package colour

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Expected values below were produced by running the original JavaScript
// under node; if Go disagrees, the Go is wrong, not the table.

const deTol = 1e-9

type fillCase struct {
	in, fill, on string
	de           float64
	flipped      bool
}
type readableCase struct {
	in, hex string
	de      float64
}
type tameCase struct {
	in         string
	light      string
	lightToned bool
	deep       string
	deepToned  bool
}
type mixCase struct{ in, want string }
type contrastCase struct {
	a, b string
	want float64
}
type overCase struct {
	fg, bg string
	alpha  float64
	want   string
}
type channelCase struct{ in, want string }

var fillCases = []fillCase{
	{"#6366f1", "#6265f0", "#ffffff", 0.0031665004397781146, false},
	{"#3b82f6", "#2b71e4", "#ffffff", 0.05276673009522643, false},
	{"#06b6d4", "#06b6d4", "#111827", 0, true},
	{"#10b981", "#10b981", "#111827", 0, true},
	{"#f59e0b", "#f59e0b", "#111827", 0, true},
	{"#f43f5e", "#e1294f", "#ffffff", 0.051562975444730035, false},
	{"#a855f7", "#9e4aec", "#ffffff", 0.03195240547173188, false},
	{"#f97316", "#f97316", "#111827", 0, true},
	{"#9a4a26", "#9a4a26", "#ffffff", 0, false},
	{"#8f2d2d", "#8f2d2d", "#ffffff", 0, false},
	{"#8a6a1f", "#8a6a1f", "#ffffff", 0, false},
	{"#5b6be0", "#5b6be0", "#ffffff", 0, false},
	{"#2f7d4f", "#2f7d4f", "#ffffff", 0, false},
	{"#c2410c", "#c2410c", "#ffffff", 0, false},
	{"#0e7490", "#0e7490", "#ffffff", 0, false},
	{"#7c3aed", "#7c3aed", "#ffffff", 0, false},
	{"#94651b", "#94651b", "#ffffff", 0, false},
	{"#0f766e", "#0f766e", "#ffffff", 0, false},
	{"#ffff00", "#ffff00", "#111827", 0, true},
	{"#00ff00", "#00ff00", "#111827", 0, true},
	{"#111111", "#111111", "#ffffff", 0, false},
	{"#fafafa", "#fafafa", "#111827", 0, true},
}

var readableLightBgCases = []readableCase{
	{"#6366f1", "#5f61eb", 0.015897352041840153},
	{"#3b82f6", "#286fe1", 0.059927153431477996},
	{"#06b6d4", "#007e94", 0.17112153514380474},
	{"#10b981", "#00835a", 0.15967514469226687},
	{"#f59e0b", "#a16600", 0.21191066421551885},
	{"#f43f5e", "#dd224c", 0.06328089483952477},
	{"#a855f7", "#9b46e8", 0.04279132011359006},
	{"#f97316", "#bd5300", 0.1396172361548939},
	{"#9a4a26", "#9a4a26", 0},
	{"#8f2d2d", "#8f2d2d", 0},
	{"#8a6a1f", "#8a6a1f", 0},
	{"#5b6be0", "#5968dd", 0.009060778211916502},
	{"#2f7d4f", "#2f7d4f", 0},
	{"#c2410c", "#c2410c", 0},
	{"#0e7490", "#0e7490", 0},
	{"#7c3aed", "#7c3aed", 0},
	{"#94651b", "#94651b", 0},
	{"#0f766e", "#0f766e", 0},
	{"#ffff00", "#777700", 0.42673772602293175},
	{"#00ff00", "#008600", 0.34773109866096935},
	{"#111111", "#111111", 0},
	{"#fafafa", "#737373", 0.4295768912577877},
}

var readableDarkBgCases = []readableCase{
	{"#6366f1", "#7a82ff", 0.07849910262340827},
	{"#3b82f6", "#478dff", 0.03314897614288408},
	{"#06b6d4", "#06b6d4", 0},
	{"#10b981", "#10b981", 0},
	{"#f59e0b", "#f59e0b", 0},
	{"#f43f5e", "#ff4c68", 0.03156946567592035},
	{"#a855f7", "#b46cff", 0.05217103583884146},
	{"#f97316", "#f97316", 0},
	{"#9a4a26", "#ce7855", 0.1558401663644777},
	{"#8f2d2d", "#d9706b", 0.2199034800119932},
	{"#8a6a1f", "#ab8a43", 0.10720609216897127},
	{"#5b6be0", "#7285fd", 0.08300720443328109},
	{"#2f7d4f", "#529e6e", 0.10821649760338765},
	{"#c2410c", "#eb673b", 0.11662357085200101},
	{"#0e7490", "#429ab7", 0.12498038239679998},
	{"#7c3aed", "#9c77ff", 0.13758908327797073},
	{"#94651b", "#b78642", 0.11082246495810212},
	{"#0f766e", "#449d94", 0.12858883184703518},
	{"#ffff00", "#ffff00", 0},
	{"#00ff00", "#00ff00", 0},
	{"#111111", "#8f8f8f", 0.4724034930625383},
	{"#fafafa", "#fafafa", 0},
}

var tameCases = []tameCase{
	{"#6366f1", "#6366f1", false, "#6366f1", false},
	{"#3b82f6", "#3b82f6", false, "#3b82f6", false},
	{"#06b6d4", "#06b6d4", false, "#06b6d4", false},
	{"#10b981", "#10b981", false, "#10b981", false},
	{"#f59e0b", "#f59e0b", false, "#f59e0b", false},
	{"#f43f5e", "#f43f5e", false, "#f43f5e", false},
	{"#a855f7", "#a855f7", false, "#a855f7", false},
	{"#f97316", "#f97316", false, "#f97316", false},
	{"#9a4a26", "#9a4a26", false, "#9a4a26", false},
	{"#8f2d2d", "#8f2d2d", false, "#8f2d2d", false},
	{"#8a6a1f", "#8a6a1f", false, "#8a6a1f", false},
	{"#5b6be0", "#5b6be0", false, "#5b6be0", false},
	{"#2f7d4f", "#2f7d4f", false, "#2f7d4f", false},
	{"#c2410c", "#c2410c", false, "#c2410c", false},
	{"#0e7490", "#0e7490", false, "#0e7490", false},
	{"#7c3aed", "#7c3aed", false, "#7c3aed", false},
	{"#94651b", "#94651b", false, "#94651b", false},
	{"#0f766e", "#0f766e", false, "#0f766e", false},
	{"#ffff00", "#c3c464", true, "#c3c464", true},
	{"#00ff00", "#8fd28a", true, "#8fd28a", true},
	{"#111111", "#2e2e2e", true, "#111111", false},
	{"#fafafa", "#bebebe", true, "#bebebe", true},
}

var mixCases = []mixCase{
	{"#6366f1", "#a3aefc"},
	{"#3b82f6", "#94bdfe"},
	{"#06b6d4", "#97d8e8"},
	{"#10b981", "#97d9b8"},
	{"#f59e0b", "#fccb93"},
	{"#f43f5e", "#ff9fa5"},
	{"#a855f7", "#cea7ff"},
	{"#f97316", "#ffb58f"},
	{"#9a4a26", "#cb9a86"},
	{"#8f2d2d", "#c68b87"},
	{"#8a6a1f", "#beab87"},
	{"#5b6be0", "#9faff2"},
	{"#2f7d4f", "#8fb69b"},
	{"#c2410c", "#e49a83"},
	{"#0e7490", "#86b1c1"},
	{"#7c3aed", "#b19cfb"},
	{"#94651b", "#c4a986"},
	{"#0f766e", "#87b2ad"},
	{"#ffff00", "#feffa3"},
	{"#00ff00", "#a4ff9c"},
	{"#111111", "#717171"},
	{"#fafafa", "#fcfcfc"},
}

var contrastCases = []contrastCase{
	{"#6366f1", "#ffffff", 4.466894269549531},
	{"#ffff00", "#ffffff", 1.0738392309265699},
	{"#111111", "#fafafa", 18.09129157920412},
	{"#10b981", "#111827", 6.993344803733124},
	{"#000000", "#ffffff", 21},
	{"#fff", "#000", 21},
}

var overCases = []overCase{
	{"#6366f1", "#ffffff", 0.5, "#b1b3f8"},
	{"#f43f5e", "#111827", 0.18, "#3a1f31"},
	{"#10b981", "#fafafa", 0.07, "#eaf5f2"},
}

var channelCases = []channelCase{
	{"#6366f1", "99 102 241"},
	{"#000000", "0 0 0"},
	{"#ffffff", "255 255 255"},
	{"#fa0", "255 170 0"},
}

func near(a, b float64) bool { return math.Abs(a-b) <= deTol }

func TestFillFor(t *testing.T) {
	for _, c := range fillCases {
		t.Run(c.in, func(t *testing.T) {
			got := FillFor(c.in)
			if got.Fill != c.fill || got.On != c.on || got.Flipped != c.flipped || !near(got.DeltaE, c.de) {
				t.Errorf("FillFor(%s) = %+v, want fill=%s on=%s de=%v flipped=%v", c.in, got, c.fill, c.on, c.de, c.flipped)
			}
		})
	}
}

func TestReadable(t *testing.T) {
	groups := []struct {
		name   string
		bgs    []string
		darker bool
		cases  []readableCase
	}{
		{"darker on light", []string{"#f9fafb", "#ffffff"}, true, readableLightBgCases},
		{"lighter on dark", []string{"#111827", "#1f2937"}, false, readableDarkBgCases},
	}
	for _, g := range groups {
		for _, c := range g.cases {
			t.Run(g.name+"/"+c.in, func(t *testing.T) {
				got := Readable(c.in, g.bgs, 4.5, g.darker)
				if got.Hex != c.hex || !near(got.DeltaE, c.de) {
					t.Errorf("Readable(%s) = %+v, want %s de=%v", c.in, got, c.hex, c.de)
				}
			})
		}
	}
}

func TestTame(t *testing.T) {
	for _, c := range tameCases {
		t.Run(c.in, func(t *testing.T) {
			if got, toned := Tame(c.in, false); got != c.light || toned != c.lightToned {
				t.Errorf("Tame(%s,false) = %s,%v want %s,%v", c.in, got, toned, c.light, c.lightToned)
			}
			if got, toned := Tame(c.in, true); got != c.deep || toned != c.deepToned {
				t.Errorf("Tame(%s,true) = %s,%v want %s,%v", c.in, got, toned, c.deep, c.deepToned)
			}
		})
	}
}

func TestMix(t *testing.T) {
	for _, c := range mixCases {
		t.Run(c.in, func(t *testing.T) {
			if got := Mix(c.in, "#ffffff", 0.45); got != c.want {
				t.Errorf("Mix(%s) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestContrast(t *testing.T) {
	for _, c := range contrastCases {
		t.Run(c.a+"/"+c.b, func(t *testing.T) {
			if got := Contrast(c.a, c.b); !near(got, c.want) {
				t.Errorf("Contrast = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOver(t *testing.T) {
	for _, c := range overCases {
		if got := Over(c.fg, c.bg, c.alpha); got != c.want {
			t.Errorf("Over(%s,%s,%v) = %s, want %s", c.fg, c.bg, c.alpha, got, c.want)
		}
	}
}

func TestRGBChannels(t *testing.T) {
	for _, c := range channelCases {
		if got := RGBChannels(c.in); got != c.want {
			t.Errorf("RGBChannels(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidHex(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"#6366f1", true}, {"#ABCDEF", true}, {"#abc", false}, {"6366f1", false},
		{"#6366f", false}, {"#6366f1f", false}, {"#gggggg", false}, {"", false},
	}
	for _, tc := range tests {
		if got := ValidHex(tc.in); got != tc.want {
			t.Errorf("ValidHex(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLower(t *testing.T) {
	if got := Lower("#ABCDEF"); got != "#abcdef" {
		t.Errorf("Lower = %s", got)
	}
}

// menuPins are the JS-generated expectations shared with
// test/js/customize_menu_colour.test.mjs, so the editor preview and the real
// menu are pinned to the same hexes.
type menuPins struct {
	Dark   map[string]string `json:"dark"`
	Tinted map[string]string `json:"tinted"`
}

func loadMenuPins(t *testing.T) menuPins {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "js", "fixtures", "menu_colour_pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p menuPins
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMenuDark(t *testing.T) {
	pins := loadMenuPins(t)
	for in, want := range pins.Dark {
		t.Run(in, func(t *testing.T) {
			got := MenuDark(in)
			if got != want {
				t.Errorf("MenuDark(%s) = %s, want %s", in, got, want)
			}
			if c := Contrast(got, "#ffffff"); c < 7 {
				t.Errorf("MenuDark(%s) = %s has contrast %.2f, want >= 7", in, got, c)
			}
			if again := MenuDark(got); again != got {
				t.Errorf("not idempotent: %s -> %s", got, again)
			}
		})
	}
	if got := MenuDark("#6366F1"); got != pins.Dark["#6366f1"] {
		t.Errorf("upper-case input = %s, want the lower-case result", got)
	}
}

func TestMenuTinted(t *testing.T) {
	for in, want := range loadMenuPins(t).Tinted {
		t.Run(in, func(t *testing.T) {
			got := MenuTinted(in)
			if got != want {
				t.Errorf("MenuTinted(%s) = %s, want %s", in, got, want)
			}
			if c := Contrast(got, "#ffffff"); c < 7 {
				t.Errorf("MenuTinted(%s) = %s has contrast %.2f, want >= 7", in, got, c)
			}
		})
	}
}

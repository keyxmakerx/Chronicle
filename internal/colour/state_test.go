package colour

import "testing"

func TestNearAmber(t *testing.T) {
	for _, tc := range []struct {
		hex  string
		want bool
	}{
		{"#f59e0b", true},  // amber
		{"#f97316", true},  // orange, hue 25
		{"#a16207", true},  // lantern
		{"#b45309", true},  // brass
		{"#c2410c", false}, // ember, hue 17
		{"#9a4a26", false}, // sienna, hue 18
		{"#84cc16", false}, // lime, hue 84
		{"#6366f1", false},
		{"#8a8350", false}, // desaturated: sat under 45%
		{"#5c3d00", false}, // dark: lightness under 30%
	} {
		if got := NearAmber(tc.hex); got != tc.want {
			t.Errorf("NearAmber(%s) = %v, want %v", tc.hex, got, tc.want)
		}
	}
}

func TestOffState(t *testing.T) {
	for _, tc := range []struct {
		hex   string
		moved bool
	}{
		{"#dc2626", true},  // the state red itself
		{"#16a34a", true},  // the state green itself
		{"#f43f5e", true},  // rose, 11 degrees from red
		{"#6366f1", false}, // indigo
		{"#10b981", false}, // emerald, 18 degrees from green
		{"#c2410c", false}, // ember
	} {
		out, moved := OffState(tc.hex)
		if moved != tc.moved {
			t.Errorf("OffState(%s) moved = %v, want %v (%s)", tc.hex, moved, tc.moved, out)
		}
		if !moved && out != tc.hex {
			t.Errorf("OffState(%s) changed an unmoved colour to %s", tc.hex, out)
		}
		if moved {
			if _, _, l := hsl(out); l <= 0 {
				t.Errorf("OffState(%s) = %s collapsed", tc.hex, out)
			}
			if again, m2 := OffState(out); m2 || again != out {
				t.Errorf("OffState(%s) = %s is not stable (%s)", tc.hex, out, again)
			}
		}
	}
}

// The picker's JS (test/js/customize_accent_rules.test.mjs) pins these same
// two results, so the preview and the saved colour agree to the hex digit.
func TestOffStateMatchesPicker(t *testing.T) {
	for in, want := range map[string]string{"#dc2626": "#b05f56", "#16a34a": "#6b9472"} {
		if got, _ := OffState(in); got != want {
			t.Errorf("OffState(%s) = %s, want %s", in, got, want)
		}
	}
}

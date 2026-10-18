package layouts

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/colour"
)

func TestStyleKitsMatchStylesheet(t *testing.T) {
	raw, err := os.ReadFile("../../../static/css/style_kits.css")
	if err != nil {
		t.Fatal(err)
	}
	b := string(raw)
	withTestFonts(t)
	for id, k := range czKits {
		if !strings.Contains(b, `[data-cz-kit="`+id+`"] {`) || !strings.Contains(b, `.dark[data-cz-kit="`+id+`"] {`) {
			t.Errorf("style_kits.css has no light and dark palette for %s", id)
		}
		for _, f := range append(append([]string{}, k.headFonts...), k.bodyFonts...) {
			if len(customizeFonts()[f]) == 0 {
				t.Errorf("%s uses face %s, which has no self-hosted files", id, f)
			}
		}
	}
}

func TestAppearanceCSSStyleKit(t *testing.T) {
	withTestFonts(t)
	cases := []struct {
		name               string
		a                  AppearanceData
		accent             string
		wantHead, wantBody bool
		wantOswald         bool
		wantAccent         bool
	}{
		{"kit faces replace tall capitals", AppearanceData{PageLook: "annals", StyleKit: "ornate"}, "", true, true, false, false},
		{"owner faces win", AppearanceData{PageLook: "annals", StyleKit: "ornate", HeadingFont: "cinzel", BodyFont: "lora"}, "", false, false, false, false},
		{"the default colour keeps the kit's", AppearanceData{PageLook: "annals", StyleKit: "lumen"}, "#6366f1", true, true, false, false},
		{"a picked colour replaces it", AppearanceData{PageLook: "annals", StyleKit: "sketchbook"}, "#2f6b3f", true, true, false, true},
		{"material is the base", AppearanceData{PageLook: "annals", StyleKit: "material"}, "#2f6b3f", false, false, true, false},
		{"no kit on Standard pages", AppearanceData{PageLook: "standard", StyleKit: "ornate"}, "#2f6b3f", false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetAppearance(context.Background(), &tc.a)
			if tc.accent != "" {
				ctx = SetAccentColor(ctx, tc.accent)
			}
			css := AppearanceCSS(ctx)
			if got := strings.Contains(css, "--kit-hf:"); got != tc.wantHead {
				t.Errorf("kit heading face = %v, want %v: %s", got, tc.wantHead, css)
			}
			if got := strings.Contains(css, "--kit-bf:"); got != tc.wantBody {
				t.Errorf("kit text face = %v, want %v: %s", got, tc.wantBody, css)
			}
			if got := strings.Contains(css, "--annals-hcase:uppercase;"); got != tc.wantOswald {
				t.Errorf("tall capitals = %v, want %v", got, tc.wantOswald)
			}
			if got := strings.Contains(css, "--kit-user-acc:"); got != tc.wantAccent {
				t.Errorf("kit accent = %v, want %v: %s", got, tc.wantAccent, css)
			}
		})
	}
}

// A picked colour must read as link text on each kit's page and paper in
// both themes, however light or dark it was picked.
func TestStyleKitAccentReads(t *testing.T) {
	re := regexp.MustCompile(`--kit-user-acc:(#[0-9a-f]{6});`)
	for id, k := range czKits {
		for _, c := range []string{"#f2e8a0", "#1a1a40", "#a3283a", "#7fd4ff"} {
			a := AppearanceData{PageLook: "annals", StyleKit: id}
			css := AppearanceCSS(SetAccentColor(SetAppearance(context.Background(), &a), c))
			m := re.FindAllStringSubmatch(css, -1)
			if len(m) != 2 {
				t.Fatalf("%s %s: want a light and a dark accent: %s", id, c, css)
			}
			for i, theme := range []string{"light", "dark"} {
				for _, bg := range []string{k.page[i], k.paper[i]} {
					if r := colour.Contrast(m[i][1], bg); r < 4.5 {
						t.Errorf("%s %s %s: %s on %s is %.2f:1", id, c, theme, m[i][1], bg, r)
					}
				}
			}
		}
	}
}

package layouts

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
)

// withTestFonts points the font list at the real static tree and resets the
// once-loaded cache, restoring both so other tests see the original state.
func withTestFonts(t *testing.T) {
	t.Helper()
	origFS := czFontsFS
	czFontsFS = os.DirFS("../../../static")
	czFontsOnce = sync.Once{}
	czFonts = nil
	t.Cleanup(func() {
		czFontsFS = origFS
		czFontsOnce = sync.Once{}
		czFonts = nil
	})
}

func TestAppearanceClassicIsEmpty(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"no appearance", context.Background()},
		{"empty appearance", SetAppearance(context.Background(), &AppearanceData{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AppearanceCSS(tc.ctx); got != "" {
				t.Errorf("AppearanceCSS = %q, want empty", got)
			}
			if got := AppearanceAttrs(tc.ctx); len(got) != 0 {
				t.Errorf("AppearanceAttrs = %v, want none", got)
			}
		})
	}
}

func TestAppearanceAttrs(t *testing.T) {
	cases := []struct {
		name string
		a    AppearanceData
		want map[string]string
	}{
		{"heading same", AppearanceData{HeadingFont: "same"}, map[string]string{}},
		{"heading cinzel", AppearanceData{HeadingFont: "cinzel"}, map[string]string{"data-cz-heading": "cinzel"}},
		{"all", AppearanceData{NavStyle: "comet", NavStrength: "lively", NavPageName: "hidden", ButtonStyle: "glow",
			Elevation: "flat", TypeScale: "roomy", ReduceMotion: true},
			map[string]string{"data-cz-nav": "comet", "data-cz-strength": "lively", "data-cz-pagename": "hidden",
				"data-cz-btn": "glow", "data-cz-elev": "flat", "data-cz-scale": "roomy", "data-cz-reduce": "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.a
			got := AppearanceAttrs(SetAppearance(context.Background(), &a))
			if len(got) != len(tc.want) {
				t.Fatalf("attrs = %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("attr %s = %v, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestAppearanceCSSTones(t *testing.T) {
	ctx := SetAppearance(context.Background(), &AppearanceData{PageTone: "warm"})
	css := AppearanceCSS(ctx)
	warm := czTones["warm"]
	for _, want := range []string{
		":root{", ":root.dark{",
		"--color-bg-primary:" + warm[0].bg + ";",
		"--color-bg-primary:" + warm[1].bg + ";",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("css missing %q in %s", want, css)
		}
	}
	rootAt := strings.Index(css, ":root{")
	darkAt := strings.Index(css, ":root.dark{")
	if strings.Index(css, warm[0].bg) > darkAt || !strings.Contains(css[darkAt:], warm[1].bg) || rootAt > darkAt {
		t.Errorf("light tokens must sit in :root and dark in :root.dark: %s", css)
	}
}

func TestAppearanceCSSAccent(t *testing.T) {
	cases := []struct {
		name       string
		accent     string
		action     string
		wantFill   bool
		wantOnFill string
	}{
		{"accent only", "#6366f1", "", true, ""},
		{"action keeps buttons", "#6366f1", "#10b981", false, ""},
		{"amber needs dark words", "#f59e0b", "", true, "--color-accent-on-fill:#111827"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetAccentColor(context.Background(), tc.accent)
			if tc.action != "" {
				ctx = SetAccentAction(ctx, tc.action)
			}
			css := AppearanceCSS(ctx)
			if !strings.Contains(css, "--color-accent-link:") {
				t.Errorf("link shade missing: %s", css)
			}
			if has := strings.Contains(css, "--color-accent-fill:"); has != tc.wantFill {
				t.Errorf("accent-fill present = %v, want %v: %s", has, tc.wantFill, css)
			}
			if has := strings.Contains(css, "--color-accent-on-fill:"); has != tc.wantFill {
				t.Errorf("on-fill present = %v, want %v", has, tc.wantFill)
			}
			if tc.wantOnFill != "" && !strings.Contains(css, tc.wantOnFill) {
				t.Errorf("css missing %q: %s", tc.wantOnFill, css)
			}
			if !strings.Contains(css, ":root.dark{--color-accent-link:") {
				t.Errorf("dark link shade missing: %s", css)
			}
		})
	}
}

func TestAppearanceCSSFonts(t *testing.T) {
	withTestFonts(t)
	cases := []struct {
		name      string
		a         AppearanceData
		wantParts []string
		notParts  []string
	}{
		{"heading same", AppearanceData{HeadingFont: "same"}, nil, []string{"@font-face", "--font-heading"}},
		{"heading cinzel", AppearanceData{HeadingFont: "cinzel"},
			[]string{"@font-face{font-family:'Cinzel'", "--font-heading:'Cinzel'", "--cz-hw:600"}, nil},
		{"body inter adds nothing", AppearanceData{BodyFont: "inter"}, nil, []string{"@font-face", "--font-campaign"}},
		{"body lora", AppearanceData{BodyFont: "lora"}, []string{"@font-face{font-family:'Lora'", "--font-campaign:'Lora'"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.a
			css := AppearanceCSS(SetAppearance(context.Background(), &a))
			for _, w := range tc.wantParts {
				if !strings.Contains(css, w) {
					t.Errorf("missing %q in %s", w, css)
				}
			}
			for _, w := range tc.notParts {
				if strings.Contains(css, w) {
					t.Errorf("unexpected %q in %s", w, css)
				}
			}
		})
	}
}

func TestFontFaceCSS(t *testing.T) {
	withTestFonts(t)
	if got := fontFaceCSS("nonexistent"); got != "" {
		t.Errorf("unknown id = %q, want empty", got)
	}
	got := fontFaceCSS("cinzel")
	for _, w := range []string{"font-display:swap", "/static/fonts/customize/", "format('woff2')", "unicode-range:"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %s", w, got)
		}
	}
}

func TestTopbarWantsLightWords(t *testing.T) {
	cases := []struct {
		name  string
		style *TopbarStyleData
		want  bool
	}{
		{"default header", nil, false},
		{"empty mode", &TopbarStyleData{}, false},
		{"image", &TopbarStyleData{Mode: "image", ImagePath: "a.png"}, true},
		{"dark solid", &TopbarStyleData{Mode: "solid", Color: "#0f172a"}, true},
		{"light solid", &TopbarStyleData{Mode: "solid", Color: "#f4eddd"}, false},
		{"dark gradient", &TopbarStyleData{Mode: "gradient", GradientFrom: "#0f172a", GradientTo: "#1e1b4b"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := topbarWantsLightWords(ctxWithTopbarStyle(tc.style)); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTopbarScrimStyle(t *testing.T) {
	cases := []struct {
		name  string
		style *TopbarStyleData
		want  string
	}{
		{"nil is medium", nil, "rgba(0,0,0,0.55), rgba(0,0,0,0.25)"},
		{"medium", &TopbarStyleData{Mode: "image"}, "rgba(0,0,0,0.55), rgba(0,0,0,0.25)"},
		{"light", &TopbarStyleData{Mode: "image", Scrim: "light"}, "rgba(0,0,0,0.38), rgba(0,0,0,0.15)"},
		{"strong", &TopbarStyleData{Mode: "image", Scrim: "strong"}, "rgba(0,0,0,0.72), rgba(0,0,0,0.45)"},
		{"unknown is medium", &TopbarStyleData{Mode: "image", Scrim: "bogus"}, "rgba(0,0,0,0.55), rgba(0,0,0,0.25)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := topbarScrimStyle(ctxWithTopbarStyle(tc.style))
			if !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want substring %q", got, tc.want)
			}
		})
	}
}

func TestTopbarWidgets(t *testing.T) {
	cases := []struct {
		name string
		tc   *TopbarContentData
		want []string
		nil_ bool
	}{
		{"nil content", nil, nil, true},
		{"legacy quote", &TopbarContentData{Mode: "quote"}, []string{"text"}, false},
		{"legacy links", &TopbarContentData{Mode: "links"}, []string{"links"}, false},
		{"legacy none", &TopbarContentData{Mode: "none"}, nil, true},
		{"explicit empty beats mode", &TopbarContentData{Mode: "quote", Widgets: []string{}}, []string{}, false},
		{"explicit order", &TopbarContentData{Mode: "links", Widgets: []string{"text", "links"}}, []string{"text", "links"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tc.TopbarWidgets()
			if tc.nil_ != (got == nil) {
				t.Fatalf("nil-ness: got %#v", got)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNavPageNameHidden(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"no appearance", context.Background(), false},
		{"row", SetAppearance(context.Background(), &AppearanceData{NavPageName: "row"}), false},
		{"unset", SetAppearance(context.Background(), &AppearanceData{}), false},
		{"hidden", SetAppearance(context.Background(), &AppearanceData{NavPageName: "hidden"}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := navPageNameHidden(tc.ctx); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTopbarRendersWidgetsInOrder proves the stored order, not a fixed one,
// decides which widget comes first in the header.
func TestTopbarRendersWidgetsInOrder(t *testing.T) {
	content := func(w ...string) *TopbarContentData {
		return &TopbarContentData{
			Mode: "links", Quote: "QUOTE-MARKER", Widgets: w,
			Links: []TopbarLinkData{{Label: "LINK-MARKER", URL: "/x"}},
		}
	}
	cases := []struct {
		name       string
		widgets    []string
		wantQuote  bool
		wantLink   bool
		quoteFirst bool
	}{
		{"text then links", []string{"text", "links"}, true, true, true},
		{"links then text", []string{"links", "text"}, true, true, false},
		{"links only", []string{"links"}, false, true, false},
		{"none", []string{}, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetCampaignID(context.Background(), "camp-1")
			ctx = SetTopbarContent(ctx, content(tc.widgets...))
			var buf bytes.Buffer
			if err := Topbar().Render(ctx, &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			html := buf.String()
			q, l := strings.Index(html, "QUOTE-MARKER"), strings.Index(html, "LINK-MARKER")
			if (q >= 0) != tc.wantQuote || (l >= 0) != tc.wantLink {
				t.Fatalf("quote at %d, link at %d; want quote=%v link=%v", q, l, tc.wantQuote, tc.wantLink)
			}
			if tc.wantQuote && tc.wantLink && (q < l) != tc.quoteFirst {
				t.Errorf("quote at %d, link at %d; quoteFirst want %v", q, l, tc.quoteFirst)
			}
		})
	}
}

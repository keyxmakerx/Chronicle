package layouts

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/colour"
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

func TestAppearanceAttrs_HeaderAndCorner(t *testing.T) {
	cases := []struct {
		name string
		a    AppearanceData
		want map[string]string
	}{
		{"slim header, plain corner add nothing", AppearanceData{}, map[string]string{}},
		{"tall header", AppearanceData{HeaderHeight: "tall"}, map[string]string{"data-cz-hdr": "tall"}},
		{"unknown height ignored", AppearanceData{HeaderHeight: "huge"}, map[string]string{}},
		{"subtitle corner", AppearanceData{SidebarCorner: "subtitle"}, map[string]string{"data-cz-corner": "subtitle"}},
		{"banner corner", AppearanceData{SidebarCorner: "banner"}, map[string]string{"data-cz-corner": "banner"}},
		{"unknown corner ignored", AppearanceData{SidebarCorner: "mural"}, map[string]string{}},
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

func TestAppearanceCSSMenu(t *testing.T) {
	cases := []struct {
		name     string
		a        AppearanceData
		accent   string
		wantBg   string // "" means no menu tokens at all
		wantText string
	}{
		{"charcoal is the stylesheet's own", AppearanceData{}, "", "", ""},
		{"charcoal named", AppearanceData{SidebarColour: "charcoal"}, "", "", ""},
		{"ink", AppearanceData{SidebarColour: "ink"}, "", "#0e1424", "#cdd5e3"},
		{"tinted follows the accent", AppearanceData{SidebarColour: "tinted"}, "#10b981", colour.MenuTinted("#10b981"), "#cbd5e1"},
		{"tinted without an accent uses Chronicle's", AppearanceData{SidebarColour: "tinted"}, "", colour.MenuTinted("#6366f1"), "#cbd5e1"},
		{"own is darkened", AppearanceData{SidebarColour: "own", SidebarOwn: "#9a4a26"}, "", colour.MenuDark("#9a4a26"), "#cbd5e1"},
		{"own that already reads is kept", AppearanceData{SidebarColour: "own", SidebarOwn: "#2b4a3a"}, "", "#2b4a3a", "#cbd5e1"},
		{"own without a colour draws nothing", AppearanceData{SidebarColour: "own"}, "", "", ""},
		{"own with a bad colour draws nothing", AppearanceData{SidebarColour: "own", SidebarOwn: "green"}, "", "", ""},
		{"unknown choice draws nothing", AppearanceData{SidebarColour: "light"}, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.a
			ctx := SetAppearance(context.Background(), &a)
			if tc.accent != "" {
				ctx = SetAccentColor(ctx, tc.accent)
			}
			css := AppearanceCSS(ctx)
			if tc.wantBg == "" {
				if strings.Contains(css, "--color-sidebar-bg") {
					t.Errorf("unexpected menu tokens in %q", css)
				}
				return
			}
			want := "--color-sidebar-bg:" + tc.wantBg + ";"
			// Both themes: the stylesheet's own :root.dark rule would
			// out-rank a :root-only token.
			if n := strings.Count(css, want); n != 2 {
				t.Errorf("menu colour %s appears %d times in %q, want in :root and :root.dark", tc.wantBg, n, css)
			}
			if !strings.Contains(css, "--cz-sb-text:"+tc.wantText+";") {
				t.Errorf("menu words %s missing in %q", tc.wantText, css)
			}
			if c := colour.Contrast(tc.wantBg, "#ffffff"); c < 7 {
				t.Errorf("menu colour %s has only %.2f:1 against white words", tc.wantBg, c)
			}
		})
	}

	t.Run("any own colour keeps white words at 7:1", func(t *testing.T) {
		for _, hex := range []string{"#ffffff", "#f59e0b", "#fde68a", "#6366f1", "#10b981", "#808080", "#000000"} {
			a := AppearanceData{SidebarColour: "own", SidebarOwn: hex}
			css := AppearanceCSS(SetAppearance(context.Background(), &a))
			m := regexp.MustCompile(`--color-sidebar-bg:(#[0-9a-f]{6});`).FindStringSubmatch(css)
			if m == nil {
				t.Fatalf("%s: no menu colour in %q", hex, css)
			}
			if c := colour.Contrast(m[1], "#ffffff"); c < 7 {
				t.Errorf("%s -> %s has only %.2f:1", hex, m[1], c)
			}
		}
	})
}

func TestAppearanceCSSPeekGlow(t *testing.T) {
	cases := []struct {
		name string
		a    AppearanceData
		want string
	}{
		{"follows the accent by default", AppearanceData{}, ""},
		{"accent named", AppearanceData{PeekGlow: "accent", PeekGlowColour: "#3b9fb5"}, ""},
		{"own colour", AppearanceData{PeekGlow: "own", PeekGlowColour: "#3b9fb5"}, "--peek-glow-rgb:59 159 181;"},
		{"own without a colour", AppearanceData{PeekGlow: "own"}, ""},
		{"own with a bad colour", AppearanceData{PeekGlow: "own", PeekGlowColour: "teal"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.a
			css := AppearanceCSS(SetAppearance(context.Background(), &a))
			if tc.want == "" {
				if strings.Contains(css, "--peek-glow-rgb") {
					t.Errorf("unexpected glow token in %q", css)
				}
			} else if !strings.Contains(css, tc.want) {
				t.Errorf("css %q missing %q", css, tc.want)
			}
		})
	}
}

func TestNavCorner(t *testing.T) {
	cases := []struct {
		name                   string
		ctx                    context.Context
		kind, subtitle, banner string
	}{
		{"no appearance", context.Background(), "plain", "", ""},
		{"empty", SetAppearance(context.Background(), &AppearanceData{}), "plain", "", ""},
		{"subtitle", SetAppearance(context.Background(), &AppearanceData{SidebarCorner: "subtitle", SidebarSubtitle: "Session 23"}), "subtitle", "Session 23", ""},
		{"subtitle with no words falls back to plain", SetAppearance(context.Background(), &AppearanceData{SidebarCorner: "subtitle"}), "plain", "", ""},
		{"banner", SetAppearance(context.Background(), &AppearanceData{SidebarCorner: "banner", SidebarBanner: "2026/09/b.png"}), "banner", "", "2026/09/b.png"},
		{"banner with no picture falls back to plain", SetAppearance(context.Background(), &AppearanceData{SidebarCorner: "banner"}), "plain", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, s, b := NavCorner(tc.ctx)
			if k != tc.kind || s != tc.subtitle || b != tc.banner {
				t.Errorf("NavCorner = %q %q %q, want %q %q %q", k, s, b, tc.kind, tc.subtitle, tc.banner)
			}
		})
	}
}

func TestNavBrandRendersCorner(t *testing.T) {
	cases := []struct {
		name    string
		a       *AppearanceData
		want    []string
		notWant []string
	}{
		{"plain", nil, []string{`class="nav-brand-name"`, `class="nav-brand-bg"`}, []string{"nav-brand-sub", "nav-banner"}},
		{"subtitle", &AppearanceData{SidebarCorner: "subtitle", SidebarSubtitle: `The <Drowned> Crown`},
			[]string{`class="nav-brand-sub"`, "The &lt;Drowned&gt; Crown"}, []string{"nav-banner"}},
		{"banner", &AppearanceData{SidebarCorner: "banner", SidebarBanner: "2026/09/b.png"},
			[]string{`class="nav-banner"`}, []string{"nav-brand-sub"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetCampaignName(SetCampaignID(context.Background(), "camp-1"), "Ashfall")
			if tc.a != nil {
				ctx = SetAppearance(ctx, tc.a)
			}
			var buf bytes.Buffer
			if err := campaignNavBrand().Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in %s", w, html)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("unexpected %q in %s", w, html)
				}
			}
			if !strings.Contains(html, `href="/campaigns/camp-1"`) {
				t.Errorf("the corner must still link to the dashboard: %s", html)
			}
		})
	}
}

func TestTopbarMovingBackground(t *testing.T) {
	cases := []struct {
		name     string
		style    *TopbarStyleData
		wantAxis string // "" means no moving strip
		wantGrad string
	}{
		{"horizontal", &TopbarStyleData{Mode: "moving", GradientFrom: "#0f172a", GradientTo: "#3b1d5e", GradientDir: "to-r"}, "x", "repeating-linear-gradient(90deg, #0f172a 0%, #3b1d5e 16.6667%, #0f172a 33.3333%)"},
		{"diagonal drifts sideways", &TopbarStyleData{Mode: "moving", GradientFrom: "#0f172a", GradientTo: "#3b1d5e", GradientDir: "to-br"}, "x", "90deg"},
		{"vertical", &TopbarStyleData{Mode: "moving", GradientFrom: "#0f172a", GradientTo: "#3b1d5e", GradientDir: "to-b"}, "y", "repeating-linear-gradient(180deg"},
		{"plain gradient is still", &TopbarStyleData{Mode: "gradient", GradientFrom: "#0f172a", GradientTo: "#3b1d5e"}, "", ""},
		{"moving without colours draws nothing", &TopbarStyleData{Mode: "moving"}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetCampaignID(context.Background(), "camp-1")
			ctx = SetTopbarStyle(ctx, tc.style)
			var buf bytes.Buffer
			if err := Topbar().Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			has := strings.Contains(html, `data-widget="header-motion"`)
			if (tc.wantAxis != "") != has {
				t.Fatalf("header-motion present = %v, want %v", has, tc.wantAxis != "")
			}
			if tc.wantAxis == "" {
				return
			}
			if !strings.Contains(html, `data-axis="`+tc.wantAxis+`"`) {
				t.Errorf("axis %q missing in %s", tc.wantAxis, html)
			}
			if !strings.Contains(html, tc.wantGrad) {
				t.Errorf("gradient %q missing", tc.wantGrad)
			}
			if !strings.Contains(html, "cz-hdr-light") {
				t.Error("two dark colours need light header words")
			}
		})
	}
}

func TestTopbarNewWidgets(t *testing.T) {
	const noteMarker = "Chronicle.openQuickCapture"
	const searchMarker = "Search Ashfall"
	render := func(authed bool, role int, widgets ...string) string {
		ctx := SetCampaignName(SetCampaignID(context.Background(), "camp-1"), "Ashfall")
		ctx = SetIsAuthenticated(ctx, authed)
		ctx = SetCampaignRole(ctx, role)
		ctx = SetTopbarContent(ctx, &TopbarContentData{Mode: "widgets", Widgets: widgets,
			Quote: "TEXT-MARKER", Links: []TopbarLinkData{{Label: "LINK-MARKER", URL: "/x"}}})
		var buf bytes.Buffer
		if err := Topbar().Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	t.Run("a member sees the note button and the search box", func(t *testing.T) {
		html := render(true, 1, "note", "search")
		for _, w := range []string{noteMarker, searchMarker, "Quick note"} {
			if !strings.Contains(html, w) {
				t.Errorf("missing %q", w)
			}
		}
	})
	t.Run("the search icon steps aside for the search box", func(t *testing.T) {
		with := render(true, 1, "search")
		without := render(true, 1, "note")
		iconTag := func(html string) string {
			i := strings.Index(html, `id="topbar-search-trigger"`)
			j := strings.Index(html[i:], ">")
			return html[i : i+j]
		}
		if !strings.Contains(iconTag(with), "md:hidden") {
			t.Errorf("the icon should hide at md+ beside a search box: %s", iconTag(with))
		}
		if strings.Contains(iconTag(without), "md:hidden") {
			t.Errorf("the icon stays without a search box: %s", iconTag(without))
		}
	})
	t.Run("a visitor gets no note button", func(t *testing.T) {
		html := render(false, 0, "note")
		if strings.Contains(html, noteMarker) || strings.Contains(html, "topbar-tray") {
			t.Errorf("a signed-out visitor must not be offered quick notes: %s", html)
		}
	})
	t.Run("a one-widget header has no tray", func(t *testing.T) {
		html := render(true, 1, "search")
		if strings.Contains(html, `id="topbar-tray"`) || strings.Contains(html, `x-ref="moreBtn"`) {
			t.Error("one widget needs no +N button or tray")
		}
	})
	t.Run("empty widgets don't count toward +N", func(t *testing.T) {
		ctx := SetCampaignID(context.Background(), "camp-1")
		ctx = SetTopbarContent(ctx, &TopbarContentData{Mode: "widgets", Widgets: []string{"links", "text", "search"}})
		var buf bytes.Buffer
		if err := Topbar().Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), `id="topbar-tray"`) {
			t.Error("links with no links and empty text draw nothing, so there is nothing to tray")
		}
	})
	t.Run("narrow screens: first widget stays, the rest go behind +N", func(t *testing.T) {
		html := render(true, 1, "links", "text", "note", "search")
		for _, w := range []string{`+3`, `id="topbar-tray"`, `aria-controls="topbar-tray"`, `:aria-expanded="more ? 'true' : 'false'"`,
			`@keydown.escape.window`, `$refs.moreBtn.focus()`, `@click.outside="more = false"`} {
			if !strings.Contains(html, w) {
				t.Errorf("missing %q", w)
			}
		}
		// The first widget is shown at every width; the others are wrapped to
		// appear from md up, and repeated once in the tray.
		if n := strings.Count(html, "hidden md:flex"); n != 3 {
			t.Errorf("%d desktop-only wrappers, want 3", n)
		}
		if n := strings.Count(html, "fa-pen"); n != 2 {
			t.Errorf("note appears %d times (bar + tray), want 2", n)
		}
		if n := strings.Count(html, "LINK-MARKER"); n != 1 {
			t.Errorf("the first widget appears %d times, want once", n)
		}
	})
}

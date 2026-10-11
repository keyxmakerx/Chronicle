package campaigns

import (
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// TestBuildCustomizeState pins how saved settings become the Customize
// page's starting draft: Classic fills every unset choice, the older
// accent and font slots carry over, and stored pictures get a URL the page
// can show.
func TestBuildCustomizeState(t *testing.T) {
	const pic = "2026/09/b7c17bb1-6563-462c-8b49-5b2e8bd57108.png"
	backdrop := "2026/08/aaaa.jpg"
	cases := []struct {
		name     string
		settings string
		backdrop *string
		check    func(t *testing.T, st customizeState)
	}{
		{"never customised is Classic", "", nil, func(t *testing.T, st customizeState) {
			d := st.Draft
			if d.Look != "classic" || d.Header.Bg != "solid" || d.Header.Solid != "page" || d.Colours.Accent != classicAccent {
				t.Errorf("want the Classic header and accent, got look=%q bg=%q solid=%q accent=%q", d.Look, d.Header.Bg, d.Header.Solid, d.Colours.Accent)
			}
			if d.Colours.S1 != nil || d.Colours.S2 != nil {
				t.Error("surface colours must follow the chrome accent (null) when unset")
			}
			if d.Type.Body != "inter" || d.Type.Heading != "same" || d.Nav.Style != "ring" || d.Buttons.Style != "lift" || d.Motion.Speed != "standard" {
				t.Errorf("unset choices must be Classic's, got %+v %+v", d.Type, d.Nav)
			}
			if d.Header.Widgets == nil || d.Header.Links == nil {
				t.Error("widgets and links must be empty lists, never null")
			}
		}},
		{"older slots carry over", `{"accent_color":"#3b82f6","accent_app":"#10b981","font_family":"serif","topbar_style":{"mode":"gradient","gradient_from":"#0f172a","gradient_to":"#3b1d5e","gradient_dir":"to-br"},"topbar_content":{"mode":"quote","quote":"Hi"}}`, nil, func(t *testing.T, st customizeState) {
			d := st.Draft
			if d.Colours.Accent != "#3b82f6" || d.Colours.S1 == nil || *d.Colours.S1 != "#10b981" {
				t.Errorf("accent and app colour must carry over, got %q %v", d.Colours.Accent, d.Colours.S1)
			}
			if d.Type.Body != "sourceserif" {
				t.Errorf("font_family serif must become sourceserif, got %q", d.Type.Body)
			}
			if d.Header.Bg != "gradient" || d.Header.Dir != "br" || d.Header.To != "#3b1d5e" {
				t.Errorf("gradient header must read back, got %+v", d.Header)
			}
			if strings.Join(d.Header.Widgets, ",") != "text" || d.Header.Text != "Hi" {
				t.Errorf("quote mode must read back as the text widget, got %v %q", d.Header.Widgets, d.Header.Text)
			}
		}},
		{"stored pictures get URLs", `{"brand_logo":"` + pic + `","topbar_style":{"mode":"image","image_path":"` + pic + `","scrim":"strong"}}`, &backdrop, func(t *testing.T, st customizeState) {
			d := st.Draft
			if d.Header.Bg != "image" || d.Header.Image != pic || d.Header.Scrim != "strong" || d.Brand.Logo != pic || d.Brand.Backdrop != backdrop {
				t.Errorf("pictures must read back by stored name, got %+v %+v", d.Header, d.Brand)
			}
			if u := st.Pictures[pic]; u != "/media/b7c17bb1-6563-462c-8b49-5b2e8bd57108" {
				t.Errorf("picture URL must go through MediaURL, got %q", u)
			}
			if st.Pictures[backdrop] == "" {
				t.Error("the backdrop needs a URL too")
			}
		}},
		{"saved choices win, unknown ones fall back", `{"appearance":{"look":"ember","nav_style":"comet","body_font":"nope","heading_font":"cinzel","reduce_motion":true}}`, nil, func(t *testing.T, st customizeState) {
			d := st.Draft
			if d.Look != "ember" || d.Nav.Style != "comet" || d.Type.Heading != "cinzel" || !d.Motion.ReduceAll {
				t.Errorf("saved choices must read back, got %+v", d)
			}
			if d.Type.Body != "inter" {
				t.Errorf("an unknown font must fall back to Classic's, got %q", d.Type.Body)
			}
		}},
		{"a broken header falls back to the page header", `{"topbar_style":{"mode":"gradient","gradient_from":"red"}}`, nil, func(t *testing.T, st customizeState) {
			if st.Draft.Header.Bg != "solid" || st.Draft.Header.Solid != "page" {
				t.Errorf("an invalid gradient must read as the default header, got %+v", st.Draft.Header)
			}
		}},
		{"never customised is a slim header and a charcoal menu", "", nil, func(t *testing.T, st customizeState) {
			d := st.Draft
			if d.Header.Height != "slim" {
				t.Errorf("height = %q, want slim", d.Header.Height)
			}
			if d.Colours.Sidebar != "charcoal" || d.Sidebar.Corner != "plain" || d.Sidebar.Glow != "accent" {
				t.Errorf("menu defaults wrong: %q %+v", d.Colours.Sidebar, d.Sidebar)
			}
			if d.Sidebar.Own != startMenuOwn || d.Sidebar.GlowColour != startGlowOwn || d.Sidebar.Banner != "" || d.Sidebar.Subtitle != "" {
				t.Errorf("menu starting values wrong: %+v", d.Sidebar)
			}
		}},
		{"saved menu and header choices read back", `{"appearance":{"header_height":"tall","sidebar_colour":"own","sidebar_own":"#123456","sidebar_corner":"banner","sidebar_banner":"` + pic + `","peek_glow":"own","peek_glow_colour":"#abcdef"},"topbar_style":{"mode":"moving","gradient_from":"#0f172a","gradient_to":"#3b1d5e"},"topbar_content":{"mode":"widgets","widgets":["note","search"]}}`, nil, func(t *testing.T, st customizeState) {
			d := st.Draft
			if d.Header.Height != "tall" || d.Header.Bg != "moving" || d.Header.From != "#0f172a" || d.Header.To != "#3b1d5e" {
				t.Errorf("header read back wrong: %+v", d.Header)
			}
			if strings.Join(d.Header.Widgets, ",") != "note,search" {
				t.Errorf("widgets = %v", d.Header.Widgets)
			}
			if d.Colours.Sidebar != "own" || d.Sidebar.Own != "#123456" || d.Sidebar.Corner != "banner" || d.Sidebar.Banner != pic ||
				d.Sidebar.Glow != "own" || d.Sidebar.GlowColour != "#abcdef" {
				t.Errorf("menu read back wrong: %q %+v", d.Colours.Sidebar, d.Sidebar)
			}
			if st.Pictures[pic] == "" {
				t.Error("the banner needs a URL the editor can show")
			}
		}},
		{"a sky header reads back", `{"topbar_style":{"mode":"sky"}}`, nil, func(t *testing.T, st customizeState) {
			if st.Draft.Header.Bg != "sky" {
				t.Errorf("want the sky header, got %q", st.Draft.Header.Bg)
			}
		}},
		{"a broken own colour falls back to the starting colour", `{"appearance":{"sidebar_colour":"own","sidebar_own":"red","peek_glow":"own","peek_glow_colour":"nope"}}`, nil, func(t *testing.T, st customizeState) {
			if st.Draft.Sidebar.Own != startMenuOwn || st.Draft.Sidebar.GlowColour != startGlowOwn {
				t.Errorf("invalid stored colours must not reach the editor: %+v", st.Draft.Sidebar)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Ashfall", Settings: c.settings, BackdropPath: c.backdrop}, MemberRole: RoleOwner}
			st := buildCustomizeState(context.Background(), cc)
			if st.Campaign != "Ashfall" {
				t.Errorf("campaign name = %q", st.Campaign)
			}
			c.check(t, st)
		})
	}
}

// TestLookTab_MountsWithState pins the editor's frame: the widget mount,
// its parseable starting state, and the picture input the script uses.
func TestLookTab_MountsWithState(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: `Ash "fall" <b>`}, MemberRole: RoleOwner}
	var sb strings.Builder
	if err := lookTab(cc, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render lookTab: %v", err)
	}
	out := sb.String()
	for _, want := range []string{`data-widget="customize-look"`, `data-campaign-id="camp-1"`, `id="file"`, `/static/css/customize.css`, `id="savebtn"`} {
		if !strings.Contains(out, want) {
			t.Errorf("lookTab missing %q", want)
		}
	}
	m := regexp.MustCompile(`data-state="([^"]*)"`).FindStringSubmatch(out)
	if m == nil {
		t.Fatal("no data-state attribute")
	}
	var st customizeState
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &st); err != nil {
		t.Fatalf("data-state is not JSON: %v", err)
	}
	if st.Campaign != `Ash "fall" <b>` || st.Draft.Look != "classic" {
		t.Errorf("data-state round trip lost data: %+v", st)
	}
	if !strings.Contains(out, `data-sky-calendar=""`) {
		t.Error("without a calendar the example sky has no calendar to draw")
	}
	sb.Reset()
	if err := lookTab(cc, "tok").Render(layouts.SetSkyCalendarID(context.Background(), "cal-9"), &sb); err != nil {
		t.Fatalf("render lookTab: %v", err)
	}
	if !strings.Contains(sb.String(), `data-sky-calendar="cal-9"`) {
		t.Error("the example sky must be given the campaign's calendar")
	}
}

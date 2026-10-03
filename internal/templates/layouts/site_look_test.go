package layouts

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

func renderToString(t *testing.T, ctx context.Context, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func plainBody() templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := w.Write([]byte("<p>body</p>"))
		return err
	})
}

func TestSiteTitleAndFavicon(t *testing.T) {
	tests := []struct {
		name        string
		look        sitelook.Settings
		wantTitle   string
		wantFavicon string // substring of the icon link
		notFavicon  string
	}{
		{"never saved", sitelook.Settings{}, "<title>Discover | Chronicle</title>", `/static/img/favicon.svg`, "/media/"},
		{"renamed", sitelook.Settings{Configured: true, Name: "Dragon Hold"}, "<title>Discover | Dragon Hold</title>", `/static/img/favicon.svg`, "/media/"},
		{"name is escaped", sitelook.Settings{Configured: true, Name: `A<b>"&`}, "<title>Discover | A&lt;b&gt;&#34;&amp;</title>", `/static/img/favicon.svg`, ""},
		{"logo as icon", sitelook.Settings{Configured: true, Logo: "2026/10/a.png", LogoAsFavicon: true},
			"<title>Discover | Chronicle</title>", `type="image/png" href="/media/a"`, `/static/img/favicon.svg`},
		{"webp logo type", sitelook.Settings{Configured: true, Logo: "2026/10/a.webp", LogoAsFavicon: true},
			"", `type="image/webp"`, `/static/img/favicon.svg`},
		{"box unticked keeps the shipped icon", sitelook.Settings{Configured: true, Logo: "2026/10/a.png"},
			"", `/static/img/favicon.svg`, "/media/a"},
		{"ticked but no logo keeps the shipped icon", sitelook.Settings{Configured: true, LogoAsFavicon: true},
			"", `/static/img/favicon.svg`, "/media/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetSiteLook(context.Background(), tc.look)
			out := renderToString(t, ctx, Base("Discover"))
			head := out[:strings.Index(out, "<!-- Theme")]
			if tc.wantTitle != "" && !strings.Contains(head, tc.wantTitle) {
				t.Errorf("head missing %q:\n%s", tc.wantTitle, head)
			}
			if !strings.Contains(head, tc.wantFavicon) {
				t.Errorf("head missing icon %q:\n%s", tc.wantFavicon, head)
			}
			if tc.notFavicon != "" && strings.Contains(head, tc.notFavicon) {
				t.Errorf("head wrongly contains %q:\n%s", tc.notFavicon, head)
			}
		})
	}
}

func TestSiteAuthStage(t *testing.T) {
	tests := []struct {
		name       string
		look       sitelook.Settings
		wantStyle  string
		wantNoText []string
		wantPanel  bool
	}{
		{"never saved is plain", sitelook.Settings{}, "", []string{"style="}, false},
		{"plain background", sitelook.Settings{Configured: true, Background: sitelook.BackgroundPlain, Look: "ember"}, "", []string{"style="}, false},
		{"look colours", sitelook.Settings{Configured: true, Look: "ember", Background: sitelook.BackgroundLook},
			`background:linear-gradient(135deg, #1f2937, #4a1512);`, nil, true},
		{"look background without a look falls back to plain", sitelook.Settings{Configured: true, Background: sitelook.BackgroundLook}, "", []string{"style="}, false},
		{"picture", sitelook.Settings{Configured: true, Background: sitelook.BackgroundPicture, Picture: "2026/10/bg.jpg"},
			`<img src="/media/bg" alt="" aria-hidden="true" class="absolute inset-0 w-full h-full object-cover">`, nil, true},
		{"picture background without a picture falls back to plain", sitelook.Settings{Configured: true, Background: sitelook.BackgroundPicture}, "", []string{"style="}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetSiteLook(context.Background(), tc.look)
			out := renderToString(t, templ.WithChildren(ctx, plainBody()), SiteAuthStage(""))
			if !strings.Contains(out, "<p>body</p>") {
				t.Fatalf("stage lost its children: %s", out)
			}
			if tc.wantStyle != "" && !strings.Contains(out, tc.wantStyle) {
				t.Errorf("missing %q in %s", tc.wantStyle, out)
			}
			for _, n := range tc.wantNoText {
				if strings.Contains(out, n) {
					t.Errorf("unexpected %q in %s", n, out)
				}
			}
			if panel := strings.Contains(out, "rounded-2xl"); panel != tc.wantPanel {
				t.Errorf("panel = %v, want %v", panel, tc.wantPanel)
			}
			if tc.wantStyle == "" != strings.Contains(out, "bg-surface px-4") {
				t.Errorf("plain stage should keep the shipped classes: %s", out)
			}
		})
	}
}

func TestSiteAuthMark(t *testing.T) {
	tests := []struct {
		name string
		look sitelook.Settings
		want []string
		not  []string
	}{
		{"never saved keeps the book icon", sitelook.Settings{}, []string{"fa-book-open"}, []string{"<img", "bg-accent text-white"}},
		{"letter on accent", sitelook.Settings{Configured: true, Name: "dragon hold"}, []string{">D<", `aria-label="dragon hold"`}, []string{"fa-book-open", "<img"}},
		{"default name letter", sitelook.Settings{Configured: true}, []string{">C<"}, []string{"fa-book-open"}},
		{"logo", sitelook.Settings{Configured: true, Logo: "2026/10/a.png"}, []string{`<img src="/media/a"`}, []string{"fa-book-open"}},
		{"name is escaped", sitelook.Settings{Configured: true, Name: `<x>`}, []string{`aria-label="&lt;x&gt;"`}, []string{"<x>"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := renderToString(t, SetSiteLook(context.Background(), tc.look), SiteAuthMark())
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(out, n) {
					t.Errorf("unexpected %q in %s", n, out)
				}
			}
		})
	}
}

func TestSiteWelcomeLineEscapes(t *testing.T) {
	ctx := SetSiteLook(context.Background(), sitelook.Settings{Configured: true, Welcome: `<script>alert(1)</script> "hi"`})
	out := renderToString(t, ctx, SiteWelcomeLine())
	if strings.Contains(out, "<script>") || !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("welcome not escaped: %s", out)
	}
	if out := renderToString(t, SetSiteLook(context.Background(), sitelook.Settings{Configured: true}), SiteWelcomeLine()); out != "" {
		t.Errorf("no welcome should render nothing, got %q", out)
	}
}

// TestApplySiteLook covers the one place a page outside a campaign borrows the
// look, and that a campaign page is left exactly as its own settings made it.
func TestApplySiteLook(t *testing.T) {
	ember := sitelook.Settings{Configured: true, Look: "ember"}
	tests := []struct {
		name         string
		look         sitelook.Settings
		campaign     string
		wantAccent   string
		wantTopbar   bool
		wantHeading  string
		wantSameCtx  bool
		wantCampaign string
	}{
		{name: "outside a campaign", look: ember, wantAccent: "#c2410c", wantTopbar: true, wantHeading: "cinzel"},
		{name: "classic has no heading font", look: sitelook.Settings{Configured: true, Look: "classic"}, wantAccent: "#6366f1", wantTopbar: true},
		{name: "no look chosen changes nothing", look: sitelook.Settings{Configured: true}, wantSameCtx: true},
		{name: "never saved changes nothing", look: sitelook.Settings{}, wantSameCtx: true},
		{name: "inside a campaign changes nothing", look: ember, campaign: "c1", wantSameCtx: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetSiteLook(context.Background(), tc.look)
			if tc.campaign != "" {
				ctx = SetCampaignID(ctx, tc.campaign)
			}
			got := ApplySiteLook(ctx)
			if got != ctx != !tc.wantSameCtx {
				t.Errorf("context replaced = %v, want %v", got != ctx, !tc.wantSameCtx)
			}
			if GetAccentColor(got) != tc.wantAccent {
				t.Errorf("accent = %q, want %q", GetAccentColor(got), tc.wantAccent)
			}
			if (GetTopbarStyle(got) != nil) != tc.wantTopbar {
				t.Errorf("topbar style set = %v, want %v", GetTopbarStyle(got) != nil, tc.wantTopbar)
			}
			heading := ""
			if a := GetAppearance(got); a != nil {
				heading = a.HeadingFont
			}
			if heading != tc.wantHeading {
				t.Errorf("heading font = %q, want %q", heading, tc.wantHeading)
			}
		})
	}
}

// TestCampaignPageKeepsItsOwnLook renders the app layout for a campaign page
// with a site look saved and checks the campaign's own accent and top bar are
// what reach the page, with none of the site look's colours.
func TestCampaignPageKeepsItsOwnLook(t *testing.T) {
	ctx := SetSiteLook(context.Background(), sitelook.Settings{Configured: true, Look: "ember", Name: "Hold"})
	ctx = SetCampaignID(ctx, "c1")
	ctx = SetCampaignName(ctx, "My Campaign")
	ctx = SetAccentColor(ctx, "#123456")
	ctx = SetTopbarStyle(ctx, &TopbarStyleData{Mode: "solid", Color: "#abcdef"})
	ctx = ApplySiteLook(ctx)
	out := renderToString(t, ctx, App("Dashboard"))
	for _, want := range []string{"#123456", "background-color: #abcdef;", "<title>Dashboard | Hold</title>"} {
		if !strings.Contains(out, want) {
			t.Errorf("campaign page missing %q", want)
		}
	}
	for _, not := range []string{"#c2410c", "#1f2937", "#4a1512", "data-cz-heading"} {
		if strings.Contains(out, not) {
			t.Errorf("campaign page picked up the site look's %q", not)
		}
	}
}

// TestOutsideCampaignPageTakesSiteLook is the other half: the same layout
// outside a campaign carries the site's accent, top bar and heading font, and
// the brand in the default sidebar.
func TestOutsideCampaignPageTakesSiteLook(t *testing.T) {
	ctx := SetSiteLook(context.Background(), sitelook.Settings{Configured: true, Look: "ember", Name: "Dragon Hold"})
	ctx = ApplySiteLook(ctx)
	out := renderToString(t, ctx, App("Discover"))
	for _, want := range []string{"#c2410c", "linear-gradient(to right, #1f2937, #4a1512)", `data-cz-heading="cinzel"`, "Dragon Hold", "<title>Discover | Dragon Hold</title>"} {
		if !strings.Contains(out, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

// TestUnsavedSiteLookLeavesPagesAsShipped pins "until the admin saves,
// everything looks exactly as today" for the shared layout.
func TestUnsavedSiteLookLeavesPagesAsShipped(t *testing.T) {
	ctx := ApplySiteLook(SetSiteLook(context.Background(), sitelook.Settings{}))
	out := renderToString(t, ctx, App("Discover"))
	for _, want := range []string{"<title>Discover | Chronicle</title>", "/static/img/favicon.svg", ">Chronicle</span>"} {
		if !strings.Contains(out, want) {
			t.Errorf("page missing %q", want)
		}
	}
	for _, not := range []string{"data-cz-heading", "linear-gradient(to right", "bg-accent text-white text-xs"} {
		if strings.Contains(out, not) {
			t.Errorf("unsaved site look changed the page: found %q", not)
		}
	}
}

// TestSiteAuthStage_SignedPictureURL checks a signed media URL (query string
// with & and =) reaches the page once-escaped, so the browser requests the
// real signature.
func TestSiteAuthStage_SignedPictureURL(t *testing.T) {
	ctx := SetSiteLook(context.Background(), sitelook.Settings{Configured: true, Background: sitelook.BackgroundPicture, Picture: "2026/10/bg.jpg"})
	ctx = SetMediaURLFunc(ctx, func(id string) string { return "/media/" + id + "?expires=1700000000&sig=ab12-_" })
	out := renderToString(t, templ.WithChildren(ctx, plainBody()), SiteAuthStage(""))
	if !strings.Contains(out, `<img src="/media/bg?expires=1700000000&amp;sig=ab12-_"`) {
		t.Errorf("signed URL mangled: %s", out)
	}
}

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func renderAuthPage(t *testing.T, look sitelook.Settings, c templ.Component) string {
	t.Helper()
	ctx := layouts.ApplySiteLook(layouts.SetSiteLook(context.Background(), look))
	var b bytes.Buffer
	if err := c.Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func panelMarker(doors bool) string {
	if doors {
		return "doors-panel"
	}
	return "rounded-2xl"
}

// unsavedMarker is what a site that never saved a look renders: the built-in
// scenes on the doors pages, the page colour elsewhere.
func unsavedMarker(doors bool) string {
	if doors {
		return `data-mode="scenes"`
	}
	return "min-h-screen flex items-center justify-center bg-surface px-4"
}

// TestAuthPagesCarryTheSiteBrand renders all four pages that share the
// sign-in stage, saved and unsaved.
func TestAuthPagesCarryTheSiteBrand(t *testing.T) {
	pages := []struct {
		name string
		page templ.Component
		// doors pages sit on the three-door stage, the others on the plain stage.
		doors bool
	}{
		{"login", LoginPage("tok", "", "", "", "", LoginOptions{}), true},
		{"register", RegisterPage("tok", &RegisterRequest{}, "", "", false, "open"), true},
		{"forgot password", ForgotPasswordPage("tok", "", "", true), false},
		{"reset password", ResetPasswordPage("tok", "t", "", ""), false},
	}
	saved := sitelook.Settings{Configured: true, Name: "Dragon Hold", Look: "ember", Background: sitelook.BackgroundLook}
	for _, p := range pages {
		t.Run(p.name+"/saved", func(t *testing.T) {
			out := renderAuthPage(t, saved, p.page)
			for _, want := range []string{
				"<title>", "| Dragon Hold</title>", `aria-label="Dragon Hold"`, ">D<",
				"linear-gradient(135deg, #1f2937, #4a1512)", panelMarker(p.doors),
			} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q", want)
				}
			}
			if strings.Contains(out, "fa-book-open") {
				t.Errorf("still shows Chronicle's book icon")
			}
		})
		t.Run(p.name+"/unsaved is as shipped", func(t *testing.T) {
			out := renderAuthPage(t, sitelook.Settings{}, p.page)
			for _, want := range []string{"| Chronicle</title>", "fa-book-open", unsavedMarker(p.doors)} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q", want)
				}
			}
			for _, not := range []string{"rounded-2xl bg-surface shadow-2xl", "linear-gradient(135deg", "data-site-welcome"} {
				if strings.Contains(out, not) {
					t.Errorf("unsaved site changed the page: %q", not)
				}
			}
		})
	}
}

func TestLoginPageSiteLook(t *testing.T) {
	tests := []struct {
		name string
		look sitelook.Settings
		want []string
		not  []string
	}{
		{"name in the sentence", sitelook.Settings{Configured: true, Name: "Dragon Hold"}, []string{"Sign in to your Dragon Hold account"}, nil},
		{"unsaved sentence", sitelook.Settings{}, []string{"Sign in to your Chronicle account"}, nil},
		{"welcome line", sitelook.Settings{Configured: true, Welcome: "Roll for initiative"}, []string{`data-site-welcome>Roll for initiative</p>`}, nil},
		{"welcome is escaped", sitelook.Settings{Configured: true, Welcome: `<img src=x onerror=alert(1)>`},
			[]string{`&lt;img src=x onerror=alert(1)&gt;`}, []string{"<img src=x"}},
		{"name is escaped", sitelook.Settings{Configured: true, Name: `<script>x</script>`}, []string{`&lt;script&gt;x&lt;/script&gt;`}, []string{"<script>x"}},
		{"picture background", sitelook.Settings{Configured: true, Background: sitelook.BackgroundPicture, Picture: "2026/10/bg.jpg"},
			[]string{`data-mode="picture"`, `<img src="/media/bg" alt="">`}, []string{"doors-set"}},
		{"plain background keeps the page colour", sitelook.Settings{Configured: true, Look: "ember", Background: sitelook.BackgroundPlain},
			[]string{`data-mode="plain"`}, []string{"linear-gradient(135deg", "doors-set"}},
		{"never saved shows the scenes", sitelook.Settings{},
			[]string{`data-mode="scenes"`, "castle-1-back", "dragon-4-action", "tavern-2-mid"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := renderAuthPage(t, tc.look, LoginPage("tok", "", "", "", "", LoginOptions{}))
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(out, n) {
					t.Errorf("unexpected %q", n)
				}
			}
		})
	}
}

// TestSiteLookAccentOnAuthPages checks the look's accent reaches the sign-in
// pages (they are outside a campaign) through the layout's accent CSS.
func TestSiteLookAccentOnAuthPages(t *testing.T) {
	out := renderAuthPage(t, sitelook.Settings{Configured: true, Look: "forest"}, LoginPage("tok", "", "", "", "", LoginOptions{}))
	if !strings.Contains(out, "--color-accent:#2f7d4f") {
		t.Errorf("accent of the forest look missing from the page head")
	}
}

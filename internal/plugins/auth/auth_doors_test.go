// auth_doors_test.go pins the three-door sign-in page: each URL opens its own
// tab, the tabs keep the redirect, and the scene follows the open door.

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderDoor(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestDoorsOpenTheirOwnTab(t *testing.T) {
	tests := []struct {
		name string
		page templ.Component
		door string
	}{
		{"login", LoginPage("tok", "", "", "", "", LoginOptions{}), "signin"},
		{"register", RegisterPage("tok", nil, "", "", false, "open"), "new"},
		{"register gated", RegisterPage("tok", nil, "", "", true, "invite"), "new"},
		{"join", JoinPage("tok", "", "", "", ""), "code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := renderDoor(t, tt.page)
			if !strings.Contains(html, `id="doors-stage" class="doors" data-door="`+tt.door+`"`) {
				t.Errorf("stage should open on %q", tt.door)
			}
			if !strings.Contains(html, `id="doors-tabs" class="doors-tabs" role="tablist" aria-label="Ways in" data-active="`+tt.door+`"`) {
				t.Errorf("tab row should mark %q open", tt.door)
			}
			if n := strings.Count(html, `aria-selected="true"`); n != 1 {
				t.Errorf("exactly one tab is selected, got %d", n)
			}
			for _, label := range []string{">Sign in<", ">I&#39;m new<", ">I have a code<"} {
				if !strings.Contains(html, label) {
					t.Errorf("missing tab %q", label)
				}
			}
			if strings.Contains(html, `class="card p-8`) {
				t.Errorf("the form must not sit in a box")
			}
		})
	}
}

func TestDoorTabsKeepTheRedirect(t *testing.T) {
	html := renderDoor(t, LoginPage("tok", "", "", "", "/join/abc123", LoginOptions{}))
	for _, want := range []string{
		`href="/login?redirect=%2Fjoin%2Fabc123"`,
		`href="/register?redirect=%2Fjoin%2Fabc123"`,
		`href="/join"`,
		`hx-select="#doors-panel"`, `hx-select-oob="#doors-tabs"`, `hx-push-url="true"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestJoinDoorCarriesTheJoinRedirect(t *testing.T) {
	html := renderDoor(t, JoinPage("tok", "", "", "", ""))
	if !strings.Contains(html, `href="/login?redirect=%2Fjoin"`) || !strings.Contains(html, `href="/register?redirect=%2Fjoin"`) {
		t.Errorf("signing in from the code tab should come back to /join: %s", html)
	}
}

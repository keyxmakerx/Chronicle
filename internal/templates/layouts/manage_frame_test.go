package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// Only the owner manages a campaign: a Scribe, a Player, an owner viewing as
// a player (role reads Player) and a visitor all get no Manage rows.
func TestNavManageRows_OwnerOnly(t *testing.T) {
	tests := []struct {
		name string
		c    navStateCase
		want []string
	}{
		{"owner", navStateCase{role: 3, path: "/campaigns/c1"}, []string{"Overview", "People", "What players changed", "Customize", "Game & features", "Your AI", "Trash", "Settings"}},
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1"}, nil},
		{"player", navStateCase{role: 1, path: "/campaigns/c1"}, nil},
		{"owner viewing as player", navStateCase{role: 1, player: true, path: "/campaigns/c1"}, nil},
		{"visitor", navStateCase{role: 0, path: "/campaigns/c1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, r := range NavManageRows(tt.c.ctx()) {
				got = append(got, r.Label)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("rows = %v, want %v", got, tt.want)
			}
		})
	}
}

// The Foundry page joins the owner's Manage rows only when the campaign syncs.
func TestNavManageRows_FoundryNeedsSyncAPI(t *testing.T) {
	ctx := SetEnabledAddons(navStateCase{role: 3, path: "/campaigns/c1"}.ctx(), map[string]bool{"sync-api": true})
	var got []string
	for _, r := range NavManageRows(ctx) {
		got = append(got, r.Label)
	}
	want := "Overview,People,What players changed,Customize,Game & features,Foundry,Your AI,Trash,Settings"
	if strings.Join(got, ",") != want {
		t.Fatalf("rows = %v, want %s", got, want)
	}
}

// The sidebar is the only Manage menu: the frame is a title and one quiet way
// back to the desk, never a second row of the same links.
func TestManageFrame_TitleAndWayBack(t *testing.T) {
	tests := []struct {
		name     string
		data     ManageFrameData
		wantBack bool
	}{
		{"a page leads back to the desk", ManageFrameData{Title: "People"}, true},
		{"the desk has no word leading to itself", ManageFrameData{Title: "Manage", Home: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := navStateCase{role: 3, path: "/campaigns/c1/members"}.ctx()
			if err := ManageFrame(tt.data).Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, want := range []string{"<h1", tt.data.Title, "data-manage-frame"} {
				if !strings.Contains(out, want) {
					t.Errorf("frame is missing %q: %s", want, out)
				}
			}
			if got := strings.Contains(out, `href="/campaigns/c1/dashboard"`); got != tt.wantBack {
				t.Errorf("way back to the desk = %v, want %v: %s", got, tt.wantBack, out)
			}
			if strings.Contains(out, "<nav") || strings.Count(out, "href=") > 1 {
				t.Errorf("frame repeats the Manage links: %s", out)
			}
		})
	}
}

// A page that names its desk line draws that line's sentence under the title,
// from whatever the app registered; a page with no line, or a resolver that
// has nothing true to say, draws none, and the frame never fails for it.
func TestManageFrame_Sentence(t *testing.T) {
	t.Cleanup(func() { SetManageSentence(nil) })
	say := func(words string) ManageSentenceFunc {
		return func(_ context.Context, line string) templ.Component {
			if words == "" {
				return nil
			}
			return templ.Raw(`<div class="manage-sentence" data-line="` + line + `">` + words + `</div>`)
		}
	}
	tests := []struct {
		name string
		fn   ManageSentenceFunc
		line string
		want string
	}{
		{"line and resolver", say("4 people."), "people", "4 people."},
		{"no line", say("4 people."), "", ""},
		{"nothing to say", say(""), "people", ""},
		{"no resolver", nil, "people", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetManageSentence(tt.fn)
			var buf bytes.Buffer
			ctx := navStateCase{role: 3, path: "/campaigns/c1/members"}.ctx()
			if err := ManageFrame(ManageFrameData{Title: "People", Line: tt.line}).Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if got := strings.Contains(out, tt.want); tt.want != "" && !got {
				t.Errorf("missing %q: %s", tt.want, out)
			}
			if tt.want == "" && strings.Contains(out, "manage-sentence") {
				t.Errorf("no sentence expected: %s", out)
			}
			// The sentence sits after the title, outside the heading band.
			if tt.want != "" && strings.Index(out, tt.want) < strings.Index(out, "</header>") {
				t.Errorf("sentence must come after the header: %s", out)
			}
		})
	}
}

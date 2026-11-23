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
		{"owner", navStateCase{role: 3, path: "/campaigns/c1"}, []string{"Campaign settings", "People", "What players changed", "Customize", "Game & features", "Rolling tables", "Own entries", "Your AI", "Trash", "Campaign details", "Connections", "Data"}},
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1"}, nil},
		{"player", navStateCase{role: 1, path: "/campaigns/c1"}, nil},
		{"owner viewing as player", navStateCase{role: 1, player: true, path: "/campaigns/c1"}, nil},
		{"visitor", navStateCase{role: 0, path: "/campaigns/c1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, r := range NavManagePages(tt.c.ctx()) {
				got = append(got, r.Label)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("pages = %v, want %v", got, tt.want)
			}
			// The sidebar shows one row for all of them, or none.
			rows := NavManageRows(tt.c.ctx())
			if (len(tt.want) == 0) != (len(rows) == 0) || len(rows) > 1 {
				t.Fatalf("sidebar rows = %d for %d pages", len(rows), len(tt.want))
			}
			if len(rows) == 1 && (rows[0].Key != NavManageKey || rows[0].URL != "/campaigns/c1/dashboard") {
				t.Fatalf("the one row = %+v", rows[0])
			}
		})
	}
}

// The Foundry page joins the owner's Manage rows only when the campaign syncs.
func TestNavManageRows_FoundryNeedsSyncAPI(t *testing.T) {
	ctx := SetEnabledAddons(navStateCase{role: 3, path: "/campaigns/c1"}.ctx(), map[string]bool{"sync-api": true})
	var got []string
	for _, r := range NavManagePages(ctx) {
		got = append(got, r.Label)
	}
	want := "Campaign settings,People,What players changed,Customize,Game & features,Rolling tables,Own entries,Foundry,Your AI,Trash,Campaign details,Connections,Data"
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

// A page that names its desk line draws that line's figures beside the title,
// from whatever the app registered; a page with no line, a page that opts out,
// or a resolver that has nothing true to show, draws none, and the frame never
// fails for it.
func TestManageFrame_Figure(t *testing.T) {
	t.Cleanup(func() { SetManageFigure(nil) })
	show := func(words string) ManageFigureFunc {
		return func(_ context.Context, line string) templ.Component {
			if words == "" {
				return nil
			}
			return templ.Raw(`<span id="manage-figure" class="manage-fig" data-line="` + line + `">` + words + `</span>`)
		}
	}
	tests := []struct {
		name     string
		fn       ManageFigureFunc
		line     string
		noFigure bool
		want     string
	}{
		{"line and resolver", show("5 members"), "people", false, "5 members"},
		{"no line", show("5 members"), "", false, ""},
		{"nothing to show", show(""), "people", false, ""},
		{"no resolver", nil, "people", false, ""},
		{"page opts out", show("5 members"), "people", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetManageFigure(tt.fn)
			var buf bytes.Buffer
			ctx := navStateCase{role: 3, path: "/campaigns/c1/members"}.ctx()
			if err := ManageFrame(ManageFrameData{Title: "People", Line: tt.line, NoFigure: tt.noFigure}).Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if tt.want != "" && !strings.Contains(out, tt.want) {
				t.Errorf("missing %q: %s", tt.want, out)
			}
			if tt.want == "" && strings.Contains(out, "manage-fig") {
				t.Errorf("no figure expected: %s", out)
			}
			if strings.Contains(out, "manage-sentence") {
				t.Errorf("the sentence under the title is gone: %s", out)
			}
			if tt.want != "" {
				// The figure sits inside the header, after the title and
				// before the header closes, so it shares the title's line.
				at, h1, end := strings.Index(out, tt.want), strings.Index(out, "</h1>"), strings.Index(out, "</header>")
				if at < h1 || at > end {
					t.Errorf("figure must sit beside the title inside the header: %s", out)
				}
			}
		})
	}
}

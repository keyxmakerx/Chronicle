package layouts

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

// The tab a page lights: its own, else its line's, else Overview for the
// board; a page under no tab lights none.
func TestManageTabFor(t *testing.T) {
	tests := []struct {
		name string
		d    ManageFrameData
		want string
	}{
		{"the board", ManageFrameData{Home: true}, "overview"},
		{"a page names its tab", ManageFrameData{Tab: "connections", Line: "ai"}, "connections"},
		{"campaign by its line", ManageFrameData{Line: "settings"}, "campaign"},
		{"a tool page by its line", ManageFrameData{Line: "changes"}, "people"},
		{"rolling tables sit under Game", ManageFrameData{Line: "tables"}, "game"},
		{"your AI sits under Connections", ManageFrameData{Line: "ai"}, "connections"},
		{"the Trash sits under Data", ManageFrameData{Line: "trash"}, "data"},
		{"no tab", ManageFrameData{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manageTabFor(tt.d); got != tt.want {
				t.Errorf("tab = %q, want %q", got, tt.want)
			}
		})
	}
	keys := map[string]bool{}
	for _, tab := range ManageTabs() {
		keys[tab.Key] = true
	}
	for line, tab := range manageTabOfLine {
		if !keys[tab] {
			t.Errorf("line %q lights %q, which is not a tab", line, tab)
		}
	}
	if !manageLanding(ManageFrameData{Home: true}) || !manageLanding(ManageFrameData{Landing: true}) || manageLanding(ManageFrameData{Line: "trash"}) {
		t.Error("only the board and a tab's own page keep their title for the screen reader")
	}
}

// The owner gets the banner and the six tabs, the page's tab lit and Find in
// the banner; anyone else, an owner previewing as a player included, and a
// resolver with nothing to show, get the plain title.
func TestManageFrame_BannerAndTabs(t *testing.T) {
	t.Cleanup(func() { SetManageMast(nil) })
	mast := &ManageMast{Name: "Saltmarsh", Archived: true, Facts: []string{"Draw Steel", "5 members"}}
	render := func(ctx context.Context, d ManageFrameData) string {
		var buf bytes.Buffer
		if err := ManageFrame(d).Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	SetManageMast(func(context.Context) *ManageMast { return mast })
	owner := navStateCase{role: 3, path: "/campaigns/c1/members"}.ctx()

	out := render(owner, ManageFrameData{Title: "People", Line: "people", Landing: true})
	for _, want := range []string{
		`data-settings-mast`, `>Saltmarsh<`, `data-settings-access`, `Private`, `Archived`, `>Draw Steel<`, `>5 members<`,
		`<nav class="stg-bar" aria-label="Campaign settings">`, `href="/campaigns/c1/dashboard">Campaign settings</a>`,
		`data-settings-tab="people"`, `<h1 class="manage-title sr-only">People</h1>`, `manage-find-field`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner frame is missing %q", want)
		}
	}
	for _, tab := range ManageTabs() {
		if !strings.Contains(out, `href="/campaigns/c1`+tab.Path+`"`) {
			t.Errorf("tab %q does not lead to %s", tab.Label, tab.Path)
		}
	}
	lit := regexp.MustCompile(`data-settings-tab-link="([a-z]+)"[^>]*aria-current="page"`).FindAllStringSubmatch(out, -1)
	if len(lit) != 1 || lit[0][1] != "people" {
		t.Errorf("lit tabs = %v, want only people", lit)
	}
	if n := len(regexp.MustCompile(`\sdata-manage-find[\s>=]`).FindAllString(out, -1)); n != 1 {
		t.Errorf("the frame draws one Find, got %d", n)
	}

	board := render(owner, ManageFrameData{Title: "Campaign settings", Home: true})
	if !strings.Contains(board, `<p class="stg-eyebrow">Campaign settings</p>`) || strings.Contains(board, `class="stg-eyebrow" href=`) {
		t.Error("on the board the eyebrow names the place and leads nowhere")
	}
	if !regexp.MustCompile(`data-settings-tab-link="overview"[^>]*aria-current="page"`).MatchString(board) {
		t.Error("the board lights Overview")
	}

	page := render(owner, ManageFrameData{Title: "Trash", Line: "trash"})
	if !strings.Contains(page, `<h1 class="manage-title cb-title">Trash</h1>`) {
		t.Error("a tool page under a tab keeps its visible title")
	}

	for name, ctx := range map[string]context.Context{
		"player":                  navStateCase{role: 1, path: "/campaigns/c1"}.ctx(),
		"scribe":                  navStateCase{role: 2, path: "/campaigns/c1"}.ctx(),
		"owner viewing as player": navStateCase{role: 1, player: true, path: "/campaigns/c1"}.ctx(),
	} {
		if out := render(ctx, ManageFrameData{Title: "People", Line: "people"}); strings.Contains(out, "data-settings-mast") || strings.Contains(out, "stg-bar") {
			t.Errorf("%s got the owner's banner or tabs", name)
		}
	}
	SetManageMast(func(context.Context) *ManageMast { return nil })
	if out := render(owner, ManageFrameData{Title: "People", Line: "people"}); strings.Contains(out, "stg-bar") || !strings.Contains(out, `<h1 class="manage-title cb-title">People</h1>`) {
		t.Error("with nothing to draw the frame falls back to its plain title")
	}
}

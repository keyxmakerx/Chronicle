package layouts

import (
	"strings"
	"testing"
)

// nav_tools_word_test.go pins the touch-screen Edit word: a member who pins
// rows gets exactly one, on the first heading with a row whose pin or dots
// wait behind it; the owner, who arranges the sidebar in its editor, and a
// visitor get none.

func TestNavToolsWordSection(t *testing.T) {
	personalMaps := NavRowView{Key: "app:maps", Kind: "app", Label: "Maps", URL: "/campaigns/c1/maps", Personal: true}
	personalCat := NavRowView{Key: "cat:1", Kind: "category", Label: "Locations", URL: "/campaigns/c1/locations", TypeID: 1, Personal: true}
	cases := []struct {
		name     string
		role     int
		player   bool
		archived bool
		secs     []NavSectionView // nil keeps the shared fixture
		want     string
	}{
		{name: "player", role: 1, want: "apps"},
		{name: "scribe", role: 2, want: "apps"},
		{name: "owner arranges in the editor", role: 3, want: ""},
		{name: "owner viewing as a player", role: 3, player: true, want: ""},
		{name: "visitor", role: 0, want: ""},
		{name: "archived campaign", role: 1, archived: true, want: ""},
		{name: "a row they pinned has dots that wait", role: 1, secs: []NavSectionView{
			{ID: "pinned", Kind: "pinned", Label: "Pinned", Rows: []NavRowView{personalMaps}},
			{ID: "apps", Kind: "apps", Label: "Apps", Rows: []NavRowView{{Key: "app:notes", Kind: "app", Label: "Journal"}}},
		}, want: "pinned"},
		{name: "a category they pinned keeps its pin in sight", role: 1, secs: []NavSectionView{
			{ID: "pinned", Kind: "pinned", Label: "Pinned", Rows: []NavRowView{personalCat}},
			{ID: "categories", Kind: "categories", Label: "Categories", Rows: []NavRowView{{Key: "cat:2", Kind: "category", Label: "Factions", TypeID: 2}}},
		}, want: "categories"},
		{name: "the top block comes first whatever the order", role: 1, secs: []NavSectionView{
			{ID: "categories", Kind: "categories", Label: "Categories", Rows: []NavRowView{{Key: "cat:2", Kind: "category", Label: "Factions", TypeID: 2}}},
			{ID: "apps", Kind: "apps", Label: "Apps", Rows: []NavRowView{{Key: "app:notes", Kind: "app", Label: "Journal"}}},
		}, want: "apps"},
		{name: "nothing to pin", role: 1, secs: []NavSectionView{
			{ID: "pinned", Kind: "pinned", Label: "Pinned", Rows: []NavRowView{{Key: "app:notes", Kind: "app", Label: "Journal"}}},
		}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := navStateCase{role: tc.role, player: tc.player, archived: tc.archived, path: "/campaigns/c1"}.ctx()
			if tc.secs != nil {
				ctx = SetNavSections(ctx, tc.secs)
			}
			if got := NavToolsWordSection(ctx); got != tc.want {
				t.Errorf("NavToolsWordSection = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCampaignNav_ToolsWordDrawnOnce(t *testing.T) {
	player := renderNavList(t, navStateCase{role: 1, path: "/campaigns/c1"}.ctx())
	if got := strings.Count(player, "data-nav-tools-toggle"); got != 1 {
		t.Fatalf("a player's sidebar has %d Edit words, want 1", got)
	}
	apps := player[strings.Index(player, `data-nav-section="apps"`):]
	apps = apps[:strings.Index(apps, `id="nav-b-apps"`)]
	for _, want := range []string{"has-tools-word", "data-nav-tools-toggle", `aria-pressed="false"`, ">Edit<", ">Done<"} {
		if !strings.Contains(apps, want) {
			t.Errorf("the Apps heading is missing %q: %s", want, apps)
		}
	}
	owner := renderNavList(t, navStateCase{role: 3, path: "/campaigns/c1"}.ctx())
	if strings.Contains(owner, "data-nav-tools-toggle") {
		t.Errorf("the owner's sidebar must not have the touch Edit word; its Edit is the editor's")
	}
}

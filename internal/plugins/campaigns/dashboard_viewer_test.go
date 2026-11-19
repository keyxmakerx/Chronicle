// dashboard_viewer_test.go pins which home blocks each viewer is shown: a
// block appears only when that viewer can see something in it, and the owner,
// who fills the campaign, sees every block.
package campaigns

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func TestDashOwnerOnlyLazyBlocks(t *testing.T) {
	tests := []struct {
		role  Role
		shown bool
	}{
		{RoleNone, false},
		{RolePlayer, false},
		{RoleScribe, false},
		{RoleOwner, true},
	}
	for _, tt := range tests {
		cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: tt.role}
		for _, block := range []struct{ typ, marker string }{
			{"activity_feed", "/activity/embed"},
			{"sync_status", "/foundry-vtt/dashboard-sync-block"},
		} {
			out := renderCardWithCtx(t, context.Background(), DashboardBlockSwitch(cc, DashboardBlock{Type: block.typ}, nil))
			if got := strings.Contains(out, block.marker); got != tt.shown {
				t.Errorf("role %d, %s: shown = %v, want %v", tt.role, block.typ, got, tt.shown)
			}
		}
	}
}

func TestDashQuickActions_NoComingSoonTile(t *testing.T) {
	for _, role := range []Role{RoleNone, RolePlayer, RoleOwner} {
		cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: role}
		out := renderCardWithCtx(t, context.Background(), dashQuickActions(cc))
		if strings.Contains(out, "Coming soon") {
			t.Errorf("role %d: the stale Maps tile is back: %s", role, out)
		}
		if !strings.Contains(out, "All pages") {
			t.Errorf("role %d: All pages is missing: %s", role, out)
		}
	}
}

func TestDashVisibleCategories(t *testing.T) {
	ctx := layouts.SetEntityTypes(context.Background(), []layouts.SidebarEntityType{
		{ID: 1, Slug: "characters", NamePlural: "Characters"},
		{ID: 2, Slug: "secrets", NamePlural: "Secrets"},
	})
	// Counts are the viewer's own: nothing in Secrets is open to them.
	ctx = layouts.SetEntityCounts(ctx, map[int]int{1: 4, 2: 0})
	tests := []struct {
		role Role
		want []string
	}{
		{RoleNone, []string{"Characters"}},
		{RolePlayer, []string{"Characters"}},
		{RoleOwner, []string{"Characters", "Secrets"}},
	}
	for _, tt := range tests {
		cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: tt.role}
		var got []string
		for _, et := range dashVisibleCategories(ctx, cc) {
			got = append(got, et.NamePlural)
		}
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("role %d: got %v, want %v", tt.role, got, tt.want)
		}
	}

	// With nothing open to them, a visitor gets no Categories block at all.
	empty := layouts.SetEntityCounts(ctx, map[int]int{1: 0, 2: 0})
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RoleNone}
	if out := strings.TrimSpace(renderCardWithCtx(t, empty, dashCategoryGrid(cc, nil))); out != "" {
		t.Errorf("want no Categories block, got %s", out)
	}
}

// The Annals line shows a bare number beside each name, so the link itself
// has to say what the number counts, and an empty category is marked so it
// can step back.
func TestDashCategoryGridNamesEachCount(t *testing.T) {
	ctx := layouts.SetEntityTypes(context.Background(), []layouts.SidebarEntityType{
		{ID: 1, Slug: "characters", NamePlural: "Characters"},
		{ID: 2, Slug: "quests", NamePlural: "Quests"},
		{ID: 3, Slug: "heroes", NamePlural: "Heroes"},
	})
	ctx = layouts.SetEntityCounts(ctx, map[int]int{1: 17, 2: 1, 3: 0})
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RoleOwner}
	out := renderCardWithCtx(t, ctx, dashCategoryGrid(cc, nil))
	for _, want := range []string{
		`aria-label="Characters, 17 pages"`,
		`aria-label="Quests, 1 page"`,
		`aria-label="Heroes, 0 pages"`,
		`href="/campaigns/camp-1/quests"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
	if got := strings.Count(out, "data-empty"); got != 1 {
		t.Errorf("want only the empty category marked, got %d marks", got)
	}
	if strings.Contains(out, "cb-line-more") {
		t.Errorf("three categories need nothing folded: %s", out)
	}
}

func TestSplitCategoryLine(t *testing.T) {
	tests := []struct {
		name                   string
		n, wantShown, wantRest int
	}{
		{"one", 1, 1, 0},
		{"at the limit", categoryLineMax, categoryLineMax, 0},
		{"one over shows it rather than hiding one word", categoryLineMax + 1, categoryLineMax + 1, 0},
		{"two over folds", categoryLineMax + 2, categoryLineMax, 2},
		{"many", 200, categoryLineMax, 200 - categoryLineMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown, rest := splitCategoryLine(make([]layouts.SidebarEntityType, tt.n))
			if len(shown) != tt.wantShown || len(rest) != tt.wantRest {
				t.Errorf("got %d shown and %d behind the word, want %d and %d", len(shown), len(rest), tt.wantShown, tt.wantRest)
			}
		})
	}
}

// Past the limit the rest sit behind a quiet word that counts them.
func TestDashCategoryGridFoldsALongList(t *testing.T) {
	n := categoryLineMax + 12
	types := make([]layouts.SidebarEntityType, n)
	counts := map[int]int{}
	for i := range types {
		types[i] = layouts.SidebarEntityType{ID: i + 1, Slug: "c" + strconv.Itoa(i), NamePlural: "Cat " + strconv.Itoa(i)}
		counts[i+1] = 1
	}
	ctx := layouts.SetEntityCounts(layouts.SetEntityTypes(context.Background(), types), counts)
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: RolePlayer}
	out := renderCardWithCtx(t, ctx, dashCategoryGrid(cc, nil))
	if !strings.Contains(out, "and 12 more") {
		t.Errorf("want the word to count the hidden categories: %s", out)
	}
	if got := strings.Count(out, `class="cb-word `); got != n {
		t.Errorf("every category is still on the page: got %d links, want %d", got, n)
	}
}

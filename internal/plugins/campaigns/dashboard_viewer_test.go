// dashboard_viewer_test.go pins which home blocks each viewer is shown: a
// block appears only when that viewer can see something in it, and the owner,
// who fills the campaign, sees every block.
package campaigns

import (
	"context"
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
		if !strings.Contains(out, "All Pages") {
			t.Errorf("role %d: All Pages is missing: %s", role, out)
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

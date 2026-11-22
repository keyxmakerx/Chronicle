package entities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A block that offers a page tab joins the panel under a system-drawn page by
// what the viewer has there: first with rows, after Items with only an add
// button, absent with neither or when the read fails.
func TestSystemPageTabListBlockTab(t *testing.T) {
	tests := []struct {
		name   string
		rows   int
		canAdd bool
		err    error
		want   string
	}{
		{"rows seen", 2, false, nil, "demo,notes,files,posts,links"},
		{"rows and may add", 1, true, nil, "demo,notes,files,posts,links"},
		{"may add only", 0, true, nil, "notes,demo,files,posts,links"},
		{"neither", 0, false, nil, "notes,files,posts,links"},
		{"failed read", 3, true, errors.New("boom"), "notes,files,posts,links"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewBlockRegistry()
			reg.Register(BlockMeta{Type: "demo", PageTab: &BlockPageTab{Label: "Demo", Count: ".d", Act: ".a",
				State: func(context.Context, BlockRenderContext) (int, bool, error) { return tt.rows, tt.canAdd, tt.err }}}, nil)
			prev := GetGlobalBlockRegistry()
			SetGlobalBlockRegistry(reg)
			defer SetGlobalBlockRegistry(prev)

			ctx := layouts.SetUserID(context.Background(), "u1")
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
			var keys []string
			for _, tab := range systemPageTabList(ctx, cc, &Entity{ID: "e1"}, nil, "") {
				keys = append(keys, tab.Key)
			}
			if got := strings.Join(keys, ","); got != tt.want {
				t.Errorf("tabs = %s, want %s", got, tt.want)
			}
		})
	}
}

// With no registry, or no block offering a tab, the panel is unchanged.
func TestSystemPageTabListNoBlockTabs(t *testing.T) {
	prev := GetGlobalBlockRegistry()
	SetGlobalBlockRegistry(NewBlockRegistry())
	defer SetGlobalBlockRegistry(prev)
	ctx := layouts.SetUserID(context.Background(), "u1")
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	if n := len(systemPageTabList(ctx, cc, &Entity{ID: "e1"}, nil, "")); n != 4 {
		t.Errorf("tabs = %d, want 4", n)
	}
}

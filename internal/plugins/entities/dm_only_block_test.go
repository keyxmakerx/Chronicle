package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestDmOnlyBlockShownToWhoCanWriteIt pins that a block marked DM-only is
// drawn for everyone who can author DM-only content, members with DM access
// included, and hidden from plain players.
func TestDmOnlyBlockShownToWhoCanWriteIt(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	block := TemplateBlock{ID: "b-entry", Type: "entry", Config: map[string]any{"visibility": "dm_only"}}
	entity := &Entity{ID: "e1", CampaignID: "c1"}
	camp := &campaigns.Campaign{ID: "c1"}
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		want bool
	}{
		{"owner", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true}, true},
		{"member with DM access", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true}, true},
		{"player", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			if err := entityBlock(block, tt.cc, entity, &EntityType{ID: 1}, false, "csrf").Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(sb.String(), "GM Only"); got != tt.want {
				t.Fatalf("DM-only block shown = %v, want %v", got, tt.want)
			}
		})
	}
}

// The Director-only badge sits on its own line above the block, never laid
// over it: a block's top-right corner often holds the block's own control,
// which an absolutely placed badge hid and took the clicks of.
func TestDmOnlyBadgeNeverCoversTheBlock(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	camp := &campaigns.Campaign{ID: "c1"}
	cc := &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true}
	block := TemplateBlock{ID: "b-entry", Type: "entry", Config: map[string]any{"visibility": "dm_only"}}
	var sb strings.Builder
	if err := entityBlock(block, cc, &Entity{ID: "e1", CampaignID: "c1"}, &EntityType{ID: 1}, false, "csrf").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	html := sb.String()
	row := strings.Index(html, "data-dm-badge-row")
	badge := strings.Index(html, `class="cb-dm`)
	if row < 0 || badge < row {
		t.Fatalf("the badge is not in its own row above the block: %.400s", html)
	}
	tag := html[badge:]
	tag = tag[:strings.Index(tag, ">")]
	for _, laid := range []string{"absolute", "fixed", "z-10"} {
		if strings.Contains(tag, laid) {
			t.Errorf("the badge is laid over the block (%q): %s", laid, tag)
		}
	}
}

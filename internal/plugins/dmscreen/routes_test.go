package dmscreen

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCanOpen(t *testing.T) {
	tests := []struct {
		name string
		cc   campaigns.CampaignContext
		want bool
	}{
		{"owner", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner, IsMember: true}, true},
		{"scribe", campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsMember: true}, true},
		{"player", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true}, false},
		{"player with DM access", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true, IsDmGranted: true}, true},
		{"non-member", campaigns.CampaignContext{MemberRole: campaigns.RoleNone}, false},
		{"site admin who isn't a member", campaigns.CampaignContext{MemberRole: campaigns.RoleNone, IsSiteAdmin: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanOpen(&tt.cc); got != tt.want {
				t.Errorf("CanOpen = %v, want %v", got, tt.want)
			}
		})
	}
}

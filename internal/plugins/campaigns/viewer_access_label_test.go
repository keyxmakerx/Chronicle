package campaigns

import (
	"testing"
	"time"
)

func TestViewerAccessLabel(t *testing.T) {
	until := time.Now().Add(time.Hour)
	tests := []struct {
		name string
		cc   CampaignContext
		want string
	}{
		{"owner", CampaignContext{MemberRole: RoleOwner}, "Owner"},
		{"co-DM", CampaignContext{MemberRole: RoleScribe, IsDmGranted: true}, "Co-DM"},
		{"scribe", CampaignContext{MemberRole: RoleScribe}, "Scribe"},
		{"player with DM access", CampaignContext{MemberRole: RolePlayer, IsDmGranted: true}, "Player with DM access"},
		{"player", CampaignContext{MemberRole: RolePlayer}, "Player"},
		{"look-around visitor", CampaignContext{MemberRole: RoleNone, VisitUntil: &until}, "Visitor"},
		{"public visitor has no chip", CampaignContext{MemberRole: RoleNone}, ""},
		{"admin without a role has no chip", CampaignContext{MemberRole: RoleNone, IsSiteAdmin: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ViewerAccessLabel(&tt.cc); got != tt.want {
				t.Errorf("ViewerAccessLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

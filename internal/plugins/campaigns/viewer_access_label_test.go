package campaigns

import "testing"

func TestViewerAccessLabel(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ViewerAccessLabel(&tt.cc); got != tt.want {
				t.Errorf("ViewerAccessLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

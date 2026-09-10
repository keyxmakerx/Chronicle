package campaigns

import (
	"context"
	"strings"
	"testing"
)

// TestSettingsPeopleTab_OffersScribeSelect pins the only live HTML
// surface for changing a member's role. GET /members redirects to
// Settings > People, and that tab used to render a static PLAYER
// badge with no dropdown — operators had no way to promote a Player
// to Scribe from the UI they actually see.
func TestSettingsPeopleTab_OffersScribeSelect(t *testing.T) {
	cc := &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test"},
		MemberRole: RoleOwner,
	}
	members := []CampaignMember{
		{UserID: "u-owner", DisplayName: "Owner", Email: "o@example.com", Role: RoleOwner},
		{UserID: "u-player", DisplayName: "Player", Email: "p@example.com", Role: RolePlayer},
	}

	var sb strings.Builder
	if err := settingsPeopleTab(cc, members, nil, "tok", true).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render people tab: %v", err)
	}
	html := sb.String()

	if !strings.Contains(html, `/campaigns/camp-1/members/u-player/role`) {
		t.Error("player row must expose PUT /members/:uid/role")
	}
	if !strings.Contains(html, `value="scribe"`) {
		t.Error("role select must offer Scribe")
	}
	if strings.Contains(html, `/campaigns/camp-1/members/u-owner/role`) {
		t.Error("owner row must not expose a role-change form")
	}
}

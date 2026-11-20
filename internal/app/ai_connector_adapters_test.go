package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// oneMemberCampaigns answers for one campaign and one member of it.
type oneMemberCampaigns struct {
	campaigns.CampaignService
	member campaigns.CampaignMember
}

func (f oneMemberCampaigns) GetByID(context.Context, string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: "camp-1", Name: "Ashfall"}, nil
}

func (f oneMemberCampaigns) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	m := f.member
	return &m, nil
}

// A guest player is refused as a key maker; a player with an account isn't.
func TestAIConnectorOwners_GuestRefused(t *testing.T) {
	cases := []struct {
		name  string
		guest bool
		want  error
	}{
		{"player with an account", false, nil},
		{"guest player", true, connector.ErrGuest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := aiConnectorOwners{campaigns: oneMemberCampaigns{member: campaigns.CampaignMember{
				UserID: "u", Role: campaigns.RolePlayer, IsGuest: tc.guest}}}
			_, err := o.CheckMember(context.Background(), "camp-1", "u")
			if !errors.Is(err, tc.want) {
				t.Fatalf("CheckMember = %v, want %v", err, tc.want)
			}
		})
	}
}

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type houseRulesCampaigns struct {
	campaigns.CampaignService
	settings string
	err      error
}

func (s houseRulesCampaigns) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &campaigns.Campaign{ID: id, Settings: s.settings}, nil
}

type houseRulesHub []campaigns.PluginHubAddon

func (h houseRulesHub) ListForPluginHub(context.Context, string) ([]campaigns.PluginHubAddon, error) {
	return h, nil
}

// The AI's house-rule kind names the campaign's system the way the picker
// does, so a campaign with the add-on on and no saved system is not refused.
func TestNewAIDeps_HouseRulesSystemFollowsPicker(t *testing.T) {
	on := houseRulesHub{{Slug: "drawsteel", Category: "system", Enabled: true}}
	off := houseRulesHub{{Slug: "drawsteel", Category: "system"}}
	tests := []struct {
		name string
		svc  houseRulesCampaigns
		hub  campaigns.AddonLister
		want string
	}{
		{"saved system", houseRulesCampaigns{settings: `{"system_id":"dnd5e"}`}, on, "dnd5e"},
		{"only the add-on is on", houseRulesCampaigns{settings: `{}`}, on, "drawsteel"},
		{"add-on off", houseRulesCampaigns{settings: `{}`}, off, ""},
		{"no lister", houseRulesCampaigns{settings: `{}`}, nil, ""},
		{"campaign unreadable", houseRulesCampaigns{err: errors.New("gone")}, on, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newAIDeps(aiDeps{Campaigns: tt.svc, AddonLister: tt.hub})
			if got := d.HouseRules.SystemOf(context.Background(), "c-1"); got != tt.want {
				t.Errorf("SystemOf = %q, want %q", got, tt.want)
			}
		})
	}
}

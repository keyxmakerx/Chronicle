package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
)

type fakeKitMembers struct{ picks map[string]string }

func (f fakeKitMembers) MemberLookPicks(context.Context, *campaigns.CampaignContext, string) (map[string]string, error) {
	return f.picks, nil
}

func kitCampaign(t *testing.T, ap campaigns.Appearance) *campaigns.Campaign {
	t.Helper()
	b, err := json.Marshal(campaigns.CampaignSettings{Appearance: &ap})
	if err != nil {
		t.Fatal(err)
	}
	return &campaigns.Campaign{ID: "c1", Settings: string(b)}
}

// The Style drawer's kit row: which kit the campaign gives the dock, and
// whether the viewer may pick their own, as the owner's "who can change"
// setting for the DM Screen decides.
func TestScreenKitAdapter(t *testing.T) {
	cases := []struct {
		name     string
		ap       campaigns.Appearance
		role     campaigns.Role
		dm       bool
		screen   string
		campaign string
		canPick  bool
		mine     string
		owner    bool
	}{
		{name: "screen's own kit wins", ap: campaigns.Appearance{DmScreenKit: "sharp", StyleKit: "ornate", PageLook: "annals"}, role: campaigns.RoleOwner, screen: dmscreen.ScreenDM, campaign: "sharp", owner: true},
		{name: "pages' kit under Material", ap: campaigns.Appearance{StyleKit: "ornate", PageLook: "annals"}, role: campaigns.RoleOwner, screen: dmscreen.ScreenDM, campaign: "ornate", owner: true},
		{name: "pages' kit off Material is none", ap: campaigns.Appearance{StyleKit: "ornate"}, role: campaigns.RoleOwner, screen: dmscreen.ScreenDM, campaign: "material", owner: true},
		{name: "unknown kit is none", ap: campaigns.Appearance{DmScreenKit: "neon"}, role: campaigns.RoleOwner, screen: dmscreen.ScreenDM, campaign: "material", owner: true},
		{name: "co-DM picks by default", ap: campaigns.Appearance{}, role: campaigns.RolePlayer, dm: true, screen: dmscreen.ScreenDM, campaign: "material", canPick: true, mine: "lumen"},
		{name: "owner only: co-DM cannot", ap: campaigns.Appearance{AccessDmScreen: campaigns.LookAccessOwner}, role: campaigns.RolePlayer, dm: true, screen: dmscreen.ScreenDM, campaign: "material"},
		{name: "players' screen follows the campaign", ap: campaigns.Appearance{DmScreenKit: "flowing"}, role: campaigns.RolePlayer, screen: dmscreen.ScreenPlayer, campaign: "flowing"},
	}
	a := &screenKitAdapter{members: fakeKitMembers{picks: map[string]string{campaigns.LookPartDmScreen: "lumen"}}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: kitCampaign(t, c.ap), MemberRole: c.role, IsMember: true, IsDmGranted: c.dm}
			got, err := a.ScreenKit(context.Background(), cc, "u1", c.screen)
			if err != nil {
				t.Fatal(err)
			}
			if got.Campaign != c.campaign || got.CanPick != c.canPick || got.Mine != c.mine || got.Owner != c.owner {
				t.Fatalf("got %+v", got)
			}
			if got.CanPick && got.Part != campaigns.LookPartDmScreen {
				t.Fatalf("part %q", got.Part)
			}
		})
	}
}

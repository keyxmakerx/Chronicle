package syncapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// cutsceneCampaigns answers the two lookups dmUser makes, each able to fail.
type cutsceneCampaigns struct {
	campaigns.CampaignService
	role      campaigns.Role
	memberErr error
	granted   bool
	grantErr  error
}

func (s *cutsceneCampaigns) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	if s.memberErr != nil {
		return nil, s.memberErr
	}
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: s.role}, nil
}

func (s *cutsceneCampaigns) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return s.granted, s.grantErr
}

type fakeCutsceneAPI struct{ gotCampaign, gotUser string }

func (f *fakeCutsceneAPI) MemberShowings(_ context.Context, campaignID, userID string) (any, int, error) {
	f.gotCampaign, f.gotUser = campaignID, userID
	return []map[string]string{{"showingId": "sh-1"}}, 1, nil
}

// The route names whose showings to list, so only the owner or a co-DM may
// call it, and a lookup that fails refuses rather than lets through.
func TestCutsceneAPIDMUser(t *testing.T) {
	down := errors.New("db down")
	tests := []struct {
		name string
		svc  *cutsceneCampaigns
		want int
	}{
		{name: "a player's key", svc: &cutsceneCampaigns{role: campaigns.RolePlayer}, want: http.StatusForbidden},
		{name: "a scribe's key", svc: &cutsceneCampaigns{role: campaigns.RoleScribe}, want: http.StatusForbidden},
		{name: "a co-DM's key", svc: &cutsceneCampaigns{role: campaigns.RolePlayer, granted: true}, want: http.StatusOK},
		{name: "the owner's key", svc: &cutsceneCampaigns{role: campaigns.RoleOwner}, want: http.StatusOK},
		{name: "member lookup fails, no grant", svc: &cutsceneCampaigns{memberErr: down}, want: http.StatusForbidden},
		{name: "both lookups fail", svc: &cutsceneCampaigns{memberErr: down, grantErr: down}, want: http.StatusForbidden},
		{name: "grant lookup fails for a player", svc: &cutsceneCampaigns{role: campaigns.RolePlayer, granted: true, grantErr: down}, want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeCutsceneAPI{}
			h := NewCutsceneAPIHandler(api, tt.svc, "cutscenes")
			c, rec := dmScreenContext(http.MethodGet, "", 1)
			c.Request().URL.RawQuery = "user=ana"
			err := h.Showings(c)
			got := http.StatusOK
			if err != nil {
				got = dmStatusOf(t, err)
			} else if rec.Code != http.StatusOK {
				got = rec.Code
			}
			if got != tt.want {
				t.Fatalf("status %d, want %d", got, tt.want)
			}
			if tt.want == http.StatusOK && (api.gotCampaign != "camp-1" || api.gotUser != "ana") {
				t.Errorf("listed %q in %q", api.gotUser, api.gotCampaign)
			}
			if tt.want != http.StatusOK && api.gotUser != "" {
				t.Errorf("a refused key reached the service")
			}
		})
	}
}

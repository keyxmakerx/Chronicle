package syncapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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

type fakeCutsceneAPI struct {
	gotCampaign, gotUser string
	gotShowing           string
	gotMoment            int
	gotWay, gotText      string
}

func (f *fakeCutsceneAPI) Answer(_ context.Context, campaignID, showingID string, moment int, userID, way, text string) (any, error) {
	f.gotCampaign, f.gotShowing, f.gotMoment, f.gotUser, f.gotWay, f.gotText = campaignID, showingID, moment, userID, way, text
	return map[string]any{"moment": moment, "way": way, "text": text}, nil
}

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

// A Foundry player's answer goes through only on an owner's or co-DM's key,
// for the player the body names, at the moment in the path.
func TestCutsceneAPIAnswer(t *testing.T) {
	tests := []struct {
		name   string
		svc    *cutsceneCampaigns
		body   string
		moment string
		want   int
	}{
		{name: "the owner relays an answer", svc: &cutsceneCampaigns{role: campaigns.RoleOwner}, body: `{"user":"ana","way":"write","text":"The bell rang twice."}`, moment: "1", want: http.StatusOK},
		{name: "a co-DM relays an answer", svc: &cutsceneCampaigns{role: campaigns.RolePlayer, granted: true}, body: `{"user":"ana","way":"director"}`, moment: "0", want: http.StatusOK},
		{name: "a player's key", svc: &cutsceneCampaigns{role: campaigns.RolePlayer}, body: `{"user":"ana","way":"write","text":"x"}`, moment: "0", want: http.StatusForbidden},
		{name: "no player named", svc: &cutsceneCampaigns{role: campaigns.RoleOwner}, body: `{"way":"write","text":"x"}`, moment: "0", want: http.StatusBadRequest},
		{name: "moment not a number", svc: &cutsceneCampaigns{role: campaigns.RoleOwner}, body: `{"user":"ana","way":"write","text":"x"}`, moment: "one", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeCutsceneAPI{gotMoment: -1}
			h := NewCutsceneAPIHandler(api, tt.svc, "cutscenes")
			c, rec := dmScreenContext(http.MethodPost, tt.body, 1, "shid", "sh-1", "m", tt.moment)
			err := h.Answer(c)
			got := rec.Code
			if err != nil {
				got = dmStatusOf(t, err)
			}
			if got != tt.want {
				t.Fatalf("status %d, want %d", got, tt.want)
			}
			if tt.want != http.StatusOK {
				if api.gotUser != "" {
					t.Errorf("a refused call reached the service")
				}
				return
			}
			if api.gotCampaign != "camp-1" || api.gotShowing != "sh-1" || api.gotUser != "ana" || strconv.Itoa(api.gotMoment) != tt.moment {
				t.Errorf("answered %+v", api)
			}
		})
	}
}

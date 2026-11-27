package addons

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// toggleService stands in for the add-ons service on the campaign toggle:
// it names addon 7 "Calendar" and can be told to refuse.
type toggleService struct {
	AddonService
	fail error
}

func (s toggleService) EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error {
	return s.fail
}

func (s toggleService) DisableForCampaign(ctx context.Context, campaignID string, addonID int) error {
	return s.fail
}

func (toggleService) GetByID(ctx context.Context, id int) (*Addon, error) {
	return &Addon{ID: id, Name: "Calendar"}, nil
}

type loggedToggle struct{ campaignID, userID, action, subject string }

type recordingCampaignLog struct{ got []loggedToggle }

func (r *recordingCampaignLog) LogCampaignActivity(ctx context.Context, campaignID, userID, action, subject string) error {
	r.got = append(r.got, loggedToggle{campaignID, userID, action, subject})
	return nil
}

// TestToggleCampaignAddon_LogsToCampaignActivity pins that each campaign
// feature switch writes one line to that campaign's activity log, saying
// who, which feature and which way, and that a refused switch writes none,
// whether the service refuses it or the role gate stops a player first.
func TestToggleCampaignAddon_LogsToCampaignActivity(t *testing.T) {
	cases := []struct {
		name   string
		action string
		role   campaigns.Role
		fail   error
		want   []loggedToggle
	}{
		{"owner switches on", "enable", campaigns.RoleOwner, nil, []loggedToggle{{"c1", "u1", "addon.enabled", "Calendar"}}},
		{"owner switches off", "disable", campaigns.RoleOwner, nil, []loggedToggle{{"c1", "u1", "addon.disabled", "Calendar"}}},
		{"service refuses", "enable", campaigns.RoleOwner, errors.New("no"), nil},
		{"player is refused by the role gate", "enable", campaigns.RolePlayer, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := &recordingCampaignLog{}
			h := NewHandler(toggleService{fail: tc.fail})
			h.SetCampaignActivityLogger(log)

			form := url.Values{"action": {tc.action}}
			req := httptest.NewRequest(http.MethodPut, "/campaigns/c1/addons/7/toggle", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "addonID", Value: "7"}})
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role})
			c.Set("auth_user_id", "u1")

			_ = campaigns.RequireRole(campaigns.RoleOwner)(h.ToggleCampaignAddon)(c)

			if len(log.got) != len(tc.want) {
				t.Fatalf("logged %v, want %v", log.got, tc.want)
			}
			for i := range tc.want {
				if log.got[i] != tc.want[i] {
					t.Errorf("logged %v, want %v", log.got[i], tc.want[i])
				}
			}
		})
	}
}

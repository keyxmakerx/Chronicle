// backlinks_gm_wiring_test.go pins that BacklinksFragment computes canSeeGM
// from the viewer's raw MemberRole (the same Scribe+ bar GetEntry and
// GetFieldsAPI use for GM secrets/field values), not a hard-coded value and
// not cc.VisibilityRole() — which promotes a DM-granted Co-DM to Owner for
// dm_only CONTENT visibility, a different axis from seeing GM secrets.
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// backlinksGMWiringSvc records the canSeeGM value BacklinksFragment passes to
// GetBacklinksWithSnippets.
type backlinksGMWiringSvc struct {
	EntityService
	entity      *Entity
	gotCanSeeGM bool
}

func (s *backlinksGMWiringSvc) GetByID(_ context.Context, _ string) (*Entity, error) {
	e := *s.entity
	return &e, nil
}

func (s *backlinksGMWiringSvc) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: true}, nil
}

func (s *backlinksGMWiringSvc) GetBacklinksWithSnippets(_ context.Context, _, _ string, _ int, _ string, canSeeGM bool) ([]BacklinkEntry, error) {
	s.gotCanSeeGM = canSeeGM
	return nil, nil
}

func TestBacklinksFragment_CanSeeGMWiring(t *testing.T) {
	entity := &Entity{ID: "e1", CampaignID: "c1"}

	tests := []struct {
		name         string
		cc           *campaigns.CampaignContext
		wantCanSeeGM bool
	}{
		{
			name:         "owner",
			cc:           &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true},
			wantCanSeeGM: true,
		},
		{
			name:         "scribe",
			cc:           &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe, IsMember: true},
			wantCanSeeGM: true,
		},
		{
			name:         "player",
			cc:           &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true},
			wantCanSeeGM: false,
		},
		{
			name:         "anonymous visitor on a public campaign",
			cc:           &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleNone},
			wantCanSeeGM: false,
		},
		{
			// A Co-DM sees dm_only CONTENT (VisibilityRole promotes them to
			// Owner for that), but is still a Player for GM-secret purposes —
			// matching GetEntry/GetFieldsAPI, which also gate on raw
			// MemberRole, not VisibilityRole/IsDmGranted.
			name:         "co-dm (DM-granted player) does not see secrets, matching GetEntry/GetFieldsAPI",
			cc:           &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true},
			wantCanSeeGM: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &backlinksGMWiringSvc{entity: entity}
			h := &Handler{service: svc}

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/backlinks", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "e1")
			c.Set("campaign_context", tt.cc)
			auth.SetSession(c, &auth.Session{UserID: "user-1"})

			if err := h.BacklinksFragment(c); err != nil {
				t.Fatalf("BacklinksFragment: %v", err)
			}
			if svc.gotCanSeeGM != tt.wantCanSeeGM {
				t.Errorf("canSeeGM passed to GetBacklinksWithSnippets = %v, want %v", svc.gotCanSeeGM, tt.wantCanSeeGM)
			}
		})
	}
}

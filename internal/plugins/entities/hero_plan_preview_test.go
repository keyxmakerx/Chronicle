package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// recordingPlanner remembers who the last plan was read for and how.
type recordingPlanner struct {
	user     string
	director bool
}

func (p *recordingPlanner) HeroPlan(_ context.Context, _, userID string, director bool) (*HeroPlan, error) {
	p.user, p.director = userID, director
	return &HeroPlan{Steps: []HeroStep{}}, nil
}

// TestHeroPlanAPI_ViewAsPlayer pins that the hero builder and creator, which
// ask with ?preview=player, read the plan as a player gets it while a
// Director has "view as player" on, so entries marked Not here are gone
// there as they are for players. The Directors' tools never ask and keep the
// whole list, and the cookie gives a player nothing.
func TestHeroPlanAPI_ViewAsPlayer(t *testing.T) {
	tests := []struct {
		name         string
		role         campaigns.Role
		cookie       bool
		query        string
		wantDirector bool
		wantCanPin   bool
	}{
		{"director, preview off", campaigns.RoleOwner, false, "?preview=player", true, true},
		{"director viewing as player on a player's page", campaigns.RoleOwner, true, "?preview=player", false, false},
		{"director viewing as player in a Directors' tool", campaigns.RoleOwner, true, "", true, true},
		{"player with a stray cookie", campaigns.RolePlayer, true, "?preview=player", false, false},
		{"scribe", campaigns.RoleScribe, false, "?preview=player", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yes := true
			p := &recordingPlanner{}
			h := &Handler{
				service:     &planAccessSvc{types: []EntityType{{ID: 1, Name: "Hero", Enabled: true, Claimable: &yes}}},
				charLists:   planAccessLists{ids: []int{1}},
				heroPlanner: p,
			}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/characters/new/plan"+tc.query, nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "c1"})
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})
			c.Set("auth_user_id", "u1")
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role})
			if err := h.HeroPlanAPI(c); err != nil {
				t.Fatalf("plan: %v", err)
			}
			if p.director != tc.wantDirector || p.user != "u1" {
				t.Fatalf("read for %q director=%v, want u1 director=%v", p.user, p.director, tc.wantDirector)
			}
			// The pin control is offered only to the Director team seeing
			// the creator as themselves.
			if got := strings.Contains(rec.Body.String(), `"canPin":true`); got != tc.wantCanPin {
				t.Errorf("canPin = %v, want %v", got, tc.wantCanPin)
			}
		})
	}
}

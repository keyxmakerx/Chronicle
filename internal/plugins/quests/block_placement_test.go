package quests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type placementStub struct {
	yes   map[string]bool
	calls []string
	role  int
}

func (p *placementStub) GuestMaySee(_ context.Context, _, blockType, on, _ string, role int) bool {
	p.calls = append(p.calls, blockType+"|"+on)
	p.role = role
	return p.yes[blockType+"|"+on]
}

// A guest gets Open quests only where the owner placed it: the Guests home or
// a page the placement check vouches for. A member always passes.
func TestOpenQuestsBlockPagePlacement(t *testing.T) {
	d := &QuestDigest{TypeSlug: "quest", Total: 1, Open: []DigestQuest{{EntityID: "a", Title: "Find the bell", Status: StatusActive}}}
	guestHome := func() *campaigns.Campaign {
		camp := &campaigns.Campaign{ID: "c1", IsPublic: true}
		layout := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{
			Blocks: []campaigns.DashboardBlock{{ID: "b", Type: campaigns.BlockOpenQuests}}}}}}}
		full, err := camp.SetRoleDashboardJSON(campaigns.HomeGuest, layout)
		if err != nil {
			t.Fatal(err)
		}
		camp.DashboardLayout = full
		return camp
	}
	pub := &campaigns.Campaign{ID: "c1", IsPublic: true}
	member := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	tests := []struct {
		name      string
		cc        *campaigns.CampaignContext
		query     string
		wantBody  bool
		wantReads int
		wantAsked bool
	}{
		{"guest, placed on the Guests home", &campaigns.CampaignContext{Campaign: guestHome()}, "", true, 1, false},
		{"guest, on a public page whose layout has the block", &campaigns.CampaignContext{Campaign: pub}, "?on=page-ok", true, 1, true},
		{"guest, on a public page without the block", &campaigns.CampaignContext{Campaign: pub}, "?on=page-bare", false, 0, true},
		{"guest, on a private page", &campaigns.CampaignContext{Campaign: pub}, "?on=page-private", false, 0, true},
		{"guest, on a page that does not exist", &campaigns.CampaignContext{Campaign: pub}, "?on=nope", false, 0, true},
		{"guest, no placement anywhere", &campaigns.CampaignContext{Campaign: pub}, "", false, 0, false},
		{"member, wherever it is", member, "?on=nope", true, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dig := &digestStub{d: d}
			stub := &placementStub{yes: map[string]bool{campaigns.BlockOpenQuests + "|page-ok": true}}
			h := &Handler{quests: dig}
			h.SetPlacementChecker(stub)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/open-quests"+tt.query, nil), rec)
			c.Set("campaign_context", tt.cc)
			if err := h.OpenQuestsBlock(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if got := strings.Contains(rec.Body.String(), "Find the bell"); got != tt.wantBody {
				t.Errorf("shows the quest = %v, want %v:\n%s", got, tt.wantBody, rec.Body.String())
			}
			if dig.calls != tt.wantReads {
				t.Errorf("digest reads = %d, want %d", dig.calls, tt.wantReads)
			}
			if (len(stub.calls) > 0) != tt.wantAsked {
				t.Errorf("placement asked = %v, want %v", len(stub.calls) > 0, tt.wantAsked)
			}
			if tt.wantAsked && stub.role != 0 {
				t.Errorf("a guest is asked about as role %d, want 0", stub.role)
			}
		})
	}
}

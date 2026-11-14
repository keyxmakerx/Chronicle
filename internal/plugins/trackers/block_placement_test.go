package trackers

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

// A guest gets the clocks only where the owner placed them: the Guests home or
// a page the placement check vouches for. A member always passes.
func TestWorldClocksBlockPagePlacement(t *testing.T) {
	list := []SharedTracker{{EntityID: "p", Name: "The flood rises", Shape: ShapeClock, Value: 2, Bounded: true, Max: 6}}
	guestHome := func() *campaigns.Campaign {
		camp := &campaigns.Campaign{ID: "c1", IsPublic: true}
		layout := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{
			Blocks: []campaigns.DashboardBlock{{ID: "b", Type: campaigns.BlockWorldClocks}}}}}}}
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
			stubSvc := &sharedStub{list: list}
			stub := &placementStub{yes: map[string]bool{campaigns.BlockWorldClocks + "|page-ok": true}}
			h := NewHandler(stubSvc)
			h.SetPlacementChecker(stub)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/world-clocks"+tt.query, nil), rec)
			c.Set("campaign_context", tt.cc)
			if err := h.WorldClocksBlock(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if got := strings.Contains(rec.Body.String(), "The flood rises"); got != tt.wantBody {
				t.Errorf("shows the clock = %v, want %v:\n%s", got, tt.wantBody, rec.Body.String())
			}
			if stubSvc.calls != tt.wantReads {
				t.Errorf("reads = %d, want %d", stubSvc.calls, tt.wantReads)
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

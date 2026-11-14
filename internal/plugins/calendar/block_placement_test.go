package calendar

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

// A guest gets Today in the world only where the owner placed it: the Guests
// home or a page the placement check vouches for. A member always passes.
func TestTodayInWorldBlockPagePlacement(t *testing.T) {
	pub := &campaigns.Campaign{ID: "c1", IsPublic: true}
	member := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	tests := []struct {
		name      string
		cc        *campaigns.CampaignContext
		query     string
		wantBody  bool
		wantAsked bool
	}{
		{"guest, placed on the Guests home", &campaigns.CampaignContext{Campaign: guestCampaign(t, true)}, "", true, false},
		{"guest, on a public page whose layout has the block", &campaigns.CampaignContext{Campaign: pub}, "?on=page-ok", true, true},
		{"guest, on a public page without the block", &campaigns.CampaignContext{Campaign: pub}, "?on=page-bare", false, true},
		{"guest, on a private page", &campaigns.CampaignContext{Campaign: pub}, "?on=page-private", false, true},
		{"guest, on a page that does not exist", &campaigns.CampaignContext{Campaign: pub}, "?on=nope", false, true},
		{"guest, no placement anywhere", &campaigns.CampaignContext{Campaign: pub}, "", false, false},
		{"member, wherever it is", member, "?on=nope", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{svc: todaySvc{fakeToday: &fakeToday{cal: todayCalendar()}}}
			stub := &placementStub{yes: map[string]bool{campaigns.BlockTodayInWorld + "|page-ok": true}}
			h.SetPlacementChecker(stub)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/today-in-world"+tt.query, nil), rec)
			c.Set("campaign_context", tt.cc)
			if err := h.TodayInWorldBlock(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d", rec.Code)
			}
			if got := strings.Contains(rec.Body.String(), "Emberfall 14, 1492"); got != tt.wantBody {
				t.Errorf("shows the date = %v, want %v:\n%s", got, tt.wantBody, rec.Body.String())
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

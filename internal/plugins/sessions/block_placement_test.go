package sessions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// placementStub answers GuestMaySee from a fixed table and records what it
// was asked, so a test can see the handler passed the page, block and role on.
type placementStub struct {
	yes   map[string]bool // "blockType|pageID" -> allowed
	calls []placementCall
}

type placementCall struct {
	campaignID, blockType, on, userID string
	role                              int
}

func (p *placementStub) GuestMaySee(_ context.Context, campaignID, blockType, on, userID string, role int) bool {
	p.calls = append(p.calls, placementCall{campaignID, blockType, on, userID, role})
	return p.yes[blockType+"|"+on]
}

// nightsStub serves one night for a guest read. A member read returns err so
// the handler stops after the gate without needing the member-only services.
type nightsStub struct {
	SessionService
	night GameNight
	err   error
	reads int
}

func (s *nightsStub) ListGameNights(context.Context, string, string, string, string, []NightMember) ([]GameNight, error) {
	s.reads++
	if s.err != nil {
		return nil, s.err
	}
	return []GameNight{s.night}, nil
}

func guestHomePlacing(t *testing.T, blockType string) *campaigns.Campaign {
	t.Helper()
	camp := &campaigns.Campaign{ID: "c1", IsPublic: true}
	layout := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{
		Blocks: []campaigns.DashboardBlock{{ID: "b", Type: blockType}}}}}}}
	full, err := camp.SetRoleDashboardJSON(campaigns.HomeGuest, layout)
	if err != nil {
		t.Fatal(err)
	}
	camp.DashboardLayout = full
	return camp
}

// A guest gets a game-night block only where the owner placed it: the Guests
// home, or a page the placement check vouches for. A member always passes.
func TestGameNightBlocksGuestPlacement(t *testing.T) {
	blocks := []struct {
		blockType string
		route     string
		serve     func(*Handler, *echo.Context) error
	}{
		{campaigns.BlockNextGame, "/campaigns/c1/next-game", (*Handler).NextGameBlock},
		{campaigns.BlockWhenWeCouldPlay, "/campaigns/c1/when-we-could-play", (*Handler).WhenWeCouldPlayBlock},
	}
	tonight := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	for _, b := range blocks {
		pub := &campaigns.Campaign{ID: "c1", IsPublic: true}
		member := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
		tests := []struct {
			name      string
			cc        *campaigns.CampaignContext
			query     string
			wantBody  bool
			wantReads int
			wantCall  bool
		}{
			{"guest, placed on the Guests home", &campaigns.CampaignContext{Campaign: guestHomePlacing(t, b.blockType)}, "", true, 1, false},
			{"guest, on a public page whose layout has the block", &campaigns.CampaignContext{Campaign: pub}, "?on=page-ok", true, 1, true},
			{"guest, on a public page without the block", &campaigns.CampaignContext{Campaign: pub}, "?on=page-bare", false, 0, true},
			{"guest, on a private page", &campaigns.CampaignContext{Campaign: pub}, "?on=page-private", false, 0, true},
			{"guest, on a page that does not exist", &campaigns.CampaignContext{Campaign: pub}, "?on=nope", false, 0, true},
			{"guest, no placement anywhere", &campaigns.CampaignContext{Campaign: pub}, "", false, 0, false},
			{"member, wherever it is", member, "?tz=UTC&on=nope", false, 1, false},
		}
		for _, tt := range tests {
			t.Run(b.blockType+"/"+tt.name, func(t *testing.T) {
				svc := &nightsStub{night: GameNight{SessionID: "s1", Name: "Hidden Title", Date: tonight, Time: "19:00", TZ: "UTC"}}
				if tt.cc.IsMember {
					svc.err = context.Canceled
				}
				stub := &placementStub{yes: map[string]bool{b.blockType + "|page-ok": true}}
				h := &Handler{svc: svc}
				h.SetPlacementChecker(stub)
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, b.route+tt.query, nil), rec)
				c.Set("campaign_context", tt.cc)
				if err := b.serve(h, c); err != nil {
					t.Fatal(err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("status %d", rec.Code)
				}
				if got := rec.Body.Len() > 0; got != tt.wantBody {
					t.Errorf("body present = %v, want %v:\n%s", got, tt.wantBody, rec.Body.String())
				}
				if svc.reads != tt.wantReads {
					t.Errorf("night reads = %d, want %d", svc.reads, tt.wantReads)
				}
				if tt.wantBody {
					if !strings.Contains(rec.Body.String(), "7 pm") {
						t.Errorf("a guest sees the time:\n%s", rec.Body.String())
					}
					if strings.Contains(rec.Body.String(), "Hidden Title") {
						t.Errorf("a guest must not see the night's title:\n%s", rec.Body.String())
					}
				}
				if (len(stub.calls) > 0) != tt.wantCall {
					t.Fatalf("placement asked = %v, want %v", len(stub.calls) > 0, tt.wantCall)
				}
				if tt.wantCall {
					want := placementCall{campaignID: "c1", blockType: b.blockType, on: strings.TrimPrefix(tt.query, "?on="), role: 0}
					if stub.calls[0] != want {
						t.Errorf("placement asked %+v, want %+v", stub.calls[0], want)
					}
				}
			})
		}
	}
}

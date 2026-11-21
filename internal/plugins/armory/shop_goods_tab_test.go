package armory

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Only a Director's request may ask the gallery for shop goods; for anyone
// else ?goods=1 is ignored, so the list a player or Scribe pages through never
// holds them.
func TestIndex_ShopGoodsViewIsDirectorOnly(t *testing.T) {
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c"}, MemberRole: campaigns.RoleOwner}
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		url  string
		want bool
	}{
		{"owner asks", owner, "/campaigns/c/armory?goods=1", true},
		{"owner does not ask", owner, "/campaigns/c/armory", false},
		{"co-DM asks", coDMContext(), "/campaigns/c/armory?goods=1", true},
		{"player asks", plainPlayerContext(), "/campaigns/c/armory?goods=1", false},
		{"scribe asks", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c"}, MemberRole: campaigns.RoleScribe}, "/campaigns/c/armory?goods=1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &shopGoodsCapture{}
			c, _ := newArmoryTestContext(tt.cc, http.MethodGet, tt.url)
			_ = NewHandler(svc).Index(c)
			if !svc.called {
				t.Fatal("ListItems was never called")
			}
			if svc.first.ShopGoods != tt.want {
				t.Errorf("ShopGoods = %v, want %v", svc.first.ShopGoods, tt.want)
			}
		})
	}
}

// shopGoodsCapture records the options of the gallery's own list call (the
// first) and stops there.
type shopGoodsCapture struct {
	fakeRoleCapturingService
	called bool
	first  ItemListOptions
}

func (f *shopGoodsCapture) ListItems(_ context.Context, _ string, _ int, _ string, o ItemListOptions) ([]ItemCard, int, error) {
	if !f.called {
		f.called, f.first = true, o
	}
	return nil, 0, errStopAfterCapture
}

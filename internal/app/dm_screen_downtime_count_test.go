package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
)

type downtimeAddons struct{ addons.AddonService }

func (downtimeAddons) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return true, nil
}

type downtimeStash struct {
	armory.StashService
	page *armory.StashesPageView
}

func (downtimeStash) IsDowntimeOpen(context.Context, string) (bool, error) { return false, nil }
func (s downtimeStash) StashesPage(context.Context, string, armory.Actor) (*armory.StashesPageView, error) {
	return s.page, nil
}

// Starting downtime puts waiting shop purchases through as well as moves, so
// the DM Screen's count (which words the "Start downtime?" question) must
// include both.
func TestDMDowntimeAdapter_CountsPurchasesAndMoves(t *testing.T) {
	tests := []struct {
		name      string
		page      *armory.StashesPageView
		wantCount int
	}{
		{"nothing waiting", &armory.StashesPageView{}, 0},
		{"moves only", &armory.StashesPageView{Pending: make([]armory.MoveLine, 2)}, 2},
		{"purchases only", &armory.StashesPageView{PendingPurchases: make([]armory.PurchaseRequestLine, 2)}, 2},
		{"both", &armory.StashesPageView{Pending: make([]armory.MoveLine, 1), PendingPurchases: make([]armory.PurchaseRequestLine, 3)}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &dmDowntimeAdapter{stash: downtimeStash{page: tt.page}, addons: downtimeAddons{}}
			_, n, ok, err := a.Downtime(context.Background(), "c1", dmscreen.Viewer{UserID: "u1", Role: 3})
			if err != nil || !ok {
				t.Fatalf("Downtime: ok=%v err=%v", ok, err)
			}
			if n != tt.wantCount {
				t.Errorf("count = %d, want %d", n, tt.wantCount)
			}
		})
	}
}

package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// deskTrashStub answers only what the desk's Trash provider reads.
type deskTrashStub struct {
	PageSafetyService
	pages  []TrashItem
	others []TrashedThing
	days   int
}

func (s deskTrashStub) ListTrash(context.Context, string) ([]TrashItem, error) { return s.pages, nil }
func (s deskTrashStub) ListOtherTrash(context.Context, string) []TrashedThing  { return s.others }
func (s deskTrashStub) RetentionDays(context.Context) int                      { return s.days }

func TestDeskProvider_Facts(t *testing.T) {
	pages := []TrashItem{{ID: "p1", Name: "Old Vess Tollhouse"}, {ID: "p2", Name: "Brother Aldous"}}
	others := []TrashedThing{{Kind: "map", Label: "Map", ID: "m1", Name: "Harbour"}}
	f, err := NewDeskProvider(deskTrashStub{pages: pages, others: others, days: 30}).Facts(context.Background(), campaigns.DeskRequest{CampaignID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Trash == nil || f.Trash.Items != 3 || f.Trash.Days != 30 {
		t.Errorf("trash facts = %+v, want 3 things kept 30 days", f.Trash)
	}
}

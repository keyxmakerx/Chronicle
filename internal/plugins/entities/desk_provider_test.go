package entities

import (
	"context"
	"testing"
	"time"

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

func TestDeskProvider_Fold(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	me := "owner-1"
	pages := []TrashItem{
		{ID: "p1", Name: "Old Vess Tollhouse", DeletedAt: now.Add(-48 * time.Hour), DeletedBy: &me},
		{ID: "p2", Name: "Brother Aldous", DeletedAt: now.Add(-24 * time.Hour), DeletedByName: "Sam"},
	}
	others := []TrashedThing{{Kind: "map", Label: "Map", ID: "m1", Name: "Harbour", DeletedAt: now.Add(-1 * time.Hour)}}
	p := NewDeskProvider(deskTrashStub{pages: pages, others: others, days: 30})
	f, err := p.Fold(context.Background(), campaigns.DeskRequest{CampaignID: "c1", OwnerID: me, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ lead, meta, url string }{
		{"Harbour", "Map, 30 days left", "/campaigns/c1/trash/map/m1/restore"},
		{"Brother Aldous", "removed by Sam, 29 days left", "/campaigns/c1/trash/p2/restore"},
		{"Old Vess Tollhouse", "removed by you, 28 days left", "/campaigns/c1/trash/p1/restore"},
	}
	if len(f.Rows) != len(want) {
		t.Fatalf("rows = %d, want %d (newest first)", len(f.Rows), len(want))
	}
	for i, w := range want {
		r := f.Rows[i]
		if r.Lead != w.lead || r.Meta != w.meta || r.Act == nil || r.Act.URL != w.url || r.Act.Method != "POST" {
			t.Errorf("row %d = %+v act=%+v, want %+v", i, r, r.Act, w)
		}
	}

	empty, _ := NewDeskProvider(deskTrashStub{days: 30}).Fold(context.Background(), campaigns.DeskRequest{CampaignID: "c1", Now: now})
	if empty.Note != "Trash is empty." || len(empty.Rows) != 0 {
		t.Errorf("an empty Trash says so: %+v", empty)
	}
}

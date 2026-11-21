package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

type visEntities struct {
	entities.EntityService
	page      *entities.Entity
	reconcile []int
}

func (v *visEntities) GetByID(context.Context, string) (*entities.Entity, error) {
	if v.page == nil {
		return nil, apperror.NewNotFound("entity not found")
	}
	return v.page, nil
}
func (v *visEntities) ReconcileShopGoodVisibility(_ context.Context, _, _ string, n int) error {
	v.reconcile = append(v.reconcile, n)
	return nil
}

type visRelations struct {
	relations.RelationService
	list []relations.Relation
}

func (v visRelations) ListByEntity(context.Context, string, string) ([]relations.Relation, error) {
	return v.list, nil
}

// A shop good is public while at least one public "sold by" listing remains;
// Director-only listings, other relation types and ordinary pages do not count.
func TestShopGoodVisibilityObserver(t *testing.T) {
	sold := func(dm bool) relations.Relation { return relations.Relation{RelationType: "sold by", DmOnly: dm} }
	tests := []struct {
		name string
		page *entities.Entity
		list []relations.Relation
		want []int // reconcile calls
	}{
		{"one public listing", &entities.Entity{CampaignID: "c", ShopGood: true}, []relations.Relation{sold(false), sold(true)}, []int{1}},
		{"only hidden listings", &entities.Entity{CampaignID: "c", ShopGood: true}, []relations.Relation{sold(true)}, []int{0}},
		{"last public listing removed", &entities.Entity{CampaignID: "c", ShopGood: true}, nil, []int{0}},
		{"other relation types do not count", &entities.Entity{CampaignID: "c", ShopGood: true}, []relations.Relation{{RelationType: "mentions"}}, []int{0}},
		{"an ordinary page is skipped", &entities.Entity{CampaignID: "c"}, []relations.Relation{sold(true)}, nil},
		{"a missing page is skipped", nil, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ents := &visEntities{page: tc.page}
			obs := shopGoodVisibilityObserver(ents, visRelations{list: tc.list})
			if err := obs(context.Background(), "c", "g"); err != nil {
				t.Fatal(err)
			}
			if len(ents.reconcile) != len(tc.want) || (len(tc.want) > 0 && ents.reconcile[0] != tc.want[0]) {
				t.Errorf("reconcile calls = %v, want %v", ents.reconcile, tc.want)
			}
		})
	}
}

// Sync hears nothing of a shop good: the live-event adapter drops its created
// and updated events, which also keeps them out of the change feed (that is
// recorded from the same bus).
func TestEntityEventAdapter_DropsShopGoods(t *testing.T) {
	rec := &feedRecorder{}
	bus := &countBus{}
	// The same wiring as production: the change feed records at the bus.
	a := &entityEventPublisherAdapter{bus: syncapi.NewRecordingEventBus(bus, rec)}
	good := &entities.Entity{ID: "g", CampaignID: "c", ShopGood: true}
	page := &entities.Entity{ID: "p", CampaignID: "c"}
	a.PublishEntityEvent("created", "c", "g", good)
	a.PublishEntityEvent("updated", "c", "g", good)
	if bus.n != 0 || rec.n != 0 {
		t.Fatalf("%d live events and %d change-feed rows for a shop good", bus.n, rec.n)
	}
	a.PublishEntityEvent("created", "c", "p", page)
	a.PublishEntityEvent("deleted", "c", "g", &entities.Entity{ID: "g", CampaignID: "c"})
	if bus.n != 2 || rec.n != 2 {
		t.Errorf("%d live events and %d feed rows for an ordinary page and a withdrawal, want 2 each", bus.n, rec.n)
	}
}

type countBus struct{ n int }

func (b *countBus) Publish(*ws.Message) { b.n++ }

type feedRecorder struct{ n int }

func (f *feedRecorder) Append(context.Context, string, string, string, string) (int64, error) {
	f.n++
	return int64(f.n), nil
}
func (f *feedRecorder) MarkGap(context.Context, string) error { return nil }

package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// suffixedShop is a campaign whose shop page took the slug
// hessa-ord-s-workshop-2: an older page of that name, in the trash, still
// holds the plain slug. GetBySlug is exact, as on the real service.
type suffixedShop struct{ entities.EntityService }

var saltreach = "saltreach-id"

var suffixedPages = []entities.Entity{
	{ID: "shop", CampaignID: "c", Name: "Hessa Ord's Workshop", Slug: "hessa-ord-s-workshop-2", TypeSlug: "shop", ParentID: &saltreach},
	{ID: "potion", CampaignID: "c", Name: "Healing Potion", Slug: "healing-potion", TypeSlug: "item"},
}

func (suffixedShop) GetBySlug(_ context.Context, c, slug string) (*entities.Entity, error) {
	for i := range suffixedPages {
		if suffixedPages[i].CampaignID == c && suffixedPages[i].Slug == slug {
			e := suffixedPages[i]
			return &e, nil
		}
	}
	return nil, apperror.NewNotFound("entity not found")
}

func (s suffixedShop) GetByNameSlug(ctx context.Context, c, slug string) (*entities.Entity, error) {
	if e, err := s.GetBySlug(ctx, c, slug); err == nil {
		return e, nil
	}
	return s.GetBySlug(ctx, c, slug+"-2")
}

type stockRels struct{ made []string }

func (r *stockRels) Create(_ context.Context, _, src, dst, typ, _, _ string, _ json.RawMessage, _ ...bool) (*relations.Relation, error) {
	r.made = append(r.made, src+" "+typ+" "+dst)
	return &relations.Relation{}, nil
}
func (r *stockRels) ListByEntity(context.Context, string, string) ([]relations.Relation, error) {
	return nil, nil
}
func (r *stockRels) UpdateMetadata(context.Context, int, json.RawMessage) error { return nil }
func (r *stockRels) Delete(context.Context, int) error                          { return nil }

// TestAIFindsAShopWhoseSlugHasASuffix: the AI's stock, room and storefront
// blocks name a nested shop with an apostrophe whose slug is not its name's
// slug; through aiPageNames each one reaches it.
func TestAIFindsAShopWhoseSlugHasASuffix(t *testing.T) {
	ctx := context.Background()
	pages := aiPageNames{suffixedShop{}}
	owner := records.Actor{UserID: "o", Role: 3}
	for _, name := range []string{"Hessa Ord's Workshop", "Hessa Ord’s Workshop"} {
		rels := &stockRels{}
		stock := records.ShopStockKind(pages, rels, nil)
		rec := records.Record{Kind: "shop-stock", Action: records.ActionCreate, Fields: map[string]any{"shop": name, "item": "Healing Potion", "price": 5.0}}
		if p := stock.Plan(ctx, "c", owner, rec); p.Error != "" || len(p.Warnings) != 0 {
			t.Fatalf("%s: plan %+v", name, p)
		}
		if err := stock.Apply(ctx, "c", owner, rec); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(rels.made) != 1 || rels.made[0] != "shop sells potion" {
			t.Fatalf("%s: links %v", name, rels.made)
		}
	}

	rooms, fronts := &shopRoomsFake{rooms: map[string]json.RawMessage{}}, &shopFrontsFake{fronts: map[string]json.RawMessage{}}
	looks := aiShopLooksAdapter{rooms: rooms, fronts: fronts}
	room := records.NewShopRoomKind(looks, pages, ownsAll{})
	front := records.NewStorefrontKind(looks, pages, ownsAll{})
	shop := func(kind string, f map[string]any) records.Record {
		f["shop"] = "Hessa Ord's Workshop"
		return records.Record{Kind: kind, Action: records.ActionUpdate, Fields: f}
	}
	if err := room.Apply(ctx, "c", owner, shop("shop-room", map[string]any{"type": "apothecary"})); err != nil || rooms.rooms["shop"] == nil {
		t.Fatalf("room: %v", err)
	}
	if err := front.Apply(ctx, "c", owner, shop("storefront", map[string]any{"style": "painted"})); err != nil || fronts.fronts["shop"] == nil {
		t.Fatalf("storefront: %v", err)
	}

	// Without aiPageNames the exact slug lookup misses the shop, which is
	// the failure this adapter exists for.
	bare := records.ShopStockKind(suffixedShop{}, &stockRels{}, nil)
	rec := records.Record{Kind: "shop-stock", Action: records.ActionCreate, Fields: map[string]any{"shop": "Hessa Ord's Workshop", "item": "Healing Potion"}}
	if err := bare.Apply(ctx, "c", owner, rec); err == nil {
		t.Fatal("an exact slug lookup found the suffixed shop; the test no longer shows the bug")
	}
}

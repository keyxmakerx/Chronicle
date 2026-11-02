package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// typesOnlySvc answers GetEntityTypes with a fixed list.
type typesOnlySvc struct {
	entities.EntityService
	types []entities.EntityType
}

func (s *typesOnlySvc) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return s.types, nil
}

// The Items category a campaign starts with has no preset category, because
// no game system made it; the Armory must still count its pages as items.
func TestArmoryItemTypeFinder_CountsTheStartingItemsCategory(t *testing.T) {
	item := "item"
	svc := &typesOnlySvc{types: []entities.EntityType{
		{ID: 1, Slug: "character", Name: "Character"},
		{ID: 4, Slug: "item", Name: "Item"},
		{ID: 9, Slug: "dnd5e-magic-item", Name: "Magic Item", PresetCategory: &item},
	}}
	a := &armoryItemTypeFinderAdapter{svc: svc}
	ids, err := a.FindItemTypeIDs(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 4 || ids[1] != 9 {
		t.Fatalf("item type ids %v, want [4 9]", ids)
	}
	infos, err := a.FindItemTypes(context.Background(), "c1")
	if err != nil || len(infos) != 2 || infos[0].Name != "Item" {
		t.Fatalf("item types %+v, %v", infos, err)
	}
}

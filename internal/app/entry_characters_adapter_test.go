package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type entryCharEntities struct {
	entities.EntityService
	pages []entities.Entity
}

func (f entryCharEntities) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	for i := range f.pages {
		if f.pages[i].ID == id {
			return &f.pages[i], nil
		}
	}
	return nil, nil
}

func (f entryCharEntities) SearchRoster(_ context.Context, campaignID string, typeIDs []int, _ entities.RosterSearch, _ int, _ string) ([]entities.Entity, error) {
	var out []entities.Entity
	for _, p := range f.pages {
		for _, t := range typeIDs {
			if p.CampaignID == campaignID && p.EntityTypeID == t {
				out = append(out, p)
			}
		}
	}
	return out, nil
}

type entryCharLists []int

func (l entryCharLists) CharacterTypeIDs(context.Context, string) ([]int, error) { return l, nil }

func TestEntryCharactersAdapter(t *testing.T) {
	tyne := "u-tyne"
	a := entryCharactersAdapter{
		entities: entryCharEntities{pages: []entities.Entity{
			{ID: "wren", CampaignID: "c1", EntityTypeID: 2, Name: "Wren", OwnerUserID: &tyne},
			{ID: "inn", CampaignID: "c1", EntityTypeID: 5, Name: "The Inn"},
			{ID: "far", CampaignID: "c2", EntityTypeID: 2, Name: "Far"},
			{ID: "twin1", CampaignID: "c1", EntityTypeID: 2, Name: "Twin"},
			{ID: "twin2", CampaignID: "c1", EntityTypeID: 2, Name: "twin"},
		}},
		lists: entryCharLists{2},
	}
	ctx := context.Background()
	tests := []struct {
		name, id, wantOwner string
		want                bool
	}{
		{"claimed character", "wren", tyne, true},
		{"not a character type", "inn", "", false},
		{"another campaign's page", "far", "", false},
		{"missing", "ghost", "", false},
		{"empty id", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := a.Character(ctx, "c1", tt.id)
			if err != nil {
				t.Fatal(err)
			}
			if (c != nil) != tt.want || (c != nil && c.OwnerUserID != tt.wantOwner) {
				t.Fatalf("Character(%q) = %+v", tt.id, c)
			}
		})
	}
	if c, _ := a.CharacterNamed(ctx, "c1", " WREN "); c == nil || c.ID != "wren" {
		t.Fatalf("by name = %+v", c)
	}
	if c, _ := a.CharacterNamed(ctx, "c1", "Twin"); c != nil {
		t.Fatalf("an ambiguous name matched %+v", c)
	}
	if c, _ := a.CharacterNamed(ctx, "c1", "The Inn"); c != nil {
		t.Fatalf("a non-character matched %+v", c)
	}
}

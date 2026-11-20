package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// peopleEntities serves a roster whose character types may mark the kind
// field or its bought traits GM-only or owner-only.
type peopleEntities struct {
	entities.EntityService
	pages     []entities.Entity
	types     map[int]*entities.EntityType
	typeReads int
}

func (f *peopleEntities) List(_ context.Context, campaignID string, typeID int, _ int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	var out []entities.Entity
	for _, p := range f.pages {
		if p.CampaignID == campaignID && p.EntityTypeID == typeID {
			out = append(out, p)
		}
	}
	return out, len(out), nil
}

func (f *peopleEntities) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	for i := range f.pages {
		if f.pages[i].ID == id {
			return &f.pages[i], nil
		}
	}
	return nil, nil
}

func (f *peopleEntities) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true}, nil
}

func (f *peopleEntities) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	f.typeReads++
	return f.types[id], nil
}

func TestEntryPeopleAdapter_RestrictedFields(t *testing.T) {
	mira, tyne := "u-mira", "u-tyne"
	fields := func(kind, traits string) map[string]any {
		return map[string]any{"ancestry": kind, "ancestry_choices_json": traits}
	}
	open := &entities.EntityType{ID: 1, Fields: []entities.FieldDefinition{{Key: "ancestry"}, {Key: "ancestry_choices_json"}}}
	gmKind := &entities.EntityType{ID: 2, Fields: []entities.FieldDefinition{{Key: "ancestry", GMOnly: true}, {Key: "ancestry_choices_json"}}}
	gmTraits := &entities.EntityType{ID: 3, Fields: []entities.FieldDefinition{{Key: "ancestry"}, {Key: "ancestry_choices_json", GMOnly: true}}}
	ownerKind := &entities.EntityType{ID: 4, Fields: []entities.FieldDefinition{{Key: "ancestry", OwnerOnly: true}, {Key: "ancestry_choices_json", OwnerOnly: true}}}

	page := func(id string, typeID int, owner *string, added ...entities.FieldDefinition) entities.Entity {
		e := entities.Entity{ID: id, CampaignID: "c1", EntityTypeID: typeID, Name: id, OwnerUserID: owner, FieldsData: fields("Dwarf", `["Grounded"]`)}
		if len(added) > 0 {
			e.FieldOverrides = &entities.FieldOverrides{Added: added}
		}
		return e
	}
	player := systems.PeopleReader{UserID: mira, Role: int(campaigns.RolePlayer)}
	director := systems.PeopleReader{UserID: "u-dir", Role: int(campaigns.RoleOwner)}

	tests := []struct {
		name       string
		page       entities.Entity
		reader     systems.PeopleReader
		wantPerson bool
		wantTraits bool
	}{
		{"open fields reach a player", page("p-open", 1, &tyne), player, true, true},
		{"GM-only kind is not a player's", page("p-gmkind", 2, &tyne), player, false, false},
		{"GM-only kind is the Director's", page("p-gmkind", 2, &tyne), director, true, true},
		{"GM-only traits keep the kind, drop the traits", page("p-gmtraits", 3, &tyne), player, true, false},
		{"owner-only kind hidden from another player", page("p-owner", 4, &tyne), player, false, false},
		{"owner-only kind served to its owner", page("p-owner", 4, &mira), player, true, true},
		{"field added GM-only on this page alone", page("p-added", 1, &tyne, entities.FieldDefinition{Key: "ancestry", GMOnly: true}), player, false, false},
		{"unreadable category hides everything", page("p-notype", 9, &tyne), player, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := &peopleEntities{pages: []entities.Entity{tt.page}, types: map[int]*entities.EntityType{1: open, 2: gmKind, 3: gmTraits, 4: ownerKind}}
			a := entryPeopleAdapter{entities: ents, lists: entryCharLists{tt.page.EntityTypeID}}
			ctx := context.Background()

			list, err := a.People(ctx, "c1", "ancestry", tt.reader, 500)
			if err != nil {
				t.Fatal(err)
			}
			if (len(list) == 1) != tt.wantPerson {
				t.Fatalf("People = %+v, want person %v", list, tt.wantPerson)
			}
			if tt.wantPerson && (len(list[0].Traits) > 0) != tt.wantTraits {
				t.Fatalf("People traits = %v, want traits %v", list[0].Traits, tt.wantTraits)
			}

			one, err := a.Person(ctx, "c1", tt.page.ID, "ancestry", tt.reader)
			if err != nil {
				t.Fatal(err)
			}
			if (one != nil) != tt.wantPerson {
				t.Fatalf("Person = %+v, want person %v", one, tt.wantPerson)
			}
			if one != nil && (len(one.Traits) > 0) != tt.wantTraits {
				t.Fatalf("Person traits = %v, want traits %v", one.Traits, tt.wantTraits)
			}
		})
	}
}

// One category read per type, however many characters share it.
func TestEntryPeopleAdapter_ReadsEachTypeOnce(t *testing.T) {
	tyne := "u-tyne"
	ents := &peopleEntities{types: map[int]*entities.EntityType{1: {ID: 1, Fields: []entities.FieldDefinition{{Key: "ancestry"}}}}}
	for i := 0; i < 50; i++ {
		ents.pages = append(ents.pages, entities.Entity{ID: string(rune('a'+i%26)) + string(rune('a'+i/26)), CampaignID: "c1", EntityTypeID: 1, Name: "x", OwnerUserID: &tyne, FieldsData: map[string]any{"ancestry": "Dwarf"}})
	}
	a := entryPeopleAdapter{entities: ents, lists: entryCharLists{1}}
	list, err := a.People(context.Background(), "c1", "ancestry", systems.PeopleReader{UserID: "u-mira", Role: int(campaigns.RolePlayer)}, 500)
	if err != nil || len(list) != 50 {
		t.Fatalf("People = %d, %v", len(list), err)
	}
	if ents.typeReads != 1 {
		t.Fatalf("category read %d times, want 1", ents.typeReads)
	}
}

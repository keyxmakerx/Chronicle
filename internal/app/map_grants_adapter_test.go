package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

type grantsEntities struct {
	entities.EntityService
	owned map[string][]entities.Entity // user -> characters
	byID  map[string]*entities.Entity
}

func (g grantsEntities) ListByOwner(_ context.Context, _, user string) ([]entities.Entity, error) {
	return g.owned[user], nil
}
func (g grantsEntities) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	return g.byID[id], nil
}

type grantsRelations struct {
	relations.RelationService
	byEntity map[string][]relations.Relation
}

func (g grantsRelations) ListByEntity(_ context.Context, _, id string) ([]relations.Relation, error) {
	return g.byEntity[id], nil
}

type grantsMembers []campaigns.CampaignMember

func (g grantsMembers) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return g, nil
}

// codm holds a co-DM grant.
func (g grantsMembers) IsUserDmGranted(_ context.Context, _, user string) (bool, error) {
	return user == "codm", nil
}

func handout(id, mapID, marker string) *entities.Entity {
	return &entities.Entity{ID: id, CampaignID: "c1", Name: "Map: " + mapID, MapID: &mapID,
		FieldsData: map[string]any{handoutMarkerField: marker}}
}

// A Player sees a map while a character they own holds its handout on a line
// they can see: not a Director-only line, not an item merely pointed at the
// map, not another campaign's page.
func TestMapGrantsAdapter(t *testing.T) {
	plain := handout("i-plain", "m3", "")
	foreign := handout("i-foreign", "m4", "m4")
	foreign.CampaignID = "c2"
	ents := grantsEntities{
		owned: map[string][]entities.Entity{"mira": {{ID: "hero"}}, "bram": {{ID: "bram-hero"}}},
		byID: map[string]*entities.Entity{
			"i-m1": handout("i-m1", "m1", "m1"), "i-m2": handout("i-m2", "m2", "m2"),
			"i-plain": plain, "i-foreign": foreign,
		},
	}
	rel := func(target, name string, dmOnly bool) relations.Relation {
		return relations.Relation{RelationType: hasItemType, TargetEntityID: target, TargetEntityName: name, DmOnly: dmOnly, CreatedBy: "gm"}
	}
	rels := grantsRelations{byEntity: map[string][]relations.Relation{
		"hero": {
			rel("i-m1", "Map: m1", false),
			rel("i-m2", "Map: m2", true), // a Director-only line
			rel("i-plain", "Map: m3", false),
			rel("i-foreign", "Map: m4", false),
			{RelationType: "Knows", TargetEntityID: "i-m2", TargetEntityName: "Map: m2"},
		},
	}}
	a := &mapGrantsAdapter{entities: ents, relations: rels, members: grantsMembers{
		{UserID: "mira", Role: campaigns.RolePlayer}, {UserID: "bram", Role: campaigns.RolePlayer},
		{UserID: "sam", Role: campaigns.RoleScribe}, {UserID: "gm", Role: campaigns.RoleOwner},
		{UserID: "codm", Role: campaigns.RolePlayer},
	}}

	got, err := a.GivenMapIDs(context.Background(), "c1", "mira")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got["m1"] {
		t.Fatalf("mira was given %v, want only m1", got)
	}
	if got, _ := a.GivenMapIDs(context.Background(), "c1", "bram"); len(got) != 0 {
		t.Fatalf("bram was given %v, want none", got)
	}
	if got, _ := a.GivenMapIDs(context.Background(), "c1", ""); len(got) != 0 {
		t.Fatalf("no user was given %v, want none", got)
	}
	holders, err := a.MapHolders(context.Background(), "c1", "m1")
	if err != nil || len(holders) != 1 || holders[0] != "mira" {
		t.Fatalf("holders of m1 = %v %v, want [mira]", holders, err)
	}
}

// Only a line the Director made is a give. Anyone who can edit a character
// can add a Has Item line by hand: a Scribe, or the player re-adding a line
// the GM took back. Neither may show the map.
func TestMapGrantsAdapter_OnlyTheDirectorGives(t *testing.T) {
	ents := grantsEntities{
		owned: map[string][]entities.Entity{"mira": {{ID: "hero"}}},
		byID:  map[string]*entities.Entity{"i-m1": handout("i-m1", "m1", "m1")},
	}
	members := grantsMembers{
		{UserID: "mira", Role: campaigns.RolePlayer}, {UserID: "sam", Role: campaigns.RoleScribe},
		{UserID: "gm", Role: campaigns.RoleOwner}, {UserID: "codm", Role: campaigns.RolePlayer},
	}
	cases := []struct {
		name, creator string
		want          bool
	}{
		{"the Owner's give", "gm", true},
		{"a co-DM's give", "codm", true},
		{"a line a Scribe added by hand", "sam", false},
		{"the player re-adding the line", "mira", false},
		{"a creator who left the campaign", "gone", false},
		{"no recorded creator", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rels := grantsRelations{byEntity: map[string][]relations.Relation{"hero": {{
				RelationType: hasItemType, TargetEntityID: "i-m1", TargetEntityName: "Map: m1", CreatedBy: tc.creator,
			}}}}
			a := &mapGrantsAdapter{entities: ents, relations: rels, members: members}
			got, err := a.GivenMapIDs(context.Background(), "c1", "mira")
			if err != nil {
				t.Fatal(err)
			}
			if got["m1"] != tc.want {
				t.Fatalf("map shown = %v, want %v", got["m1"], tc.want)
			}
		})
	}
}

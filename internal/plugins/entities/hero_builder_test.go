package entities

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestBuilderHeroes pins which of the viewer's pages the hero builder fills
// in and what it shows of each: only character types, and for a player no
// GM-only value, from the type or from the page's own added fields.
func TestBuilderHeroes(t *testing.T) {
	types := []EntityType{
		{ID: 1, Name: "Hero", Fields: []FieldDefinition{{Key: "class"}, {Key: "secret", GMOnly: true}}},
		{ID: 2, Name: "Location"},
	}
	hero := Entity{ID: "e1", Name: "Tess", EntityTypeID: 1, FieldsData: map[string]any{
		"class": "Fury", "secret": "Owes the Board", "level": float64(3),
	}}
	withAdded := hero
	withAdded.FieldOverrides = &FieldOverrides{Added: []FieldDefinition{{Key: "class", GMOnly: true}}}
	place := Entity{ID: "e2", Name: "The Ford", EntityTypeID: 2, FieldsData: map[string]any{"class": "x"}}

	tests := []struct {
		name  string
		mine  []Entity
		staff bool
		want  []BuilderHero
	}{
		{"player: GM-only left out", []Entity{hero}, false,
			[]BuilderHero{{ID: "e1", Name: "Tess", Values: map[string]string{"class": "Fury"}, Hidden: []string{"secret"}}}},
		{"staff: everything shown", []Entity{hero}, true,
			[]BuilderHero{{ID: "e1", Name: "Tess", Values: map[string]string{"class": "Fury", "secret": "Owes the Board"}, Hidden: []string{}}}},
		{"page's own GM-only field", []Entity{withAdded}, false,
			[]BuilderHero{{ID: "e1", Name: "Tess", Values: map[string]string{}, Hidden: []string{"class", "secret"}}}},
		{"not a character type", []Entity{place}, false, []BuilderHero{}},
		{"none", nil, false, []BuilderHero{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mine []CastMember
			for _, e := range tc.mine {
				mine = append(mine, CastMember{Entity: e, IsViewer: true})
			}
			got := builderHeroes(context.Background(), mine, []int{1}, types, tc.staff)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestHeroBuilderView_HeroesJSON pins the mount's wire shape, which
// hero_builder.js reads as id, name, values, hidden and sheet.
func TestHeroBuilderView_HeroesJSON(t *testing.T) {
	v := &HeroBuilderView{Heroes: []BuilderHero{{ID: "e1", Name: "Tess", Values: map[string]string{"class": "Fury"}, Hidden: []string{}, Sheet: true}}}
	got := v.HeroesJSON()
	want := `[{"id":"e1","name":"Tess","values":{"class":"Fury"},"hidden":[],"sheet":true}]`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// planAccessSvc answers the two reads the plan's access check makes.
type planAccessSvc struct {
	EntityService
	types []EntityType
	owned []Entity
}

func (s *planAccessSvc) GetEntityTypes(_ context.Context, _ string) ([]EntityType, error) {
	return s.types, nil
}

func (s *planAccessSvc) ListByOwner(_ context.Context, _, _ string) ([]Entity, error) {
	return s.owned, nil
}

type planAccessLists struct {
	CharacterListManager
	ids []int
}

func (l planAccessLists) CharacterTypeIDs(_ context.Context, _ string) ([]int, error) {
	return l.ids, nil
}

// TestHeroPlanAPI_Access pins who below Director may read the plan: whoever
// may make a hero, and a player who already has one, whose builder fills it
// in from the same lists. Anyone else is refused.
func TestHeroPlanAPI_Access(t *testing.T) {
	yes, no := true, false
	claimable := EntityType{ID: 1, Name: "Hero", Enabled: true, Claimable: &yes}
	closed := EntityType{ID: 1, Name: "Hero", Enabled: true, Claimable: &no}
	place := EntityType{ID: 2, Name: "Location", Enabled: true}
	tests := []struct {
		name   string
		role   campaigns.Role
		types  []EntityType
		owned  []Entity
		status int
	}{
		{"player who may make a hero", campaigns.RolePlayer, []EntityType{claimable}, nil, http.StatusOK},
		{"player who already has one", campaigns.RolePlayer, []EntityType{claimable}, []Entity{{ID: "e1", EntityTypeID: 1}}, http.StatusOK},
		{"player with a hero players may no longer claim", campaigns.RolePlayer, []EntityType{closed}, []Entity{{ID: "e1", EntityTypeID: 1}}, http.StatusOK},
		{"player with no hero and none to make", campaigns.RolePlayer, []EntityType{closed}, nil, http.StatusForbidden},
		{"player who owns only a place", campaigns.RolePlayer, []EntityType{closed, place}, []Entity{{ID: "e2", EntityTypeID: 2}}, http.StatusForbidden},
		{"not a member", campaigns.RoleNone, []EntityType{claimable}, []Entity{{ID: "e1", EntityTypeID: 1}}, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{
				service:     &planAccessSvc{types: tc.types, owned: tc.owned},
				charLists:   planAccessLists{ids: []int{1}},
				heroPlanner: &stubPlanner{plan: &HeroPlan{Steps: []HeroStep{}}},
			}
			err := runIdentityCallOn(t, h, (*Handler).HeroPlanAPI, tc.role, "u1", "c1", "")
			wantStatus(t, err, tc.status)
		})
	}
}

package entities

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestHeroAccessFor(t *testing.T) {
	no := false
	claimable := &EntityType{ID: 1, Slug: "drawsteel-character", Enabled: true}
	locked := &EntityType{ID: 2, Slug: "drawsteel-character", Claimable: &no, Enabled: true}
	tests := []struct {
		name     string
		role     campaigns.Role
		claiming bool
		target   *EntityType
		owns     bool
		want     heroAccess
	}{
		{"no type", campaigns.RoleOwner, true, nil, false, heroAccess{}},
		{"scribe makes heroes for the table", campaigns.RoleScribe, false, locked, true, heroAccess{Allowed: true}},
		{"owner", campaigns.RoleOwner, false, claimable, false, heroAccess{Allowed: true}},
		{"player with claiming on", campaigns.RolePlayer, true, claimable, false, heroAccess{Allowed: true, ForSelf: true}},
		{"player who already has a hero", campaigns.RolePlayer, true, claimable, true, heroAccess{}},
		{"player with claiming off", campaigns.RolePlayer, false, claimable, false, heroAccess{}},
		{"player and an unclaimable type", campaigns.RolePlayer, true, locked, false, heroAccess{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := heroAccessFor(tt.role, tt.claiming, tt.target, tt.owns); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPickHeroType(t *testing.T) {
	no := false
	types := []EntityType{
		{ID: 1, Slug: "npc", Enabled: true},
		{ID: 2, Slug: "player-character", PresetCategory: strPtr(PresetCategoryPlayerCharacter), Claimable: &no, Enabled: true},
		{ID: 3, Slug: "drawsteel-hero", PresetCategory: strPtr("character"), Enabled: true},
		{ID: 4, Slug: "off-character", PresetCategory: strPtr("character"), Enabled: false},
	}
	tests := []struct {
		name          string
		ids           []int
		claimableOnly bool
		want          int
	}{
		{"system character first", []int{1, 2, 3}, false, 3},
		{"then the player character", []int{1, 2}, false, 2},
		{"then the first listed", []int{1}, false, 1},
		{"disabled types are skipped", []int{4}, false, 0},
		{"players only get claimable types", []int{1, 2}, true, 0},
		{"nothing listed", nil, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickHeroType(types, tt.ids, tt.claimableOnly)
			id := 0
			if got != nil {
				id = got.ID
			}
			if id != tt.want {
				t.Errorf("got type %d, want %d", id, tt.want)
			}
		})
	}
}

func ancestryProps() map[string]any {
	return map[string]any{
		"ancestry_points": float64(3),
		"signature_traits": []any{
			map[string]any{"name": "Grounded"},
		},
		"purchased_traits": []any{
			map[string]any{"name": "Spark Off Your Skin", "cost": float64(2)},
			map[string]any{"name": "Great Fortitude", "cost": float64(2)},
			map[string]any{"name": "Durable", "cost": float64(1)},
		},
	}
}

func TestHeroBuyList(t *testing.T) {
	costs, budget, ok := heroBuyList(ancestryProps())
	if !ok || budget != 3 || len(costs) != 3 || costs["durable"] != 1 {
		t.Fatalf("got %v %v %v", costs, budget, ok)
	}
	if _, _, ok := heroBuyList(map[string]any{"purchased_traits": ancestryProps()["purchased_traits"]}); ok {
		t.Error("a list without a budget can't be bought from")
	}
	negative := map[string]any{"ancestry_points": float64(3), "traits": []any{map[string]any{"name": "Free lunch", "cost": float64(-5)}}}
	if _, _, ok := heroBuyList(negative); ok {
		t.Error("a negative cost must not make a buy list")
	}
	if _, _, ok := heroBuyList(map[string]any{"ancestry_points": float64(3), "notes": []any{"a", "b"}}); ok {
		t.Error("a list of plain strings is not a buy list")
	}
}

func TestBuildHeroFields(t *testing.T) {
	plan := &HeroPlan{Steps: []HeroStep{
		{FieldKey: "ancestry", Label: "Ancestry", Choices: []HeroChoice{
			{Name: "Dwarf", Properties: ancestryProps()},
			{Name: "Human"},
		}},
		{FieldKey: "kit", Label: "Kit", Choices: []HeroChoice{{Name: "Mountain"}}},
	}}
	tests := []struct {
		name    string
		picks   map[string]HeroPick
		want    map[string]any
		wantErr string
	}{
		{"names are stored as the entry spells them",
			map[string]HeroPick{"ancestry": {Name: "dwarf"}, "kit": {Name: "Mountain"}},
			map[string]any{"ancestry": "Dwarf", "kit": "Mountain"}, ""},
		{"bought options go beside the entry",
			map[string]HeroPick{"ancestry": {Name: "Dwarf", Options: []string{"Spark Off Your Skin", "Durable"}}},
			map[string]any{"ancestry": "Dwarf", "ancestry_choices_json": `["Spark Off Your Skin","Durable"]`}, ""},
		{"skipped steps are left blank",
			map[string]HeroPick{"kit": {}},
			map[string]any{}, ""},
		{"unknown step", map[string]HeroPick{"class": {Name: "Fury"}}, nil, "no step"},
		{"unknown entry", map[string]HeroPick{"ancestry": {Name: "Elf"}}, nil, "not on the ancestry list"},
		{"options without an entry", map[string]HeroPick{"ancestry": {Options: []string{"Durable"}}}, nil, "before its options"},
		{"entry with nothing to buy", map[string]HeroPick{"ancestry": {Name: "Human", Options: []string{"Durable"}}}, nil, "nothing to choose"},
		{"over budget", map[string]HeroPick{"ancestry": {Name: "Dwarf", Options: []string{"Spark Off Your Skin", "Great Fortitude"}}}, nil, "cost 4 points"},
		{"bought twice", map[string]HeroPick{"ancestry": {Name: "Dwarf", Options: []string{"Durable", "durable"}}}, nil, "can't be chosen"},
		{"unknown option", map[string]HeroPick{"ancestry": {Name: "Dwarf", Options: []string{"Wings"}}}, nil, "can't be chosen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildHeroFields(plan, tt.picks)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("%s = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

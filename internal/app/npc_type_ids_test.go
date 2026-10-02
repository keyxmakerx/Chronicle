package app

import (
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestNPCTypeIDs(t *testing.T) {
	str := func(s string) *string { return &s }
	id := func(i int) *int { return &i }

	tests := []struct {
		name  string
		types []entities.EntityType
		want  []int
	}{
		{
			name: "default seed: character only, not location or item",
			types: []entities.EntityType{
				{ID: 1, Slug: "character", Enabled: true},
				{ID: 2, Slug: "location", Enabled: true},
				{ID: 4, Slug: "item", PresetCategory: str("item"), Enabled: true},
			},
			want: []int{1},
		},
		{
			name: "player-character sub-type is left out, other sub-types kept",
			types: []entities.EntityType{
				{ID: 1, Slug: "character", Enabled: true},
				{ID: 2, Slug: "player-character", PresetCategory: str("player_character"), ParentTypeID: id(1), Enabled: true},
				{ID: 3, Slug: "villain", ParentTypeID: id(1), Enabled: true},
			},
			want: []int{1, 3},
		},
		{
			name: "genre and system-pack types",
			types: []entities.EntityType{
				{ID: 5, Slug: "npc", Enabled: true},
				{ID: 6, Slug: "creature", Enabled: true},
				{ID: 7, Slug: "drawsteel-monster", Enabled: true},
				{ID: 8, Slug: "dnd5e-character", Enabled: true},
				{ID: 9, Slug: "beast", PresetCategory: str("creature"), Enabled: true},
			},
			want: []int{5, 6, 7, 8, 9},
		},
		{
			name: "disabled type is left out",
			types: []entities.EntityType{
				{ID: 1, Slug: "character", Enabled: false},
			},
			want: nil,
		},
		{
			name: "parent cycle does not loop",
			types: []entities.EntityType{
				{ID: 1, Slug: "a", ParentTypeID: id(2), Enabled: true},
				{ID: 2, Slug: "b", ParentTypeID: id(1), Enabled: true},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := npcTypeIDs(tt.types); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("npcTypeIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

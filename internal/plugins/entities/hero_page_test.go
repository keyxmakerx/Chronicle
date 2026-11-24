package entities

import "testing"

func TestIsHeroPage(t *testing.T) {
	owner := "u1"
	keys := []string{"class", "ancestry"}
	tests := []struct {
		name   string
		e      *Entity
		pcType bool
		want   bool
	}{
		{"nil page", nil, false, false},
		{"plain NPC", &Entity{FieldsData: map[string]any{"role": "Smith"}}, false, false},
		{"claimed by a player", &Entity{OwnerUserID: &owner}, false, true},
		{"marked playable", &Entity{Playable: true}, false, true},
		{"player character sub-type", &Entity{}, true, true},
		{"class picked", &Entity{FieldsData: map[string]any{"class": "Tactician"}}, false, true},
		{"hero choice picked", &Entity{FieldsData: map[string]any{"ancestry": "Human"}}, false, true},
		{"blank class", &Entity{FieldsData: map[string]any{"class": "  "}}, false, false},
		{"pick key not listed", &Entity{FieldsData: map[string]any{"kit": "Panther"}}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHeroPage(tt.e, tt.pcType, keys); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// Importing must never move the DM-only marker between categories through a
// shared slug: an ordinary category would vanish for players, or Adventures
// pages would appear for them.
func TestMatchExistingType_KeepsTheDMOnlyMarkerWhereItWas(t *testing.T) {
	adv := entities.PresetCategoryAdventures
	seeded := entities.EntityType{ID: 9, Slug: "adventures", PresetCategory: &adv}
	handMade := entities.EntityType{ID: 5, Slug: "adventures"}

	tests := []struct {
		name     string
		existing []entities.EntityType
		file     campaigns.ExportEntityType
		wantID   int
	}{
		{"Adventures finds the seeded Adventures", []entities.EntityType{seeded}, campaigns.ExportEntityType{Slug: "adventures", PresetCategory: &adv}, 9},
		{"Adventures never becomes a hand-made category with the same slug", []entities.EntityType{handMade}, campaigns.ExportEntityType{Slug: "adventures", PresetCategory: &adv}, 0},
		{"Adventures renamed in the file still finds the seeded one", []entities.EntityType{seeded}, campaigns.ExportEntityType{Slug: "my-quests", PresetCategory: &adv}, 9},
		{"an ordinary category never takes over the seeded Adventures by slug", []entities.EntityType{seeded}, campaigns.ExportEntityType{Slug: "adventures"}, 0},
		{"an ordinary category still matches by slug", []entities.EntityType{handMade}, campaigns.ExportEntityType{Slug: "adventures"}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchExistingType(tt.existing, map[int]bool{}, tt.file)
			switch {
			case tt.wantID == 0 && got != nil:
				t.Fatalf("matched type %d, want none", got.ID)
			case tt.wantID != 0 && (got == nil || got.ID != tt.wantID):
				t.Fatalf("matched %+v, want type %d", got, tt.wantID)
			}
		})
	}
}

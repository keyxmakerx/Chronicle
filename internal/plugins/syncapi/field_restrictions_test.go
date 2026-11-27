package syncapi

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type failingTypesSvc struct{ entities.EntityService }

func (failingTypesSvc) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return nil, errors.New("db down")
}

func heroExport() *systems.CharacterFieldsResponse {
	return &systems.CharacterFieldsResponse{
		PresetSlug: "drawsteel-character",
		Fields: []systems.CharacterFieldExport{
			{Key: "wealth", Type: "number", FoundryPath: "system.hero.wealth", FoundryWritable: true},
			{Key: "renown", Type: "number", FoundryPath: "system.hero.renown", FoundryWritable: true},
			{Key: "age", Type: "string", FoundryPath: "system.biography.age", FoundryWritable: true},
			{Key: "gm_notes", Type: "markdown", GMOnly: true, FoundryClearPath: "system.biography.director"},
		},
	}
}

// A field the campaign's Director marked GM-only on their own category goes
// to the Foundry module unmapped, like one the manifest marks; one marked
// owner-only keeps its mapping and says so. A sub-category's marks count, and
// another category's do not.
func TestRestrictToCampaign(t *testing.T) {
	character := "character"
	types := []entities.EntityType{
		{ID: 1, Slug: "drawsteel-character", PresetCategory: &character, Fields: []entities.FieldDefinition{
			{Key: "wealth", GMOnly: true},
		}},
		{ID: 2, Slug: "player-characters", ParentTypeID: intPtr(1), Fields: []entities.FieldDefinition{
			{Key: "age", OwnerOnly: true},
		}},
		{ID: 3, Slug: "locations", Fields: []entities.FieldDefinition{
			{Key: "renown", GMOnly: true},
		}},
	}
	h := NewAPIHandler(nil, &stubTypesSvc{types: types}, nil, nil)
	resp := heroExport()
	if err := h.restrictToCampaign(context.Background(), "c1", "character", resp); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]systems.CharacterFieldExport{}
	for _, f := range resp.Fields {
		byKey[f.Key] = f
	}
	tests := []struct {
		key                 string
		gm, owner, writable bool
		path, clearPath     string
	}{
		{"wealth", true, false, false, "", "system.hero.wealth"},
		{"renown", false, false, true, "system.hero.renown", ""},
		{"age", false, true, true, "system.biography.age", ""},
		{"gm_notes", true, false, false, "", "system.biography.director"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			f := byKey[tt.key]
			if f.GMOnly != tt.gm || f.OwnerOnly != tt.owner || f.FoundryWritable != tt.writable ||
				f.FoundryPath != tt.path || f.FoundryClearPath != tt.clearPath {
				t.Errorf("%s exported as %+v", tt.key, f)
			}
		})
	}
}

// Without the campaign's categories nothing says which fields are hidden, so
// the export is refused rather than sent with the manifest's marks alone.
func TestRestrictToCampaign_FailsClosed(t *testing.T) {
	h := NewAPIHandler(nil, failingTypesSvc{}, nil, nil)
	if err := h.restrictToCampaign(context.Background(), "c1", "character", heroExport()); err == nil {
		t.Fatal("want an error when the categories can't load")
	}
}

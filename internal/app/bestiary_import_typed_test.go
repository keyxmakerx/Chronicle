package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubCreatureTypes serves a fixed set of creature types and records the page
// the adapter creates.
type stubCreatureTypes struct {
	stubImportEntitySvc
	types []entities.EntityType
}

func (s *stubCreatureTypes) GetEntityTypesByPresetCategory(context.Context, string, string) ([]entities.EntityType, error) {
	return s.types, nil
}

// TestBestiaryImport_LandsOnTheCreatureType: an imported creature is a page
// of the campaign's creature type, the publication's own system's type when
// there are several, and a campaign with none is told so instead of getting
// an untyped page.
func TestBestiaryImport_LandsOnTheCreatureType(t *testing.T) {
	ds := entities.EntityType{ID: 7, Slug: "drawsteel-creature", Enabled: true}
	plain := entities.EntityType{ID: 3, Slug: "creature", Enabled: true}
	off := entities.EntityType{ID: 2, Slug: "old-creature", Enabled: false}
	tests := []struct {
		name     string
		types    []entities.EntityType
		systemID string
		wantType int
	}{
		{"the only creature type", []entities.EntityType{plain}, "drawsteel", 3},
		{"the publication's system's own type first", []entities.EntityType{plain, ds}, "drawsteel", 7},
		{"another system's publication takes the first", []entities.EntityType{plain, ds}, "dnd5e", 3},
		{"a switched-off type is skipped", []entities.EntityType{off, plain}, "", 3},
		{"no creature type is refused", []entities.EntityType{off}, "drawsteel", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esvc := &stubCreatureTypes{types: tt.types}
			a := &bestiaryEntityCreatorAdapter{svc: esvc, campaignSvc: &stubImportCampaignSvc{}}
			_, err := a.CreateFromStatblock(context.Background(), "camp-1", "user-1", "Goblin", tt.systemID, json.RawMessage(`{"name":"Goblin"}`))
			if tt.wantType == 0 {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != 400 || len(esvc.created) != 0 {
					t.Fatalf("err = %v, created %d; want a 400 and no page", err, len(esvc.created))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := esvc.created[0].EntityTypeID; got != tt.wantType {
				t.Errorf("page type = %d, want %d", got, tt.wantType)
			}
		})
	}
}

// TestCreatureFields pins how a statblock fills a creature type's fields.
func TestCreatureFields(t *testing.T) {
	defs := []entities.FieldDefinition{
		{Key: "level"}, {Key: "role"}, {Key: "keywords"}, {Key: "traits"}, {Key: "abilities_json"}, {Key: "conditions_json"},
	}
	sb := json.RawMessage(`{"name":"Goblin","level":1,"role":"Harrier","keywords":["Goblin","Humanoid"],` +
		`"traits":[{"name":"Crafty"}],"abilities":[{"name":"Spear"}],"conditions_json":[],"secret":"x","faction":null}`)
	got := creatureFields(defs, sb)
	want := map[string]any{
		"statblock_json":  string(sb),
		"level":           float64(1),
		"role":            "Harrier",
		"keywords":        "Goblin, Humanoid",
		"traits":          `[{"name":"Crafty"}]`,
		"abilities_json":  `[{"name":"Spear"}]`,
		"conditions_json": `[]`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fields =\n %v\nwant\n %v", got, want)
	}
	if got := creatureFields(defs, json.RawMessage(`not json`)); len(got) != 1 {
		t.Errorf("unreadable statblock gave %v, want only statblock_json", got)
	}
}

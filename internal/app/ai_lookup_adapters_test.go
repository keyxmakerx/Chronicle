package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// fakeChoices is the pick-list service; it records whether it was asked
// as a Director and lists a Director-only entry only then.
type fakeChoices struct {
	systems.CharacterChoiceService
	asDirector bool
}

func (f *fakeChoices) CharacterChoiceList(ctx context.Context, _, field string) (*systems.ChoiceList, error) {
	f.asDirector = systems.ChoiceViewerIsDirector(ctx)
	l := &systems.ChoiceList{FieldKey: field, SystemName: "Draw Steel", Choices: []systems.Choice{
		{Slug: "dwarf", Name: "Dwarf", Source: "Heroes", Summary: "Stone-born."},
	}}
	if f.asDirector {
		l.Choices = append(l.Choices, systems.Choice{Slug: "lost-kin", Name: "Lost Kin", Source: systems.ChoiceSourceCampaign})
	}
	return l, nil
}

// The lookup's privacy decides the Director flag: Safe never reads the
// list as a Director, so hidden campaign entries stay out.
func TestAISystemEntriesAdapter_Privacy(t *testing.T) {
	tests := []struct {
		name     string
		director bool
		want     int
	}{
		{"safe reads as a player", false, 1},
		{"permitted reads as a Director", true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeChoices{}
			system, list, err := aiSystemEntriesAdapter{choices: f}.Entries(context.Background(), "camp", "ancestry", tt.director)
			if err != nil {
				t.Fatal(err)
			}
			if f.asDirector != tt.director || len(list) != tt.want || system != "Draw Steel" {
				t.Fatalf("director %v, %d entries, system %q", f.asDirector, len(list), system)
			}
			if e := list[0]; e.Name != "Dwarf" || e.Slug != "dwarf" || e.Source != "Heroes" || e.Summary != "Stone-born." {
				t.Errorf("entry %+v", e)
			}
		})
	}
}

// fakeScreenParty is the DM Screen's party panel: it applies the narrower
// it is given and builds the subtitle from what is left.
type fakeScreenParty struct {
	dmscreen.Service
	heroes []dmscreen.Hero
}

func (f *fakeScreenParty) Party(_ context.Context, _ string, _ dmscreen.Viewer, narrow dmscreen.FieldNarrower) ([]dmscreen.HeroView, error) {
	var out []dmscreen.HeroView
	for _, h := range f.heroes {
		fields := h.Fields
		if narrow != nil {
			fields = narrow(h)
		}
		sub, _ := fields["class"].(string)
		out = append(out, dmscreen.HeroView{ID: h.ID, Name: h.Name, Subtitle: sub})
	}
	return out, nil
}

type fakeTypeReader struct{ fields map[int][]entities.FieldDefinition }

func (f fakeTypeReader) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	defs, ok := f.fields[id]
	if !ok {
		return nil, context.Canceled
	}
	return &entities.EntityType{ID: id, Fields: defs}, nil
}

// In a player's view the party is read without GM-only and owner-only sheet
// values, and a hero whose category can't be read shows none; a Director
// view reads them as stored.
func TestAIPartyAdapter_PlayerViewNarrowsSheetValues(t *testing.T) {
	screen := &fakeScreenParty{heroes: []dmscreen.Hero{
		{ID: "a", TypeID: 1, Name: "Aldric", Fields: map[string]any{"class": "Fury"}},
		{ID: "b", TypeID: 2, Name: "Bram", Fields: map[string]any{"class": "Lich in disguise"}},
		{ID: "c", TypeID: 9, Name: "Cora", Fields: map[string]any{"class": "Tactician"}},
	}}
	types := fakeTypeReader{fields: map[int][]entities.FieldDefinition{
		1: {{Key: "class"}},
		2: {{Key: "class", GMOnly: true}},
	}}
	p := &aiPartyAdapter{screen: screen, types: types}

	tests := []struct {
		name  string
		actor records.Actor
		want  map[string]string
	}{
		{"player view", records.Actor{UserID: "u1", Role: int(campaigns.RolePlayer)}, map[string]string{"Aldric": "Fury", "Bram": "", "Cora": ""}},
		{"director view", records.Actor{UserID: "u1", Role: int(campaigns.RoleOwner)}, map[string]string{"Aldric": "Fury", "Bram": "Lich in disguise", "Cora": "Tactician"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			party, err := p.Party(context.Background(), "camp", tt.actor)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range party.Heroes {
				if h.Subtitle != tt.want[h.Name] {
					t.Errorf("%s subtitle = %q, want %q", h.Name, h.Subtitle, tt.want[h.Name])
				}
			}
		})
	}
}

package app

import (
	"context"
	"testing"

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

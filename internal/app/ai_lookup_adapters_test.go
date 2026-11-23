package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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

// fakeMarkRules is the hero-choice rules' Directors' list: Memonek is Not
// here for everyone and Dwarf has one mark for a single player. err, when
// set, is what it answers instead.
type fakeMarkRules struct {
	systems.ChoiceRuleService
	asked int
	err   error
}

func (f *fakeMarkRules) DirectorEntries(_ context.Context, _ string, a systems.EntryActor, field, as string) (*systems.DirectorEntryList, error) {
	f.asked++
	if !a.IsDirector || as != "" {
		return nil, apperror.NewForbidden("directors only")
	}
	if f.err != nil {
		return nil, f.err
	}
	return &systems.DirectorEntryList{FieldKey: field, Entries: []systems.DirectorEntry{
		{Key: "dwarf", Source: "package", Name: "Dwarf", State: systems.RuleOpen, Players: []systems.PlayerMark{{UserID: "pat"}}},
		{Key: "memonek", Source: "package", Name: "Memonek", State: systems.RuleNotOffered, Reason: "Not on Therin."},
	}}, nil
}

type keyedChoices struct{ systems.CharacterChoiceService }

func (keyedChoices) CharacterChoiceList(_ context.Context, _, field string) (*systems.ChoiceList, error) {
	return &systems.ChoiceList{FieldKey: field, SystemName: "Draw Steel", Choices: []systems.Choice{
		{Slug: "dwarf", Key: "dwarf", Name: "Dwarf", Source: "package"},
		{Slug: "memonek", Key: "memonek", Name: "Memonek", Source: "package"},
	}}, nil
}

// Marks reach the AI only on a Director's lookup, so a connection that reads
// what players see never learns what a Director took off the list; a field
// the rules cannot mark still lists, and a server fault is not hidden.
func TestAISystemEntriesAdapter_Marks(t *testing.T) {
	tests := []struct {
		name      string
		director  bool
		err       error
		wantAsked int
		wantMark  string
		wantErr   bool
	}{
		{name: "director sees marks", director: true, wantAsked: 1, wantMark: systems.RuleNotOffered},
		{name: "safe never asks", director: false},
		{name: "unmarkable field lists without marks", director: true, err: apperror.NewValidation("not a pick list"), wantAsked: 1},
		{name: "server fault fails the lookup", director: true, err: errors.New("db down"), wantAsked: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := &fakeMarkRules{err: tt.err}
			_, list, err := aiSystemEntriesAdapter{choices: keyedChoices{}, rules: rules}.Entries(context.Background(), "camp", "ancestry", tt.director)
			if rules.asked != tt.wantAsked {
				t.Errorf("asked the rules %d times, want %d", rules.asked, tt.wantAsked)
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil || len(list) != 2 {
				t.Fatalf("err %v, %d entries", err, len(list))
			}
			if list[1].Mark != tt.wantMark {
				t.Errorf("Memonek mark %q, want %q", list[1].Mark, tt.wantMark)
			}
			if tt.wantMark != "" && (list[1].MarkReason != "Not on Therin." || list[0].Mark != "" || list[0].PlayerMarks != 1) {
				t.Errorf("entries %+v", list)
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
		{ID: "d", TypeID: 1, Name: "Dell", Fields: map[string]any{"class": "Doppelganger"}, PageRestricted: []string{"class"}},
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
		{"player view", records.Actor{UserID: "u1", Role: int(campaigns.RolePlayer)}, map[string]string{"Aldric": "Fury", "Bram": "", "Cora": "", "Dell": ""}},
		{"director view", records.Actor{UserID: "u1", Role: int(campaigns.RoleOwner)}, map[string]string{"Aldric": "Fury", "Bram": "Lich in disguise", "Cora": "Tactician", "Dell": "Doppelganger"}},
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

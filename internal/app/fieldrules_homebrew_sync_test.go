package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

type fakeCategories struct {
	types   []entities.EntityType
	updates int
}

func (f *fakeCategories) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return append([]entities.EntityType{}, f.types...), nil
}

func (f *fakeCategories) UpdateEntityType(_ context.Context, id int, in entities.UpdateEntityTypeInput) (*entities.EntityType, error) {
	f.updates++
	for i := range f.types {
		if f.types[i].ID == id {
			f.types[i].Fields = in.Fields
			return &f.types[i], nil
		}
	}
	return nil, nil
}

func (f *fakeCategories) field(typeID int, key string) *entities.FieldDefinition {
	for _, t := range f.types {
		if t.ID != typeID {
			continue
		}
		for i := range t.Fields {
			if t.Fields[i].Key == key {
				return &t.Fields[i]
			}
		}
	}
	return nil
}

type fakeRuleStore struct {
	rules []fieldrules.Rule
	words []fieldrules.LocalWord
	eras  map[int]string
}

func (f *fakeRuleStore) RuleNamed(_ context.Context, _, name string) (*fieldrules.Rule, error) {
	for i := range f.rules {
		if f.rules[i].Name == name {
			return &f.rules[i], nil
		}
	}
	return nil, nil
}

func (f *fakeRuleStore) SaveRule(_ context.Context, _ string, in fieldrules.RuleInput, _ fieldrules.Viewer) (*fieldrules.Rule, error) {
	r := fieldrules.Rule{ID: in.ID, Name: in.Name, Follows: in.Follows, Rows: in.Rows}
	for i := range f.rules {
		if f.rules[i].ID == in.ID {
			f.rules[i] = r
			return &f.rules[i], nil
		}
	}
	r.ID = len(f.rules) + 100
	f.rules = append(f.rules, r)
	return &r, nil
}

func (f *fakeRuleStore) Words(context.Context, string, fieldrules.Viewer) ([]fieldrules.LocalWord, error) {
	return append([]fieldrules.LocalWord{}, f.words...), nil
}

func (f *fakeRuleStore) SaveWord(_ context.Context, _ string, in fieldrules.WordInput, _ fieldrules.Viewer) (*fieldrules.LocalWord, error) {
	w := fieldrules.LocalWord{ID: in.ID, Word: in.Word, Local: in.Local, PlaceID: in.PlaceID, EraID: in.EraID, DMOnly: in.DMOnly}
	for i := range f.words {
		if f.words[i].ID == in.ID {
			f.words[i] = w
			return &w, nil
		}
	}
	w.ID = len(f.words) + 1
	f.words = append(f.words, w)
	return &w, nil
}

func (f *fakeRuleStore) Eras(context.Context, string, fieldrules.Viewer) map[int]string { return f.eras }

// syncWorld is a campaign with Characters (a hand-made "notes" field kept
// owner-only), Items and Creatures, one page and one era.
func syncWorld(t *testing.T) (homebrew.Service, *fakeCategories, *fakeRuleStore) {
	t.Helper()
	cats := &fakeCategories{types: []entities.EntityType{
		{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character",
			Fields: []entities.FieldDefinition{{Key: "notes", Label: "Notes", Type: "text", OwnerOnly: true}}},
		{ID: 2, Name: "Item", NamePlural: "Items", Slug: "item"},
		{ID: 3, Name: "Creature", NamePlural: "Creatures", Slug: "creature"},
	}}
	store := &fakeRuleStore{eras: map[int]string{7: "The Reign"}}
	reg := homebrew.NewRegistry()
	place := func(_ context.Context, _, id string) (string, bool) { return "Saltreach", id == "saltreach" }
	if err := registerFieldRuleParts(reg, place, fieldRuleSync{cats: cats, rules: store}); err != nil {
		t.Fatal(err)
	}
	return homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil), cats, store
}

func savePart(t *testing.T, svc homebrew.Service, kind, data string) error {
	t.Helper()
	_, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{{Key: "p", Kind: kind, Label: "P", Data: json.RawMessage(data)}}})
	return err
}

const guildRank = `{"category":"Characters","label":"Guild rank","type":"rule","rule":{"follows":"echelon","rows":[{"is":"1","gives":"Copper"}]}}`

func TestFieldPartSavesFieldAndRule(t *testing.T) {
	svc, cats, store := syncWorld(t)
	for round := 1; round <= 2; round++ {
		if err := savePart(t, svc, "field", guildRank); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	if len(store.rules) != 1 || store.rules[0].Name != "Guild rank" || store.rules[0].Follows != "echelon" {
		t.Fatalf("rules = %+v, want one Guild rank rule following echelon", store.rules)
	}
	f := cats.field(1, "guild_rank")
	if f == nil || f.Type != entities.FieldTypeRule || f.RuleID != store.rules[0].ID {
		t.Fatalf("guild_rank = %+v, want a rule field using rule %d", f, store.rules[0].ID)
	}
	if n := len(cats.types[0].Fields); n != 2 {
		t.Errorf("Characters has %d fields after two saves, want 2", n)
	}
	if notes := cats.field(1, "notes"); notes == nil || !notes.OwnerOnly {
		t.Errorf("the hand-made notes field changed: %+v", notes)
	}
}

func TestItemPartLinksToItsCategory(t *testing.T) {
	svc, cats, _ := syncWorld(t)
	err := savePart(t, svc, "item", `{"category":"Items","fields":[{"label":"Source creature","type":"link","linksTo":"Creatures"},{"label":"Remnant kind","type":"select","options":["Bone","Shell"]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if f := cats.field(2, "source_creature"); f == nil || f.LinkTo != 3 {
		t.Errorf("source_creature = %+v, want a link to Creatures (3)", f)
	}
	if f := cats.field(2, "remnant_kind"); f == nil || len(f.Options) != 2 {
		t.Errorf("remnant_kind = %+v, want two options", f)
	}
}

func TestFieldPartRefusals(t *testing.T) {
	tests := []struct {
		name, kind, data string
	}{
		{"unknown category", "field", `{"category":"Ships","label":"Hull"}`},
		{"link to an unknown category", "field", `{"category":"Items","label":"Maker","type":"link","linksTo":"Smiths"}`},
		{"changes a hand-made field's kind", "field", `{"category":"Characters","label":"Notes","type":"number"}`},
		{"unknown era", "words", `{"era":"The Thaw","words":[{"word":"Mark","local":"Bit"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cats, store := syncWorld(t)
			err := savePart(t, svc, tt.kind, tt.data)
			if hbCode(err) != 422 {
				t.Fatalf("err = %v, want 422", err)
			}
			if cats.updates != 0 || len(store.rules) != 0 || len(store.words) != 0 {
				t.Errorf("a refused recipe still wrote: %d category saves, %d rules, %d words", cats.updates, len(store.rules), len(store.words))
			}
		})
	}
}

func TestWordsPartSavesWords(t *testing.T) {
	svc, _, store := syncWorld(t)
	steps := []struct {
		data string
		want int
	}{
		{`{"words":[{"word":"Mark","local":"Bit"}]}`, 0}, // a template saves nothing
		{`{"placeId":"saltreach","words":[{"word":"Mark","local":"Grain"}]}`, 1},
		{`{"placeId":"saltreach","words":[{"word":"mark","local":"Pinch"}]}`, 1}, // replaces, not adds
		{`{"era":"The Reign","words":[{"word":"Mark","local":"Bit","director":true}]}`, 2},
	}
	for i, s := range steps {
		if err := savePart(t, svc, "words", s.data); err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
		if len(store.words) != s.want {
			t.Fatalf("step %d: %d words, want %d", i+1, len(store.words), s.want)
		}
	}
	if store.words[0].Local != "Pinch" || store.words[0].PlaceID != "saltreach" {
		t.Errorf("Saltreach's word = %+v, want Pinch", store.words[0])
	}
	if store.words[1].EraID != 7 || !store.words[1].DMOnly {
		t.Errorf("the Reign's word = %+v, want era 7 and Director-only", store.words[1])
	}
}

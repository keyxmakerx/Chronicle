package entities

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRename mimics the fieldrules word table: a word
// is renamed when the lower-cased text is a key of the table.
func fakeRename(words map[string]string, text string) (string, bool) {
	l, ok := words[strings.ToLower(text)]
	return l, ok
}

func TestShapeFields(t *testing.T) {
	linkDef := FieldDefinition{Key: "home", Type: FieldTypeLink}
	ruleDef := FieldDefinition{Key: "rank", Type: FieldTypeRule, RuleID: 7}
	words := map[string]string{"mark": "Grain", "gold": "Sun"}
	links := map[string]linkedPage{"p-1": {Name: "Ironhold", Href: "/campaigns/c/entities/p-1"}}

	tests := []struct {
		name   string
		defs   []FieldDefinition
		data   map[string]any
		gives  map[int]string
		rename func(map[string]string, string) (string, bool)
		want   map[string]FieldShown
	}{
		{"link shows page name and href", []FieldDefinition{linkDef}, map[string]any{"home": "p-1"}, nil, nil,
			map[string]FieldShown{"home": {Text: "Ironhold", Href: "/campaigns/c/entities/p-1"}}},
		{"link to a page not in the map shows nothing, never the id", []FieldDefinition{linkDef}, map[string]any{"home": "p-9"}, nil, nil,
			map[string]FieldShown{"home": {}}},
		{"empty link is skipped", []FieldDefinition{linkDef}, map[string]any{"home": "  "}, nil, nil, map[string]FieldShown{}},
		{"absent link is skipped", []FieldDefinition{linkDef}, map[string]any{}, nil, nil, map[string]FieldShown{}},
		{"rule shows its result when nothing is stored", []FieldDefinition{ruleDef}, map[string]any{}, map[int]string{7: "Silver"}, nil,
			map[string]FieldShown{"rank": {Text: "Silver"}}},
		{"rule with no result and no value is skipped", []FieldDefinition{ruleDef}, map[string]any{}, nil, nil, map[string]FieldShown{}},
		{"typed value wins and records what the rule gives", []FieldDefinition{ruleDef}, map[string]any{"rank": "Gold"}, map[int]string{7: "Silver"}, nil,
			map[string]FieldShown{"rank": {Text: "Gold", ByHand: true, RuleGives: "Silver"}}},
		{"typed value equal to the rule, ignoring case, has no RuleGives", []FieldDefinition{ruleDef}, map[string]any{"rank": "silver"}, map[int]string{7: "Silver"}, nil,
			map[string]FieldShown{"rank": {Text: "silver", ByHand: true}}},
		{"typed value with no rule result", []FieldDefinition{ruleDef}, map[string]any{"rank": "Gold"}, nil, nil,
			map[string]FieldShown{"rank": {Text: "Gold", ByHand: true}}},
		{"blank typed value falls back to the rule", []FieldDefinition{ruleDef}, map[string]any{"rank": "  "}, map[int]string{7: "Silver"}, nil,
			map[string]FieldShown{"rank": {Text: "Silver"}}},
		{"rule result is renamed by a local word", []FieldDefinition{ruleDef}, map[string]any{}, map[int]string{7: "Mark"}, fakeRename,
			map[string]FieldShown{"rank": {Text: "Grain", Usual: "Mark"}}},
		{"typed rule value is renamed and keeps ByHand", []FieldDefinition{ruleDef}, map[string]any{"rank": "Gold"}, map[int]string{7: "Mark"}, fakeRename,
			map[string]FieldShown{"rank": {Text: "Sun", Usual: "Gold", ByHand: true, RuleGives: "Mark"}}},
		{"text value with a local word", []FieldDefinition{{Key: "coin", Type: "text"}}, map[string]any{"coin": "Mark"}, nil, fakeRename,
			map[string]FieldShown{"coin": {Text: "Grain", Usual: "Mark"}}},
		{"select value with a local word", []FieldDefinition{{Key: "coin", Type: "select"}}, map[string]any{"coin": "Gold"}, nil, fakeRename,
			map[string]FieldShown{"coin": {Text: "Sun", Usual: "Gold"}}},
		{"text with no local word is not sent", []FieldDefinition{{Key: "coin", Type: "text"}}, map[string]any{"coin": "Pearl"}, nil, fakeRename,
			map[string]FieldShown{}},
		{"text with a nil rename func is left alone", []FieldDefinition{{Key: "coin", Type: "text"}}, map[string]any{"coin": "Mark"}, nil, nil,
			map[string]FieldShown{}},
		{"rule with a nil rename func is not renamed", []FieldDefinition{ruleDef}, map[string]any{}, map[int]string{7: "Mark"}, nil,
			map[string]FieldShown{"rank": {Text: "Mark"}}},
		{"rename to the same text is not a rename", []FieldDefinition{{Key: "coin", Type: "text"}}, map[string]any{"coin": "Mark"}, nil,
			func(map[string]string, string) (string, bool) { return "Mark", true },
			map[string]FieldShown{}},
		{"empty text is skipped", []FieldDefinition{{Key: "coin", Type: "text"}}, map[string]any{"coin": ""}, nil, fakeRename, map[string]FieldShown{}},
		{"untyped (older) field counts as text", []FieldDefinition{{Key: "coin"}}, map[string]any{"coin": "Mark"}, nil, fakeRename,
			map[string]FieldShown{"coin": {Text: "Grain", Usual: "Mark"}}},
		{"number never renamed", []FieldDefinition{{Key: "n", Type: "number"}}, map[string]any{"n": "Mark"}, nil, fakeRename, map[string]FieldShown{}},
		{"checkbox never renamed", []FieldDefinition{{Key: "c", Type: "checkbox"}}, map[string]any{"c": "Mark"}, nil, fakeRename, map[string]FieldShown{}},
		{"url never renamed", []FieldDefinition{{Key: "u", Type: "url"}}, map[string]any{"u": "Mark"}, nil, fakeRename, map[string]FieldShown{}},
		{"textarea never renamed", []FieldDefinition{{Key: "t", Type: "textarea"}}, map[string]any{"t": "Mark"}, nil, fakeRename, map[string]FieldShown{}},
		{"non-string stored value is read as text", []FieldDefinition{ruleDef}, map[string]any{"rank": 3}, map[int]string{7: "Silver"}, nil,
			map[string]FieldShown{"rank": {Text: "3", ByHand: true, RuleGives: "Silver"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shapeFields(tc.defs, tc.data, links, tc.gives, words, tc.rename)
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for k, w := range tc.want {
				if g, ok := got[k]; !ok || g != w {
					t.Errorf("%s = %+v (present %v), want %+v", k, g, ok, w)
				}
			}
		})
	}
}

func TestRestrictedKeys(t *testing.T) {
	typeDefs := []FieldDefinition{
		{Key: "open"},
		{Key: "gm", GMOnly: true},
		{Key: "owner", OwnerOnly: true},
	}
	entityDefs := []FieldDefinition{
		{Key: "extra_gm", GMOnly: true},
		{Key: "extra_open"},
		{Key: "both", GMOnly: true, OwnerOnly: true},
	}
	tests := []struct {
		name    string
		canGM   bool
		isOwner bool
		lists   [][]FieldDefinition
		want    []string
	}{
		{"GM sees everything", true, false, [][]FieldDefinition{typeDefs, entityDefs}, nil},
		{"GM who is also owner", true, true, [][]FieldDefinition{typeDefs}, nil},
		{"player hides GMOnly and OwnerOnly", false, false, [][]FieldDefinition{typeDefs}, []string{"gm", "owner"}},
		{"owner still loses GMOnly but keeps OwnerOnly", false, true, [][]FieldDefinition{typeDefs}, []string{"gm"}},
		{"several lists are merged", false, false, [][]FieldDefinition{typeDefs, entityDefs}, []string{"gm", "owner", "extra_gm", "both"}},
		{"owner with merged lists: GMOnly+OwnerOnly is still hidden", false, true, [][]FieldDefinition{typeDefs, entityDefs}, []string{"gm", "extra_gm", "both"}},
		{"no lists", false, false, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := restrictedKeys(tc.canGM, tc.isOwner, tc.lists...)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for _, k := range tc.want {
				if !got[k] {
					t.Errorf("%q should be hidden; got %v", k, got)
				}
			}
		})
	}
}

// fakeFieldWords answers only RuleExists; the rest is not used by saves.
type fakeFieldWords struct{ rules map[int]bool }

func (f fakeFieldWords) Evaluate(context.Context, string, []int, map[string]any) map[int]string {
	return nil
}
func (f fakeFieldWords) WordsFor(context.Context, string, string, int, string) map[string]string {
	return nil
}
func (f fakeFieldWords) Rename(map[string]string, string) (string, bool) { return "", false }
func (f fakeFieldWords) RuleExists(_ context.Context, _ string, id int) bool {
	return f.rules[id]
}

func TestCheckFieldKinds(t *testing.T) {
	types := &mockEntityTypeRepo{findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
		switch id {
		case 3:
			return &EntityType{ID: 3, CampaignID: "camp"}, nil
		case 4:
			return &EntityType{ID: 4, CampaignID: "other"}, nil
		}
		return nil, errors.New("not found")
	}}
	tests := []struct {
		name    string
		fw      FieldWords
		in      FieldDefinition
		wantErr string
		want    FieldDefinition
	}{
		{"link to a category of this campaign", nil, FieldDefinition{Label: "Home", Type: FieldTypeLink, LinkTo: 3, RuleID: 9}, "",
			FieldDefinition{Label: "Home", Type: FieldTypeLink, LinkTo: 3}},
		{"link without a category", nil, FieldDefinition{Label: "Home", Type: FieldTypeLink}, "Home field links to", FieldDefinition{}},
		{"link to another campaign's category", nil, FieldDefinition{Label: "Home", Type: FieldTypeLink, LinkTo: 4}, "doesn't have", FieldDefinition{}},
		{"link to a category that does not exist", nil, FieldDefinition{Label: "Home", Type: FieldTypeLink, LinkTo: 99}, "doesn't have", FieldDefinition{}},
		{"rule that exists", fakeFieldWords{rules: map[int]bool{7: true}}, FieldDefinition{Label: "Rank", Type: FieldTypeRule, RuleID: 7, LinkTo: 3}, "",
			FieldDefinition{Label: "Rank", Type: FieldTypeRule, RuleID: 7}},
		{"rule without a rule id", nil, FieldDefinition{Label: "Rank", Type: FieldTypeRule}, "which rule", FieldDefinition{}},
		{"rule the campaign does not have", fakeFieldWords{rules: map[int]bool{}}, FieldDefinition{Label: "Rank", Type: FieldTypeRule, RuleID: 7}, "rule this campaign doesn't have", FieldDefinition{}},
		{"rule accepted when no rules service is wired", nil, FieldDefinition{Label: "Rank", Type: FieldTypeRule, RuleID: 7}, "",
			FieldDefinition{Label: "Rank", Type: FieldTypeRule, RuleID: 7}},
		{"stray settings are cleared on a text field", fakeFieldWords{}, FieldDefinition{Label: "Note", Type: "text", LinkTo: 3, RuleID: 7}, "",
			FieldDefinition{Label: "Note", Type: "text"}},
		{"stray settings are cleared on a select field", nil, FieldDefinition{Label: "Pick", Type: "select", LinkTo: 3, RuleID: 7, Options: []string{"a"}}, "",
			FieldDefinition{Label: "Pick", Type: "select", Options: []string{"a"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &entityService{types: types}
			if tc.fw != nil {
				s.fieldWords = tc.fw
			}
			fields := []FieldDefinition{tc.in}
			err := s.checkFieldKinds(context.Background(), "camp", fields)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			g, w := fields[0], tc.want
			if g.Label != w.Label || g.Type != w.Type || g.LinkTo != w.LinkTo || g.RuleID != w.RuleID || len(g.Options) != len(w.Options) {
				t.Errorf("field = %+v, want %+v", g, w)
			}
		})
	}
}

func TestCheckLinkValues(t *testing.T) {
	const self = "page-self"
	pages := map[string]*Entity{
		"good":   {ID: "good", CampaignID: "camp", EntityTypeID: 3},
		"wrong":  {ID: "wrong", CampaignID: "camp", EntityTypeID: 8},
		"alien":  {ID: "alien", CampaignID: "other", EntityTypeID: 3},
		"anytyp": {ID: "anytyp", CampaignID: "camp", EntityTypeID: 8},
		self:     {ID: self, CampaignID: "camp", EntityTypeID: 3},
	}
	lookups := 0
	ents := &mockEntityRepo{findByIDFn: func(_ context.Context, id string) (*Entity, error) {
		lookups++
		if e, ok := pages[id]; ok {
			return e, nil
		}
		return nil, errors.New("not found")
	}}
	et := &EntityType{ID: 3, CampaignID: "camp", Fields: []FieldDefinition{
		{Key: "home", Label: "Home", Type: FieldTypeLink, LinkTo: 3},
		{Key: "anywhere", Label: "Anywhere", Type: FieldTypeLink},
		{Key: "note", Label: "Note", Type: "text"},
	}}
	types := &mockEntityTypeRepo{findByIDFn: func(context.Context, int) (*EntityType, error) { return et, nil }}
	s := &entityService{types: types, entities: ents}
	page := func() *Entity { return &Entity{ID: self, CampaignID: "camp", EntityTypeID: 3} }

	tests := []struct {
		name        string
		e           *Entity
		oldD, newD  map[string]any
		wantErr     string
		wantLookups int
	}{
		{"nil entity", nil, nil, map[string]any{"home": "good"}, "", 0},
		{"right page accepted", page(), nil, map[string]any{"home": "good"}, "", 1},
		{"unchanged value is not re-checked", page(), map[string]any{"home": "alien"}, map[string]any{"home": "alien"}, "", 0},
		{"changed value is checked", page(), map[string]any{"home": "good"}, map[string]any{"home": "alien"}, "Home must link", 1},
		{"page in another campaign", page(), nil, map[string]any{"home": "alien"}, "Home must link", 1},
		{"page that does not exist", page(), nil, map[string]any{"home": "ghost"}, "Home must link", 1},
		{"the page itself", page(), nil, map[string]any{"home": self}, "Home must link", 1},
		{"wrong category", page(), nil, map[string]any{"home": "wrong"}, "Home must link", 1},
		{"any category when LinkTo is unset", page(), nil, map[string]any{"anywhere": "anytyp"}, "", 1},
		{"cleared link is accepted", page(), map[string]any{"home": "good"}, map[string]any{"home": ""}, "", 0},
		{"absent link is accepted", page(), nil, map[string]any{"note": "hi"}, "", 0},
		{"surrounding spaces are ignored", page(), map[string]any{"home": "good"}, map[string]any{"home": " good "}, "", 0},
		{"entity-level added link field is checked", &Entity{ID: self, CampaignID: "camp", EntityTypeID: 3,
			FieldOverrides: &FieldOverrides{Added: []FieldDefinition{{Key: "ally", Label: "Ally", Type: FieldTypeLink, LinkTo: 3}}}},
			nil, map[string]any{"ally": "wrong"}, "Ally must link", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookups = 0
			err := s.checkLinkValues(context.Background(), tc.e, tc.oldD, tc.newD)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if lookups != tc.wantLookups {
				t.Errorf("page lookups = %d, want %d", lookups, tc.wantLookups)
			}
		})
	}

	t.Run("a category that cannot be loaded does not block the save", func(t *testing.T) {
		bad := &entityService{types: &mockEntityTypeRepo{}, entities: ents}
		if err := bad.checkLinkValues(context.Background(), page(), nil, map[string]any{"home": "alien"}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})
}

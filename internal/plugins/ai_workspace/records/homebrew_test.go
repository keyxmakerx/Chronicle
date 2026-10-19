package records

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeHB is an in-memory homebrew service that records what was saved.
type fakeHB struct {
	rules        []HomebrewRule
	words        []HomebrewWord
	eras         map[int]string
	checkRuleErr error
	checkWordErr error

	savedRules   []HomebrewRule
	savedWords   []HomebrewWord
	deletedRules []int
	deletedWords []int
}

func (f *fakeHB) Rules(context.Context, string, Actor) ([]HomebrewRule, error) { return f.rules, nil }
func (f *fakeHB) RuleNamed(_ context.Context, _, name string) (*HomebrewRule, error) {
	for i := range f.rules {
		if sameName(f.rules[i].Name, name) {
			r := f.rules[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (f *fakeHB) CheckRule(HomebrewRule) error { return f.checkRuleErr }
func (f *fakeHB) SaveRule(_ context.Context, _ string, in HomebrewRule, _ Actor) error {
	f.savedRules = append(f.savedRules, in)
	return nil
}
func (f *fakeHB) DeleteRule(_ context.Context, _ string, id int, _ Actor) error {
	f.deletedRules = append(f.deletedRules, id)
	return nil
}
func (f *fakeHB) Words(context.Context, string, Actor) ([]HomebrewWord, error) { return f.words, nil }
func (f *fakeHB) CheckWord(context.Context, string, HomebrewWord, Actor) error {
	return f.checkWordErr
}
func (f *fakeHB) SaveWord(_ context.Context, _ string, in HomebrewWord, _ Actor) error {
	f.savedWords = append(f.savedWords, in)
	return nil
}
func (f *fakeHB) DeleteWord(_ context.Context, _ string, id int, _ Actor) error {
	f.deletedWords = append(f.deletedWords, id)
	return nil
}
func (f *fakeHB) Eras(context.Context, string, Actor) map[int]string { return f.eras }

// fakeCats is an in-memory category store; UpdateEntityType keeps the input
// so tests can see exactly which field list was sent.
type fakeCats struct {
	types   []entities.EntityType
	updated []entities.UpdateEntityTypeInput
	updID   []int
}

func (f *fakeCats) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}
func (f *fakeCats) UpdateEntityType(_ context.Context, id int, in entities.UpdateEntityTypeInput) (*entities.EntityType, error) {
	f.updID = append(f.updID, id)
	f.updated = append(f.updated, in)
	return nil, nil
}

func fp(v float64) *float64 { return &v }

func TestHomebrewKinds_DirectorsOnly(t *testing.T) {
	hb := &fakeHB{}
	cats := &fakeCats{types: []entities.EntityType{{ID: 1, Name: "Items"}}}
	kinds := []struct {
		name string
		kind Kind
		r    Record
	}{
		{"field-rule", FieldRuleKind{Svc: hb}, rec("field-rule", ActionCreate, "Rank", map[string]any{"follows": "x"}, "")},
		{"local-word", LocalWordKind{Svc: hb, Entities: fakeEntities{}}, rec("local-word", ActionCreate, "Mark", map[string]any{"local": "Grain", "era": "Old"}, "")},
		{"category-field", CategoryFieldKind{Types: cats}, rec("category-field", ActionCreate, "Source", map[string]any{"category": "Items"}, "")},
	}
	for _, tc := range kinds {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.kind.Plan(context.Background(), camp, player, tc.r)
			if !strings.Contains(p.Error, "Directors") {
				t.Errorf("Plan error = %q, want a Directors-only refusal", p.Error)
			}
			if err := tc.kind.Apply(context.Background(), camp, player, tc.r); err == nil {
				t.Error("Apply must refuse a non-Director")
			}
			if len(hb.savedRules)+len(hb.savedWords)+len(cats.updated) != 0 {
				t.Error("a refused Apply must save nothing")
			}
		})
	}
}

func TestFieldRuleKind_Plan(t *testing.T) {
	existing := HomebrewRule{ID: 4, Name: "Guild rank", Follows: "echelon", Rows: []RuleRow{{Is: "1", Gives: "Copper"}}}
	tests := []struct {
		name      string
		action    string
		rname     string
		fields    map[string]any
		checkErr  error
		wantErr   string
		wantInSum string
	}{
		{"create new", ActionCreate, "Fresh", map[string]any{"follows": "level", "rows": []any{map[string]any{"is": "1", "gives": "A"}}}, nil, "", "new rule: reads level, 1 row"},
		{"create exists", ActionCreate, "guild RANK", map[string]any{"follows": "x"}, nil, "already a rule", ""},
		{"update missing", ActionUpdate, "Nope", nil, nil, "no rule called", ""},
		{"delete missing", ActionDelete, "Nope", nil, nil, "no rule called", ""},
		{"delete exists", ActionDelete, "Guild rank", nil, nil, "", "removes the rule"},
		{"update keeps follows and rows", ActionUpdate, "Guild rank", map[string]any{}, nil, "", "changes the rule: reads echelon, 1 row"},
		{"update names rows", ActionUpdate, "Guild rank", map[string]any{"rows": []any{map[string]any{"gives": "a", "is": "1"}, map[string]any{"gives": "b", "is": "2"}}}, nil, "", "reads echelon, 2 rows"},
		{"no name", ActionCreate, "", nil, nil, "needs a name", ""},
		{"row that is not a map", ActionUpdate, "Guild rank", map[string]any{"rows": []any{"oops"}}, nil, "row 1", ""},
		{"row with a bad number", ActionUpdate, "Guild rank", map[string]any{"rows": []any{map[string]any{"from": "abc", "gives": "x"}}}, nil, "row 1", ""},
		{"service check refuses", ActionCreate, "Fresh", map[string]any{"follows": "x"}, apperror.NewBadRequest("rows overlap"), "rows overlap", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hb := &fakeHB{rules: []HomebrewRule{existing}, checkRuleErr: tc.checkErr}
			p := FieldRuleKind{Svc: hb}.Plan(context.Background(), camp, owner, rec("field-rule", tc.action, tc.rname, tc.fields, ""))
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", p.Error, tc.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tc.wantInSum) {
				t.Fatalf("plan = %+v, want summary containing %q", p, tc.wantInSum)
			}
		})
	}
}

func TestFieldRuleKind_RowsParsing(t *testing.T) {
	hb := &fakeHB{}
	rows := []any{
		map[string]any{"is": "Gold", "gives": "rich"},
		map[string]any{"From": "4", "to": 9, "gives": "mid"},
		map[string]any{"from": 10.5, "gives": "top"},
		map[string]any{"to": "2", "gives": "low"},
	}
	r := rec("field-rule", ActionCreate, "Wealth", map[string]any{"follows": "coins", "rows": rows}, "")
	if err := (FieldRuleKind{Svc: hb}).Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	want := []RuleRow{
		{Is: "Gold", Gives: "rich"},
		{From: fp(4), To: fp(9), Gives: "mid"},
		{From: fp(10.5), Gives: "top"},
		{To: fp(2), Gives: "low"},
	}
	if len(hb.savedRules) != 1 || !reflect.DeepEqual(hb.savedRules[0].Rows, want) {
		t.Fatalf("saved = %+v, want rows %+v", hb.savedRules, want)
	}
	if hb.savedRules[0].ID != 0 || hb.savedRules[0].Follows != "coins" {
		t.Errorf("a create is a new rule following coins: %+v", hb.savedRules[0])
	}
}

func TestFieldRuleKind_Apply(t *testing.T) {
	existing := HomebrewRule{ID: 4, Name: "Guild rank", Follows: "echelon", Rows: []RuleRow{{Is: "1", Gives: "Copper"}}}
	ctx := context.Background()

	t.Run("update keeps follows and rows when not named", func(t *testing.T) {
		hb := &fakeHB{rules: []HomebrewRule{existing}}
		if err := (FieldRuleKind{Svc: hb}).Apply(ctx, camp, owner, rec("field-rule", ActionUpdate, "Guild rank", map[string]any{}, "")); err != nil {
			t.Fatal(err)
		}
		if len(hb.savedRules) != 1 || !reflect.DeepEqual(hb.savedRules[0], existing) {
			t.Fatalf("saved = %+v, want the rule unchanged %+v", hb.savedRules, existing)
		}
	})
	t.Run("update changes only follows", func(t *testing.T) {
		hb := &fakeHB{rules: []HomebrewRule{existing}}
		if err := (FieldRuleKind{Svc: hb}).Apply(ctx, camp, owner, rec("field-rule", ActionUpdate, "Guild rank", map[string]any{"follows": "tier"}, "")); err != nil {
			t.Fatal(err)
		}
		got := hb.savedRules[0]
		if got.ID != 4 || got.Follows != "tier" || !reflect.DeepEqual(got.Rows, existing.Rows) {
			t.Errorf("saved = %+v", got)
		}
	})
	t.Run("delete removes by id", func(t *testing.T) {
		hb := &fakeHB{rules: []HomebrewRule{existing}}
		if err := (FieldRuleKind{Svc: hb}).Apply(ctx, camp, owner, rec("field-rule", ActionDelete, "Guild rank", nil, "")); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(hb.deletedRules, []int{4}) || len(hb.savedRules) != 0 {
			t.Errorf("deleted %v saved %v", hb.deletedRules, hb.savedRules)
		}
	})
	t.Run("create over an existing rule is refused", func(t *testing.T) {
		hb := &fakeHB{rules: []HomebrewRule{existing}}
		if err := (FieldRuleKind{Svc: hb}).Apply(ctx, camp, owner, rec("field-rule", ActionCreate, "Guild rank", map[string]any{"follows": "x"}, "")); err == nil {
			t.Fatal("want an error")
		}
		if len(hb.savedRules) != 0 {
			t.Error("nothing should be saved")
		}
	})
}

func TestLocalWordKind_Plan(t *testing.T) {
	ctx := context.Background()
	pages := fakeEntities{"saltreach": {ID: "p-1", Name: "Saltreach", CampaignID: camp}, "elsewhere": {ID: "p-2", Name: "Elsewhere", CampaignID: "other"}}
	hb := func() *fakeHB {
		return &fakeHB{
			eras:  map[int]string{2: "Age of Ash"},
			words: []HomebrewWord{{ID: 9, Word: "Mark", Local: "Grain", PlaceID: "p-1"}, {ID: 10, Word: "Mark", Local: "Ember", EraID: 2}},
		}
	}
	tests := []struct {
		name    string
		action  string
		word    string
		fields  map[string]any
		made    map[string]bool
		wantErr string
		wantSum string
	}{
		{"neither place nor era", ActionCreate, "Rank", map[string]any{"local": "x"}, nil, "`place` or `era`", ""},
		{"both place and era", ActionCreate, "Rank", map[string]any{"local": "x", "place": "Saltreach", "era": "Age of Ash"}, nil, "`place` or `era`", ""},
		{"unknown place", ActionCreate, "Rank", map[string]any{"local": "x", "place": "Nowhere"}, nil, "no page called", ""},
		{"place in another campaign", ActionCreate, "Rank", map[string]any{"local": "x", "place": "Elsewhere"}, nil, "no page called", ""},
		{"unknown era", ActionCreate, "Rank", map[string]any{"local": "x", "era": "Age of Glass"}, nil, "no era called", ""},
		{"place made in this paste, create", ActionCreate, "Rank", map[string]any{"local": "Step", "place": "New Town"}, map[string]bool{"new-town": true}, "", "in a place this paste makes"},
		{"place made in this paste, update", ActionUpdate, "Rank", map[string]any{"local": "Step", "place": "New Town"}, map[string]bool{"new-town": true}, "no local word", ""},
		{"create new in place", ActionCreate, "Rank", map[string]any{"local": "Step", "place": "Saltreach"}, nil, "", `calls Rank "Step"`},
		{"create duplicates place word", ActionCreate, "mark", map[string]any{"local": "x", "place": "Saltreach"}, nil, "already has a local name", ""},
		{"create same word in another era", ActionCreate, "Mark", map[string]any{"local": "x", "era": "Age of Ash"}, nil, "already has a local name", ""},
		{"update matches word and place", ActionUpdate, "MARK", map[string]any{"local": "Seed", "place": "Saltreach"}, nil, "", `calls MARK "Seed"`},
		{"update missing word", ActionUpdate, "Rank", map[string]any{"local": "x", "place": "Saltreach"}, nil, "no local word", ""},
		{"delete matches era", ActionDelete, "Mark", map[string]any{"era": "age of ash"}, nil, "", "goes back to the usual word"},
		{"no name", ActionCreate, "", map[string]any{"local": "x", "era": "Age of Ash"}, nil, "needs `name`", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := rec("local-word", tc.action, tc.word, tc.fields, "")
			r.Made = tc.made
			p := LocalWordKind{Svc: hb(), Entities: pages}.Plan(ctx, camp, owner, r)
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", p.Error, tc.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tc.wantSum) {
				t.Fatalf("plan = %+v, want summary containing %q", p, tc.wantSum)
			}
		})
	}
	t.Run("service check refuses", func(t *testing.T) {
		h := hb()
		h.checkWordErr = apperror.NewBadRequest("word too long")
		p := LocalWordKind{Svc: h, Entities: pages}.Plan(ctx, camp, owner, rec("local-word", ActionCreate, "Rank", map[string]any{"local": "x", "place": "Saltreach"}, ""))
		if !strings.Contains(p.Error, "word too long") {
			t.Errorf("error = %q", p.Error)
		}
	})
}

func TestLocalWordKind_Apply(t *testing.T) {
	ctx := context.Background()
	pages := fakeEntities{"saltreach": {ID: "p-1", Name: "Saltreach", CampaignID: camp}}
	mk := func() *fakeHB {
		return &fakeHB{eras: map[int]string{2: "Age of Ash"}, words: []HomebrewWord{{ID: 9, Word: "Mark", Local: "Grain", PlaceID: "p-1", DMOnly: true}}}
	}
	t.Run("create in place", func(t *testing.T) {
		hb := mk()
		r := rec("local-word", ActionCreate, "Rank", map[string]any{"local": "Step", "place": "Saltreach"}, "")
		if err := (LocalWordKind{Svc: hb, Entities: pages}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		want := HomebrewWord{Word: "Rank", Local: "Step", PlaceID: "p-1"}
		if len(hb.savedWords) != 1 || hb.savedWords[0] != want {
			t.Errorf("saved = %+v, want %+v", hb.savedWords, want)
		}
	})
	t.Run("update keeps local and director flag when not named", func(t *testing.T) {
		hb := mk()
		r := rec("local-word", ActionUpdate, "Mark", map[string]any{"place": "Saltreach"}, "")
		if err := (LocalWordKind{Svc: hb, Entities: pages}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if len(hb.savedWords) != 1 || hb.savedWords[0].ID != 9 || hb.savedWords[0].Local != "Grain" || !hb.savedWords[0].DMOnly {
			t.Errorf("saved = %+v", hb.savedWords)
		}
	})
	t.Run("update can clear the director flag", func(t *testing.T) {
		hb := mk()
		r := rec("local-word", ActionUpdate, "Mark", map[string]any{"place": "Saltreach", "director": false}, "")
		if err := (LocalWordKind{Svc: hb, Entities: pages}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if hb.savedWords[0].DMOnly {
			t.Error("director: false should clear the flag")
		}
	})
	t.Run("delete", func(t *testing.T) {
		hb := mk()
		r := rec("local-word", ActionDelete, "Mark", map[string]any{"place": "Saltreach"}, "")
		if err := (LocalWordKind{Svc: hb, Entities: pages}).Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(hb.deletedWords, []int{9}) {
			t.Errorf("deleted = %v", hb.deletedWords)
		}
	})
	t.Run("a place made in this paste is not there at apply time", func(t *testing.T) {
		hb := mk()
		r := rec("local-word", ActionCreate, "Rank", map[string]any{"local": "Step", "place": "New Town"}, "")
		r.Made = map[string]bool{"new-town": true}
		err := LocalWordKind{Svc: hb, Entities: pages}.Apply(ctx, camp, owner, r)
		if err == nil || len(hb.savedWords) != 0 {
			t.Errorf("err = %v saved = %v; want a refusal and nothing saved", err, hb.savedWords)
		}
	})
}

// itemsCategory is a sheet with fields that carry settings a rewrite of the
// list must not lose.
func itemsCategory() []entities.EntityType {
	return []entities.EntityType{
		{ID: 5, Name: "Item", NamePlural: "Items", Slug: "item", Fields: []entities.FieldDefinition{
			{Key: "weight", Label: "Weight", Type: "number", Section: "Basics"},
			{Key: "kind", Label: "Kind", Type: "select", Options: []string{"Gear", "Loot"}, Choices: "item_kinds", Play: &entities.FieldPlay{}},
			{Key: "secret", Label: "Secret", Type: "text", GMOnly: true},
		}},
		{ID: 6, Name: "Creature", NamePlural: "Creatures", Slug: "creature"},
	}
}

func catField(action, name string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	if _, ok := fields["category"]; !ok {
		fields["category"] = "Items"
	}
	return rec("category-field", action, name, fields, "")
}

func TestCategoryFieldKind_Create(t *testing.T) {
	ctx := context.Background()
	hb := &fakeHB{rules: []HomebrewRule{{ID: 12, Name: "Guild rank"}}}
	tests := []struct {
		name    string
		fields  map[string]any
		rname   string
		rules   HomebrewAPI
		wantErr string
		want    entities.FieldDefinition
	}{
		{"key made from the label", map[string]any{"type": "link", "links_to": "Creatures", "section": "Remnant"}, "Source creature", hb, "",
			entities.FieldDefinition{Key: "source_creature", Label: "Source creature", Type: "link", Section: "Remnant", LinkTo: 6}},
		{"links_to by singular name", map[string]any{"type": "link", "links_to": "creature"}, "Owner", hb, "",
			entities.FieldDefinition{Key: "owner", Label: "Owner", Type: "link", LinkTo: 6}},
		{"explicit key and director", map[string]any{"key": "Hidden_Note", "director": true}, "Note", hb, "",
			entities.FieldDefinition{Key: "Hidden_Note", Label: "Note", Type: "text", GMOnly: true}},
		{"plain text default", nil, "Colour", hb, "", entities.FieldDefinition{Key: "colour", Label: "Colour", Type: "text"}},
		{"select with options", map[string]any{"type": "SELECT", "options": []any{"Red", " Blue ", ""}}, "Hue", hb, "",
			entities.FieldDefinition{Key: "hue", Label: "Hue", Type: "select", Options: []string{"Red", "Blue"}}},
		{"rule by name", map[string]any{"type": "rule", "rule": "guild rank"}, "Rank", hb, "",
			entities.FieldDefinition{Key: "rank", Label: "Rank", Type: "rule", RuleID: 12}},
		{"duplicate by label", nil, "weight", hb, "already has a field", entities.FieldDefinition{}},
		{"duplicate by key", map[string]any{"key": "kind"}, "Something else", hb, "already has a field", entities.FieldDefinition{}},
		{"select without options", map[string]any{"type": "select"}, "Hue", hb, "needs `options`", entities.FieldDefinition{}},
		{"link without links_to", map[string]any{"type": "link"}, "Owner", hb, "needs `links_to`", entities.FieldDefinition{}},
		{"link to unknown category", map[string]any{"type": "link", "links_to": "Dragons"}, "Owner", hb, "no category called", entities.FieldDefinition{}},
		{"rule without rule", map[string]any{"type": "rule"}, "Rank", hb, "needs `rule`", entities.FieldDefinition{}},
		{"rules off", map[string]any{"type": "rule", "rule": "Guild rank"}, "Rank", nil, "not available", entities.FieldDefinition{}},
		{"unknown type", map[string]any{"type": "date"}, "When", hb, "is not one of", entities.FieldDefinition{}},
		{"unknown category", map[string]any{"category": "Dragons"}, "Wings", hb, "no category called", entities.FieldDefinition{}},
		{"label with no letters", nil, "!!!", hb, "needs a letter or number", entities.FieldDefinition{}},
		{"no name", nil, "", hb, "needs a name", entities.FieldDefinition{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cats := &fakeCats{types: itemsCategory()}
			k := CategoryFieldKind{Types: cats, Rules: tc.rules}
			r := catField(ActionCreate, tc.rname, tc.fields)
			if tc.wantErr != "" {
				if p := k.Plan(ctx, camp, owner, r); !strings.Contains(p.Error, tc.wantErr) {
					t.Errorf("Plan error = %q, want %q", p.Error, tc.wantErr)
				}
				if err := k.Apply(ctx, camp, owner, r); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Apply error = %v, want %q", err, tc.wantErr)
				}
				if len(cats.updated) != 0 {
					t.Error("a refused create must not save")
				}
				return
			}
			if p := k.Plan(ctx, camp, owner, r); p.Error != "" {
				t.Fatalf("Plan error = %q", p.Error)
			}
			if err := k.Apply(ctx, camp, owner, r); err != nil {
				t.Fatal(err)
			}
			if len(cats.updated) != 1 || cats.updID[0] != 5 {
				t.Fatalf("updates = %d ids %v, want one on category 5", len(cats.updated), cats.updID)
			}
			got := cats.updated[0].Fields
			if len(got) != 4 {
				t.Fatalf("sent %d fields, want the 3 old ones and the new one", len(got))
			}
			if !reflect.DeepEqual(got[:3], itemsCategory()[0].Fields) {
				t.Errorf("existing fields changed: %+v", got[:3])
			}
			if !reflect.DeepEqual(got[3], tc.want) {
				t.Errorf("new field = %+v, want %+v", got[3], tc.want)
			}
		})
	}
}

func TestCategoryFieldKind_Update(t *testing.T) {
	ctx := context.Background()
	hb := &fakeHB{rules: []HomebrewRule{{ID: 12, Name: "Guild rank"}}}
	t.Run("changes only named keys", func(t *testing.T) {
		cats := &fakeCats{types: itemsCategory()}
		k := CategoryFieldKind{Types: cats, Rules: hb}
		r := catField(ActionUpdate, "Kind", map[string]any{"options": []any{"Gear", "Loot", "Relic"}, "section": "Details"})
		if err := k.Apply(ctx, camp, owner, r); err != nil {
			t.Fatal(err)
		}
		got := cats.updated[0].Fields
		want := itemsCategory()[0].Fields
		want[1].Options = []string{"Gear", "Loot", "Relic"}
		want[1].Section = "Details"
		if !reflect.DeepEqual(got, want) {
			t.Errorf("fields = %+v\nwant     %+v", got, want)
		}
		if got[1].Choices != "item_kinds" || got[1].Play == nil {
			t.Error("Choices and Play must survive an update that does not name them")
		}
	})
	t.Run("rename_to changes the label only", func(t *testing.T) {
		cats := &fakeCats{types: itemsCategory()}
		k := CategoryFieldKind{Types: cats}
		if err := k.Apply(ctx, camp, owner, catField(ActionUpdate, "Weight", map[string]any{"rename_to": "Heft"})); err != nil {
			t.Fatal(err)
		}
		w := cats.updated[0].Fields[0]
		if w.Label != "Heft" || w.Key != "weight" || w.Type != "number" || w.Section != "Basics" {
			t.Errorf("field = %+v", w)
		}
	})
	t.Run("director false clears the flag", func(t *testing.T) {
		cats := &fakeCats{types: itemsCategory()}
		k := CategoryFieldKind{Types: cats}
		if err := k.Apply(ctx, camp, owner, catField(ActionUpdate, "Secret", map[string]any{"director": false})); err != nil {
			t.Fatal(err)
		}
		if cats.updated[0].Fields[2].GMOnly {
			t.Error("GMOnly should be cleared")
		}
	})
	t.Run("does not change the stored category", func(t *testing.T) {
		cats := &fakeCats{types: itemsCategory()}
		k := CategoryFieldKind{Types: cats}
		_ = k.Apply(ctx, camp, owner, catField(ActionUpdate, "Weight", map[string]any{"rename_to": "Heft"}))
		if cats.types[0].Fields[0].Label != "Weight" {
			t.Error("the working copy leaked into the loaded category")
		}
	})
	t.Run("missing field", func(t *testing.T) {
		k := CategoryFieldKind{Types: &fakeCats{types: itemsCategory()}}
		if p := k.Plan(ctx, camp, owner, catField(ActionUpdate, "Nope", nil)); !strings.Contains(p.Error, "has no field called") {
			t.Errorf("error = %q", p.Error)
		}
	})
	t.Run("update that breaks the type rules is refused", func(t *testing.T) {
		cats := &fakeCats{types: itemsCategory()}
		k := CategoryFieldKind{Types: cats}
		err := k.Apply(ctx, camp, owner, catField(ActionUpdate, "Weight", map[string]any{"type": "select"}))
		if err == nil || !strings.Contains(err.Error(), "needs `options`") || len(cats.updated) != 0 {
			t.Errorf("err = %v updated = %d", err, len(cats.updated))
		}
	})
}

func TestCategoryFieldKind_Delete(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		field    string
		wantKeys []string
	}{
		{"first", "Weight", []string{"kind", "secret"}},
		{"middle by label", "kind", []string{"weight", "secret"}},
		{"last", "Secret", []string{"weight", "kind"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cats := &fakeCats{types: itemsCategory()}
			k := CategoryFieldKind{Types: cats}
			r := catField(ActionDelete, tc.field, nil)
			p := k.Plan(ctx, camp, owner, r)
			if p.Error != "" || len(p.Warnings) == 0 {
				t.Fatalf("plan = %+v, want a warning and no error", p)
			}
			if err := k.Apply(ctx, camp, owner, r); err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, f := range cats.updated[0].Fields {
				keys = append(keys, f.Key)
			}
			if !reflect.DeepEqual(keys, tc.wantKeys) {
				t.Errorf("kept %v, want %v", keys, tc.wantKeys)
			}
		})
	}
	t.Run("missing field", func(t *testing.T) {
		k := CategoryFieldKind{Types: &fakeCats{types: itemsCategory()}}
		if p := k.Plan(ctx, camp, owner, catField(ActionDelete, "Nope", nil)); p.Error == "" {
			t.Error("want an error")
		}
	})
}

// A rule this paste makes comes earlier but is not saved while planning, so
// Plan only warns; Apply, run after the rule row commits, refuses a rule
// that still is not there.
func TestCategoryFieldKind_MissingRule(t *testing.T) {
	ctx := context.Background()
	cats := &fakeCats{types: itemsCategory()}
	k := CategoryFieldKind{Types: cats, Rules: &fakeHB{}}
	r := catField(ActionCreate, "Rank", map[string]any{"type": "rule", "rule": "Guild rank"})

	p := k.Plan(ctx, camp, owner, r)
	if p.Error != "" || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "no rule called") {
		t.Errorf("plan = %+v, want one warning and no error", p)
	}
	err := k.Apply(ctx, camp, owner, r)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || !strings.Contains(ae.Message, "no rule called") {
		t.Errorf("Apply err = %v, want a bad request about the rule", err)
	}
	if len(cats.updated) != 0 {
		t.Error("nothing should be saved")
	}
}

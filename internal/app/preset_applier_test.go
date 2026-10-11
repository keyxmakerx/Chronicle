package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

func TestMapPresetFieldType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"number", "number"},
		{"boolean", "checkbox"},
		{"enum", "select"},
		{"markdown", "textarea"},
		{"list", "textarea"},
		{"url", "url"},
		{"string", "text"},
		{"", "text"},
		{"something-unknown", "text"},
	}
	for _, c := range cases {
		if got := mapPresetFieldType(c.in); got != c.want {
			t.Errorf("mapPresetFieldType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMapPresetFields(t *testing.T) {
	// nil / empty → nil (service normalizes to []).
	if got := mapPresetFields(nil); got != nil {
		t.Errorf("expected nil for no fields, got %v", got)
	}

	// Maps key/label/type; drops the Foundry-sync annotations (those go to the
	// Foundry module via the character-fields API, not the entity-type schema).
	in := []systems.FieldDef{
		{Key: "might", Label: "Might", Type: "number", FoundryPath: "system.characteristics.might"},
		{Key: "backstory", Label: "Backstory", Type: "markdown"},
		{Key: "abilities_json", Label: "Abilities", Type: "string", FoundryCollection: "items"},
	}
	got := mapPresetFields(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 mapped fields, got %d", len(got))
	}
	if got[0].Key != "might" || got[0].Label != "Might" || got[0].Type != "number" {
		t.Errorf("might mapped wrong: %+v", got[0])
	}
	if got[1].Type != "textarea" { // markdown → textarea
		t.Errorf("backstory type = %q, want textarea", got[1].Type)
	}
	if got[2].Type != "text" { // string → text
		t.Errorf("abilities type = %q, want text", got[2].Type)
	}
}

// TestMapPresetFields_CarriesPlay pins that a manifest play block survives
// preset application onto the stored field definition, deep-copied, and that a
// field without one stays nil.
func TestMapPresetFields_CarriesPlay(t *testing.T) {
	zero, tru := 0.0, true
	play := &systems.PlayDef{
		Edit: "owner", Kind: "conditions", Min: &zero, MaxField: "hp_max", Step: 1,
		Options: []string{"dazed", "bleeding"}, MaxLength: 12, ToFoundry: &tru, CombatAuthority: "foundry",
	}
	got := mapPresetFields([]systems.FieldDef{
		{Key: "conditions", Label: "Conditions", Type: "string", Play: play},
		{Key: "notes", Label: "Notes", Type: "markdown"},
	})
	if got[1].Play != nil {
		t.Errorf("field without play got %+v", got[1].Play)
	}
	p := got[0].Play
	if p == nil {
		t.Fatal("play block lost in preset application")
	}
	if p.Edit != "owner" || p.Kind != "conditions" || p.MaxField != "hp_max" || p.Step != 1 ||
		p.MaxLength != 12 || p.CombatAuthority != "foundry" || p.Min == nil || *p.Min != 0 ||
		p.ToFoundry == nil || !*p.ToFoundry || len(p.Options) != 2 {
		t.Errorf("play not carried faithfully: %+v", p)
	}
	play.Options[0] = "mutated"
	if p.Options[0] != "dazed" {
		t.Error("stored options alias the manifest's slice")
	}
}

func TestPlayByCategory_SharedKeysAcrossSystems(t *testing.T) {
	owner := &systems.PlayDef{Edit: "owner", Kind: "counter"}
	gm := &systems.PlayDef{Edit: "gm", Kind: "counter"}
	sys := func(id string, fields ...systems.FieldDef) *systems.SystemManifest {
		return &systems.SystemManifest{ID: id, EntityPresets: []systems.EntityPresetDef{{Category: "character", Fields: fields}}}
	}
	cases := []struct {
		name      string
		manifests []*systems.SystemManifest
		key       string
		wantSet   bool // key present in the map
		wantPlay  bool // and carrying a block
	}{
		{"one system", []*systems.SystemManifest{sys("a", systems.FieldDef{Key: "level", Play: owner})}, "level", true, true},
		{"one system clears", []*systems.SystemManifest{sys("a", systems.FieldDef{Key: "level"})}, "level", true, false},
		{"same block twice", []*systems.SystemManifest{sys("a", systems.FieldDef{Key: "level", Play: owner}), sys("b", systems.FieldDef{Key: "level", Play: owner})}, "level", true, true},
		{"block and none", []*systems.SystemManifest{sys("a", systems.FieldDef{Key: "level"}), sys("b", systems.FieldDef{Key: "level", Play: owner})}, "level", false, false},
		{"different blocks", []*systems.SystemManifest{sys("a", systems.FieldDef{Key: "level", Play: owner}), sys("b", systems.FieldDef{Key: "level", Play: gm})}, "level", false, false},
		{"conflict stays out", []*systems.SystemManifest{sys("a", systems.FieldDef{Key: "level", Play: owner}), sys("b", systems.FieldDef{Key: "level", Play: gm}), sys("c", systems.FieldDef{Key: "level", Play: owner})}, "level", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := playByCategory(tc.manifests)["character"][tc.key]
			if ok != tc.wantSet || (got != nil) != tc.wantPlay {
				t.Errorf("present=%v play=%+v, want present=%v play=%v", ok, got, tc.wantSet, tc.wantPlay)
			}
		})
	}
}

// fakePresetEntities is the slice of the entity service the preset walk
// touches. Embedding the interface makes any other call panic, which keeps
// the test honest about what the applier is allowed to use.
type fakePresetEntities struct {
	entities.EntityService
	types      []entities.EntityType
	adoptable  bool // the default Characters type is free to adopt
	adoptErr   error
	adopted    int
	created    []entities.CreateEntityTypeInput
	reconciled []int
}

func (f *fakePresetEntities) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}

func (f *fakePresetEntities) AdoptDefaultCharacterType(_ context.Context, _, _ string, _ []entities.FieldDefinition) (*entities.EntityType, error) {
	if f.adoptErr != nil {
		return nil, f.adoptErr
	}
	if !f.adoptable {
		return nil, nil
	}
	f.adopted++
	return &entities.EntityType{ID: 1}, nil
}

func (f *fakePresetEntities) CreateEntityType(_ context.Context, _ string, in entities.CreateEntityTypeInput) (*entities.EntityType, error) {
	f.created = append(f.created, in)
	return &entities.EntityType{ID: 99}, nil
}

func (f *fakePresetEntities) ReconcileEntityTypeFields(_ context.Context, id int, _ []entities.FieldDefinition) (int, error) {
	f.reconciled = append(f.reconciled, id)
	return 0, nil
}

// TestApplyPresets_CharacterSheetHome pins where a system's character preset
// lands: on the default Characters type when it is free, on an already-bound
// type when one exists, and on a new type only when neither applies.
func TestApplyPresets_CharacterSheetHome(t *testing.T) {
	manifest := &systems.SystemManifest{ID: "sys", EntityPresets: []systems.EntityPresetDef{
		{Slug: "hero", Name: "Hero", NamePlural: "Heroes", Category: "character",
			Fields: []systems.FieldDef{{Key: "might", Label: "Might", Type: "number"}}},
	}}
	bound := "character"
	tests := []struct {
		name        string
		fake        *fakePresetEntities
		wantAdopted int
		wantCreated int
		wantReused  bool // the preset merged into an existing type instead
		wantCount   int
	}{
		{"default Characters is upgraded in place, no second type",
			&fakePresetEntities{adoptable: true}, 1, 0, false, 1},
		{"no free Characters type creates the system type",
			&fakePresetEntities{adoptable: false}, 0, 1, false, 1},
		{"a failed adoption does not create a duplicate",
			&fakePresetEntities{adoptErr: errors.New("db down")}, 0, 0, false, 0},
		{"a type already bound to the category is reused, never adopted over",
			&fakePresetEntities{adoptable: true, types: []entities.EntityType{{ID: 5, Name: "Anything", PresetCategory: &bound}}}, 0, 0, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newPresetApplier(tc.fake)
			n, err := p.applyManifestPresets(context.Background(), "c1", "sys", manifest, true, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.fake.adopted != tc.wantAdopted || len(tc.fake.created) != tc.wantCreated {
				t.Errorf("adopted=%d created=%d, want adopted=%d created=%d",
					tc.fake.adopted, len(tc.fake.created), tc.wantAdopted, tc.wantCreated)
			}
			if (len(tc.fake.reconciled) > 0) != tc.wantReused {
				t.Errorf("reconciled=%v, want reuse=%v", tc.fake.reconciled, tc.wantReused)
			}
			if n != tc.wantCount {
				t.Errorf("count = %d, want %d", n, tc.wantCount)
			}
		})
	}
}

// TestApplyPresets_ReconcileNeverAdopts keeps the upgrade path additive-only:
// a GM may have removed the system type on purpose, so ReconcileSystemPresets
// must not bind the Characters type behind their back.
func TestApplyPresets_ReconcileNeverAdopts(t *testing.T) {
	fake := &fakePresetEntities{adoptable: true}
	manifest := &systems.SystemManifest{ID: "sys", EntityPresets: []systems.EntityPresetDef{{Slug: "hero", Name: "Hero", Category: "character"}}}
	if _, err := newPresetApplier(fake).applyManifestPresets(context.Background(), "c1", "sys", manifest, false, nil); err != nil {
		t.Fatal(err)
	}
	if fake.adopted != 0 || len(fake.created) != 0 {
		t.Errorf("reconcile adopted=%d created=%d, want none", fake.adopted, len(fake.created))
	}
}

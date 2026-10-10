package entities

import (
	"context"
	"testing"
)

// presetHomeStore is an in-memory entity_types table behind the mock repo, so
// tests assert on the stored result of AdoptPresetCategory and field merges
// rather than on call shapes.
type presetHomeStore struct {
	types   map[int]*EntityType
	adopted int // AdoptPresetCategory calls that reached the store
}

func newPresetHomeStore(ts ...EntityType) *presetHomeStore {
	s := &presetHomeStore{types: map[int]*EntityType{}}
	for i := range ts {
		t := ts[i]
		s.types[t.ID] = &t
	}
	return s
}

func (s *presetHomeStore) repo() *mockEntityTypeRepo {
	return &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			cp := *s.types[id]
			return &cp, nil
		},
		findBySlugFn: func(_ context.Context, campaignID, slug string) (*EntityType, error) {
			for _, t := range s.types {
				if t.CampaignID == campaignID && t.Slug == slug {
					cp := *t
					return &cp, nil
				}
			}
			return nil, errNotFound()
		},
		listAllFn: func(context.Context) ([]EntityType, error) {
			var out []EntityType
			for _, t := range s.types {
				out = append(out, *t)
			}
			return out, nil
		},
		updateFn: func(_ context.Context, et *EntityType) error {
			cp := *et
			s.types[et.ID] = &cp
			return nil
		},
		adoptPresetCategoryFn: func(_ context.Context, toID int, category string, retireID *int) error {
			s.adopted++
			if retireID != nil {
				r := s.types[*retireID]
				r.PresetCategory, r.Enabled = nil, false
			}
			if to := s.types[toID]; to.PresetCategory == nil || *to.PresetCategory == "" {
				to.PresetCategory = &category
			}
			return nil
		},
	}
}

func errNotFound() error {
	_, err := (&mockEntityTypeRepo{}).FindByID(context.Background(), 0)
	return err
}

func charsType(id int, campaign string, preset *string) EntityType {
	return EntityType{ID: id, CampaignID: campaign, Slug: DefaultCharacterTypeSlug, Name: "Character",
		NamePlural: "Characters", Icon: "fa-user", Color: "#3b82f6", Enabled: true, PresetCategory: preset,
		Fields: []FieldDefinition{{Key: "bio", Label: "Bio", Type: "text"}}}
}

func heroType(id int, campaign string, parent *int) EntityType {
	return EntityType{ID: id, CampaignID: campaign, Slug: "hero", Name: "Hero", NamePlural: "Heroes",
		Icon: "fa-user", Color: "#5b8def", Enabled: true, PresetCategory: strp("character"), ParentTypeID: parent,
		Fields: []FieldDefinition{{Key: "might", Label: "Might", Type: "number"}, {Key: "bio", Label: "Bio", Type: "text"}}}
}

func TestAdoptDefaultCharacterType(t *testing.T) {
	declared := []FieldDefinition{{Key: "might", Label: "Might", Type: "number"}, {Key: "bio", Label: "Bio", Type: "text"}}
	tests := []struct {
		name       string
		types      []EntityType
		wantAdopt  bool
		wantPreset string // Characters' stored preset category afterwards
		wantFields int
	}{
		{"default Characters is upgraded in place", []EntityType{charsType(1, "c1", nil)}, true, "character", 2},
		{"empty preset string counts as unset", []EntityType{charsType(1, "c1", strp(""))}, true, "character", 2},
		{"no Characters type: caller creates its own", nil, false, "", 0},
		{"a different preset category is untouched", []EntityType{charsType(1, "c1", strp("player_character"))}, false, "player_character", 1},
		{"a nested type with the slug is not the default", func() []EntityType {
			c := charsType(1, "c1", nil)
			c.ParentTypeID = intp(9)
			return []EntityType{c}
		}(), false, "", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newPresetHomeStore(tc.types...)
			svc := newTestService(&mockEntityRepo{}, store.repo())

			got, err := svc.AdoptDefaultCharacterType(context.Background(), "c1", "character", declared)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if (got != nil) != tc.wantAdopt {
				t.Fatalf("adopted = %v, want %v", got != nil, tc.wantAdopt)
			}
			if len(tc.types) == 0 {
				return
			}
			stored := store.types[1]
			gotPreset := ""
			if stored.PresetCategory != nil {
				gotPreset = *stored.PresetCategory
			}
			if gotPreset != tc.wantPreset {
				t.Errorf("preset = %q, want %q", gotPreset, tc.wantPreset)
			}
			if len(stored.Fields) != tc.wantFields {
				t.Errorf("fields = %d, want %d (additive merge only when adopted)", len(stored.Fields), tc.wantFields)
			}
			if len(store.types) != len(tc.types) {
				t.Errorf("type count changed to %d; adoption must not create a second type", len(store.types))
			}
		})
	}
}

func TestReconcileCharacterPresetHome(t *testing.T) {
	tests := []struct {
		name         string
		types        []EntityType
		counts       map[int]int
		wantChanged  int
		wantCharsSet bool // Characters carries preset "character" afterwards
		wantHeroOn   bool // hero still enabled afterwards
		wantHeroSet  bool // hero still carries the preset afterwards
	}{
		{"empty top-level system type is moved and disabled",
			[]EntityType{charsType(1, "c1", nil), heroType(2, "c1", nil)}, nil, 1, true, false, false},
		{"empty system type nested under Characters is moved",
			[]EntityType{charsType(1, "c1", nil), heroType(2, "c1", intp(1))}, nil, 1, true, false, false},
		{"system type with pages is left alone",
			[]EntityType{charsType(1, "c1", nil), heroType(2, "c1", nil)}, map[int]int{2: 3}, 0, false, true, true},
		{"Characters already preset is left alone",
			[]EntityType{charsType(1, "c1", strp("player_character")), heroType(2, "c1", nil)}, nil, 0, false, true, true},
		{"no system type is a no-op",
			[]EntityType{charsType(1, "c1", nil)}, nil, 0, false, true, false},
		{"system type nested under another type is left alone",
			[]EntityType{charsType(1, "c1", nil), heroType(2, "c1", intp(7))}, nil, 0, false, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newPresetHomeStore(tc.types...)
			repo := newTestService(&mockEntityRepo{
				countByTypeFn: func(context.Context, string, int, string) (map[int]int, error) { return tc.counts, nil },
			}, store.repo())

			n, err := repo.ReconcileCharacterPresetHome(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n != tc.wantChanged {
				t.Errorf("changed = %d, want %d", n, tc.wantChanged)
			}
			chars := store.types[1]
			if got := chars.PresetCategory != nil && *chars.PresetCategory == "character"; got != tc.wantCharsSet {
				t.Errorf("Characters has character preset = %v, want %v", got, tc.wantCharsSet)
			}
			hero := store.types[2]
			if hero != nil {
				if hero.Enabled != tc.wantHeroOn {
					t.Errorf("hero enabled = %v, want %v", hero.Enabled, tc.wantHeroOn)
				}
				if got := hero.PresetCategory != nil; got != tc.wantHeroSet {
					t.Errorf("hero keeps preset = %v, want %v", got, tc.wantHeroSet)
				}
			}
			if tc.wantChanged == 1 {
				if len(chars.Fields) != 2 || chars.Fields[1].Key != "might" {
					t.Errorf("Characters fields = %+v, want bio then might (additive merge)", chars.Fields)
				}
			}
			for _, id := range []int{1, 2} {
				if store.types[id] == nil && len(tc.types) >= id {
					t.Errorf("type %d was deleted; the reconciler must never delete", id)
				}
			}

			// Second run: nothing further to do.
			n2, err := repo.ReconcileCharacterPresetHome(context.Background())
			if err != nil || n2 != 0 {
				t.Errorf("second run = (%d, %v), want (0, nil)", n2, err)
			}
		})
	}
}

// Once Characters carries the system's preset it is the system's character
// type, so the claiming add-on must not premake a Player Character type beside it.
func TestEnsurePlayerCharacterType_CharactersIsTheSystemType(t *testing.T) {
	charPreset := "character"
	tests := []struct {
		name       string
		preset     *string
		wantCreate bool
	}{
		{"Characters bound to the system preset", &charPreset, false},
		{"plain Characters still gets the premade type", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			created := false
			chars := EntityType{ID: 1, CampaignID: "camp-1", Name: "Character", Slug: DefaultCharacterTypeSlug, PresetCategory: tc.preset, Enabled: true}
			typeRepo := &mockEntityTypeRepo{
				listByCampaignFn: func(_ context.Context, _ string) ([]EntityType, error) {
					return []EntityType{chars}, nil
				},
				findByIDFn: func(_ context.Context, _ int) (*EntityType, error) { c := chars; return &c, nil },
				createFn:   func(_ context.Context, et *EntityType) error { created = true; et.ID = 9; return nil },
			}
			svc := newTestService(&mockEntityRepo{}, typeRepo)
			svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})
			if err := svc.EnsurePlayerCharacterType(context.Background(), "camp-1"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if created != tc.wantCreate {
				t.Errorf("created = %v, want %v", created, tc.wantCreate)
			}
		})
	}
}

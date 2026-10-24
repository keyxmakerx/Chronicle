package entities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func advPreset() *string { s := PresetCategoryAdventures; return &s }

func TestViewerSeesType(t *testing.T) {
	adv := &EntityType{ID: 9, PresetCategory: advPreset()}
	plain := &EntityType{ID: 1}
	other := "item"
	item := &EntityType{ID: 2, PresetCategory: &other}

	tests := []struct {
		name string
		et   *EntityType
		role int
		want bool
	}{
		{"anonymous visitor, Adventures", adv, permissions.RoleNone, false},
		{"player, Adventures", adv, permissions.RolePlayer, false},
		{"scribe, Adventures (a Scribe sees what players see)", adv, permissions.RoleScribe, false},
		{"owner, Adventures", adv, permissions.RoleOwner, true},
		{"player, ordinary category", plain, permissions.RolePlayer, true},
		{"anonymous, ordinary category", plain, permissions.RoleNone, true},
		{"player, another preset category", item, permissions.RolePlayer, true},
		{"nil type fails open only for DMOnly, never panics", nil, permissions.RolePlayer, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ViewerSeesType(tt.et, tt.role); got != tt.want {
				t.Errorf("ViewerSeesType = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTypesForViewer_DropsAdventuresBelowOwner(t *testing.T) {
	types := []EntityType{{ID: 1, Slug: "npc"}, {ID: 2, Slug: "adventures", PresetCategory: advPreset()}, {ID: 3, Slug: "place"}}

	tests := []struct {
		name string
		role int
		want []int
	}{
		{"anonymous", permissions.RoleNone, []int{1, 3}},
		{"player", permissions.RolePlayer, []int{1, 3}},
		{"scribe", permissions.RoleScribe, []int{1, 3}},
		{"owner", permissions.RoleOwner, []int{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TypesForViewer(types, tt.role)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d types, want %d", len(got), len(tt.want))
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Errorf("type %d = %d, want %d", i, got[i].ID, id)
				}
			}
		})
	}
	if len(types) != 3 {
		t.Error("filtering must not shrink the caller's slice")
	}
}

// The seed lists are what a new campaign starts from, so each must carry
// exactly one Adventures category.
func TestSeedListsCarryAdventures(t *testing.T) {
	lists := map[string][]EntityType{"standard": defaultEntityTypes}
	for _, p := range GenrePresets() {
		lists["genre:"+p.ID] = p.Types
	}
	for name, list := range lists {
		t.Run(name, func(t *testing.T) {
			n := 0
			for i := range list {
				if list[i].DMOnly() {
					n++
					if list[i].Slug != SlugAdventures || !list[i].IsDefault || !list[i].Enabled {
						t.Errorf("seeded Adventures is wrong: %+v", list[i])
					}
				}
			}
			if n != 1 {
				t.Errorf("want exactly one Adventures category, got %d", n)
			}
		})
	}
}

func TestAdventureLayout_UsesValidBlocksAndSizes(t *testing.T) {
	// The sizes mirror the block registrations in internal/app.
	sizes := map[string][]string{
		"read_aloud":      {"2x1", "2x2", "4x1", "4x2"},
		"page_table":      {"2x2", "4x2"},
		"page_steps":      {"1x2", "2x2", "4x2"},
		"page_encounters": {"1x2", "2x2", "4x2"},
		"page_map":        {"2x2", "4x2", "4x3"},
	}
	valid := func(typ string) bool {
		switch typ {
		case "title", "entry", "image", "attributes", "details", BlockSubPages, "read_aloud", "page_table", "page_steps", "page_encounters", "page_map":
			return true
		}
		return false
	}
	tileSizes := func(typ string, _ map[string]any) []string { return sizes[typ] }
	if err := ValidateLayout(AdventureLayout(), valid, nil, tileSizes); err != nil {
		t.Fatalf("the Adventures layout does not validate: %v", err)
	}
}

func TestAdventureLayout_MapFollowsTheStory(t *testing.T) {
	var ids []string
	for _, b := range AdventureLayout().Rows[0].Columns[0].Blocks {
		ids = append(ids, b.ID+":"+b.Type)
	}
	want := "blk-title:title,blk-entry:entry,blk-adv-map:page_map,blk-trials:" + BlockSubPages
	if got := strings.Join(ids, ","); got != want {
		t.Errorf("story column = %s, want %s", got, want)
	}
}

func TestCheckEntityAccess_DMOnlyCategory(t *testing.T) {
	tests := []struct {
		name     string
		role     int
		visib    VisibilityMode
		private  bool
		typeErr  bool
		wantView bool
	}{
		{"player, public page", permissions.RolePlayer, VisibilityDefault, false, false, false},
		{"anonymous, public page", permissions.RoleNone, VisibilityDefault, false, false, false},
		{"player, custom-visibility page", permissions.RolePlayer, VisibilityCustom, false, false, false},
		{"scribe, public page", permissions.RoleScribe, VisibilityDefault, false, false, false},
		{"owner", permissions.RoleOwner, VisibilityDefault, false, false, true},
		{"player, type lookup fails (fail closed)", permissions.RolePlayer, VisibilityDefault, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entityRepo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
					return &Entity{ID: "e1", EntityTypeID: 9, IsPrivate: tt.private, Visibility: tt.visib}, nil
				},
			}
			typeRepo := &mockEntityTypeRepo{
				findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
					if tt.typeErr {
						return nil, errors.New("db down")
					}
					return &EntityType{ID: id, PresetCategory: advPreset()}, nil
				},
			}
			// A custom grant that would otherwise let the player in must not
			// matter: the permission repo is a stub that always allows.
			permRepo := &mockPermissionRepo{}
			svc := NewEntityService(entityRepo, typeRepo, permRepo)
			perm, err := svc.CheckEntityAccess(context.Background(), "e1", tt.role, "u1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if perm.CanView != tt.wantView {
				t.Errorf("CanView = %v, want %v", perm.CanView, tt.wantView)
			}
			if !tt.wantView && perm.CanEdit {
				t.Error("a page the viewer cannot see must not be editable")
			}
		})
	}
}

func TestEnsureAdventures(t *testing.T) {
	t.Run("creates once with the trial layout and a free slug", func(t *testing.T) {
		var created *EntityType
		typeRepo := &mockEntityTypeRepo{
			slugExistsFn:   func(_ context.Context, _, slug string) (bool, error) { return slug == "adventures", nil },
			maxSortOrderFn: func(context.Context, string) (int, error) { return 6, nil },
			createFn:       func(_ context.Context, et *EntityType) error { et.ID = 41; created = et; return nil },
		}
		svc := NewEntityService(&mockEntityRepo{}, typeRepo, &mockPermissionRepo{}).(*entityService)
		et, made, err := svc.EnsureAdventures(context.Background(), "camp-1")
		if err != nil || !made {
			t.Fatalf("made=%v err=%v", made, err)
		}
		if created == nil || !created.DMOnly() || et.ID != 41 {
			t.Fatalf("category not created as DM-only: %+v", created)
		}
		if created.Slug != "adventures-2" {
			t.Errorf("slug = %q, a hand-made category already holds the plain one", created.Slug)
		}
		if created.SortOrder != 7 || created.CampaignID != "camp-1" {
			t.Errorf("sort order / campaign wrong: %+v", created)
		}
		if len(created.Layout.Rows) != 2 || created.Layout.Rows[1].Kind != "tiles" {
			t.Errorf("layout is not the trial page: %+v", created.Layout)
		}
	})

	t.Run("a second press creates nothing", func(t *testing.T) {
		creates := 0
		existing := EntityType{ID: 41, CampaignID: "camp-1", PresetCategory: advPreset()}
		typeRepo := &mockEntityTypeRepo{
			listByPresetCategoryFn: func(context.Context, string, string) ([]EntityType, error) { return []EntityType{existing}, nil },
			createFn:               func(context.Context, *EntityType) error { creates++; return nil },
		}
		svc := NewEntityService(&mockEntityRepo{}, typeRepo, &mockPermissionRepo{}).(*entityService)
		et, made, err := svc.EnsureAdventures(context.Background(), "camp-1")
		if err != nil || made || et.ID != 41 || creates != 0 {
			t.Fatalf("et=%+v made=%v err=%v creates=%d", et, made, err, creates)
		}
	})

	t.Run("a lost race returns the winner's category", func(t *testing.T) {
		calls := 0
		winner := EntityType{ID: 50, CampaignID: "camp-1", PresetCategory: advPreset()}
		typeRepo := &mockEntityTypeRepo{
			listByPresetCategoryFn: func(context.Context, string, string) ([]EntityType, error) {
				calls++
				if calls == 1 {
					return nil, nil
				}
				return []EntityType{winner}, nil
			},
			createFn: func(context.Context, *EntityType) error { return errors.New("duplicate slug") },
		}
		svc := NewEntityService(&mockEntityRepo{}, typeRepo, &mockPermissionRepo{}).(*entityService)
		et, made, err := svc.EnsureAdventures(context.Background(), "camp-1")
		if err != nil || made || et.ID != 50 {
			t.Fatalf("et=%+v made=%v err=%v", et, made, err)
		}
	})
}

func TestCreateEntityType_AdventuresIsReserved(t *testing.T) {
	svc := NewEntityService(&mockEntityRepo{}, &mockEntityTypeRepo{}, &mockPermissionRepo{})
	_, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{
		Name: "Secrets", NamePlural: "Secrets", Icon: "fa-x", Color: "#112233", PresetCategory: PresetCategoryAdventures,
	})
	assertAppError(t, err, 400)
}

func TestCreateEntityType_NoKindUnderAdventures(t *testing.T) {
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			return &EntityType{ID: id, CampaignID: "camp-1", PresetCategory: advPreset()}, nil
		},
	}
	svc := NewEntityService(&mockEntityRepo{}, typeRepo, &mockPermissionRepo{})
	parent := 9
	_, err := svc.CreateEntityType(context.Background(), "camp-1", CreateEntityTypeInput{
		Name: "Kind", NamePlural: "Kinds", Icon: "fa-x", Color: "#112233", ParentTypeID: &parent,
	})
	assertAppError(t, err, 400)
	if err != nil && !strings.Contains(err.Error(), "DM team only") {
		t.Errorf("error should say why: %v", err)
	}
}

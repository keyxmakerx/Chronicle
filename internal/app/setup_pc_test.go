package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakePCEntitySvc is a minimal entities.EntityService for provider tests. It
// embeds the interface so only the four methods the PC provider calls need
// implementations; any other call would nil-panic (and would be a test bug).
type fakePCEntitySvc struct {
	entities.EntityService

	snapshot    entities.PCSetupSnapshot
	ensureErr   error
	mergeResult entities.MergeResult
	mergeErr    error
	home        *entities.CharacterHomeMove
	moveResult  entities.CharacterHomeMove

	order       []string
	updatedID   int
	updatedName string
}

func (f *fakePCEntitySvc) PlayerCharacterSetupSnapshot(_ context.Context, _ string) (entities.PCSetupSnapshot, error) {
	return f.snapshot, nil
}
func (f *fakePCEntitySvc) EnsurePlayerCharacterType(_ context.Context, _ string) error {
	f.order = append(f.order, "ensure")
	return f.ensureErr
}
func (f *fakePCEntitySvc) MergeDuplicatePlayerCharacterType(_ context.Context, _ string) (entities.MergeResult, error) {
	f.order = append(f.order, "merge")
	return f.mergeResult, f.mergeErr
}
func (f *fakePCEntitySvc) CharacterHomeCandidate(_ context.Context, _ string) (*entities.CharacterHomeMove, error) {
	return f.home, nil
}
func (f *fakePCEntitySvc) MoveCharacterPresetHome(_ context.Context, _ string) (entities.CharacterHomeMove, error) {
	f.order = append(f.order, "home")
	return f.moveResult, nil
}
func (f *fakePCEntitySvc) UpdateEntityType(_ context.Context, id int, input entities.UpdateEntityTypeInput) (*entities.EntityType, error) {
	f.order = append(f.order, "update")
	f.updatedID = id
	f.updatedName, _ = input.Name.Get()
	return &entities.EntityType{ID: id, Name: f.updatedName}, nil
}

func dupSnapshot() entities.PCSetupSnapshot {
	parent := 1
	return entities.PCSetupSnapshot{
		DefaultCharsParentID: &parent,
		GenericPCTypes:       []entities.EntityType{{ID: 2, Name: "Player Characters", Slug: "player-character"}},
		SystemCharTypes:      []entities.EntityType{{ID: 3, Name: "Heroes", Slug: "drawsteel-character"}},
		GenericPCCount:       3,
		SubCategoryCount:     2,
	}
}

func hasCheck(checks []addons.SetupCheck, id string) bool {
	for _, c := range checks {
		if c.ID == id {
			return true
		}
	}
	return false
}

func hasQuestion(qs []addons.SetupQuestion, id string) bool {
	for _, q := range qs {
		if q.ID == id {
			return true
		}
	}
	return false
}

func TestPCProviderRunChecks(t *testing.T) {
	t.Run("duplicate present → emits pc.duplicate warning + naming + entities/subcats", func(t *testing.T) {
		p := newPCSetupProvider(&fakePCEntitySvc{snapshot: dupSnapshot()})
		checks, err := p.RunChecks(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, id := range []string{"pc.entities", "pc.subcategories", "pc.duplicate", "pc.naming"} {
			if !hasCheck(checks, id) {
				t.Errorf("expected check %q in %+v", id, checks)
			}
		}
	})

	t.Run("no duplicate (system only) → no pc.duplicate", func(t *testing.T) {
		parent := 1
		snap := entities.PCSetupSnapshot{
			DefaultCharsParentID: &parent,
			SystemCharTypes:      []entities.EntityType{{ID: 3, Name: "Heroes", Slug: "drawsteel-character"}},
		}
		p := newPCSetupProvider(&fakePCEntitySvc{snapshot: snap})
		checks, err := p.RunChecks(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hasCheck(checks, "pc.duplicate") {
			t.Errorf("did not expect pc.duplicate, got %+v", checks)
		}
		if !hasCheck(checks, "pc.naming") {
			t.Errorf("expected pc.naming for the surviving system type")
		}
	})
}

func TestPCProviderQuestions(t *testing.T) {
	t.Run("duplicate → naming choice + merge bool", func(t *testing.T) {
		p := newPCSetupProvider(&fakePCEntitySvc{snapshot: dupSnapshot()})
		qs, err := p.Questions(context.Background(), "camp-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !hasQuestion(qs, "naming") || !hasQuestion(qs, "merge") {
			t.Errorf("expected naming + merge questions, got %+v", qs)
		}
	})
}

func TestPCProviderApply(t *testing.T) {
	t.Run("merge=true, naming=system → ensure then merge, no rename", func(t *testing.T) {
		fake := &fakePCEntitySvc{
			snapshot:    dupSnapshot(),
			mergeResult: entities.MergeResult{Moved: 3, RemovedTypeID: 2, TargetTypeID: 3, TargetName: "Heroes"},
		}
		p := newPCSetupProvider(fake)
		res, err := p.Apply(context.Background(), "camp-1", map[string]string{"merge": "true", "naming": "system"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Completed {
			t.Error("expected Completed result")
		}
		if got := len(fake.order); got != 2 || fake.order[0] != "ensure" || fake.order[1] != "merge" {
			t.Errorf("call order = %v, want [ensure merge]", fake.order)
		}
	})

	t.Run("naming=custom → renames the surviving type after ensure", func(t *testing.T) {
		fake := &fakePCEntitySvc{snapshot: dupSnapshot()}
		p := newPCSetupProvider(fake)
		_, err := p.Apply(context.Background(), "camp-1", map[string]string{"naming": "custom", "naming_custom": "Champions"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Survivor of dupSnapshot is the system type (id 3, "Heroes").
		if fake.updatedID != 3 || fake.updatedName != "Champions" {
			t.Errorf("rename target = (id %d, name %q), want (3, Champions)", fake.updatedID, fake.updatedName)
		}
		last := fake.order[len(fake.order)-1]
		if last != "update" {
			t.Errorf("expected rename (update) to run last, order = %v", fake.order)
		}
	})

	t.Run("merge error surfaces", func(t *testing.T) {
		fake := &fakePCEntitySvc{
			snapshot: dupSnapshot(),
			mergeErr: apperror.NewConflict("ambiguous"),
		}
		p := newPCSetupProvider(fake)
		_, err := p.Apply(context.Background(), "camp-1", map[string]string{"merge": "true"})
		if err == nil {
			t.Fatal("expected the merge conflict to surface")
		}
		if appErr, ok := err.(*apperror.AppError); !ok || appErr.Code != http.StatusConflict {
			t.Errorf("error = %v, want 409 conflict", err)
		}
	})
}

func heroesHome() *entities.CharacterHomeMove {
	return &entities.CharacterHomeMove{FromTypeID: 3, FromName: "Heroes", ToTypeID: 1, ToName: "Characters", Pages: 4}
}

func TestPCProviderCharacterHome(t *testing.T) {
	sysOnly := func() entities.PCSetupSnapshot {
		parent := 1
		return entities.PCSetupSnapshot{
			DefaultCharsParentID: &parent,
			SystemCharTypes:      []entities.EntityType{{ID: 3, Name: "Heroes", Slug: "hero"}},
		}
	}

	tests := []struct {
		name     string
		home     *entities.CharacterHomeMove
		wantStep bool
	}{
		{"heroes hold pages beside Characters → check and question", heroesHome(), true},
		{"nothing to move → neither", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newPCSetupProvider(&fakePCEntitySvc{snapshot: sysOnly(), home: tc.home})
			checks, err := p.RunChecks(context.Background(), "camp-1")
			if err != nil {
				t.Fatalf("RunChecks: %v", err)
			}
			qs, err := p.Questions(context.Background(), "camp-1")
			if err != nil {
				t.Fatalf("Questions: %v", err)
			}
			if hasCheck(checks, "pc.home") != tc.wantStep || hasQuestion(qs, "home") != tc.wantStep {
				t.Errorf("pc.home check = %v, home question = %v, want both %v",
					hasCheck(checks, "pc.home"), hasQuestion(qs, "home"), tc.wantStep)
			}
		})
	}

	t.Run("home=true moves after ensure and reports it", func(t *testing.T) {
		fake := &fakePCEntitySvc{snapshot: sysOnly(), home: heroesHome(),
			moveResult: entities.CharacterHomeMove{FromName: "Heroes", ToName: "Characters", Pages: 4}}
		res, err := newPCSetupProvider(fake).Apply(context.Background(), "camp-1", map[string]string{"home": "true"})
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if len(fake.order) != 2 || fake.order[0] != "ensure" || fake.order[1] != "home" {
			t.Errorf("call order = %v, want [ensure home]", fake.order)
		}
		want := `Moved 4 pages into "Characters" and turned off "Heroes".`
		if len(res.Messages) != 1 || res.Messages[0] != want {
			t.Errorf("messages = %q, want [%q]", res.Messages, want)
		}
	})

	t.Run("home unanswered → nothing moves", func(t *testing.T) {
		fake := &fakePCEntitySvc{snapshot: sysOnly(), home: heroesHome()}
		if _, err := newPCSetupProvider(fake).Apply(context.Background(), "camp-1", map[string]string{}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		for _, step := range fake.order {
			if step == "home" {
				t.Errorf("moved without the owner's answer: %v", fake.order)
			}
		}
	})
}

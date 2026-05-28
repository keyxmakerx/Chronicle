// classifier_v1_5_test.go covers the V1.5 verb-set classifier
// extension (C-AI-WORKSPACE-V1-G): ActionMismatch status for
// update/delete rows targeting non-existent entities; no
// StatusConflict for legitimate update targets.

package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubLookup is a tiny implementation of CampaignLookup that returns
// the supplied entity for a given slug and a fixed list of types.
type stubLookup struct {
	bySlug map[string]*entities.Entity
	types  []entities.EntityType
}

func (s *stubLookup) GetBySlug(_ context.Context, _, slug string) (*entities.Entity, error) {
	if e, ok := s.bySlug[slug]; ok {
		return e, nil
	}
	return nil, nil
}

func (s *stubLookup) GetEntityTypeBySlug(_ context.Context, _, slug string) (*entities.EntityType, error) {
	for i, t := range s.types {
		if t.Slug == slug {
			return &s.types[i], nil
		}
	}
	return nil, errors.New("not found")
}

func (s *stubLookup) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return s.types, nil
}

// TestClassify_ActionUpdate_TargetFound — action=update + entity
// exists → Status stays StatusNew (legitimate update target).
// ConflictEntity is populated for UI rendering ("Updating X").
func TestClassify_ActionUpdate_TargetFound(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-1", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
		types: []entities.EntityType{{ID: 1, Slug: "character"}},
	}
	clf := NewClassifier(lookup, "camp-1")
	pages := []ParsedPage{
		{
			Name:        "Lyra Vance",
			FrontMatter: FrontMatter{Name: "Lyra Vance", Type: "character", Action: ActionUpdate},
			Status:      StatusNew,
		},
	}
	cls, err := clf.ClassifyAll(context.Background(), pages)
	if err != nil {
		t.Fatalf("ClassifyAll: %v", err)
	}
	if cls[0].Status != StatusNew {
		t.Errorf("expected StatusNew (legitimate update target); got %q", cls[0].Status)
	}
	if cls[0].ConflictEntity == nil {
		t.Error("expected ConflictEntity populated so UI can render 'Updating X'")
	}
}

// TestClassify_ActionUpdate_TargetMissing — action=update + no entity
// → StatusActionMismatch with friendly reason.
func TestClassify_ActionUpdate_TargetMissing(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{},
		types:  []entities.EntityType{{ID: 1, Slug: "character"}},
	}
	clf := NewClassifier(lookup, "camp-1")
	pages := []ParsedPage{
		{
			Name:        "Ghost",
			FrontMatter: FrontMatter{Name: "Ghost", Type: "character", Action: ActionUpdate},
			Status:      StatusNew,
		},
	}
	cls, _ := clf.ClassifyAll(context.Background(), pages)
	if cls[0].Status != StatusActionMismatch {
		t.Errorf("expected StatusActionMismatch; got %q", cls[0].Status)
	}
	if !strings.Contains(cls[0].Reason, "Cannot update") {
		t.Errorf("expected reason containing 'Cannot update'; got %q", cls[0].Reason)
	}
}

// TestClassify_ActionDelete_TargetFound — action=delete + entity
// exists → StatusNew (legitimate delete target); UI's action chip
// branches on FrontMatter.Action for the rendering distinction.
func TestClassify_ActionDelete_TargetFound(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{
			"old-thing": {ID: "ent-old", Name: "Old Thing", Slug: "old-thing"},
		},
		types: []entities.EntityType{{ID: 1, Slug: "character"}},
	}
	clf := NewClassifier(lookup, "camp-1")
	pages := []ParsedPage{
		{
			Name:        "Old Thing",
			FrontMatter: FrontMatter{Name: "Old Thing", Type: "character", Action: ActionDelete},
			Status:      StatusNew,
		},
	}
	cls, _ := clf.ClassifyAll(context.Background(), pages)
	if cls[0].Status != StatusNew {
		t.Errorf("expected StatusNew (legitimate delete target); got %q", cls[0].Status)
	}
	if cls[0].ConflictEntity == nil {
		t.Error("expected ConflictEntity populated so UI can render 'Deleting X'")
	}
}

// TestClassify_ActionDelete_TargetMissing — action=delete + no entity
// → StatusActionMismatch with friendly reason.
func TestClassify_ActionDelete_TargetMissing(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{},
		types:  []entities.EntityType{{ID: 1, Slug: "character"}},
	}
	clf := NewClassifier(lookup, "camp-1")
	pages := []ParsedPage{
		{
			Name:        "Ghost",
			FrontMatter: FrontMatter{Name: "Ghost", Type: "character", Action: ActionDelete},
			Status:      StatusNew,
		},
	}
	cls, _ := clf.ClassifyAll(context.Background(), pages)
	if cls[0].Status != StatusActionMismatch {
		t.Errorf("expected StatusActionMismatch; got %q", cls[0].Status)
	}
	if !strings.Contains(cls[0].Reason, "Cannot delete") {
		t.Errorf("expected reason containing 'Cannot delete'; got %q", cls[0].Reason)
	}
}

// TestClassify_ActionCreate_ExistingSlug_StatusConflict — preserve V1
// behavior: action=create (or empty default) + slug-collision yields
// StatusConflict, NOT a Mismatch. The operator picks Skip / Rename /
// Update from the conflict-mode dropdown.
func TestClassify_ActionCreate_ExistingSlug_StatusConflict(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-existing", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
		types: []entities.EntityType{{ID: 1, Slug: "character"}},
	}
	clf := NewClassifier(lookup, "camp-1")
	pages := []ParsedPage{
		{
			Name:        "Lyra Vance",
			FrontMatter: FrontMatter{Name: "Lyra Vance", Type: "character", Action: ActionCreate},
			Status:      StatusNew,
		},
	}
	cls, _ := clf.ClassifyAll(context.Background(), pages)
	if cls[0].Status != StatusConflict {
		t.Errorf("expected StatusConflict (existing slug + action=create); got %q", cls[0].Status)
	}
}

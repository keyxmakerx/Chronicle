// permissions_inline_component_test.go pins the invariants that keep the
// legacy DM-only checkbox from creeping back into the create/edit forms and
// guard against a silent flip-to-public data-disclosure regression:
//
//   1. EntityCreateFormComponent must not render a `<input ... id="is_private" ...>`
//      checkbox. Permissions are set with the page's visibility chip instead.
//
//   2. EntityEditFormComponent must not render a hidden
//      `<input type="hidden" name="is_private" ...>` — the form no longer
//      carries is_private at all, since UpdateEntityInput.IsPrivate is
//      nil-preserving.
//
//   3. The polymorphic Update service preserves the entity's current
//      IsPrivate when input.IsPrivate is nil, so the form/metadata handlers
//      can drop is_private without flipping every entity to public on save.

package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestEntityCreateFormComponent_NoIsPrivateCheckbox pins that the
// create form no longer renders the legacy DM-only checkbox. The page's
// visibility chip replaces it; the hidden `is_private` input the chip's
// editor writes is allowed (and necessary).
func TestEntityCreateFormComponent_NoIsPrivateCheckbox(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}}
	entityTypes := []EntityType{{ID: 1, Name: "Character", Enabled: true}}
	component := EntityCreateFormComponent(cc, entityTypes, 1, nil, nil, "", "csrf", "")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	// Hidden input written by the chip's editor stays — the create
	// handler still reads is_private from form post. What must NOT
	// be present is the user-visible checkbox.
	if strings.Contains(html, `type="checkbox" id="is_private"`) {
		t.Errorf("create form re-introduced the DM-only checkbox; dispatch C-PERMISSIONS-INLINE-COMPONENT removed it")
	}
	if strings.Contains(html, `Private (visible to GM only)`) {
		t.Errorf("create form re-introduced the 'Private (visible to GM only)' label; dispatch C-PERMISSIONS-INLINE-COMPONENT removed it")
	}

	// Positive assertion: the permissions mount must be present, as the
	// page's own chip unfolding the corner editor, not the old slide-in.
	if !strings.Contains(html, `data-widget="permissions"`) {
		t.Errorf("create form is missing the permissions widget mount")
	}
	for _, want := range []string{`data-layout="corner"`, `class="perm-chip"`, "data-perm-corner-trigger", `id="is_private_draft"`, `data-draft-target="#is_private_draft"`} {
		if !strings.Contains(html, want) {
			t.Errorf("create form's visibility control is missing %s; it must be the page's chip in draft mode", want)
		}
	}
	if !strings.Contains(html, `data-mode="draft"`) {
		t.Errorf("create form's permissions mount must declare draft mode so the widget skips the (nonexistent) save endpoint")
	}
}

// TestEntityEditFormComponent_NoHiddenIsPrivate pins that the edit form
// ships no hidden `is_private` input. The service layer preserves
// IsPrivate when the field is absent (UpdateEntityInput.IsPrivate is *bool),
// so re-adding the hidden input would silently round-trip stale state.
func TestEntityEditFormComponent_NoHiddenIsPrivate(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Test"},
		MemberRole: campaigns.RoleOwner,
	}
	priv := true
	entity := &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Gandalf", IsPrivate: priv}
	entityType := &EntityType{ID: 1, Name: "Character"}

	component := EntityEditFormComponent(cc, entity, entityType, nil, "csrf", "")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, `name="is_private"`) {
		t.Errorf("edit form re-introduced a `name=\"is_private\"` input; the form no longer carries this field — see UpdateEntityRequest in model.go")
	}
	if !strings.Contains(html, `data-widget="permissions"`) {
		t.Errorf("edit form is missing the permissions widget mount")
	}
}

// TestUpdate_PreservesIsPrivateWhenInputNil pins that when the
// form/metadata handlers omit is_private (UpdateEntityInput.IsPrivate ==
// nil), the service keeps the entity's existing IsPrivate value — otherwise
// every entity-title save silently flips it to public.
func TestUpdate_PreservesIsPrivateWhenInputNil(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				CampaignID: "camp-1",
				Name:       "Gandalf",
				Slug:       "gandalf",
				IsPrivate:  true, // entity was private before the save.
			}, nil
		},
	}

	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name: patch.Of("Gandalf the White"),
		// IsPrivate intentionally nil — same shape the form/metadata
		// handlers produce now that the UI toggle is gone.
	})
	if err != nil {
		t.Fatalf("Update returned unexpected error: %v", err)
	}
	if !entity.IsPrivate {
		t.Errorf("Update flipped IsPrivate from true→false when input.IsPrivate was nil; the dispatch's data-disclosure guard regressed")
	}
}

// TestUpdate_SetsIsPrivateWhenInputNonNil pins the other half: the
// permissions API (and syncapi callers that still write through
// UpdateEntityInput) must be able to set IsPrivate explicitly. A
// regression that ignored a non-nil pointer would mean Foundry sync
// and the /permissions endpoint silently dropped writes.
func TestUpdate_SetsIsPrivateWhenInputNonNil(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
			return &Entity{
				ID:         "ent-1",
				CampaignID: "camp-1",
				Name:       "Gandalf",
				Slug:       "gandalf",
				IsPrivate:  false,
			}, nil
		},
	}

	priv := true
	svc := newTestService(entityRepo, &mockEntityTypeRepo{})
	entity, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{
		Name:      patch.Of("Gandalf"),
		IsPrivate: &priv,
	})
	if err != nil {
		t.Fatalf("Update returned unexpected error: %v", err)
	}
	if !entity.IsPrivate {
		t.Errorf("Update ignored non-nil input.IsPrivate; expected IsPrivate=true after explicit set")
	}
}

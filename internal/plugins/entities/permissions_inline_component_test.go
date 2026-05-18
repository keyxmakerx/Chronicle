// permissions_inline_component_test.go — pins the
// C-PERMISSIONS-INLINE-COMPONENT invariants that prevent the legacy
// DM-only checkbox from creeping back into the create form / edit form
// and that protect against the silent flip-to-public regression the
// dispatch flagged as a data-disclosure risk.
//
// The risks pinned here are:
//
//   1. EntityCreateFormComponent must not render a `<input ... id="is_private" ...>`
//      checkbox. Operators reach for "Page Permissions" via the slide-in card.
//      A new is_private checkbox would re-introduce the duplicate-affordance
//      confusion the dispatch eliminated.
//
//   2. EntityEditFormComponent must not render the legacy hidden
//      `<input type="hidden" name="is_private" ...>` that used to ride
//      on the edit-form save. The form no longer carries is_private at
//      all — UpdateEntityInput.IsPrivate is nil-preserving — so adding
//      it back would also be a regression.
//
//   3. The polymorphic Update service preserves the entity's current
//      IsPrivate when input.IsPrivate is nil. This is the load-bearing
//      service-layer behavior that lets the form/metadata handlers drop
//      is_private without flipping every entity to public on next save.

package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestEntityCreateFormComponent_NoIsPrivateCheckbox pins that the
// create form no longer renders the legacy DM-only checkbox. The
// permissions slide-in card replaces it; the hidden `is_private` input
// driven by the card is allowed (and necessary).
func TestEntityCreateFormComponent_NoIsPrivateCheckbox(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}}
	entityTypes := []EntityType{{ID: 1, Name: "Character", Enabled: true}}
	component := EntityCreateFormComponent(cc, entityTypes, 1, nil, nil, "csrf", "")

	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()

	// Hidden input set up by the slide-in card stays — the create
	// handler still reads is_private from form post. What must NOT
	// be present is the user-visible checkbox.
	if strings.Contains(html, `type="checkbox" id="is_private"`) {
		t.Errorf("create form re-introduced the DM-only checkbox; dispatch C-PERMISSIONS-INLINE-COMPONENT removed it")
	}
	if strings.Contains(html, `Private (visible to GM only)`) {
		t.Errorf("create form re-introduced the 'Private (visible to GM only)' label; dispatch C-PERMISSIONS-INLINE-COMPONENT removed it")
	}

	// Positive assertion: the permissions card mount must be present
	// so the JS widget renders the slide-in trigger in its place.
	if !strings.Contains(html, `data-widget="permissions"`) {
		t.Errorf("create form is missing the permissions widget mount; the slide-in card cannot render without it")
	}
	if !strings.Contains(html, `data-mode="draft"`) {
		t.Errorf("create form's permissions mount must declare draft mode so the widget skips the (nonexistent) save endpoint")
	}
}

// TestEntityEditFormComponent_NoHiddenIsPrivate pins that the edit
// form no longer ships the legacy hidden `is_private` input. The
// service layer preserves IsPrivate when the field is absent
// (UpdateEntityInput.IsPrivate is *bool / nil-preserving), so any
// regression that re-adds the hidden input either ignores the new
// preservation contract or silently round-trips stale state.
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

// TestUpdate_PreservesIsPrivateWhenInputNil pins the service-level
// behavior that backs the "silently flip-to-public" guard the
// dispatch flagged as critical. When the form/metadata handlers omit
// is_private (i.e. UpdateEntityInput.IsPrivate == nil), the service
// must keep the entity's existing IsPrivate value. Without this, the
// permissions toggle removal becomes a data-disclosure regression on
// every entity-title save.
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
		Name: "Gandalf the White",
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
		Name:      "Gandalf",
		IsPrivate: &priv,
	})
	if err != nil {
		t.Fatalf("Update returned unexpected error: %v", err)
	}
	if !entity.IsPrivate {
		t.Errorf("Update ignored non-nil input.IsPrivate; expected IsPrivate=true after explicit set")
	}
}

// entity_partial_update_test.go — sweep R4, the service half of the
// entities absent-means-preserve contract.
//
// Before this, entityService.Update assigned ParentID and TypeLabel
// unguarded with "" meaning "clear". Every partial caller therefore
// un-parented the entity and erased its descriptor whether it meant to or
// not: syncapi's update body had no parent_id member at all, and the
// AI-workspace commit-update path never sent one either.
package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// parentedEntity is the stored row every case starts from: parented, with a
// descriptor, private, and with a body.
func parentedEntityRepo() *mockEntityRepo {
	return &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			parent := "ent-parent"
			label := "Spymaster"
			entry := "the stored body"
			entryHTML := "<p>the stored body</p>"
			switch id {
			case "ent-parent", "ent-other-parent":
				return &Entity{ID: id, CampaignID: "camp-1", Name: "Parent"}, nil
			default:
				return &Entity{
					ID:         id,
					CampaignID: "camp-1",
					Name:       "Shadow Contact",
					Slug:       "shadow-contact",
					ParentID:   &parent,
					TypeLabel:  &label,
					IsPrivate:  true,
					Entry:      &entry,
					EntryHTML:  &entryHTML,
				}, nil
			}
		},
		findAncestorsFn: func(_ context.Context, _ string) ([]Entity, error) { return nil, nil },
		slugExistsFn:    func(_ context.Context, _, _ string) (bool, error) { return false, nil },
	}
}

func TestUpdate_ParentID_AbsentPreserves_PresentReplaces_EmptyOrNullClears(t *testing.T) {
	cases := []struct {
		name  string
		input patch.Field[string]
		want  *string // nil = must end up unparented
	}{
		{"absent preserves the parent", patch.Absent[string](), strPtr("ent-parent")},
		{"present replaces the parent", patch.Of("ent-other-parent"), strPtr("ent-other-parent")},
		{"explicit null clears the parent", patch.Null[string](), nil},
		{"present empty string clears the parent (legacy spelling of the same intent)", patch.Of(""), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(parentedEntityRepo(), &mockEntityTypeRepo{})
			got, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{ParentID: tc.input})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			switch {
			case tc.want == nil && got.ParentID != nil:
				t.Errorf("ParentID = %q, want nil", *got.ParentID)
			case tc.want != nil && got.ParentID == nil:
				t.Errorf("ParentID = nil, want %q", *tc.want)
			case tc.want != nil && *got.ParentID != *tc.want:
				t.Errorf("ParentID = %q, want %q", *got.ParentID, *tc.want)
			}
		})
	}
}

func TestUpdate_TypeLabel_AbsentPreserves_PresentReplaces_EmptyOrNullClears(t *testing.T) {
	cases := []struct {
		name  string
		input patch.Field[string]
		want  *string
	}{
		{"absent preserves the descriptor", patch.Absent[string](), strPtr("Spymaster")},
		{"present replaces the descriptor", patch.Of("Fence"), strPtr("Fence")},
		{"explicit null clears the descriptor", patch.Null[string](), nil},
		{"present empty string clears the descriptor", patch.Of(""), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(parentedEntityRepo(), &mockEntityTypeRepo{})
			got, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{TypeLabel: tc.input})
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			switch {
			case tc.want == nil && got.TypeLabel != nil:
				t.Errorf("TypeLabel = %q, want nil", *got.TypeLabel)
			case tc.want != nil && got.TypeLabel == nil:
				t.Errorf("TypeLabel = nil, want %q", *tc.want)
			case tc.want != nil && *got.TypeLabel != *tc.want:
				t.Errorf("TypeLabel = %q, want %q", *got.TypeLabel, *tc.want)
			}
		})
	}
}

// An input that mentions nothing must change nothing — the shape every
// {fields_data}-only or {status}-only push reduces to at this layer.
func TestUpdate_EmptyInputChangesNothing(t *testing.T) {
	svc := newTestService(parentedEntityRepo(), &mockEntityTypeRepo{})
	got, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "Shadow Contact" {
		t.Errorf("Name = %q, want preserved", got.Name)
	}
	if got.Slug != "shadow-contact" {
		t.Errorf("Slug = %q, want preserved (an absent name must not regenerate the slug)", got.Slug)
	}
	if got.ParentID == nil || *got.ParentID != "ent-parent" {
		t.Errorf("ParentID = %v, want preserved", got.ParentID)
	}
	if got.TypeLabel == nil || *got.TypeLabel != "Spymaster" {
		t.Errorf("TypeLabel = %v, want preserved", got.TypeLabel)
	}
	if !got.IsPrivate {
		t.Error("IsPrivate flipped to false on an empty input — the privacy break")
	}
	if got.Entry == nil || *got.Entry != "the stored body" {
		t.Errorf("Entry = %v, want preserved", got.Entry)
	}
}

// Entry keeps its long-standing "" == preserve reading byte-for-byte; an
// explicit null is the new, and only, way to blank a body through this
// input. No shipped client sends one, which is the point: the contract is
// complete without moving anybody's behaviour.
func TestUpdate_Entry_EmptyStringPreserves_NullClears(t *testing.T) {
	svc := newTestService(parentedEntityRepo(), &mockEntityTypeRepo{})
	got, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{Entry: patch.Of("")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Entry == nil || *got.Entry != "the stored body" {
		t.Errorf("Entry = %v, want preserved on an empty string (unchanged legacy semantics)", got.Entry)
	}

	svc = newTestService(parentedEntityRepo(), &mockEntityTypeRepo{})
	got, err = svc.Update(context.Background(), "ent-1", UpdateEntityInput{Entry: patch.Null[string]()})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Entry != nil || got.EntryHTML != nil {
		t.Errorf("Entry/EntryHTML = %v/%v, want both nil on an explicit null", got.Entry, got.EntryHTML)
	}
}

// The parent validations still fire when a parent IS sent — the guard added
// around them must not have made them unreachable.
func TestUpdate_ParentValidationsStillFireWhenParentIsSent(t *testing.T) {
	svc := newTestService(parentedEntityRepo(), &mockEntityTypeRepo{})
	if _, err := svc.Update(context.Background(), "ent-1", UpdateEntityInput{ParentID: patch.Of("ent-1")}); err == nil {
		t.Error("self-parenting must still be rejected")
	}
}

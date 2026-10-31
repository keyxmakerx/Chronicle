package entities

import (
	"context"
	"strings"
	"testing"
)

// search_text is matched for every reader, so a save indexes neither GM-only
// nor owner-only field values, and nothing at all when the category can't
// be read.
func TestUpdateFields_SearchTextLeavesRestrictedValuesOut(t *testing.T) {
	tests := []struct {
		name      string
		typeFound bool
		want      []string
		wantGone  []string
	}{
		{"category read", true, []string{"Barkeep"}, []string{"spy", "orphan"}},
		{"category unreadable", false, nil, []string{"spy", "orphan", "Barkeep"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, id string) (*Entity, error) {
					return &Entity{ID: id, CampaignID: "c1", EntityTypeID: 1}, nil
				},
				updateFieldsFn: func(_ context.Context, _ string, _ map[string]any, searchText string) error {
					got = searchText
					return nil
				},
			}
			types := &mockEntityTypeRepo{findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
				if !tt.typeFound {
					return nil, context.Canceled
				}
				return &EntityType{ID: id, CampaignID: "c1", Fields: []FieldDefinition{
					{Key: "title"}, {Key: "gm_notes", GMOnly: true}, {Key: "backstory", OwnerOnly: true},
				}}, nil
			}}
			svc := newTestService(repo, types)
			if err := svc.UpdateFields(context.Background(), "e1", map[string]any{"title": "Barkeep", "gm_notes": "spy", "backstory": "orphan"}); err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("search_text %q lacks %q", got, w)
				}
			}
			for _, w := range tt.wantGone {
				if strings.Contains(got, w) {
					t.Errorf("search_text %q holds %q", got, w)
				}
			}
		})
	}
}

func TestRestrictedFieldSet(t *testing.T) {
	a := []FieldDefinition{{Key: "x"}, {Key: "gm", GMOnly: true}}
	b := []FieldDefinition{{Key: "gm", GMOnly: true}, {Key: "x", OwnerOnly: true}}
	if restrictedFieldSet(a) == restrictedFieldSet(b) {
		t.Error("making a field owner-only was not seen as a change")
	}
	if restrictedFieldSet(a) != restrictedFieldSet([]FieldDefinition{{Key: "gm", GMOnly: true}, {Key: "y"}}) {
		t.Error("an open field change was seen as a restriction change")
	}
}

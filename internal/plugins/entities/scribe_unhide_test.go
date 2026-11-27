package entities

import (
	"reflect"
	"testing"
)

// A writer who may not show hidden content keeps every hidden page field as
// stored: they can neither clear its mark nor drop it, by a save or a reset.
func TestKeepRestrictedOverrides(t *testing.T) {
	secret := FieldDefinition{Key: "gm_notes", Label: "Notes", GMOnly: true}
	backstory := FieldDefinition{Key: "backstory", Label: "Backstory", OwnerOnly: true}
	open := FieldDefinition{Key: "motto", Label: "Motto"}
	stored := &FieldOverrides{Added: []FieldDefinition{secret, open, backstory}, Hidden: []string{"x"}}

	cases := []struct {
		name     string
		incoming *FieldOverrides
		want     *FieldOverrides
	}{
		{
			name:     "an unmarked copy of a hidden field keeps the stored mark",
			incoming: &FieldOverrides{Added: []FieldDefinition{{Key: "gm_notes", Label: "Notes"}, open}},
			want:     &FieldOverrides{Added: []FieldDefinition{open, secret, backstory}},
		},
		{
			name:     "dropping hidden fields does not remove them",
			incoming: &FieldOverrides{Hidden: []string{"y"}},
			want:     &FieldOverrides{Hidden: []string{"y"}, Added: []FieldDefinition{secret, backstory}},
		},
		{
			name:     "a reset keeps only the hidden fields",
			incoming: nil,
			want:     &FieldOverrides{Added: []FieldDefinition{secret, backstory}},
		},
		{
			name:     "the writer's own new fields are kept",
			incoming: &FieldOverrides{Added: []FieldDefinition{secret, backstory, {Key: "new"}}},
			want:     &FieldOverrides{Added: []FieldDefinition{{Key: "new"}, secret, backstory}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := keepRestrictedOverrides(stored, tc.incoming)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}

	// Nothing hidden: the save is the writer's, as sent.
	in := &FieldOverrides{Added: []FieldDefinition{open}}
	if got := keepRestrictedOverrides(&FieldOverrides{Added: []FieldDefinition{open}}, in); got != in {
		t.Errorf("a list with nothing hidden should pass through")
	}
	if got := keepRestrictedOverrides(nil, nil); got != nil {
		t.Errorf("a reset with nothing hidden stays a reset")
	}
}

// Moving a page to a category that leaves a hidden key unmarked would show
// its stored value; such a move is refused to a Scribe.
func TestTypeChangeUnhides(t *testing.T) {
	heroes := &EntityType{ID: 1, Fields: []FieldDefinition{{Key: "gm_notes", GMOnly: true}, {Key: "backstory", OwnerOnly: true}}}
	places := &EntityType{ID: 2, Fields: []FieldDefinition{{Key: "gm_notes"}}}
	stricter := &EntityType{ID: 3, Fields: []FieldDefinition{{Key: "gm_notes", GMOnly: true}, {Key: "backstory", GMOnly: true}}}
	ownerOnly := &EntityType{ID: 4, Fields: []FieldDefinition{{Key: "gm_notes", OwnerOnly: true}, {Key: "backstory", OwnerOnly: true}}}

	page := func(data map[string]any, ov *FieldOverrides) *Entity {
		return &Entity{FieldsData: data, FieldOverrides: ov}
	}
	cases := []struct {
		name     string
		e        *Entity
		from, to *EntityType
		want     bool
	}{
		{"a GM-only value into a category that shows it", page(map[string]any{"gm_notes": "the duke lies"}, nil), heroes, places, true},
		{"an owner-only value into a category without the key", page(map[string]any{"backstory": "orphan"}, nil), heroes, places, true},
		{"GM-only down to owner-only", page(map[string]any{"gm_notes": "x"}, nil), heroes, ownerOnly, true},
		{"hidden at least as far", page(map[string]any{"gm_notes": "x", "backstory": "y"}, nil), heroes, stricter, false},
		{"no stored value", page(map[string]any{"gm_notes": ""}, nil), heroes, places, false},
		{"a page's own hidden field travels with it", page(map[string]any{"secret": "x"}, &FieldOverrides{Added: []FieldDefinition{{Key: "secret", GMOnly: true}}}), places, places, false},
		{"an unknown category refuses", page(nil, nil), nil, places, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := typeChangeUnhides(tc.e, tc.from, tc.to); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

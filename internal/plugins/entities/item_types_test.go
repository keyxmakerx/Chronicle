package entities

import "testing"

func TestIsItemType(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		t    EntityType
		want bool
	}{
		{"the Items category a campaign starts with", EntityType{Slug: "item"}, true},
		{"the Items category with an empty preset", EntityType{Slug: "item", PresetCategory: str("")}, true},
		{"a system's item preset", EntityType{Slug: "dnd5e-magic-item", PresetCategory: str("item")}, true},
		{"an Items slug bound to another preset", EntityType{Slug: "item", PresetCategory: str("creature")}, false},
		{"a category named like items", EntityType{Slug: "items"}, false},
		{"characters", EntityType{Slug: "character", PresetCategory: str("character")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsItemType(tc.t); got != tc.want {
				t.Fatalf("IsItemType = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestItemTypes(t *testing.T) {
	str := func(s string) *string { return &s }
	types := []EntityType{
		{ID: 1, Slug: "character"},
		{ID: 2, Slug: "item"},
		{ID: 3, Slug: "dnd5e-spell", PresetCategory: str("item")},
		{ID: 4, Slug: "shop"},
	}
	got := ItemTypes(types)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("ItemTypes = %+v, want types 2 and 3 in order", got)
	}
}

package entities

import "testing"

func TestItemLookType(t *testing.T) {
	sub := "Dagger"
	blank := "  "
	cases := []struct {
		name string
		e    *Entity
		want string
	}{
		{"subtype wins", &Entity{TypeLabel: &sub, FieldsData: map[string]any{"type": "Weapon"}}, "Dagger"},
		{"sheet type when no subtype", &Entity{FieldsData: map[string]any{"type": "Potion"}}, "Potion"},
		{"blank subtype falls through", &Entity{TypeLabel: &blank, FieldsData: map[string]any{"type": "Ring"}}, "Ring"},
		{"non-text type is ignored", &Entity{FieldsData: map[string]any{"type": 3}}, ""},
		{"nothing", &Entity{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := itemLookType(tc.e); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

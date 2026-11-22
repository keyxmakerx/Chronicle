package entities

import "testing"

// The blocks that draw a page's sheet or its fields are declared in the
// registry, so a feature that warns about losing them (AI layout review)
// never keeps its own list.
func TestBlockRegistry_ShowsSheet(t *testing.T) {
	r := NewBlockRegistry()
	RegisterCoreBlocks(r)
	tests := []struct {
		block string
		want  bool
	}{
		{"attributes", true},
		{"system_panels", true},
		{"character_surface", true},
		{"game_system_sheet", true},
		{"title", false},
		{"entry", false},
		{"details", false},
		{"no_such_block", false},
	}
	for _, tc := range tests {
		if got := r.ShowsSheet(tc.block); got != tc.want {
			t.Errorf("ShowsSheet(%q) = %v, want %v", tc.block, got, tc.want)
		}
	}
}

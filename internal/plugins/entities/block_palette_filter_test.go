package entities

import "testing"

// TestHideRedundantCharacterSurface covers the palette rule that drops the
// generic core "character_surface" block when a game system owns the character
// sheet (its page renderer takes over character pages), while keeping it for
// system-agnostic campaigns.
func TestHideRedundantCharacterSurface(t *testing.T) {
	base := []BlockMeta{
		{Type: "title", Label: "Title"},
		{Type: "character_surface", Label: "Character Sheet"},
		{Type: "ext_widget", Label: "Monster Builder", WidgetSlug: "monster-builder"},
		{Type: "details", Label: "Details"},
	}

	tests := []struct {
		name               string
		systemOwns         bool
		wantLen            int
		wantHasCharSurface bool
	}{
		{
			name:               "system owns the sheet drops character_surface",
			systemOwns:         true,
			wantLen:            3,
			wantHasCharSurface: false,
		},
		{
			name:               "no system renderer keeps character_surface",
			systemOwns:         false,
			wantLen:            4,
			wantHasCharSurface: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := make([]BlockMeta, len(base))
			copy(in, base) // isolate cases — helper may reuse/return a new slice
			got := hideRedundantCharacterSurface(in, tt.systemOwns)

			if len(got) != tt.wantLen {
				t.Fatalf("len(got) = %d, want %d", len(got), tt.wantLen)
			}
			has := false
			for _, b := range got {
				if b.Type == "character_surface" {
					has = true
				}
			}
			if has != tt.wantHasCharSurface {
				t.Errorf("character_surface present = %v, want %v", has, tt.wantHasCharSurface)
			}
			// The system's own widget block must always survive the filter.
			foundWidget := false
			for _, b := range got {
				if b.WidgetSlug == "monster-builder" {
					foundWidget = true
				}
			}
			if !foundWidget {
				t.Error("system widget block (monster-builder) was unexpectedly removed")
			}
		})
	}
}

// TestHideRedundantCharacterSurface_NoBlockPresent guards the no-op path: when
// systemOwns is true but the list has no character_surface, it returns the list
// content unchanged.
func TestHideRedundantCharacterSurface_NoBlockPresent(t *testing.T) {
	in := []BlockMeta{{Type: "title"}, {Type: "details"}}
	got := hideRedundantCharacterSurface(in, true)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

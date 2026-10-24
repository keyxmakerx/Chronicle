package systems

import "testing"

// TestHasCharacterSheet pins what counts as a system drawing a character
// sheet: a renderer on the character category, or on a character preset's
// slug. A renderer for some other kind of page is not a sheet.
func TestHasCharacterSheet(t *testing.T) {
	presets := []EntityPresetDef{
		{Slug: "x-character", Category: "character"},
		{Slug: "x-creature", Category: "creature"},
	}
	tests := []struct {
		name string
		m    *SystemManifest
		want bool
	}{
		{"nil manifest", nil, false},
		{"no renderers, as the D&D package ships", &SystemManifest{EntityPresets: presets}, false},
		{"category binding", &SystemManifest{EntityPresets: presets,
			Renderers: []RendererDef{{PresetCategory: "character", Widget: "sheet"}}}, true},
		{"character preset slug", &SystemManifest{EntityPresets: presets,
			Renderers: []RendererDef{{Slug: "x-character", Widget: "sheet"}}}, true},
		{"creature page only", &SystemManifest{EntityPresets: presets,
			Renderers: []RendererDef{{Slug: "x-creature", Widget: "statblock"}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.m.HasCharacterSheet(); got != tc.want {
				t.Fatalf("HasCharacterSheet() = %v, want %v", got, tc.want)
			}
		})
	}
}

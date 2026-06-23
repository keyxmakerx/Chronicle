package entities

import "testing"

// TestCharacterSheetPaletteDedupe verifies the layout-editor palette logic in
// BlockTypesAPI: when the campaign's game system ships its own character-sheet
// widget, the generic core character_surface block is dropped so the editor
// offers exactly one character sheet — the system's.
func TestCharacterSheetPaletteDedupe(t *testing.T) {
	core := []BlockMeta{
		{Type: "title", Label: "Title"},
		{Type: blockTypeCharacterSurface, Label: "Character Sheet"},
		{Type: "attributes", Label: "Attributes"},
	}

	t.Run("system character-sheet widget drops the generic block", func(t *testing.T) {
		ext := []BlockMeta{{Type: "ext_widget", WidgetSlug: systemCharacterSheetWidgetSlug, Label: "Character Sheet"}}
		palette := append(append([]BlockMeta{}, core...), ext...)
		if hasWidgetSlug(ext, systemCharacterSheetWidgetSlug) {
			palette = filterOutBlockType(palette, blockTypeCharacterSurface)
		}
		if blocksContainType(palette, blockTypeCharacterSurface) {
			t.Error("generic character_surface block should be dropped from the palette")
		}
		if !hasWidgetSlug(palette, systemCharacterSheetWidgetSlug) {
			t.Error("the system character-sheet widget must remain in the palette")
		}
	})

	t.Run("no system character-sheet widget keeps the generic block", func(t *testing.T) {
		ext := []BlockMeta{{Type: "ext_widget", WidgetSlug: "shop-inventory"}}
		palette := append(append([]BlockMeta{}, core...), ext...)
		if hasWidgetSlug(ext, systemCharacterSheetWidgetSlug) {
			palette = filterOutBlockType(palette, blockTypeCharacterSurface)
		}
		if !blocksContainType(palette, blockTypeCharacterSurface) {
			t.Error("generic character_surface block should remain when no system sheet is present")
		}
	})
}

func blocksContainType(blocks []BlockMeta, blockType string) bool {
	for i := range blocks {
		if blocks[i].Type == blockType {
			return true
		}
	}
	return false
}

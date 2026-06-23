package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLookupEntityShowRenderer_SlugThenPreset verifies the dispatch precedence
// added for the player-character seam: a SLUG binding wins (a system's own
// bespoke type), else a PRESET_CATEGORY binding fills a Chronicle-owned category
// (e.g. the addon's player-character type), else nil (fall through to the
// layout-block dispatch).
func TestLookupEntityShowRenderer_SlugThenPreset(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	reg.Register("drawsteel-character", MakeWidgetMountRenderer("slug-widget"))
	reg.RegisterByPresetCategory("player_character", MakeWidgetMountRenderer("preset-widget"))
	SetGlobalEntityShowRendererRegistry(reg)
	defer SetGlobalEntityShowRendererRegistry(nil)

	// MakeWidgetMountRenderer tolerates a nil Entity/CC (emits empty ids), so the
	// EntityType alone drives which renderer fires — exactly what we assert.
	widgetFor := func(t *testing.T, et *EntityType) string {
		t.Helper()
		comp := lookupEntityShowRenderer(EntityShowRenderContext{EntityType: et})
		if comp == nil {
			return ""
		}
		var buf bytes.Buffer
		if err := comp.Render(context.Background(), &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}

	character := "character"
	pcPreset := "player_character"

	// Slug binding wins even though this type's preset also has a binding.
	if got := widgetFor(t, &EntityType{Slug: "drawsteel-character", PresetCategory: &character}); !strings.Contains(got, `data-widget="slug-widget"`) {
		t.Errorf("slug binding should win, got %q", got)
	}
	// No slug match → preset_category fallback (the addon's player-character category).
	if got := widgetFor(t, &EntityType{Slug: "player-character", PresetCategory: &pcPreset}); !strings.Contains(got, `data-widget="preset-widget"`) {
		t.Errorf("preset fallback expected, got %q", got)
	}
	// Neither bound → no renderer (block dispatch fallback).
	if got := widgetFor(t, &EntityType{Slug: "location"}); got != "" {
		t.Errorf("expected no renderer for an unbound type, got %q", got)
	}
}

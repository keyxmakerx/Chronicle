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

// A sheet bound to the character preset is a hero's: an NPC page keeps its
// layout, and every check of who draws the page agrees. A system's own
// slug-bound type keeps its renderer either way.
func TestEntityShowRenderer_NPCPageKeepsLayout(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	reg.Register("drawsteel-character", MakeWidgetMountRenderer("slug-widget"))
	reg.RegisterByPresetCategory("character", MakeWidgetMountRenderer("hero-sheet"))
	SetGlobalEntityShowRendererRegistry(reg)
	defer SetGlobalEntityShowRendererRegistry(nil)

	character := "character"
	preset := &EntityType{Slug: "character", PresetCategory: &character}
	bespoke := &EntityType{Slug: "drawsteel-character", PresetCategory: &character}
	tests := []struct {
		name string
		et   *EntityType
		npc  bool
		want bool
	}{
		{"hero page", preset, false, true},
		{"npc page", preset, true, false},
		{"system's own type, npc page", bespoke, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lookupEntityShowRenderer(EntityShowRenderContext{EntityType: tt.et, NPCPage: tt.npc}) != nil
			if got != tt.want {
				t.Errorf("lookup drew = %v, want %v", got, tt.want)
			}
			if has := hasEntityShowRenderer(withNPCPage(context.Background(), tt.npc), tt.et); has != tt.want {
				t.Errorf("hasEntityShowRenderer = %v, want %v", has, tt.want)
			}
		})
	}
}

// An NPC page is an unclaimed page of a type on the NPC list; a claimed page
// is a player's character, and with no lists wired nothing is an NPC page.
func TestIsNPCPage(t *testing.T) {
	owner := "user-1"
	listed := &EntityType{ID: 3}
	other := &EntityType{ID: 9}
	tests := []struct {
		name   string
		lists  CharacterListManager
		entity *Entity
		et     *EntityType
		want   bool
	}{
		{"unclaimed, listed type", &fakeCastLists{npcs: []int{3}}, &Entity{}, listed, true},
		{"claimed, listed type", &fakeCastLists{npcs: []int{3}}, &Entity{OwnerUserID: &owner}, listed, false},
		{"made by the hero creator, unclaimed", &fakeCastLists{npcs: []int{3}}, &Entity{Playable: true}, listed, false},
		{"unlisted type", &fakeCastLists{npcs: []int{3}}, &Entity{}, other, false},
		{"no lists", nil, &Entity{}, listed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{}
			if tt.lists != nil {
				h.charLists = tt.lists
			}
			if got := h.isNPCPage(context.Background(), "camp-1", tt.entity, tt.et); got != tt.want {
				t.Errorf("isNPCPage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPlayersCharacter(t *testing.T) {
	owner := "user-1"
	tests := []struct {
		name   string
		entity *Entity
		want   bool
	}{
		{"claimed", &Entity{OwnerUserID: &owner}, true},
		{"playable, unclaimed", &Entity{Playable: true}, true},
		{"neither", &Entity{}, false},
		{"no page", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPlayersCharacter(tt.entity); got != tt.want {
				t.Errorf("isPlayersCharacter = %v, want %v", got, tt.want)
			}
		})
	}
}

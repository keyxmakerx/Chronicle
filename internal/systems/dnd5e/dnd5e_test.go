package dnd5e

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

func TestNew(t *testing.T) {
	manifest := &systems.SystemManifest{
		ID:   "dnd5e",
		Name: "D&D 5th Edition",
	}

	mod, err := New(manifest, "data")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if mod.Info().ID != "dnd5e" {
		t.Errorf("ID = %q, want %q", mod.Info().ID, "dnd5e")
	}
	if mod.DataProvider() == nil {
		t.Fatal("DataProvider is nil")
	}
	if mod.TooltipRenderer() == nil {
		t.Fatal("TooltipRenderer is nil")
	}
}

func TestNew_DataLoading(t *testing.T) {
	manifest := &systems.SystemManifest{
		ID:   "dnd5e",
		Name: "D&D 5th Edition",
	}

	mod, err := New(manifest, "data")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	provider := mod.DataProvider()

	// Verify all 6 categories are loaded.
	cats := provider.Categories()
	expectedCats := []string{"classes", "conditions", "items", "monsters", "races", "spells"}
	if len(cats) != len(expectedCats) {
		t.Fatalf("categories: got %d (%v), want %d (%v)", len(cats), cats, len(expectedCats), expectedCats)
	}
	for i, cat := range cats {
		if cat != expectedCats[i] {
			t.Errorf("category[%d] = %q, want %q", i, cat, expectedCats[i])
		}
	}

	// Verify item counts per category.
	tests := []struct {
		category string
		minItems int
	}{
		{"spells", 20},
		{"monsters", 14},
		{"items", 10},
		{"classes", 12},
		{"races", 9},
		{"conditions", 15},
	}

	for _, tt := range tests {
		items, err := provider.List(tt.category)
		if err != nil {
			t.Errorf("List(%q): %v", tt.category, err)
			continue
		}
		if len(items) < tt.minItems {
			t.Errorf("%s: got %d items, want at least %d", tt.category, len(items), tt.minItems)
		}
	}
}

func TestNew_SpecificItems(t *testing.T) {
	manifest := &systems.SystemManifest{
		ID:   "dnd5e",
		Name: "D&D 5th Edition",
	}

	mod, err := New(manifest, "data")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	provider := mod.DataProvider()

	// Verify well-known items exist with correct properties.
	tests := []struct {
		category string
		id       string
		wantName string
		wantProp string
		wantVal  string
	}{
		{"spells", "fireball", "Fireball", "school", "Evocation"},
		{"monsters", "goblin", "Goblin", "type", "Humanoid (Goblinoid)"},
		{"items", "bag-of-holding", "Bag of Holding", "rarity", "Uncommon"},
		{"classes", "wizard", "Wizard", "hit_die", "d6"},
		{"races", "elf", "Elf", "size", "Medium"},
		{"conditions", "blinded", "Blinded", "effect", "Can't see, auto-fail sight checks, attacks have disadvantage, attacks against have advantage"},
	}

	for _, tt := range tests {
		t.Run(tt.category+"/"+tt.id, func(t *testing.T) {
			item, err := provider.Get(tt.category, tt.id)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if item == nil {
				t.Fatalf("item %q not found in %q", tt.id, tt.category)
			}
			if item.Name != tt.wantName {
				t.Errorf("name = %q, want %q", item.Name, tt.wantName)
			}
			if item.SystemID != "dnd5e" {
				t.Errorf("system_id = %q, want %q", item.SystemID, "dnd5e")
			}
			if val, ok := item.Properties[tt.wantProp]; ok {
				if s, ok := val.(string); ok && s != tt.wantVal {
					t.Errorf("property %q = %q, want %q", tt.wantProp, s, tt.wantVal)
				}
			} else {
				t.Errorf("property %q not found", tt.wantProp)
			}
		})
	}
}

func TestNew_Search(t *testing.T) {
	manifest := &systems.SystemManifest{
		ID:   "dnd5e",
		Name: "D&D 5th Edition",
	}

	mod, err := New(manifest, "data")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	provider := mod.DataProvider()

	// Search across categories.
	results, err := provider.Search("fire")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results for 'fire', got none")
	}

	// Verify results come from multiple categories.
	categories := make(map[string]bool)
	for _, r := range results {
		categories[r.Category] = true
	}
	if len(categories) < 2 {
		t.Errorf("expected results from multiple categories, got %d", len(categories))
	}
}

func TestNew_InvalidDataDir(t *testing.T) {
	manifest := &systems.SystemManifest{
		ID:   "dnd5e",
		Name: "D&D 5th Edition",
	}

	_, err := New(manifest, "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent data dir")
	}
}

func TestTooltipRenderer_SupportedCategories(t *testing.T) {
	r := &TooltipRenderer{}
	cats := r.SupportedCategories()

	expected := []string{"spells", "monsters", "items", "classes", "races", "conditions"}
	if len(cats) != len(expected) {
		t.Fatalf("got %d categories, want %d", len(cats), len(expected))
	}
	for i, cat := range cats {
		if cat != expected[i] {
			t.Errorf("category[%d] = %q, want %q", i, cat, expected[i])
		}
	}
}

func TestTooltipRenderer_RenderTooltip(t *testing.T) {
	r := &TooltipRenderer{}

	tests := []struct {
		name     string
		item     *systems.ReferenceItem
		contains []string
		wantErr  bool
	}{
		{
			name: "spell tooltip",
			item: &systems.ReferenceItem{
				Name:     "Fireball",
				Category: "spells",
				Summary:  "A ball of fire",
				Source:   "SRD 5.1",
				Properties: map[string]any{
					"level":  3,
					"school": "Evocation",
					"range":  "150 feet",
				},
			},
			contains: []string{
				"Fireball",
				"spells",
				"Evocation",
				"150 feet",
				"SRD 5.1",
				"system-tooltip--dnd5e",
			},
		},
		{
			name: "monster tooltip",
			item: &systems.ReferenceItem{
				Name:     "Goblin",
				Category: "monsters",
				Summary:  "A small creature",
				Properties: map[string]any{
					"cr":   "1/4",
					"type": "Humanoid",
					"hp":   "7 (2d6)",
				},
			},
			contains: []string{
				"Goblin",
				"monsters",
				"1/4",
				"Humanoid",
			},
		},
		{
			name: "condition tooltip",
			item: &systems.ReferenceItem{
				Name:     "Blinded",
				Category: "conditions",
				Summary:  "Can't see",
				Properties: map[string]any{
					"effect": "Can't see, auto-fail sight checks",
				},
			},
			contains: []string{
				"Blinded",
				"conditions",
				"Can&#39;t see, auto-fail sight checks",
				"Effect",
			},
		},
		{
			name: "class tooltip",
			item: &systems.ReferenceItem{
				Name:     "Wizard",
				Category: "classes",
				Properties: map[string]any{
					"hit_die":        "d6",
					"primary_ability": "Intelligence",
				},
			},
			contains: []string{
				"Wizard",
				"Hit Die",
				"d6",
				"Intelligence",
			},
		},
		{
			name: "race tooltip",
			item: &systems.ReferenceItem{
				Name:     "Elf",
				Category: "races",
				Properties: map[string]any{
					"speed": "30 ft.",
					"size":  "Medium",
				},
			},
			contains: []string{
				"Elf",
				"30 ft.",
				"Medium",
			},
		},
		{
			name: "item tooltip",
			item: &systems.ReferenceItem{
				Name:     "Bag of Holding",
				Category: "items",
				Properties: map[string]any{
					"rarity": "Uncommon",
					"type":   "Wondrous item",
				},
			},
			contains: []string{
				"Bag of Holding",
				"Uncommon",
				"Wondrous item",
			},
		},
		{
			name:    "nil item",
			item:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := r.RenderTooltip(tt.item)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(html, want) {
					t.Errorf("tooltip HTML missing %q\ngot: %s", want, html)
				}
			}
		})
	}
}

func TestTooltipRenderer_CategoryIsolation(t *testing.T) {
	r := &TooltipRenderer{}

	// A spell tooltip should NOT render monster-specific properties.
	spellItem := &systems.ReferenceItem{
		Name:     "Fireball",
		Category: "spells",
		Properties: map[string]any{
			"level":  3,
			"school": "Evocation",
		},
	}

	html, err := r.RenderTooltip(spellItem)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Should NOT contain monster-specific labels.
	unwanted := []string{"CR:", "HP:", "AC:", "Alignment:", "Hit Die:", "Rarity:", "Speed:"}
	for _, label := range unwanted {
		if strings.Contains(html, label) {
			t.Errorf("spell tooltip should not contain %q", label)
		}
	}
}

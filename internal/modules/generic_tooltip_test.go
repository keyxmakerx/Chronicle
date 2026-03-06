package modules

import (
	"strings"
	"testing"
)

func TestGenericTooltipRenderer_SupportedCategories(t *testing.T) {
	manifest := &ModuleManifest{
		ID: "test-system",
		Categories: []CategoryDef{
			{Slug: "spells", Name: "Spells"},
			{Slug: "monsters", Name: "Monsters"},
			{Slug: "feats", Name: "Feats"},
		},
	}

	r := NewGenericTooltipRenderer(manifest)
	cats := r.SupportedCategories()

	if len(cats) != 3 {
		t.Fatalf("got %d categories, want 3", len(cats))
	}
	expected := []string{"spells", "monsters", "feats"}
	for i, cat := range cats {
		if cat != expected[i] {
			t.Errorf("category[%d] = %q, want %q", i, cat, expected[i])
		}
	}
}

func TestGenericTooltipRenderer_RenderTooltip(t *testing.T) {
	manifest := &ModuleManifest{
		ID: "pf2e",
		Categories: []CategoryDef{
			{
				Slug: "spells",
				Name: "Spells",
				Fields: []FieldDef{
					{Key: "level", Label: "Spell Level", Type: "number"},
					{Key: "tradition", Label: "Tradition", Type: "string"},
					{Key: "range", Label: "Range", Type: "string"},
				},
			},
			{
				Slug: "feats",
				Name: "Feats",
				Fields: []FieldDef{
					{Key: "level", Label: "Level", Type: "number"},
					{Key: "traits", Label: "Traits", Type: "string"},
				},
			},
		},
	}

	r := NewGenericTooltipRenderer(manifest)

	tests := []struct {
		name     string
		item     *ReferenceItem
		contains []string
		wantErr  bool
	}{
		{
			name: "spell with manifest fields",
			item: &ReferenceItem{
				Name:     "Magic Missile",
				Category: "spells",
				Summary:  "Automatic force damage",
				Source:   "Core Rulebook",
				Properties: map[string]any{
					"level":     1,
					"tradition": "Arcane",
					"range":     "120 feet",
				},
			},
			contains: []string{
				"Magic Missile",
				"spells",
				"module-tooltip--pf2e",
				"Spell Level",   // uses manifest label, not raw key
				"Tradition",
				"120 feet",
				"Automatic force damage",
				"Core Rulebook",
			},
		},
		{
			name: "feat with different field schema",
			item: &ReferenceItem{
				Name:     "Power Attack",
				Category: "feats",
				Properties: map[string]any{
					"level":  1,
					"traits": "Fighter, Press",
				},
			},
			contains: []string{
				"Power Attack",
				"feats",
				"Traits",
				"Fighter, Press",
			},
		},
		{
			name: "item with missing properties still renders",
			item: &ReferenceItem{
				Name:     "Empty Spell",
				Category: "spells",
				Summary:  "A test spell",
			},
			contains: []string{
				"Empty Spell",
				"A test spell",
			},
		},
		{
			name: "unknown category renders without properties",
			item: &ReferenceItem{
				Name:     "Unknown Thing",
				Category: "widgets",
				Properties: map[string]any{
					"foo": "bar",
				},
			},
			contains: []string{
				"Unknown Thing",
				"widgets",
			},
		},
		{
			name:    "nil item returns error",
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

func TestGenericTooltipRenderer_OnlyShowsManifestFields(t *testing.T) {
	manifest := &ModuleManifest{
		ID: "test",
		Categories: []CategoryDef{
			{
				Slug: "items",
				Name: "Items",
				Fields: []FieldDef{
					{Key: "rarity", Label: "Rarity", Type: "string"},
				},
			},
		},
	}

	r := NewGenericTooltipRenderer(manifest)

	// Item has extra properties not in the manifest field schema.
	item := &ReferenceItem{
		Name:     "Magic Sword",
		Category: "items",
		Properties: map[string]any{
			"rarity":     "Rare",
			"weight":     "3 lbs",
			"secret_key": "should-not-appear",
		},
	}

	html, err := r.RenderTooltip(item)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Should contain the manifest-defined field.
	if !strings.Contains(html, "Rarity") {
		t.Error("missing manifest field 'Rarity'")
	}
	if !strings.Contains(html, "Rare") {
		t.Error("missing manifest field value 'Rare'")
	}

	// Should NOT contain extra properties.
	if strings.Contains(html, "weight") || strings.Contains(html, "3 lbs") {
		t.Error("tooltip should not show non-manifest properties")
	}
	if strings.Contains(html, "secret_key") {
		t.Error("tooltip should not show non-manifest properties")
	}
}

func TestGenericTooltipRenderer_FieldOrder(t *testing.T) {
	manifest := &ModuleManifest{
		ID: "test",
		Categories: []CategoryDef{
			{
				Slug: "spells",
				Name: "Spells",
				Fields: []FieldDef{
					{Key: "school", Label: "School", Type: "string"},
					{Key: "level", Label: "Level", Type: "number"},
					{Key: "range", Label: "Range", Type: "string"},
				},
			},
		},
	}

	r := NewGenericTooltipRenderer(manifest)

	item := &ReferenceItem{
		Name:     "Fireball",
		Category: "spells",
		Properties: map[string]any{
			"range":  "150 feet",
			"level":  3,
			"school": "Evocation",
		},
	}

	html, err := r.RenderTooltip(item)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Verify fields appear in manifest order (School, Level, Range),
	// not in map iteration order.
	schoolIdx := strings.Index(html, "School")
	levelIdx := strings.Index(html, "Level")
	rangeIdx := strings.Index(html, "Range")

	if schoolIdx > levelIdx || levelIdx > rangeIdx {
		t.Errorf("fields not in manifest order: School@%d, Level@%d, Range@%d",
			schoolIdx, levelIdx, rangeIdx)
	}
}

func TestGenericModule(t *testing.T) {
	// Create a temp data directory with test data.
	dir := t.TempDir()
	writeTestData(t, dir, "creatures", []ReferenceItem{
		{ID: "dragon", Name: "Dragon", Summary: "A big lizard", Properties: map[string]any{"cr": "15"}},
		{ID: "goblin", Name: "Goblin", Summary: "A small pest", Properties: map[string]any{"cr": "1/4"}},
	})

	manifest := &ModuleManifest{
		ID:      "custom-system",
		Name:    "Custom RPG",
		Version: "1.0.0",
		Categories: []CategoryDef{
			{
				Slug: "creatures",
				Name: "Creatures",
				Fields: []FieldDef{
					{Key: "cr", Label: "Challenge", Type: "string"},
				},
			},
		},
	}

	mod, err := NewGenericModule(manifest, dir)
	if err != nil {
		t.Fatalf("NewGenericModule: %v", err)
	}

	// Verify interface compliance.
	if mod.Info().ID != "custom-system" {
		t.Errorf("ID = %q, want %q", mod.Info().ID, "custom-system")
	}

	// Data provider works.
	items, err := mod.DataProvider().List("creatures")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}

	// Tooltip renderer works using manifest fields.
	item, _ := mod.DataProvider().Get("creatures", "dragon")
	html, err := mod.TooltipRenderer().RenderTooltip(item)
	if err != nil {
		t.Fatalf("RenderTooltip: %v", err)
	}
	if !strings.Contains(html, "Dragon") {
		t.Error("tooltip missing item name")
	}
	if !strings.Contains(html, "Challenge") {
		t.Error("tooltip missing manifest field label")
	}
	if !strings.Contains(html, "module-tooltip--custom-system") {
		t.Error("tooltip missing module CSS class")
	}
}

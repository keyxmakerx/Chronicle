package extensions

import (
	"testing"
)

// validManifestJSON returns a minimal valid manifest JSON for testing.
func validManifestJSON() string {
	return `{
		"manifest_version": 1,
		"id": "test-extension",
		"name": "Test Extension",
		"version": "1.0.0",
		"description": "A test extension for unit tests"
	}`
}

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(t *testing.T, m *ExtensionManifest)
	}{
		{
			name: "valid minimal manifest",
			json: validManifestJSON(),
			check: func(t *testing.T, m *ExtensionManifest) {
				if m.ID != "test-extension" {
					t.Errorf("expected id test-extension, got %s", m.ID)
				}
				if m.Name != "Test Extension" {
					t.Errorf("expected name Test Extension, got %s", m.Name)
				}
				if m.Version != "1.0.0" {
					t.Errorf("expected version 1.0.0, got %s", m.Version)
				}
			},
		},
		{
			name:    "invalid JSON",
			json:    `{invalid`,
			wantErr: true,
		},
		{
			name:    "wrong manifest version",
			json:    `{"manifest_version": 2, "id": "test-ext", "name": "Test", "version": "1.0.0", "description": "Test"}`,
			wantErr: true,
		},
		{
			name:    "missing id",
			json:    `{"manifest_version": 1, "name": "Test", "version": "1.0.0", "description": "Test"}`,
			wantErr: true,
		},
		{
			name:    "invalid id uppercase",
			json:    `{"manifest_version": 1, "id": "Test-Ext", "name": "Test", "version": "1.0.0", "description": "Test"}`,
			wantErr: true,
		},
		{
			name:    "id too short",
			json:    `{"manifest_version": 1, "id": "ab", "name": "Test", "version": "1.0.0", "description": "Test"}`,
			wantErr: true,
		},
		{
			name:    "missing name",
			json:    `{"manifest_version": 1, "id": "test-ext", "version": "1.0.0", "description": "Test"}`,
			wantErr: true,
		},
		{
			name:    "invalid version",
			json:    `{"manifest_version": 1, "id": "test-ext", "name": "Test", "version": "1.0", "description": "Test"}`,
			wantErr: true,
		},
		{
			name:    "missing description",
			json:    `{"manifest_version": 1, "id": "test-ext", "name": "Test", "version": "1.0.0"}`,
			wantErr: true,
		},
		{
			name:    "author without name",
			json:    `{"manifest_version": 1, "id": "test-ext", "name": "Test", "version": "1.0.0", "description": "Test", "author": {"email": "a@b.com"}}`,
			wantErr: true,
		},
		{
			name: "valid with author",
			json: `{"manifest_version": 1, "id": "test-ext", "name": "Test", "version": "1.0.0", "description": "Test", "author": {"name": "Dev"}}`,
			check: func(t *testing.T, m *ExtensionManifest) {
				if m.Author == nil || m.Author.Name != "Dev" {
					t.Error("expected author name Dev")
				}
			},
		},
		{
			name:    "too many keywords",
			json:    `{"manifest_version": 1, "id": "test-ext", "name": "Test", "version": "1.0.0", "description": "Test", "keywords": ["a","b","c","d","e","f","g","h","i","j","k"]}`,
			wantErr: true,
		},
		{
			name: "valid with contributes",
			json: `{
				"manifest_version": 1, "id": "test-ext", "name": "Test", "version": "1.0.0", "description": "Test",
				"contributes": {
					"entity_type_templates": [{"slug": "npc", "name": "NPC", "name_plural": "NPCs", "icon": "fa-user", "color": "#ff0000"}],
					"tag_collections": [{"slug": "biomes", "name": "Biomes", "tags": [{"name": "Forest"}]}]
				}
			}`,
			check: func(t *testing.T, m *ExtensionManifest) {
				if m.Contributes == nil {
					t.Fatal("expected contributes")
				}
				if len(m.Contributes.EntityTypeTemplates) != 1 {
					t.Error("expected 1 entity type template")
				}
				if len(m.Contributes.TagCollections) != 1 {
					t.Error("expected 1 tag collection")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseManifest([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}

func TestValidateContributes(t *testing.T) {
	tests := []struct {
		name    string
		c       *ManifestContributes
		wantErr bool
	}{
		{
			name: "entity type template missing slug",
			c: &ManifestContributes{
				EntityTypeTemplates: []EntityTypeTemplate{{Name: "NPC"}},
			},
			wantErr: true,
		},
		{
			name: "entity type template missing name",
			c: &ManifestContributes{
				EntityTypeTemplates: []EntityTypeTemplate{{Slug: "npc"}},
			},
			wantErr: true,
		},
		{
			name: "entity pack missing file",
			c: &ManifestContributes{
				EntityPacks: []EntityPack{{Slug: "pack", Name: "Pack"}},
			},
			wantErr: true,
		},
		{
			name: "entity pack absolute path",
			c: &ManifestContributes{
				EntityPacks: []EntityPack{{Slug: "pack", Name: "Pack", File: "/etc/passwd"}},
			},
			wantErr: true,
		},
		{
			name: "entity pack path traversal",
			c: &ManifestContributes{
				EntityPacks: []EntityPack{{Slug: "pack", Name: "Pack", File: "../secret.json"}},
			},
			wantErr: true,
		},
		{
			name: "theme valid",
			c: &ManifestContributes{
				Themes: []Theme{{Slug: "dark", Name: "Dark", File: "themes/dark.css"}},
			},
		},
		{
			name: "theme missing file",
			c: &ManifestContributes{
				Themes: []Theme{{Slug: "dark", Name: "Dark"}},
			},
			wantErr: true,
		},
		{
			name: "widget valid",
			c: &ManifestContributes{
				Widgets: []WidgetContribution{{Slug: "dice-roller", Name: "Dice Roller", File: "widgets/dice.js"}},
			},
		},
		{
			name: "widget missing slug",
			c: &ManifestContributes{
				Widgets: []WidgetContribution{{Name: "Dice Roller", File: "widgets/dice.js"}},
			},
			wantErr: true,
		},
		{
			name: "widget missing name",
			c: &ManifestContributes{
				Widgets: []WidgetContribution{{Slug: "dice-roller", File: "widgets/dice.js"}},
			},
			wantErr: true,
		},
		{
			name: "widget missing file",
			c: &ManifestContributes{
				Widgets: []WidgetContribution{{Slug: "dice-roller", Name: "Dice Roller"}},
			},
			wantErr: true,
		},
		{
			name: "widget non-js file",
			c: &ManifestContributes{
				Widgets: []WidgetContribution{{Slug: "dice-roller", Name: "Dice Roller", File: "widgets/dice.css"}},
			},
			wantErr: true,
		},
		{
			name: "widget path traversal",
			c: &ManifestContributes{
				Widgets: []WidgetContribution{{Slug: "dice-roller", Name: "Dice Roller", File: "../evil.js"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContributes(tt.c)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "valid relative", path: "data/entities.json"},
		{name: "valid simple", path: "manifest.json"},
		{name: "empty", path: "", wantErr: true},
		{name: "absolute", path: "/etc/passwd", wantErr: true},
		{name: "traversal dotdot", path: "../secret.json", wantErr: true},
		{name: "traversal nested", path: "data/../../secret.json", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePath(tt.path)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

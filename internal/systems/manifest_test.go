package systems

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	data := `{
		"id": "test",
		"name": "Test Module",
		"description": "A test module",
		"version": "1.0.0",
		"author": "Test Author",
		"license": "MIT",
		"icon": "fa-flask",
		"api_version": "1",
		"status": "available",
		"categories": [
			{
				"slug": "spells",
				"name": "Spells",
				"icon": "fa-wand",
				"fields": [
					{"key": "level", "label": "Level", "type": "number"}
				]
			}
		],
		"entity_presets": [
			{
				"slug": "test-char",
				"name": "Test Character",
				"name_plural": "Test Characters",
				"icon": "fa-user",
				"color": "#FF0000"
			}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writing test manifest: %v", err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if m.ID != "test" {
		t.Errorf("ID = %q, want %q", m.ID, "test")
	}
	if m.Name != "Test Module" {
		t.Errorf("Name = %q, want %q", m.Name, "Test Module")
	}
	if m.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", m.Version, "1.0.0")
	}
	if m.Author != "Test Author" {
		t.Errorf("Author = %q, want %q", m.Author, "Test Author")
	}
	if m.License != "MIT" {
		t.Errorf("License = %q, want %q", m.License, "MIT")
	}
	if m.APIVersion != "1" {
		t.Errorf("APIVersion = %q, want %q", m.APIVersion, "1")
	}
	if m.Status != StatusAvailable {
		t.Errorf("Status = %q, want %q", m.Status, StatusAvailable)
	}
	if len(m.Categories) != 1 {
		t.Fatalf("Categories len = %d, want 1", len(m.Categories))
	}
	if m.Categories[0].Slug != "spells" {
		t.Errorf("Categories[0].Slug = %q, want %q", m.Categories[0].Slug, "spells")
	}
	if len(m.Categories[0].Fields) != 1 {
		t.Fatalf("Categories[0].Fields len = %d, want 1", len(m.Categories[0].Fields))
	}
	if len(m.EntityPresets) != 1 {
		t.Fatalf("EntityPresets len = %d, want 1", len(m.EntityPresets))
	}
	if m.EntityPresets[0].Slug != "test-char" {
		t.Errorf("EntityPresets[0].Slug = %q, want %q", m.EntityPresets[0].Slug, "test-char")
	}
}

func TestLoadManifest_FileNotFound(t *testing.T) {
	_, err := LoadManifest("/nonexistent/manifest.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadManifest_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name    string
		m       SystemManifest
		wantErr bool
	}{
		{
			name: "valid minimal",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
			},
			wantErr: false,
		},
		{
			name: "missing id",
			m: SystemManifest{
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			m: SystemManifest{
				ID:         "test",
				Version:    "1.0.0",
				APIVersion: "1",
			},
			wantErr: true,
		},
		{
			name: "missing version",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				APIVersion: "1",
			},
			wantErr: true,
		},
		{
			name: "missing api_version",
			m: SystemManifest{
				ID:      "test",
				Name:    "Test",
				Version: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
				Status:     "invalid",
			},
			wantErr: true,
		},
		{
			name: "empty status defaults to coming_soon",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
				Status:     "",
			},
			wantErr: false,
		},
		{
			name: "category missing slug",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
				Categories: []CategoryDef{{Name: "Spells"}},
			},
			wantErr: true,
		},
		{
			name: "category missing name",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
				Categories: []CategoryDef{{Slug: "spells"}},
			},
			wantErr: true,
		},
		{
			name: "valid with categories",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				Version:    "1.0.0",
				APIVersion: "1",
				Categories: []CategoryDef{{Slug: "spells", Name: "Spells"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifest(&tt.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManifest() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestSystemManifest_CategoryNames(t *testing.T) {
	m := &SystemManifest{
		Categories: []CategoryDef{
			{Slug: "spells", Name: "Spells"},
			{Slug: "monsters", Name: "Monsters"},
		},
	}

	names := m.CategoryNames()
	if len(names) != 2 {
		t.Fatalf("CategoryNames() len = %d, want 2", len(names))
	}
	if names[0] != "Spells" || names[1] != "Monsters" {
		t.Errorf("CategoryNames() = %v, want [Spells Monsters]", names)
	}
}

func TestValidateManifest_DefaultsEmptyStatus(t *testing.T) {
	m := SystemManifest{
		ID:         "test",
		Name:       "Test",
		Version:    "1.0.0",
		APIVersion: "1",
		Status:     "",
	}

	if err := ValidateManifest(&m); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}

	if m.Status != StatusComingSoon {
		t.Errorf("Status = %q, want %q (should default)", m.Status, StatusComingSoon)
	}
}

package modules

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestManifest creates a valid manifest.json in the given directory.
func writeTestManifest(t *testing.T, dir, id, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("creating dir %s: %v", dir, err)
	}
	data := `{
		"id": "` + id + `",
		"name": "` + name + `",
		"version": "1.0.0",
		"api_version": "1",
		"status": "available",
		"categories": [{"slug": "items", "name": "Items"}]
	}`
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writing manifest %s: %v", path, err)
	}
}

func TestModuleLoader_DiscoverAll_HappyPath(t *testing.T) {
	base := t.TempDir()
	writeTestManifest(t, filepath.Join(base, "alpha"), "alpha", "Alpha Module")
	writeTestManifest(t, filepath.Join(base, "beta"), "beta", "Beta Module")

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	if loader.Count() != 2 {
		t.Errorf("Count() = %d, want 2", loader.Count())
	}

	all := loader.All()
	if len(all) != 2 {
		t.Fatalf("All() len = %d, want 2", len(all))
	}

	// All() should be sorted by name.
	if all[0].Name != "Alpha Module" {
		t.Errorf("All()[0].Name = %q, want %q", all[0].Name, "Alpha Module")
	}
	if all[1].Name != "Beta Module" {
		t.Errorf("All()[1].Name = %q, want %q", all[1].Name, "Beta Module")
	}
}

func TestModuleLoader_DiscoverAll_EmptyDir(t *testing.T) {
	base := t.TempDir()

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	if loader.Count() != 0 {
		t.Errorf("Count() = %d, want 0", loader.Count())
	}
}

func TestModuleLoader_DiscoverAll_SkipsInvalidManifest(t *testing.T) {
	base := t.TempDir()

	// Create a valid module.
	writeTestManifest(t, filepath.Join(base, "valid"), "valid", "Valid Module")

	// Create an invalid module (missing required fields).
	invalidDir := filepath.Join(base, "invalid")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	invalidData := `{"id": "", "name": "No ID"}`
	if err := os.WriteFile(filepath.Join(invalidDir, "manifest.json"), []byte(invalidData), 0644); err != nil {
		t.Fatalf("writing invalid manifest: %v", err)
	}

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	// Should discover only the valid module.
	if loader.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (invalid should be skipped)", loader.Count())
	}
}

func TestModuleLoader_DiscoverAll_SkipsDirWithoutManifest(t *testing.T) {
	base := t.TempDir()

	// Create a directory without manifest.json.
	if err := os.MkdirAll(filepath.Join(base, "nomodule"), 0755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	// Create a valid module.
	writeTestManifest(t, filepath.Join(base, "valid"), "valid", "Valid Module")

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	if loader.Count() != 1 {
		t.Errorf("Count() = %d, want 1", loader.Count())
	}
}

func TestModuleLoader_DiscoverAll_NonexistentDir(t *testing.T) {
	loader := NewModuleLoader("/nonexistent/path")
	err := loader.DiscoverAll()
	if err == nil {
		t.Fatal("expected error for nonexistent dir, got nil")
	}
}

func TestModuleLoader_Get(t *testing.T) {
	base := t.TempDir()
	writeTestManifest(t, filepath.Join(base, "test"), "test", "Test Module")

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	m := loader.Get("test")
	if m == nil {
		t.Fatal("Get(\"test\") returned nil")
	}
	if m.ID != "test" {
		t.Errorf("Get(\"test\").ID = %q, want %q", m.ID, "test")
	}

	// Non-existent module.
	if loader.Get("nonexistent") != nil {
		t.Error("Get(\"nonexistent\") should return nil")
	}
}

func TestModuleLoader_Dir(t *testing.T) {
	base := t.TempDir()
	writeTestManifest(t, filepath.Join(base, "test"), "test", "Test Module")

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	dir := loader.Dir("test")
	expected := filepath.Join(base, "test")
	if dir != expected {
		t.Errorf("Dir(\"test\") = %q, want %q", dir, expected)
	}

	if loader.Dir("nonexistent") != "" {
		t.Error("Dir(\"nonexistent\") should return empty string")
	}
}

func TestModuleLoader_GenericAutoInstantiation(t *testing.T) {
	base := t.TempDir()

	// Create a module directory with manifest + data (no Go factory).
	modDir := filepath.Join(base, "custom-rpg")
	writeTestManifest(t, modDir, "custom-rpg", "Custom RPG")

	// Create data directory with a JSON file.
	dataDir := filepath.Join(modDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating data dir: %v", err)
	}
	writeTestData(t, dataDir, "items", []ReferenceItem{
		{ID: "sword", Name: "Sword", Summary: "A sharp blade"},
	})

	loader := NewModuleLoader(base)
	if err := loader.DiscoverAll(); err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	// Should auto-instantiate as a generic module.
	mod := loader.GetModule("custom-rpg")
	if mod == nil {
		t.Fatal("expected generic module to be auto-instantiated")
	}

	if mod.Info().ID != "custom-rpg" {
		t.Errorf("module ID = %q, want %q", mod.Info().ID, "custom-rpg")
	}

	// Data provider should work.
	items, err := mod.DataProvider().List("items")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("got %d items, want 1", len(items))
	}

	// Tooltip renderer should work.
	if mod.TooltipRenderer() == nil {
		t.Fatal("expected non-nil tooltip renderer")
	}
	cats := mod.TooltipRenderer().SupportedCategories()
	if len(cats) != 1 || cats[0] != "items" {
		t.Errorf("supported categories = %v, want [items]", cats)
	}
}

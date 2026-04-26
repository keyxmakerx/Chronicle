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
			name: "missing version defaults to 0.0.0",
			m: SystemManifest{
				ID:         "test",
				Name:       "Test",
				APIVersion: "1",
			},
			wantErr: false,
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

func TestFieldDef_IsFoundryWritable(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name string
		f    FieldDef
		want bool
	}{
		{"nil defaults to true", FieldDef{Key: "hp"}, true},
		{"explicit true", FieldDef{Key: "hp", FoundryWritable: &trueVal}, true},
		{"explicit false", FieldDef{Key: "ac", FoundryWritable: &falseVal}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.IsFoundryWritable(); got != tt.want {
				t.Errorf("IsFoundryWritable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemManifest_CharacterPreset(t *testing.T) {
	m := &SystemManifest{
		EntityPresets: []EntityPresetDef{
			{Slug: "dnd5e-creature", Name: "Creature"},
			{Slug: "dnd5e-character", Name: "Character"},
		},
	}

	p := m.CharacterPreset()
	if p == nil {
		t.Fatal("CharacterPreset() returned nil, want non-nil")
	}
	if p.Slug != "dnd5e-character" {
		t.Errorf("CharacterPreset().Slug = %q, want %q", p.Slug, "dnd5e-character")
	}
}

func TestSystemManifest_CharacterPreset_None(t *testing.T) {
	m := &SystemManifest{
		EntityPresets: []EntityPresetDef{
			{Slug: "dnd5e-creature", Name: "Creature"},
		},
	}

	if p := m.CharacterPreset(); p != nil {
		t.Errorf("CharacterPreset() = %v, want nil", p)
	}
}

func TestSystemManifest_CharacterFieldsForAPI(t *testing.T) {
	falseVal := false
	m := &SystemManifest{
		ID:              "dnd5e",
		FoundrySystemID: "dnd5e",
		EntityPresets: []EntityPresetDef{
			{
				Slug: "dnd5e-character",
				Name: "D&D Character",
				Fields: []FieldDef{
					{Key: "str", Label: "Strength", Type: "number", FoundryPath: "system.abilities.str.value"},
					{Key: "ac", Label: "Armor Class", Type: "number", FoundryPath: "system.attributes.ac.value", FoundryWritable: &falseVal},
					{Key: "class", Label: "Class", Type: "string"},
				},
			},
		},
	}

	resp := m.CharacterFieldsForAPI()
	if resp == nil {
		t.Fatal("CharacterFieldsForAPI() returned nil")
	}

	if resp.SystemID != "dnd5e" {
		t.Errorf("SystemID = %q, want %q", resp.SystemID, "dnd5e")
	}
	if resp.FoundrySystemID != "dnd5e" {
		t.Errorf("FoundrySystemID = %q, want %q", resp.FoundrySystemID, "dnd5e")
	}
	if resp.PresetSlug != "dnd5e-character" {
		t.Errorf("PresetSlug = %q, want %q", resp.PresetSlug, "dnd5e-character")
	}
	if len(resp.Fields) != 3 {
		t.Fatalf("Fields len = %d, want 3", len(resp.Fields))
	}

	// str: has foundry_path, writable defaults to true.
	if resp.Fields[0].FoundryPath != "system.abilities.str.value" {
		t.Errorf("Fields[0].FoundryPath = %q", resp.Fields[0].FoundryPath)
	}
	if !resp.Fields[0].FoundryWritable {
		t.Error("Fields[0].FoundryWritable = false, want true")
	}

	// ac: has foundry_path but writable=false.
	if resp.Fields[1].FoundryWritable {
		t.Error("Fields[1].FoundryWritable = true, want false")
	}

	// class: no foundry_path, so writable should be false.
	if resp.Fields[2].FoundryPath != "" {
		t.Errorf("Fields[2].FoundryPath = %q, want empty", resp.Fields[2].FoundryPath)
	}
	if resp.Fields[2].FoundryWritable {
		t.Error("Fields[2].FoundryWritable = true, want false (no foundry_path)")
	}
}

func TestSystemManifest_CharacterFieldsForAPI_NoPreset(t *testing.T) {
	m := &SystemManifest{
		ID:            "custom",
		EntityPresets: []EntityPresetDef{},
	}

	if resp := m.CharacterFieldsForAPI(); resp != nil {
		t.Errorf("CharacterFieldsForAPI() = %v, want nil", resp)
	}
}

func TestLoadManifest_FoundryAnnotations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	data := `{
		"id": "test",
		"name": "Test",
		"version": "1.0.0",
		"api_version": "1",
		"foundry_system_id": "test-system",
		"entity_presets": [{
			"slug": "test-character",
			"name": "Test Character",
			"name_plural": "Test Characters",
			"icon": "fa-user",
			"color": "#FF0000",
			"fields": [
				{"key": "hp", "label": "HP", "type": "number", "foundry_path": "system.attributes.hp.value"},
				{"key": "ac", "label": "AC", "type": "number", "foundry_path": "system.attributes.ac.value", "foundry_writable": false}
			]
		}]
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writing test manifest: %v", err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	if m.FoundrySystemID != "test-system" {
		t.Errorf("FoundrySystemID = %q, want %q", m.FoundrySystemID, "test-system")
	}

	preset := m.CharacterPreset()
	if preset == nil {
		t.Fatal("CharacterPreset() returned nil")
	}
	if len(preset.Fields) != 2 {
		t.Fatalf("Fields len = %d, want 2", len(preset.Fields))
	}
	if preset.Fields[0].FoundryPath != "system.attributes.hp.value" {
		t.Errorf("Fields[0].FoundryPath = %q", preset.Fields[0].FoundryPath)
	}
	if !preset.Fields[0].IsFoundryWritable() {
		t.Error("Fields[0].IsFoundryWritable() = false, want true")
	}
	if preset.Fields[1].IsFoundryWritable() {
		t.Error("Fields[1].IsFoundryWritable() = true, want false")
	}
}

func TestBuildValidationReport_FullSystem(t *testing.T) {
	falseVal := false
	m := &SystemManifest{
		ID:              "dnd5e",
		FoundrySystemID: "dnd5e",
		Categories: []CategoryDef{
			{Slug: "spells", Name: "Spells", Fields: []FieldDef{{Key: "level"}, {Key: "school"}}},
			{Slug: "items", Name: "Items", Fields: []FieldDef{{Key: "rarity"}}},
		},
		EntityPresets: []EntityPresetDef{
			{
				Slug: "dnd5e-character",
				Name: "D&D Character",
				Fields: []FieldDef{
					{Key: "str", FoundryPath: "system.abilities.str.value"},
					{Key: "ac", FoundryPath: "system.attributes.ac.value", FoundryWritable: &falseVal},
					{Key: "class"},
				},
			},
		},
	}

	r := m.BuildValidationReport()

	if r.CategoryCount != 2 {
		t.Errorf("CategoryCount = %d, want 2", r.CategoryCount)
	}
	if r.TotalFields != 3 {
		t.Errorf("TotalFields = %d, want 3", r.TotalFields)
	}
	if r.PresetCount != 1 {
		t.Errorf("PresetCount = %d, want 1", r.PresetCount)
	}
	if !r.HasCharacterPreset {
		t.Error("HasCharacterPreset = false, want true")
	}
	if r.CharacterFieldCount != 3 {
		t.Errorf("CharacterFieldCount = %d, want 3", r.CharacterFieldCount)
	}
	if !r.FoundryCompatible {
		t.Error("FoundryCompatible = false, want true")
	}
	if r.FoundryMappedFields != 2 {
		t.Errorf("FoundryMappedFields = %d, want 2", r.FoundryMappedFields)
	}
	if r.FoundryWritableFields != 1 {
		t.Errorf("FoundryWritableFields = %d, want 1", r.FoundryWritableFields)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", r.Warnings)
	}
}

func TestBuildValidationReport_MinimalSystem(t *testing.T) {
	m := &SystemManifest{
		ID: "bare",
	}

	r := m.BuildValidationReport()

	if r.CategoryCount != 0 {
		t.Errorf("CategoryCount = %d, want 0", r.CategoryCount)
	}
	if r.FoundryCompatible {
		t.Error("FoundryCompatible = true, want false")
	}
	if r.HasCharacterPreset {
		t.Error("HasCharacterPreset = true, want false")
	}

	// Should have warnings for missing categories, presets, and character preset.
	if len(r.Warnings) < 3 {
		t.Errorf("Expected at least 3 warnings, got %d: %v", len(r.Warnings), r.Warnings)
	}
}

func TestBuildValidationReport_NoFoundryPaths(t *testing.T) {
	m := &SystemManifest{
		ID:              "custom",
		FoundrySystemID: "custom-system",
		Categories:      []CategoryDef{{Slug: "data", Name: "Data"}},
		EntityPresets: []EntityPresetDef{
			{
				Slug: "custom-character",
				Name: "Character",
				Fields: []FieldDef{
					{Key: "hp"},
					{Key: "str"},
				},
			},
		},
	}

	r := m.BuildValidationReport()

	if r.FoundryMappedFields != 0 {
		t.Errorf("FoundryMappedFields = %d, want 0", r.FoundryMappedFields)
	}

	// Should warn about foundry_system_id set but no foundry_path fields.
	found := false
	for _, w := range r.Warnings {
		if len(w) > 0 && w[0:5] == "found" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected warning about missing foundry_path, got: %v", r.Warnings)
	}
}

func TestValidateManifest_DefaultsEmptyVersion(t *testing.T) {
	m := SystemManifest{
		ID:         "test",
		Name:       "Test",
		APIVersion: "1",
		Version:    "",
	}

	if err := ValidateManifest(&m); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}

	if m.Version != "0.0.0" {
		t.Errorf("Version = %q, want %q (should default)", m.Version, "0.0.0")
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

// validRendererManifest builds a minimal manifest with a single renderer
// entry that references an existing preset and widget. Test cases mutate
// the result to drive specific validation paths.
func validRendererManifest() SystemManifest {
	return SystemManifest{
		ID:         "test-system",
		Name:       "Test",
		APIVersion: "1",
		Version:    "1.0.0",
		EntityPresets: []EntityPresetDef{
			{Slug: "test-character", Name: "Test Character"},
		},
		Widgets: []WidgetDef{
			{Slug: "test-character-card", Name: "Card", ScriptFile: "widgets/card.js"},
		},
		Renderers: []RendererDef{
			{Slug: "test-character", Widget: "test-character-card"},
		},
	}
}

func TestValidateManifest_RendererAcceptsValid(t *testing.T) {
	m := validRendererManifest()
	if err := ValidateManifest(&m); err != nil {
		t.Fatalf("ValidateManifest() error = %v", err)
	}
}

func TestValidateManifest_RendererRequiresMatchingPreset(t *testing.T) {
	m := validRendererManifest()
	m.Renderers[0].Slug = "no-such-preset"
	err := ValidateManifest(&m)
	if err == nil {
		t.Fatal("expected error for renderer slug with no matching preset")
	}
}

func TestValidateManifest_RendererRequiresMatchingWidget(t *testing.T) {
	m := validRendererManifest()
	m.Renderers[0].Widget = "no-such-widget"
	err := ValidateManifest(&m)
	if err == nil {
		t.Fatal("expected error for renderer widget with no matching widget def")
	}
}

func TestValidateManifest_RendererRejectsEmptySlug(t *testing.T) {
	m := validRendererManifest()
	m.Renderers[0].Slug = ""
	err := ValidateManifest(&m)
	if err == nil {
		t.Fatal("expected error for empty renderer slug")
	}
}

func TestValidateManifest_RendererRejectsEmptyWidget(t *testing.T) {
	m := validRendererManifest()
	m.Renderers[0].Widget = ""
	err := ValidateManifest(&m)
	if err == nil {
		t.Fatal("expected error for empty renderer widget")
	}
}

func TestValidateManifest_RendererCapEnforced(t *testing.T) {
	m := validRendererManifest()
	// Pad with valid-looking entries so the cap check (not slug/widget
	// validation) is what fails.
	for i := 0; i < maxRenderers; i++ {
		m.Renderers = append(m.Renderers, RendererDef{Slug: "test-character", Widget: "test-character-card"})
	}
	err := ValidateManifest(&m)
	if err == nil {
		t.Fatalf("expected error for %d renderers (cap %d)", len(m.Renderers), maxRenderers)
	}
}

package systems

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SystemManifest describes a module's metadata, capabilities, and content
// structure. Each module declares its manifest in a manifest.json file
// in its directory root. The manifest is the single source of truth for
// what a module provides.
type SystemManifest struct {
	// ID is the unique machine-readable identifier (e.g., "dnd5e").
	ID string `json:"id"`

	// Name is the human-readable display name.
	Name string `json:"name"`

	// Description is a short summary of what the module provides.
	Description string `json:"description"`

	// Version is the semantic version string (e.g., "0.1.0").
	Version string `json:"version"`

	// Author is the module creator's name or organization.
	Author string `json:"author"`

	// License identifies the content license (e.g., "OGL-1.0a", "ORC", "CC-BY-4.0").
	License string `json:"license"`

	// Icon is the Font Awesome icon class (e.g., "fa-dragon").
	Icon string `json:"icon"`

	// APIVersion is the system framework API version this manifest targets.
	// Used for forward compatibility checks (e.g., "1").
	APIVersion string `json:"api_version"`

	// Status indicates whether the module is available or coming soon.
	Status Status `json:"status"`

	// Categories lists the types of reference content this module provides.
	Categories []CategoryDef `json:"categories"`

	// EntityPresets are entity type templates that campaigns can adopt when
	// enabling this module (e.g., "D&D Character" with predefined fields).
	EntityPresets []EntityPresetDef `json:"entity_presets,omitempty"`

	// FoundrySystemID is the Foundry VTT game.system.id that this system
	// corresponds to (e.g., "dnd5e", "pf2e"). When set, the Foundry module
	// can automatically match this Chronicle system to the running Foundry
	// game system, enabling character sync for custom-uploaded systems.
	FoundrySystemID string `json:"foundry_system_id,omitempty"`

	// TooltipTemplate is an optional HTML template string for rendering
	// hover tooltips. Uses Go text/template syntax with ReferenceItem data.
	TooltipTemplate string `json:"tooltip_template,omitempty"`
}

// CategoryDef describes one category of reference content within a module.
type CategoryDef struct {
	// Slug is the URL-safe identifier (e.g., "spells", "monsters").
	Slug string `json:"slug"`

	// Name is the human-readable display name (e.g., "Spells", "Monsters").
	Name string `json:"name"`

	// Icon is an optional Font Awesome icon class for this category.
	Icon string `json:"icon,omitempty"`

	// Fields defines the schema for Properties keys on ReferenceItems
	// in this category. Used for structured display and filtering.
	Fields []FieldDef `json:"fields,omitempty"`
}

// FieldDef describes a single field in a category's reference item schema.
// For entity preset fields, the optional FoundryPath and FoundryWritable
// annotations enable automatic Foundry VTT character sync without needing
// a hardcoded system adapter.
type FieldDef struct {
	// Key is the property map key (e.g., "level", "school", "cr").
	Key string `json:"key"`

	// Label is the human-readable name (e.g., "Spell Level", "School").
	Label string `json:"label"`

	// Type is the field data type: "string", "number", "list", "markdown".
	Type string `json:"type"`

	// FoundryPath is the dot-notation path to the corresponding field in
	// a Foundry VTT Actor's system data (e.g., "system.abilities.str.value").
	// Used by the generic Foundry adapter to auto-generate field mappings.
	// Only meaningful on entity preset fields, not category reference fields.
	FoundryPath string `json:"foundry_path,omitempty"`

	// FoundryWritable indicates whether the generic adapter should write
	// this field back to Foundry when syncing Chronicle → Foundry.
	// Fields that are derived/calculated in Foundry (e.g., PF2e ability mods)
	// should set this to false. Defaults to true when FoundryPath is set.
	FoundryWritable *bool `json:"foundry_writable,omitempty"`
}

// IsFoundryWritable returns whether this field should be written back to
// Foundry. Returns true if foundry_writable is nil (default) or explicitly true.
func (f FieldDef) IsFoundryWritable() bool {
	if f.FoundryWritable == nil {
		return true
	}
	return *f.FoundryWritable
}

// EntityPresetDef describes an entity type template that a module provides.
// When a campaign enables the module, these presets become available as
// entity type starting points.
type EntityPresetDef struct {
	// Slug is the URL-safe identifier (e.g., "dnd5e-character").
	Slug string `json:"slug"`

	// Name is the display name (e.g., "D&D Character").
	Name string `json:"name"`

	// NamePlural is the plural display name (e.g., "D&D Characters").
	NamePlural string `json:"name_plural"`

	// Icon is the Font Awesome icon class.
	Icon string `json:"icon"`

	// Color is the hex color for the entity type badge.
	Color string `json:"color"`

	// Fields are the default field definitions for entities of this type.
	Fields []FieldDef `json:"fields,omitempty"`
}

// CharacterPreset returns the first entity preset whose slug ends with
// "-character", or nil if no character preset is defined. Used by the
// sync API to expose character field templates for actor sync.
func (m *SystemManifest) CharacterPreset() *EntityPresetDef {
	for i := range m.EntityPresets {
		if strings.HasSuffix(m.EntityPresets[i].Slug, "-character") {
			return &m.EntityPresets[i]
		}
	}
	return nil
}

// CharacterFieldsResponse is the API response shape for the character
// fields endpoint, containing field definitions with Foundry annotations.
type CharacterFieldsResponse struct {
	SystemID        string                 `json:"system_id"`
	PresetSlug      string                 `json:"preset_slug"`
	PresetName      string                 `json:"preset_name"`
	FoundrySystemID string                 `json:"foundry_system_id,omitempty"`
	Fields          []CharacterFieldExport `json:"fields"`
}

// CharacterFieldExport is a single field definition exported for the
// Foundry module's generic adapter.
type CharacterFieldExport struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	Type           string `json:"type"`
	FoundryPath    string `json:"foundry_path,omitempty"`
	FoundryWritable bool  `json:"foundry_writable"`
}

// CharacterFieldsForAPI builds the API response for character preset fields.
// Returns nil if no character preset exists.
func (m *SystemManifest) CharacterFieldsForAPI() *CharacterFieldsResponse {
	preset := m.CharacterPreset()
	if preset == nil {
		return nil
	}

	fields := make([]CharacterFieldExport, len(preset.Fields))
	for i, f := range preset.Fields {
		fields[i] = CharacterFieldExport{
			Key:             f.Key,
			Label:           f.Label,
			Type:            f.Type,
			FoundryPath:     f.FoundryPath,
			FoundryWritable: f.FoundryPath != "" && f.IsFoundryWritable(),
		}
	}

	return &CharacterFieldsResponse{
		SystemID:        m.ID,
		PresetSlug:      preset.Slug,
		PresetName:      preset.Name,
		FoundrySystemID: m.FoundrySystemID,
		Fields:          fields,
	}
}

// CategoryNames returns a flat list of category display names.
// Convenience method for backward-compatible display on admin pages.
func (m *SystemManifest) CategoryNames() []string {
	names := make([]string, len(m.Categories))
	for i, c := range m.Categories {
		names[i] = c.Name
	}
	return names
}

// LoadManifest reads a manifest.json file from disk, unmarshals it, and
// validates required fields. Returns the parsed manifest or an error.
func LoadManifest(path string) (*SystemManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var m SystemManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}

	if err := ValidateManifest(&m); err != nil {
		return nil, fmt.Errorf("validating manifest %s: %w", path, err)
	}

	return &m, nil
}

// ValidateManifest checks that a manifest has all required fields and
// valid values. Returns a descriptive error for the first violation found.
func ValidateManifest(m *SystemManifest) error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.APIVersion == "" {
		return fmt.Errorf("api_version is required")
	}

	// Validate status if provided (default to coming_soon if empty).
	if m.Status == "" {
		m.Status = StatusComingSoon
	}
	if m.Status != StatusAvailable && m.Status != StatusComingSoon {
		return fmt.Errorf("invalid status %q (must be %q or %q)", m.Status, StatusAvailable, StatusComingSoon)
	}

	// Validate categories have non-empty slugs.
	for i, cat := range m.Categories {
		if cat.Slug == "" {
			return fmt.Errorf("category %d: slug is required", i)
		}
		if cat.Name == "" {
			return fmt.Errorf("category %d (%s): name is required", i, cat.Slug)
		}
	}

	return nil
}

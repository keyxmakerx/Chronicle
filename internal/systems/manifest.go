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
type FieldDef struct {
	// Key is the property map key (e.g., "level", "school", "cr").
	Key string `json:"key"`

	// Label is the human-readable name (e.g., "Spell Level", "School").
	Label string `json:"label"`

	// Type is the field data type: "string", "number", "list", "markdown".
	Type string `json:"type"`
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

package systems

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"regexp"
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

	// RelationPresets are relation type templates that campaigns can adopt when
	// enabling this module (e.g., "has-item" for inventory tracking).
	RelationPresets []RelationPresetDef `json:"relation_presets,omitempty"`

	// FoundrySystemID is the Foundry VTT game.system.id that this system
	// corresponds to (e.g., "dnd5e", "pf2e"). When set, the Foundry module
	// can automatically match this Chronicle system to the running Foundry
	// game system, enabling character sync for custom-uploaded systems.
	FoundrySystemID string `json:"foundry_system_id,omitempty"`

	// TooltipTemplate is an optional HTML template string for rendering
	// hover tooltips. Uses Go text/template syntax with ReferenceItem data.
	TooltipTemplate string `json:"tooltip_template,omitempty"`

	// Widgets declares JS widgets provided by this system that can be added
	// to entity page layouts via the template editor. Each widget is a
	// self-contained JS file that registers via Chronicle.register().
	Widgets []WidgetDef `json:"widgets,omitempty"`

	// TextRenderers declares JS text renderers that define global utility
	// classes for processing text content. Loaded before widget scripts
	// so widgets can depend on them (e.g., DrawSteelRefRenderer).
	TextRenderers []TextRendererDef `json:"text_renderers,omitempty"`
}

// WidgetDef describes a JS widget provided by a game system module.
// Widgets are self-contained scripts that register via Chronicle.register()
// and can be placed on entity pages through the template editor palette.
type WidgetDef struct {
	// Slug is the widget registration name (e.g., "dnd5e-stat-block").
	// Must be unique and match the name passed to Chronicle.register().
	Slug string `json:"slug"`

	// Name is the display name shown in the template editor palette.
	Name string `json:"name"`

	// Icon is the Font Awesome icon class (e.g., "fa-shield-halved").
	Icon string `json:"icon"`

	// Description is tooltip text shown in the template editor palette.
	Description string `json:"description"`

	// ScriptFile is the relative path to the JS file within the system
	// directory (e.g., "widgets/stat-block.js").
	ScriptFile string `json:"script_file"`

	// File is an alias for ScriptFile for backward compatibility.
	File string `json:"file"`
}

// TextRendererDef describes a JS text renderer provided by a game system.
// Text renderers define global classes/functions that process text content
// (e.g., resolving rule references to tooltips). They load before widgets
// so widgets can depend on them.
type TextRendererDef struct {
	// Slug is the unique identifier for the text renderer.
	Slug string `json:"slug"`

	// Name is the display name.
	Name string `json:"name"`

	// File is the relative path to the JS file within the system directory.
	File string `json:"file"`

	// EntryPoint is the global variable/class name exported by the script.
	EntryPoint string `json:"entry_point,omitempty"`

	// Description is a short summary of what the renderer does.
	Description string `json:"description,omitempty"`
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

	// Category classifies the preset for feature gating (e.g., "character",
	// "item", "creature"). Used to identify which entity types belong to
	// specific features like the Armory (items) or NPC gallery (characters).
	Category string `json:"category,omitempty"`

	// FoundryActorType is the Foundry VTT Actor type string that corresponds
	// to this preset (e.g., "character", "hero", "npc"). Different game
	// systems use different actor types — D&D 5e uses "character", Draw Steel
	// uses "hero". The Foundry module reads this from the API to create and
	// filter actors of the correct type. Defaults to "character" if omitted.
	FoundryActorType string `json:"foundry_actor_type,omitempty"`

	// Fields are the default field definitions for entities of this type.
	Fields []FieldDef `json:"fields,omitempty"`
}

// RelationPresetDef describes a relation type template that a module provides.
// Used to create system-specific relation types (e.g., "has-item" for inventory).
type RelationPresetDef struct {
	// Slug is the URL-safe identifier (e.g., "has-item").
	Slug string `json:"slug"`

	// Name is the display name (e.g., "Has Item").
	Name string `json:"name"`

	// ReverseName is the reverse direction label (e.g., "In Inventory Of").
	ReverseName string `json:"reverse_name"`

	// MetadataSchema defines the JSON metadata fields for this relation.
	// Keys are field names, values describe type and default value.
	MetadataSchema map[string]RelationFieldSchema `json:"metadata_schema,omitempty"`
}

// RelationFieldSchema defines a single metadata field on a relation preset.
type RelationFieldSchema struct {
	Type    string `json:"type"`    // "number", "boolean", "string"
	Default any    `json:"default"` // Default value for new relations.
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

// ItemPreset returns the first entity preset with category "item", or nil
// if no item preset is defined. Used by the Armory plugin and item sync.
func (m *SystemManifest) ItemPreset() *EntityPresetDef {
	for i := range m.EntityPresets {
		if m.EntityPresets[i].Category == "item" {
			return &m.EntityPresets[i]
		}
	}
	return nil
}

// ItemFieldsForAPI builds the API response for item preset fields.
// Returns nil if no item preset exists. Mirrors CharacterFieldsForAPI.
func (m *SystemManifest) ItemFieldsForAPI() *CharacterFieldsResponse {
	preset := m.ItemPreset()
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
		SystemID:         m.ID,
		PresetSlug:       preset.Slug,
		PresetName:       preset.Name,
		FoundrySystemID:  m.FoundrySystemID,
		FoundryActorType: preset.FoundryActorType,
		Fields:           fields,
	}
}

// CharacterFieldsResponse is the API response shape for the character
// fields endpoint, containing field definitions with Foundry annotations.
type CharacterFieldsResponse struct {
	SystemID         string                 `json:"system_id"`
	PresetSlug       string                 `json:"preset_slug"`
	PresetName       string                 `json:"preset_name"`
	FoundrySystemID  string                 `json:"foundry_system_id,omitempty"`
	FoundryActorType string                 `json:"foundry_actor_type,omitempty"`
	Fields           []CharacterFieldExport `json:"fields"`
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
		SystemID:         m.ID,
		PresetSlug:       preset.Slug,
		PresetName:       preset.Name,
		FoundrySystemID:  m.FoundrySystemID,
		FoundryActorType: preset.FoundryActorType,
		Fields:           fields,
	}
}

// ValidationReport summarizes a system manifest's capabilities and readiness.
// Used to give campaign owners clear feedback after uploading a custom system.
type ValidationReport struct {
	// CategoryCount is the number of reference data categories.
	CategoryCount int `json:"category_count"`

	// TotalFields is the total number of fields across all categories.
	TotalFields int `json:"total_fields"`

	// PresetCount is the number of entity presets defined.
	PresetCount int `json:"preset_count"`

	// HasCharacterPreset indicates a character preset was found.
	HasCharacterPreset bool `json:"has_character_preset"`

	// CharacterFieldCount is the number of fields on the character preset.
	CharacterFieldCount int `json:"character_field_count"`

	// HasItemPreset indicates an item-category preset was found.
	HasItemPreset bool `json:"has_item_preset"`

	// ItemFieldCount is the number of fields on the item preset.
	ItemFieldCount int `json:"item_field_count"`

	// FoundryCompatible indicates foundry_system_id is set.
	FoundryCompatible bool `json:"foundry_compatible"`

	// FoundrySystemID is the declared Foundry system ID (if any).
	FoundrySystemID string `json:"foundry_system_id,omitempty"`

	// FoundryMappedFields is how many character fields have foundry_path set.
	FoundryMappedFields int `json:"foundry_mapped_fields"`

	// FoundryWritableFields is how many mapped fields are writable to Foundry.
	FoundryWritableFields int `json:"foundry_writable_fields"`

	// WidgetCount is the number of JS widgets provided by this system.
	WidgetCount int `json:"widget_count"`

	// Warnings lists non-fatal issues the owner should be aware of.
	Warnings []string `json:"warnings,omitempty"`
}

// BuildValidationReport analyzes the manifest and produces a summary of
// capabilities, Foundry compatibility, and any warnings.
func (m *SystemManifest) BuildValidationReport() *ValidationReport {
	r := &ValidationReport{
		CategoryCount:     len(m.Categories),
		PresetCount:       len(m.EntityPresets),
		WidgetCount:       len(m.Widgets),
		FoundrySystemID:   m.FoundrySystemID,
		FoundryCompatible: m.FoundrySystemID != "",
	}

	// Count category fields.
	for _, cat := range m.Categories {
		r.TotalFields += len(cat.Fields)
	}

	// Analyze character preset.
	if preset := m.CharacterPreset(); preset != nil {
		r.HasCharacterPreset = true
		r.CharacterFieldCount = len(preset.Fields)

		for _, f := range preset.Fields {
			if f.FoundryPath != "" {
				r.FoundryMappedFields++
				if f.IsFoundryWritable() {
					r.FoundryWritableFields++
				}
			}
		}
	}

	// Analyze item preset.
	if itemPreset := m.ItemPreset(); itemPreset != nil {
		r.HasItemPreset = true
		r.ItemFieldCount = len(itemPreset.Fields)
	}

	// Generate warnings.
	if r.CategoryCount == 0 {
		r.Warnings = append(r.Warnings, "No reference data categories defined")
	}
	if r.PresetCount == 0 {
		r.Warnings = append(r.Warnings, "No entity presets defined — campaigns won't get auto-created entity types")
	}
	if !r.HasCharacterPreset {
		r.Warnings = append(r.Warnings, "No character preset found (slug ending in '-character') — Foundry character sync won't work")
	}
	if r.HasCharacterPreset && !r.FoundryCompatible {
		r.Warnings = append(r.Warnings, "Character preset exists but no foundry_system_id set — Foundry auto-detection disabled")
	}
	if r.HasCharacterPreset && r.FoundryCompatible && r.FoundryMappedFields == 0 {
		r.Warnings = append(r.Warnings, "foundry_system_id is set but no fields have foundry_path — character sync will be name-only")
	}

	return r
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

// ValidFieldTypes is the whitelist of accepted field type values.
// Manifests with unknown field types are rejected during validation.
var ValidFieldTypes = map[string]bool{
	"string":   true,
	"number":   true,
	"boolean":  true,
	"list":     true,
	"markdown": true,
	"enum":     true,
	"url":      true,
}

// Manifest content limits to prevent resource exhaustion.
const (
	maxCategories         = 20
	maxFieldsPerCategory  = 100
	maxFieldsPerPreset    = 50
	maxEntityPresets      = 10
	maxRelationPresets    = 20
	maxWidgets            = 10
	maxTextRenderers      = 5
)

// slugPattern matches valid manifest IDs and preset slugs.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidateManifest checks that a manifest has all required fields and
// valid values. Returns a descriptive error for the first violation found.
func ValidateManifest(m *SystemManifest) error {
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	// Default version when omitted — the package manager overwrites this
	// with the GitHub release tag during install. This prevents validation
	// failures for manifests that rely on external version management.
	if m.Version == "" {
		m.Version = "0.0.0"
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

	// Validate ID format.
	if !slugPattern.MatchString(m.ID) {
		return fmt.Errorf("id %q must contain only lowercase letters, numbers, hyphens, and underscores", m.ID)
	}

	// Enforce content limits.
	if len(m.Categories) > maxCategories {
		return fmt.Errorf("too many categories (%d, max %d)", len(m.Categories), maxCategories)
	}
	if len(m.EntityPresets) > maxEntityPresets {
		return fmt.Errorf("too many entity presets (%d, max %d)", len(m.EntityPresets), maxEntityPresets)
	}
	if len(m.RelationPresets) > maxRelationPresets {
		return fmt.Errorf("too many relation presets (%d, max %d)", len(m.RelationPresets), maxRelationPresets)
	}

	// Validate categories.
	for i, cat := range m.Categories {
		if cat.Slug == "" {
			return fmt.Errorf("category %d: slug is required", i)
		}
		if !slugPattern.MatchString(cat.Slug) {
			return fmt.Errorf("category %d: slug %q must contain only lowercase letters, numbers, hyphens, and underscores", i, cat.Slug)
		}
		if cat.Name == "" {
			return fmt.Errorf("category %d (%s): name is required", i, cat.Slug)
		}
		if len(cat.Fields) > maxFieldsPerCategory {
			return fmt.Errorf("category %q: too many fields (%d, max %d)", cat.Slug, len(cat.Fields), maxFieldsPerCategory)
		}
		for j, f := range cat.Fields {
			if err := validateFieldDef(f, fmt.Sprintf("category %q field %d", cat.Slug, j)); err != nil {
				return err
			}
		}
	}

	// Validate entity presets.
	for i, preset := range m.EntityPresets {
		if preset.Slug == "" {
			return fmt.Errorf("entity preset %d: slug is required", i)
		}
		if !slugPattern.MatchString(preset.Slug) {
			return fmt.Errorf("entity preset %d: slug %q must contain only lowercase letters, numbers, hyphens, and underscores", i, preset.Slug)
		}
		if preset.Name == "" {
			return fmt.Errorf("entity preset %d (%s): name is required", i, preset.Slug)
		}
		if len(preset.Fields) > maxFieldsPerPreset {
			return fmt.Errorf("entity preset %q: too many fields (%d, max %d)", preset.Slug, len(preset.Fields), maxFieldsPerPreset)
		}
		for j, f := range preset.Fields {
			if err := validateFieldDef(f, fmt.Sprintf("preset %q field %d", preset.Slug, j)); err != nil {
				return err
			}
		}
	}

	// Validate widgets.
	if len(m.Widgets) > maxWidgets {
		return fmt.Errorf("too many widgets (%d, max %d)", len(m.Widgets), maxWidgets)
	}
	for i, w := range m.Widgets {
		if w.Slug == "" {
			return fmt.Errorf("widget %d: slug is required", i)
		}
		if !slugPattern.MatchString(w.Slug) {
			return fmt.Errorf("widget %d: slug %q must contain only lowercase letters, numbers, hyphens, and underscores", i, w.Slug)
		}
		if w.Name == "" {
			return fmt.Errorf("widget %d (%s): name is required", i, w.Slug)
		}
		// Accept "file" as an alias for "script_file" (backward compatibility).
		if w.ScriptFile == "" && w.File != "" {
			m.Widgets[i].ScriptFile = w.File
			w.ScriptFile = w.File
		}
		if w.ScriptFile == "" {
			return fmt.Errorf("widget %d (%s): script_file is required", i, w.Slug)
		}
		if !strings.HasSuffix(w.ScriptFile, ".js") {
			return fmt.Errorf("widget %d (%s): script_file must end in .js", i, w.Slug)
		}
		if strings.Contains(w.ScriptFile, "..") {
			return fmt.Errorf("widget %d (%s): script_file must not contain path traversal", i, w.Slug)
		}
	}

	// Validate text renderers.
	if len(m.TextRenderers) > maxTextRenderers {
		return fmt.Errorf("too many text_renderers (%d, max %d)", len(m.TextRenderers), maxTextRenderers)
	}
	for i, tr := range m.TextRenderers {
		if tr.Slug == "" {
			return fmt.Errorf("text_renderer %d: slug is required", i)
		}
		if !slugPattern.MatchString(tr.Slug) {
			return fmt.Errorf("text_renderer %d: slug %q must contain only lowercase letters, numbers, hyphens, and underscores", i, tr.Slug)
		}
		if tr.File == "" {
			return fmt.Errorf("text_renderer %d (%s): file is required", i, tr.Slug)
		}
		if !strings.HasSuffix(tr.File, ".js") {
			return fmt.Errorf("text_renderer %d (%s): file must end in .js", i, tr.Slug)
		}
		if strings.Contains(tr.File, "..") {
			return fmt.Errorf("text_renderer %d (%s): file must not contain path traversal", i, tr.Slug)
		}
	}

	// Sanitize text fields to prevent stored XSS.
	sanitizeManifestStrings(m)

	return nil
}

// validateFieldDef checks a single field definition for required fields
// and valid type values.
func validateFieldDef(f FieldDef, context string) error {
	if f.Key == "" {
		return fmt.Errorf("%s: key is required", context)
	}
	if f.Label == "" {
		return fmt.Errorf("%s (%s): label is required", context, f.Key)
	}
	if f.Type == "" {
		return fmt.Errorf("%s (%s): type is required", context, f.Key)
	}
	if !ValidFieldTypes[f.Type] {
		return fmt.Errorf("%s (%s): unknown field type %q (valid: string, number, boolean, list, markdown, enum, url)", context, f.Key, f.Type)
	}
	return nil
}

// sanitizeManifestStrings escapes HTML in all user-facing text fields
// to prevent stored XSS when manifest content is rendered in templates.
func sanitizeManifestStrings(m *SystemManifest) {
	m.Name = html.EscapeString(m.Name)
	m.Description = html.EscapeString(m.Description)
	m.Author = html.EscapeString(m.Author)

	for i := range m.Categories {
		m.Categories[i].Name = html.EscapeString(m.Categories[i].Name)
	}

	for i := range m.EntityPresets {
		m.EntityPresets[i].Name = html.EscapeString(m.EntityPresets[i].Name)
		m.EntityPresets[i].NamePlural = html.EscapeString(m.EntityPresets[i].NamePlural)
		for j := range m.EntityPresets[i].Fields {
			m.EntityPresets[i].Fields[j].Label = html.EscapeString(m.EntityPresets[i].Fields[j].Label)
		}
	}

	for i := range m.RelationPresets {
		m.RelationPresets[i].Name = html.EscapeString(m.RelationPresets[i].Name)
		m.RelationPresets[i].ReverseName = html.EscapeString(m.RelationPresets[i].ReverseName)
	}

	for i := range m.Widgets {
		m.Widgets[i].Name = html.EscapeString(m.Widgets[i].Name)
		m.Widgets[i].Description = html.EscapeString(m.Widgets[i].Description)
	}

	for i := range m.TextRenderers {
		m.TextRenderers[i].Name = html.EscapeString(m.TextRenderers[i].Name)
		m.TextRenderers[i].Description = html.EscapeString(m.TextRenderers[i].Description)
	}
}

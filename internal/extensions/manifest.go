package extensions

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ExtensionManifest describes a content extension's metadata, compatibility,
// dependencies, and what content it contributes. Parsed from manifest.json.
type ExtensionManifest struct {
	ManifestVersion int    `json:"manifest_version"`
	ID              string `json:"id"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Description     string `json:"description"`

	Author      *ManifestAuthor      `json:"author,omitempty"`
	License     string               `json:"license,omitempty"`
	Homepage    string               `json:"homepage,omitempty"`
	Repository  string               `json:"repository,omitempty"`
	Keywords    []string             `json:"keywords,omitempty"`
	Icon        string               `json:"icon,omitempty"`
	Compatibility *ManifestCompat    `json:"compatibility,omitempty"`
	RequiresAddons []string          `json:"requires_addons,omitempty"`
	Dependencies []ManifestDependency `json:"dependencies,omitempty"`
	Conflicts    []string            `json:"conflicts,omitempty"`

	Contributes *ManifestContributes `json:"contributes,omitempty"`
}

// ManifestAuthor identifies the extension creator.
type ManifestAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// ManifestCompat specifies Chronicle version requirements.
type ManifestCompat struct {
	Minimum  string `json:"minimum"`
	Maximum  string `json:"maximum,omitempty"`
	Verified string `json:"verified,omitempty"`
}

// ManifestDependency specifies another extension this one depends on.
type ManifestDependency struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// ManifestContributes declares what content this extension provides.
// Follows the VS Code "contributes" pattern.
type ManifestContributes struct {
	EntityTypeTemplates []EntityTypeTemplate `json:"entity_type_templates,omitempty"`
	EntityPacks         []EntityPack         `json:"entity_packs,omitempty"`
	CalendarPresets     []CalendarPreset     `json:"calendar_presets,omitempty"`
	TagCollections      []TagCollection      `json:"tag_collections,omitempty"`
	RelationTypes       []RelationType       `json:"relation_types,omitempty"`
	MarkerIconPacks     []MarkerIconPack     `json:"marker_icon_packs,omitempty"`
	Themes              []Theme              `json:"themes,omitempty"`
	ReferenceData       []ReferenceDataPack  `json:"reference_data,omitempty"`
	Widgets             []WidgetContribution `json:"widgets,omitempty"`
	WASMPlugins         []WASMContribution   `json:"wasm_plugins,omitempty"`
}

// EntityTypeTemplate is a pre-configured entity type with fields.
type EntityTypeTemplate struct {
	Slug       string          `json:"slug"`
	Name       string          `json:"name"`
	NamePlural string          `json:"name_plural"`
	Icon       string          `json:"icon"`
	Color      string          `json:"color"`
	Fields     []TemplateField `json:"fields,omitempty"`
}

// TemplateField defines a custom field within an entity type template.
type TemplateField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Group   string   `json:"group,omitempty"`
	Options []string `json:"options,omitempty"`
}

// EntityPack is a collection of pre-made entities.
type EntityPack struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	EntityTypeSlug string `json:"entity_type_slug"`
	File           string `json:"file"` // Relative path to JSON file.
	Count          int    `json:"count,omitempty"`
}

// CalendarPreset is a complete calendar configuration.
type CalendarPreset struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	File        string `json:"file"` // Relative path to JSON file.
}

// TagCollection is a set of pre-defined tags.
type TagCollection struct {
	Slug string        `json:"slug"`
	Name string        `json:"name"`
	Tags []TagTemplate `json:"tags"`
}

// TagTemplate defines a single tag within a collection.
type TagTemplate struct {
	Name   string `json:"name"`
	Color  string `json:"color,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// RelationType defines a suggested relation type pair.
type RelationType struct {
	Type        string `json:"type"`
	ReverseType string `json:"reverse_type"`
	Description string `json:"description,omitempty"`
}

// MarkerIconPack is a set of custom map marker icons.
type MarkerIconPack struct {
	Slug  string       `json:"slug"`
	Name  string       `json:"name"`
	Icons []MarkerIcon `json:"icons"`
}

// MarkerIcon defines a single custom marker icon.
type MarkerIcon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	File string `json:"file"` // Relative path to SVG/PNG.
}

// Theme is a CSS theme override.
type Theme struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	File        string `json:"file"`    // Relative path to CSS file.
	Preview     string `json:"preview,omitempty"` // Relative path to preview image.
}

// ReferenceDataPack extends the module system with additional reference data.
type ReferenceDataPack struct {
	Slug       string                `json:"slug"`
	Name       string                `json:"name"`
	Categories []ReferenceCategory   `json:"categories"`
}

// ReferenceCategory defines a category within a reference data pack.
type ReferenceCategory struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
	File string `json:"file"` // Relative path to JSON file.
}

// WidgetContribution declares a browser-side widget that an extension provides.
// Widget JS files are loaded into campaign pages when the extension is enabled.
// Widgets register via Chronicle.registerWidget() and mount to data-widget elements.
type WidgetContribution struct {
	Slug        string              `json:"slug"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Icon        string              `json:"icon,omitempty"`         // FontAwesome class.
	File        string              `json:"file"`                   // Relative path to JS file.
	Config      []WidgetConfigField `json:"config,omitempty"`       // Configurable data-* attributes.
}

// WidgetConfigField describes a configuration option exposed via data-* attributes
// on the widget's DOM element.
type WidgetConfigField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`                     // "string", "number", "boolean", "select".
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`         // For type "select".
	Description string   `json:"description,omitempty"`
}

// extIDPattern validates extension IDs: lowercase alphanumeric + hyphens, 3-64 chars.
var extIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// semverPattern validates semantic version strings.
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ParseManifest parses and validates a manifest.json byte slice.
func ParseManifest(data []byte) (*ExtensionManifest, error) {
	var m ExtensionManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest JSON: %w", err)
	}

	if err := ValidateManifest(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

// ValidateManifest checks that a manifest has all required fields and valid values.
func ValidateManifest(m *ExtensionManifest) error {
	if m.ManifestVersion != 1 {
		return fmt.Errorf("unsupported manifest_version %d (expected 1)", m.ManifestVersion)
	}
	if !extIDPattern.MatchString(m.ID) {
		return fmt.Errorf("invalid id %q: must be 3-64 lowercase alphanumeric + hyphens", m.ID)
	}
	if m.Name == "" || len(m.Name) > 100 {
		return fmt.Errorf("name is required and must be at most 100 characters")
	}
	if !semverPattern.MatchString(m.Version) {
		return fmt.Errorf("version %q must be semantic version (e.g., 1.0.0)", m.Version)
	}
	if m.Description == "" || len(m.Description) > 500 {
		return fmt.Errorf("description is required and must be at most 500 characters")
	}

	// Validate author if provided.
	if m.Author != nil && m.Author.Name == "" {
		return fmt.Errorf("author.name is required when author is specified")
	}

	// Validate keywords.
	if len(m.Keywords) > 10 {
		return fmt.Errorf("at most 10 keywords allowed")
	}
	for _, kw := range m.Keywords {
		if len(kw) > 30 {
			return fmt.Errorf("keyword %q exceeds 30 character limit", kw)
		}
	}

	// Validate compatibility if specified.
	if m.Compatibility != nil {
		if m.Compatibility.Minimum != "" && !semverPattern.MatchString(m.Compatibility.Minimum) {
			return fmt.Errorf("compatibility.minimum %q is not valid semver", m.Compatibility.Minimum)
		}
	}

	// Validate contributes file paths.
	if m.Contributes != nil {
		if err := validateContributes(m.Contributes); err != nil {
			return fmt.Errorf("contributes: %w", err)
		}
	}

	return nil
}

// validateContributes checks that contributed content has valid slugs and file paths.
func validateContributes(c *ManifestContributes) error {
	for i, t := range c.EntityTypeTemplates {
		if t.Slug == "" {
			return fmt.Errorf("entity_type_templates[%d]: slug is required", i)
		}
		if t.Name == "" {
			return fmt.Errorf("entity_type_templates[%d]: name is required", i)
		}
	}

	for i, p := range c.EntityPacks {
		if p.Slug == "" {
			return fmt.Errorf("entity_packs[%d]: slug is required", i)
		}
		if p.File == "" {
			return fmt.Errorf("entity_packs[%d]: file is required", i)
		}
		if err := validateFilePath(p.File); err != nil {
			return fmt.Errorf("entity_packs[%d].file: %w", i, err)
		}
	}

	for i, p := range c.CalendarPresets {
		if p.Slug == "" {
			return fmt.Errorf("calendar_presets[%d]: slug is required", i)
		}
		if p.File == "" {
			return fmt.Errorf("calendar_presets[%d]: file is required", i)
		}
		if err := validateFilePath(p.File); err != nil {
			return fmt.Errorf("calendar_presets[%d].file: %w", i, err)
		}
	}

	for i, t := range c.TagCollections {
		if t.Slug == "" {
			return fmt.Errorf("tag_collections[%d]: slug is required", i)
		}
	}

	for i, pack := range c.MarkerIconPacks {
		if pack.Slug == "" {
			return fmt.Errorf("marker_icon_packs[%d]: slug is required", i)
		}
		for j, icon := range pack.Icons {
			if icon.File == "" {
				return fmt.Errorf("marker_icon_packs[%d].icons[%d]: file is required", i, j)
			}
			if err := validateFilePath(icon.File); err != nil {
				return fmt.Errorf("marker_icon_packs[%d].icons[%d].file: %w", i, j, err)
			}
		}
	}

	for i, t := range c.Themes {
		if t.Slug == "" {
			return fmt.Errorf("themes[%d]: slug is required", i)
		}
		if t.File == "" {
			return fmt.Errorf("themes[%d]: file is required", i)
		}
		if err := validateFilePath(t.File); err != nil {
			return fmt.Errorf("themes[%d].file: %w", i, err)
		}
	}

	for i, r := range c.ReferenceData {
		if r.Slug == "" {
			return fmt.Errorf("reference_data[%d]: slug is required", i)
		}
		for j, cat := range r.Categories {
			if cat.File == "" {
				return fmt.Errorf("reference_data[%d].categories[%d]: file is required", i, j)
			}
			if err := validateFilePath(cat.File); err != nil {
				return fmt.Errorf("reference_data[%d].categories[%d].file: %w", i, j, err)
			}
		}
	}

	for i, w := range c.Widgets {
		if w.Slug == "" {
			return fmt.Errorf("widgets[%d]: slug is required", i)
		}
		if w.Name == "" {
			return fmt.Errorf("widgets[%d]: name is required", i)
		}
		if w.File == "" {
			return fmt.Errorf("widgets[%d]: file is required", i)
		}
		if err := validateFilePath(w.File); err != nil {
			return fmt.Errorf("widgets[%d].file: %w", i, err)
		}
		if !strings.HasSuffix(strings.ToLower(w.File), ".js") {
			return fmt.Errorf("widgets[%d].file: must be a .js file", i)
		}
	}

	for i, wp := range c.WASMPlugins {
		if wp.Slug == "" {
			return fmt.Errorf("wasm_plugins[%d]: slug is required", i)
		}
		if wp.Name == "" {
			return fmt.Errorf("wasm_plugins[%d]: name is required", i)
		}
		if wp.File == "" {
			return fmt.Errorf("wasm_plugins[%d]: file is required", i)
		}
		if err := validateFilePath(wp.File); err != nil {
			return fmt.Errorf("wasm_plugins[%d].file: %w", i, err)
		}
		if !strings.HasSuffix(strings.ToLower(wp.File), ".wasm") {
			return fmt.Errorf("wasm_plugins[%d].file: must be a .wasm file", i)
		}
		if len(wp.Capabilities) == 0 {
			return fmt.Errorf("wasm_plugins[%d]: at least one capability is required", i)
		}
		for j, cap := range wp.Capabilities {
			if !AllCapabilities[cap] {
				return fmt.Errorf("wasm_plugins[%d].capabilities[%d]: unknown capability %q", i, j, cap)
			}
		}
		for j, hook := range wp.Hooks {
			if !ValidHookTypes[hook] {
				return fmt.Errorf("wasm_plugins[%d].hooks[%d]: unknown hook type %q", i, j, hook)
			}
		}
		if wp.MemoryLimitMB > 256 {
			return fmt.Errorf("wasm_plugins[%d]: memory_limit_mb cannot exceed 256", i)
		}
		if wp.TimeoutSecs > 300 {
			return fmt.Errorf("wasm_plugins[%d]: timeout_secs cannot exceed 300", i)
		}
	}

	return nil
}

// validateFilePath checks that a file path is relative and safe.
func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("absolute paths not allowed: %s", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}
	return nil
}

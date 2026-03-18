// preview.go provides dry-run inspection of game system packages before
// installation. Validates ZIP contents, parses manifests, counts reference
// data, and builds a tree structure for impact visualization — all without
// writing anything to disk.
package systems

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// PreviewResult holds the complete analysis of a game system package.
// Used by both admin package approval and campaign owner upload flows.
type PreviewResult struct {
	// Manifest is the parsed system manifest.
	Manifest *SystemManifest `json:"manifest"`

	// Report is the validation summary (counts, Foundry compat, warnings).
	Report *ValidationReport `json:"report"`

	// Categories lists each category with item counts and samples.
	Categories []CategoryPreview `json:"categories"`

	// EntityPresets lists each entity preset with field details.
	EntityPresets []PresetPreview `json:"entity_presets"`

	// Warnings are non-fatal issues discovered during preview.
	Warnings []string `json:"warnings,omitempty"`

	// Errors are fatal issues that would prevent installation.
	Errors []string `json:"errors,omitempty"`

	// TreeData is the hierarchical structure for the impact diagram.
	TreeData *TreeNode `json:"tree_data"`

	// TotalItems is the total reference data item count across all categories.
	TotalItems int `json:"total_items"`

	// Valid indicates whether installation would succeed.
	Valid bool `json:"valid"`
}

// CategoryPreview summarizes a reference data category for the preview.
type CategoryPreview struct {
	Slug       string           `json:"slug"`
	Name       string           `json:"name"`
	Icon       string           `json:"icon,omitempty"`
	ItemCount  int              `json:"item_count"`
	FieldCount int              `json:"field_count"`
	Samples    []ReferenceItem  `json:"samples,omitempty"` // First 3 items for preview.
}

// PresetPreview summarizes an entity preset for the preview.
type PresetPreview struct {
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	NamePlural string    `json:"name_plural"`
	Icon       string    `json:"icon"`
	Color      string    `json:"color"`
	Category   string    `json:"category,omitempty"`
	Fields     []FieldDef `json:"fields"`
	FoundryActorType string `json:"foundry_actor_type,omitempty"`
}

// TreeNode represents a single node in the impact tree diagram.
// Nodes form a tree structure rendered by the client-side impact_tree widget.
type TreeNode struct {
	// Label is the display text for this node.
	Label string `json:"label"`

	// Icon is an optional Font Awesome icon class.
	Icon string `json:"icon,omitempty"`

	// Badge is optional count or status text shown after the label.
	Badge string `json:"badge,omitempty"`

	// Type classifies the node for styling: "root", "section", "category",
	// "preset", "field", "foundry", "warning".
	Type string `json:"type"`

	// Children are nested sub-nodes.
	Children []*TreeNode `json:"children,omitempty"`
}

// PreviewFromZIP validates a ZIP file and produces a full preview without
// writing to disk. The ZIP data is read entirely in memory.
func PreviewFromZIP(zipData []byte) (*PreviewResult, error) {
	result := &PreviewResult{Valid: true}

	if int64(len(zipData)) > maxSystemZipSize {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("ZIP exceeds maximum size of %d MB", maxSystemZipSize/(1024*1024)))
		return result, nil
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Invalid ZIP file: %v", err))
		return result, nil
	}

	// Find manifest and data files.
	var manifestFile *zip.File
	dataFiles := make(map[string]*zip.File) // "data/spells.json" → file
	for _, f := range zr.File {
		if strings.Contains(f.Name, "..") {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Path traversal detected: %s", f.Name))
			return result, nil
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == "manifest.json" {
			manifestFile = f
		} else if strings.HasPrefix(f.Name, "data/") && strings.HasSuffix(f.Name, ".json") {
			if f.UncompressedSize64 > maxDataFileSize {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Data file %s exceeds %d MB limit", f.Name, maxDataFileSize/(1024*1024)))
				result.Valid = false
			}
			dataFiles[f.Name] = f
		}
	}

	if manifestFile == nil {
		result.Valid = false
		result.Errors = append(result.Errors, "ZIP must contain a manifest.json at the root")
		return result, nil
	}

	// Parse manifest.
	manifest, err := readManifestFromZipFile(manifestFile)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Manifest error: %v", err))
		return result, nil
	}
	result.Manifest = manifest

	if len(dataFiles) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "ZIP must contain at least one data/*.json file")
	}

	// Build validation report.
	result.Report = manifest.BuildValidationReport()
	result.Warnings = append(result.Warnings, result.Report.Warnings...)

	// Analyze each category's data file.
	for _, cat := range manifest.Categories {
		cp := CategoryPreview{
			Slug:       cat.Slug,
			Name:       cat.Name,
			Icon:       cat.Icon,
			FieldCount: len(cat.Fields),
		}

		dataKey := "data/" + cat.Slug + ".json"
		if f, ok := dataFiles[dataKey]; ok {
			items, err := readItemsFromZipFile(f)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Category %q: %v", cat.Slug, err))
			} else {
				cp.ItemCount = len(items)
				result.TotalItems += len(items)
				// Keep first 3 as samples.
				if len(items) > 3 {
					cp.Samples = items[:3]
				} else {
					cp.Samples = items
				}
			}
		} else {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Category %q has no data file (%s)", cat.Slug, dataKey))
		}

		result.Categories = append(result.Categories, cp)
	}

	// Build entity preset previews.
	for _, preset := range manifest.EntityPresets {
		result.EntityPresets = append(result.EntityPresets, PresetPreview{
			Slug:             preset.Slug,
			Name:             preset.Name,
			NamePlural:       preset.NamePlural,
			Icon:             preset.Icon,
			Color:            preset.Color,
			Category:         preset.Category,
			Fields:           preset.Fields,
			FoundryActorType: preset.FoundryActorType,
		})
	}

	// Build impact tree.
	result.TreeData = BuildImpactTree(manifest, result.Categories, result.Warnings)

	return result, nil
}

// PreviewFromPackage inspects an already-downloaded package directory.
// Used by admin approval flow for packages fetched from GitHub.
func PreviewFromPackage(installPath string) (*PreviewResult, error) {
	manifestPath := installPath + "/manifest.json"
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return &PreviewResult{
			Valid:  false,
			Errors: []string{fmt.Sprintf("Manifest error: %v", err)},
		}, nil
	}

	result := &PreviewResult{
		Valid:    true,
		Manifest: manifest,
		Report:   manifest.BuildValidationReport(),
	}
	result.Warnings = append(result.Warnings, result.Report.Warnings...)

	// Load data provider to count items.
	dataDir := installPath + "/data"
	provider, err := NewJSONProvider(manifest.ID, dataDir)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not load data: %v", err))
	}

	for _, cat := range manifest.Categories {
		cp := CategoryPreview{
			Slug:       cat.Slug,
			Name:       cat.Name,
			Icon:       cat.Icon,
			FieldCount: len(cat.Fields),
		}

		if provider != nil {
			if items, listErr := provider.List(cat.Slug); listErr == nil {
				cp.ItemCount = len(items)
				result.TotalItems += len(items)
				if len(items) > 3 {
					cp.Samples = items[:3]
				} else {
					cp.Samples = items
				}
			}
		}

		result.Categories = append(result.Categories, cp)
	}

	// Entity preset previews.
	for _, preset := range manifest.EntityPresets {
		result.EntityPresets = append(result.EntityPresets, PresetPreview{
			Slug:             preset.Slug,
			Name:             preset.Name,
			NamePlural:       preset.NamePlural,
			Icon:             preset.Icon,
			Color:            preset.Color,
			Category:         preset.Category,
			Fields:           preset.Fields,
			FoundryActorType: preset.FoundryActorType,
		})
	}

	result.TreeData = BuildImpactTree(manifest, result.Categories, result.Warnings)

	return result, nil
}

// BuildImpactTree creates a hierarchical tree structure representing
// what a system package adds. Rendered as a visual flowchart in the UI.
func BuildImpactTree(manifest *SystemManifest, categories []CategoryPreview, warnings []string) *TreeNode {
	root := &TreeNode{
		Label: fmt.Sprintf("%s v%s", manifest.Name, manifest.Version),
		Icon:  manifest.Icon,
		Type:  "root",
	}

	if manifest.Author != "" {
		root.Badge = "by " + manifest.Author
	}

	// Categories section.
	if len(categories) > 0 {
		catSection := &TreeNode{
			Label: "Reference Data",
			Icon:  "fa-book",
			Badge: fmt.Sprintf("%d categories", len(categories)),
			Type:  "section",
		}
		for _, cat := range categories {
			catNode := &TreeNode{
				Label: cat.Name,
				Icon:  cat.Icon,
				Badge: fmt.Sprintf("%d items", cat.ItemCount),
				Type:  "category",
			}
			if cat.FieldCount > 0 {
				catNode.Badge += fmt.Sprintf(", %d fields", cat.FieldCount)
			}
			catSection.Children = append(catSection.Children, catNode)
		}
		root.Children = append(root.Children, catSection)
	}

	// Entity presets section.
	if len(manifest.EntityPresets) > 0 {
		presetSection := &TreeNode{
			Label: "Entity Presets",
			Icon:  "fa-shapes",
			Badge: fmt.Sprintf("%d presets", len(manifest.EntityPresets)),
			Type:  "section",
		}
		for _, preset := range manifest.EntityPresets {
			presetNode := &TreeNode{
				Label: preset.Name,
				Icon:  preset.Icon,
				Badge: fmt.Sprintf("%d fields", len(preset.Fields)),
				Type:  "preset",
			}
			for _, field := range preset.Fields {
				fieldNode := &TreeNode{
					Label: field.Label,
					Badge: field.Type,
					Type:  "field",
				}
				if field.FoundryPath != "" {
					fieldNode.Badge += " → " + field.FoundryPath
				}
				presetNode.Children = append(presetNode.Children, fieldNode)
			}
			presetSection.Children = append(presetSection.Children, presetNode)
		}
		root.Children = append(root.Children, presetSection)
	}

	// Foundry compatibility section.
	if manifest.FoundrySystemID != "" {
		foundrySection := &TreeNode{
			Label: "Foundry VTT",
			Icon:  "fa-dice-d20",
			Type:  "foundry",
		}
		foundrySection.Children = append(foundrySection.Children, &TreeNode{
			Label: "System ID",
			Badge: manifest.FoundrySystemID,
			Type:  "field",
		})

		if preset := manifest.CharacterPreset(); preset != nil {
			mapped, writable := 0, 0
			for _, f := range preset.Fields {
				if f.FoundryPath != "" {
					mapped++
					if f.IsFoundryWritable() {
						writable++
					}
				}
			}
			foundrySection.Children = append(foundrySection.Children, &TreeNode{
				Label: "Mapped Fields",
				Badge: fmt.Sprintf("%d/%d", mapped, len(preset.Fields)),
				Type:  "field",
			})
			foundrySection.Children = append(foundrySection.Children, &TreeNode{
				Label: "Writable Fields",
				Badge: fmt.Sprintf("%d/%d", writable, mapped),
				Type:  "field",
			})
		}

		root.Children = append(root.Children, foundrySection)
	}

	// Warnings section.
	if len(warnings) > 0 {
		warnSection := &TreeNode{
			Label: "Warnings",
			Icon:  "fa-triangle-exclamation",
			Badge: fmt.Sprintf("%d", len(warnings)),
			Type:  "section",
		}
		for _, w := range warnings {
			warnSection.Children = append(warnSection.Children, &TreeNode{
				Label: w,
				Type:  "warning",
			})
		}
		root.Children = append(root.Children, warnSection)
	}

	return root
}

// readManifestFromZipFile reads and validates a manifest from a ZIP entry.
func readManifestFromZipFile(f *zip.File) (*SystemManifest, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening manifest: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(io.LimitReader(rc, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest SystemManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if err := ValidateManifest(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// readItemsFromZipFile reads reference items from a data file in the ZIP.
func readItemsFromZipFile(f *zip.File) ([]ReferenceItem, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(io.LimitReader(rc, maxDataFileSize))
	if err != nil {
		return nil, err
	}

	var items []ReferenceItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("not a valid ReferenceItem array: %w", err)
	}

	return items, nil
}

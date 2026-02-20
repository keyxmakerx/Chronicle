// Package entities manages worldbuilding entities — the core content objects
// in Chronicle. Every object (characters, locations, items, organizations, etc.)
// is an entity with a configurable type. Entity types define what custom fields
// appear in the profile sidebar.
//
// This is a CORE plugin — always enabled, cannot be disabled.
package entities

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// --- Domain Models ---

// EntityType defines a category of entities within a campaign (e.g., Character,
// Location). Each campaign has its own set of entity types with configurable
// fields that drive dynamic form rendering and profile display.
type EntityType struct {
	ID         int               `json:"id"`
	CampaignID string            `json:"campaign_id"`
	Slug       string            `json:"slug"`
	Name       string            `json:"name"`
	NamePlural string            `json:"name_plural"`
	Icon       string            `json:"icon"`
	Color      string            `json:"color"`
	Fields     []FieldDefinition `json:"fields"`
	Layout     EntityTypeLayout  `json:"layout"`
	SortOrder  int               `json:"sort_order"`
	IsDefault  bool              `json:"is_default"`
	Enabled    bool              `json:"enabled"`
}

// EntityTypeLayout describes the profile page layout for entities of this type.
// Uses a row-based 12-column grid system. Stored as JSON in entity_types.layout_json.
//
// Schema: {"rows": [{"id":"r1", "columns": [{"id":"c1", "width":8, "blocks":[...]}]}]}
type EntityTypeLayout struct {
	Rows []TemplateRow `json:"rows"`
}

// TemplateRow is a horizontal row in the page template grid.
type TemplateRow struct {
	ID      string           `json:"id"`
	Columns []TemplateColumn `json:"columns"`
}

// TemplateColumn is a column within a row. Width uses a 12-column grid (1-12).
type TemplateColumn struct {
	ID     string          `json:"id"`
	Width  int             `json:"width"`
	Blocks []TemplateBlock `json:"blocks"`
}

// TemplateBlock is a content component placed inside a column.
// Valid types: "title", "image", "entry", "attributes", "details", "tags",
// "divider", "two_column", "three_column", "tabs", "section".
// Container types (two_column, three_column, tabs, section) hold sub-blocks
// in their Config map -- see template_editor.js for the config schemas.
type TemplateBlock struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// DefaultLayout returns the standard two-column layout used for new entity types.
func DefaultLayout() EntityTypeLayout {
	return EntityTypeLayout{
		Rows: []TemplateRow{
			{
				ID: "row-1",
				Columns: []TemplateColumn{
					{
						ID:    "col-1-1",
						Width: 8,
						Blocks: []TemplateBlock{
							{ID: "blk-title", Type: "title"},
							{ID: "blk-entry", Type: "entry"},
						},
					},
					{
						ID:    "col-1-2",
						Width: 4,
						Blocks: []TemplateBlock{
							{ID: "blk-image", Type: "image"},
							{ID: "blk-attrs", Type: "attributes"},
							{ID: "blk-details", Type: "details"},
						},
					},
				},
			},
		},
	}
}

// ParseLayoutJSON decodes layout JSON with backward compatibility.
// Handles three cases:
//  1. New format with "rows" key → unmarshal directly
//  2. Old format with "sections" key → convert sections to rows/columns
//  3. Empty/invalid → return DefaultLayout()
func ParseLayoutJSON(raw []byte) EntityTypeLayout {
	if len(raw) == 0 {
		return DefaultLayout()
	}

	// Try new format first (rows key).
	var layout EntityTypeLayout
	if err := json.Unmarshal(raw, &layout); err == nil && len(layout.Rows) > 0 {
		return layout
	}

	// Try old format (sections key).
	var legacy struct {
		Sections []struct {
			Key    string `json:"key"`
			Label  string `json:"label"`
			Type   string `json:"type"`
			Column string `json:"column"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy.Sections) > 0 {
		return convertLegacyLayout(legacy.Sections)
	}

	return DefaultLayout()
}

// convertLegacyLayout transforms old section-based layouts into the new
// row/column/block format.
func convertLegacyLayout(sections []struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Type   string `json:"type"`
	Column string `json:"column"`
}) EntityTypeLayout {
	var leftBlocks, rightBlocks []TemplateBlock

	for _, sec := range sections {
		blockType := sec.Type
		switch blockType {
		case "fields":
			blockType = "attributes"
		case "posts":
			blockType = "details"
		}
		block := TemplateBlock{
			ID:   fmt.Sprintf("blk-%s", sec.Key),
			Type: blockType,
		}
		if sec.Column == "left" {
			leftBlocks = append(leftBlocks, block)
		} else {
			rightBlocks = append(rightBlocks, block)
		}
	}

	// Build single row with left=sidebar (4), right=main (8).
	cols := []TemplateColumn{}
	if len(rightBlocks) > 0 {
		cols = append(cols, TemplateColumn{
			ID: "col-1-1", Width: 8, Blocks: rightBlocks,
		})
	}
	if len(leftBlocks) > 0 {
		cols = append(cols, TemplateColumn{
			ID: "col-1-2", Width: 4, Blocks: leftBlocks,
		})
	}
	if len(cols) == 0 {
		return DefaultLayout()
	}

	return EntityTypeLayout{
		Rows: []TemplateRow{{ID: "row-1", Columns: cols}},
	}
}

// FieldDefinition describes a single custom field in an entity type.
// Stored as JSON array in entity_types.fields. Drives both the edit form
// (input type) and the profile sidebar (display).
type FieldDefinition struct {
	Key     string   `json:"key"`     // Machine-readable identifier (e.g., "age", "alignment").
	Label   string   `json:"label"`   // Human-readable label (e.g., "Age", "Alignment").
	Type    string   `json:"type"`    // Input type: text, number, select, textarea, checkbox, url.
	Section string   `json:"section"` // Grouping for display (e.g., "Basics", "Appearance").
	Options []string `json:"options"` // Valid values for select fields. Empty for other types.
}

// Entity represents a single worldbuilding object — a character, location,
// item, or any other type defined in the campaign's entity types.
type Entity struct {
	ID           string         `json:"id"`
	CampaignID   string         `json:"campaign_id"`
	EntityTypeID int            `json:"entity_type_id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Entry        *string        `json:"entry,omitempty"`     // TipTap/ProseMirror JSON document.
	EntryHTML    *string        `json:"entry_html,omitempty"` // Pre-rendered HTML from entry.
	ImagePath    *string        `json:"image_path,omitempty"`
	ParentID     *string        `json:"parent_id,omitempty"`
	TypeLabel    *string        `json:"type_label,omitempty"` // Freeform subtype (e.g., "City" for a Location).
	IsPrivate    bool           `json:"is_private"`
	IsTemplate   bool           `json:"is_template"`
	FieldsData   map[string]any `json:"fields_data"`
	CreatedBy    string         `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`

	// Joined fields from entity_types (populated by repository queries).
	TypeName  string `json:"type_name,omitempty"`
	TypeIcon  string `json:"type_icon,omitempty"`
	TypeColor string `json:"type_color,omitempty"`
	TypeSlug  string `json:"type_slug,omitempty"`
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateEntityRequest holds the data submitted by the entity creation form.
type CreateEntityRequest struct {
	Name         string `json:"name" form:"name"`
	EntityTypeID int    `json:"entity_type_id" form:"entity_type_id"`
	TypeLabel    string `json:"type_label" form:"type_label"`
	IsPrivate    bool   `json:"is_private" form:"is_private"`
}

// UpdateEntityRequest holds the data submitted by the entity edit form.
type UpdateEntityRequest struct {
	Name      string `json:"name" form:"name"`
	TypeLabel string `json:"type_label" form:"type_label"`
	IsPrivate bool   `json:"is_private" form:"is_private"`
	Entry     string `json:"entry" form:"entry"`
}

// --- Service Input DTOs ---

// CreateEntityInput is the validated input for creating an entity.
type CreateEntityInput struct {
	Name         string
	EntityTypeID int
	TypeLabel    string
	IsPrivate    bool
	FieldsData   map[string]any
}

// UpdateEntityInput is the validated input for updating an entity.
type UpdateEntityInput struct {
	Name       string
	TypeLabel  string
	IsPrivate  bool
	Entry      string
	ImagePath  string
	FieldsData map[string]any
}

// --- Pagination ---

// ListOptions holds pagination parameters for list queries.
type ListOptions struct {
	Page    int
	PerPage int
}

// DefaultListOptions returns sensible defaults for pagination.
func DefaultListOptions() ListOptions {
	return ListOptions{Page: 1, PerPage: 24}
}

// Offset returns the SQL OFFSET value for the current page.
func (o ListOptions) Offset() int {
	if o.Page < 1 {
		o.Page = 1
	}
	return (o.Page - 1) * o.PerPage
}

// --- Slug Generation ---

// slugPattern matches one or more non-alphanumeric characters for replacement.
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify creates a URL-safe slug from a name. Lowercase, replace
// non-alphanumeric characters with hyphens, trim leading/trailing hyphens.
func Slugify(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = slugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "entity"
	}
	return slug
}

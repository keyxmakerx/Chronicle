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

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Domain Models ---

// EntityType defines a category of entities within a campaign (e.g., Character,
// Location). Each campaign has its own set of entity types with configurable
// fields that drive dynamic form rendering and profile display.
type EntityType struct {
	ID              int               `json:"id"`
	CampaignID      string            `json:"campaign_id"`
	Slug            string            `json:"slug"`
	Name            string            `json:"name"`
	NamePlural      string            `json:"name_plural"`
	Icon            string            `json:"icon"`
	Color           string            `json:"color"`
	PresetCategory  *string           `json:"preset_category,omitempty"`   // System preset category ("character", "item", "creature").
	ParentTypeID    *int              `json:"parent_type_id,omitempty"`    // Parent entity type ID for sub-type hierarchy.
	Description     *string           `json:"description,omitempty"`       // Rich text shown on category dashboard.
	PinnedEntityIDs []string          `json:"pinned_entity_ids,omitempty"` // Entity IDs pinned to dashboard top.
	DashboardLayout *string           `json:"dashboard_layout,omitempty"`  // JSON layout; nil = use hardcoded default.
	Fields          []FieldDefinition `json:"fields"`
	Layout          EntityTypeLayout  `json:"layout"`
	SortOrder       int               `json:"sort_order"`
	IsDefault       bool              `json:"is_default"`
	Enabled         bool              `json:"enabled"`
	ParentTypeName  *string           `json:"parent_type_name,omitempty"`  // Joined field: parent type's name (not stored).
}

// ParseCategoryDashboardLayout parses the entity type's dashboard_layout JSON
// into a campaigns.DashboardLayout struct. Returns nil if the column is NULL
// (use hardcoded default category dashboard).
func (et *EntityType) ParseCategoryDashboardLayout() *campaigns.DashboardLayout {
	if et.DashboardLayout == nil || *et.DashboardLayout == "" {
		return nil
	}
	var layout campaigns.DashboardLayout
	if err := json.Unmarshal([]byte(*et.DashboardLayout), &layout); err != nil {
		return nil
	}
	return &layout
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
// "relations", "divider", "two_column", "three_column", "tabs", "section".
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
	ID             string          `json:"id"`
	CampaignID     string          `json:"campaign_id"`
	EntityTypeID   int             `json:"entity_type_id"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Entry            *string         `json:"entry,omitempty"`              // TipTap/ProseMirror JSON document.
	EntryHTML        *string         `json:"entry_html,omitempty"`         // Pre-rendered HTML from entry.
	PlayerNotes      *string         `json:"player_notes,omitempty"`       // Player-facing ProseMirror JSON (synced as a player-visible Foundry page).
	PlayerNotesHTML  *string         `json:"player_notes_html,omitempty"`  // Pre-rendered HTML from player_notes.
	ImagePath        *string         `json:"image_path,omitempty"`
	CoverImagePath *string         `json:"cover_image_path,omitempty"` // Full-width banner image.
	ParentID       *string         `json:"parent_id,omitempty"`       // Parent entity ID (hierarchy). Mutually exclusive with ParentNodeID.
	ParentNodeID   *string         `json:"parent_node_id,omitempty"` // Parent sidebar folder node ID. Mutually exclusive with ParentID.
	SortOrder      int             `json:"sort_order"`            // Manual ordering within parent/category (0 = default).
	TypeLabel      *string         `json:"type_label,omitempty"` // Freeform subtype (e.g., "City" for a Location).
	IsPrivate      bool            `json:"is_private"`
	Visibility     VisibilityMode  `json:"visibility"`
	IsTemplate     bool            `json:"is_template"`
	FieldsData     map[string]any  `json:"fields_data"`
	FieldOverrides *FieldOverrides `json:"field_overrides,omitempty"` // Per-entity field customizations.
	PopupConfig    *PopupConfig    `json:"popup_config,omitempty"`    // Controls hover tooltip content.
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`

	// Joined fields from entity_types (populated by repository queries).
	TypeName  string `json:"type_name,omitempty"`
	TypeIcon  string `json:"type_icon,omitempty"`
	TypeColor string `json:"type_color,omitempty"`
	TypeSlug  string `json:"type_slug,omitempty"`

	// Tags is populated at the handler level via batch fetch, not by the repository.
	Tags []EntityTagInfo `json:"tags,omitempty"`
}

// FieldOverrides holds per-entity field customizations that override the
// entity type's field template. This allows individual entities to add,
// hide, or modify fields without affecting the entire category.
type FieldOverrides struct {
	Added    []FieldDefinition        `json:"added,omitempty"`    // Extra fields unique to this entity.
	Hidden   []string                 `json:"hidden,omitempty"`   // Keys of type-level fields to hide.
	Modified map[string]FieldOverride `json:"modified,omitempty"` // Per-field modifications keyed by field key.
}

// FieldOverride holds modifications to a single field (label, type, options).
type FieldOverride struct {
	Label   *string  `json:"label,omitempty"`
	Type    *string  `json:"type,omitempty"`
	Options []string `json:"options,omitempty"`
}

// PopupConfig controls what appears in the entity hover preview tooltip.
// When nil, all available sections are shown (image, attributes, entry excerpt).
type PopupConfig struct {
	ShowImage      bool `json:"showImage"`
	ShowAttributes bool `json:"showAttributes"`
	ShowEntry      bool `json:"showEntry"`
}

// DefaultPopupConfig returns the default popup configuration showing everything.
func DefaultPopupConfig() *PopupConfig {
	return &PopupConfig{
		ShowImage:      true,
		ShowAttributes: true,
		ShowEntry:      true,
	}
}

// EffectivePopupConfig returns the entity's popup config, falling back to
// defaults if not configured.
func (e *Entity) EffectivePopupConfig() *PopupConfig {
	if e.PopupConfig != nil {
		return e.PopupConfig
	}
	return DefaultPopupConfig()
}

// MergeFields combines the entity type's field definitions with per-entity
// overrides to produce the effective field list for rendering. Hidden fields
// are removed, modified fields have their properties patched, and added fields
// are appended at the end.
func MergeFields(typeFields []FieldDefinition, overrides *FieldOverrides) []FieldDefinition {
	if overrides == nil {
		return typeFields
	}

	// Build hidden set.
	hiddenSet := make(map[string]bool, len(overrides.Hidden))
	for _, key := range overrides.Hidden {
		hiddenSet[key] = true
	}

	// Filter and apply modifications.
	result := make([]FieldDefinition, 0, len(typeFields))
	for _, f := range typeFields {
		if hiddenSet[f.Key] {
			continue
		}
		if mod, ok := overrides.Modified[f.Key]; ok {
			if mod.Label != nil {
				f.Label = *mod.Label
			}
			if mod.Type != nil {
				f.Type = *mod.Type
			}
			if mod.Options != nil {
				f.Options = mod.Options
			}
		}
		result = append(result, f)
	}

	// Append added fields.
	result = append(result, overrides.Added...)
	return result
}

// EntityTagInfo holds minimal tag display data for entity cards and lists.
// Avoids importing the tags widget package from the entities plugin.
type EntityTagInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateEntityRequest holds the data submitted by the entity creation form.
type CreateEntityRequest struct {
	Name         string `json:"name" form:"name"`
	EntityTypeID int    `json:"entity_type_id" form:"entity_type_id"`
	TypeLabel    string `json:"type_label" form:"type_label"`
	ParentID     string `json:"parent_id" form:"parent_id"`
	IsPrivate    bool   `json:"is_private" form:"is_private"`
	TemplateID   int    `json:"template_id" form:"template_id"` // Optional content template to pre-fill.
}

// UpdateEntityRequest holds the data submitted by the entity edit form.
type UpdateEntityRequest struct {
	Name              string     `json:"name" form:"name"`
	TypeLabel         string     `json:"type_label" form:"type_label"`
	ParentID          string     `json:"parent_id" form:"parent_id"`
	IsPrivate         bool       `json:"is_private" form:"is_private"`
	Entry             string     `json:"entry" form:"entry"`
	ExpectedUpdatedAt *time.Time `json:"expected_updated_at"` // Optimistic concurrency (optional, JSON API only).
}

// --- Service Input DTOs ---

// CreateEntityInput is the validated input for creating an entity.
type CreateEntityInput struct {
	Name         string
	EntityTypeID int
	TypeLabel    string
	ParentID     string // Empty string = no parent.
	IsPrivate    bool
	FieldsData   map[string]any
}

// UpdateEntityInput is the validated input for updating an entity.
type UpdateEntityInput struct {
	Name              string
	TypeLabel         string
	ParentID          string // Empty string = clear parent.
	IsPrivate         bool
	Entry             string
	PlayerNotes       *string // Player-facing content (nil = don't change).
	ImagePath         string
	FieldsData        map[string]any
	ExpectedUpdatedAt *time.Time // Optimistic concurrency: reject if entity was modified after this timestamp.
}

// --- Pagination ---

// ListOptions holds pagination and sorting parameters for list queries.
type ListOptions struct {
	Page     int
	PerPage  int
	Sort     string   // "name" (default), "updated", "created"
	TagSlugs []string // Filter by tag slugs (AND logic — entity must have all listed tags).
}

// DefaultListOptions returns sensible defaults for pagination.
func DefaultListOptions() ListOptions {
	return ListOptions{Page: 1, PerPage: 24, Sort: "name"}
}

// OrderByClause returns a safe SQL ORDER BY clause based on the Sort field.
func (o ListOptions) OrderByClause() string {
	switch o.Sort {
	case "updated":
		return "ORDER BY e.updated_at DESC"
	case "created":
		return "ORDER BY e.created_at DESC"
	case "manual":
		return "ORDER BY e.sort_order ASC, e.name ASC"
	default:
		return "ORDER BY e.name ASC"
	}
}

// Offset returns the SQL OFFSET value for the current page.
func (o ListOptions) Offset() int {
	if o.Page < 1 {
		o.Page = 1
	}
	return (o.Page - 1) * o.PerPage
}

// --- Entity Type Request DTOs ---

// CreateEntityTypeRequest holds the data submitted by the entity type creation form.
type CreateEntityTypeRequest struct {
	Name         string `json:"name" form:"name"`
	NamePlural   string `json:"name_plural" form:"name_plural"`
	Icon         string `json:"icon" form:"icon"`
	Color        string `json:"color" form:"color"`
	ParentTypeID *int   `json:"parent_type_id" form:"parent_type_id"`
}

// UpdateEntityTypeRequest holds the data submitted by the entity type edit form.
type UpdateEntityTypeRequest struct {
	Name       string            `json:"name" form:"name"`
	NamePlural string            `json:"name_plural" form:"name_plural"`
	Icon       string            `json:"icon" form:"icon"`
	Color      string            `json:"color" form:"color"`
	Fields     []FieldDefinition `json:"fields"`
}

// --- Entity Type Service Input DTOs ---

// CreateEntityTypeInput is the validated input for creating an entity type.
type CreateEntityTypeInput struct {
	Name           string
	NamePlural     string
	Icon           string
	Color          string
	PresetCategory string // Optional system preset category (e.g., "item", "character").
	ParentTypeID   *int   // Optional parent type for sub-type hierarchy.
}

// UpdateEntityTypeInput is the validated input for updating an entity type.
type UpdateEntityTypeInput struct {
	Name       string
	NamePlural string
	Icon       string
	Color      string
	Fields     []FieldDefinition
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

// --- Per-Entity Permissions ---

// VisibilityMode indicates how an entity's access is determined.
// "default" uses the legacy is_private flag; "custom" uses entity_permissions.
type VisibilityMode string

const (
	// VisibilityDefault uses the legacy is_private flag for access control.
	VisibilityDefault VisibilityMode = "default"
	// VisibilityCustom uses the entity_permissions table for fine-grained access.
	VisibilityCustom VisibilityMode = "custom"
)

// SubjectType identifies what kind of subject holds a permission grant.
type SubjectType string

const (
	// SubjectRole grants access to all members at or above a campaign role level.
	SubjectRole SubjectType = "role"
	// SubjectUser grants access to a specific user by ID.
	SubjectUser SubjectType = "user"
	// SubjectGroup grants access to all members of a campaign group.
	SubjectGroup SubjectType = "group"
)

// Permission represents an access level that can be granted on an entity.
type Permission string

const (
	// PermView allows the subject to see the entity.
	PermView Permission = "view"
	// PermEdit allows the subject to see and modify the entity.
	PermEdit Permission = "edit"
)

// EntityPermission is a single access grant on an entity.
type EntityPermission struct {
	ID          int         `json:"id"`
	EntityID    string      `json:"entity_id"`
	SubjectType SubjectType `json:"subject_type"`
	SubjectID   string      `json:"subject_id"` // Role level as string ("1","2","3") or user UUID.
	Permission  Permission  `json:"permission"`
	CreatedAt   time.Time   `json:"created_at"`
}

// EffectivePermission is the resolved access level for a specific user on
// a specific entity after merging all applicable grants (role-based, user-based).
type EffectivePermission struct {
	CanView bool
	CanEdit bool
}

// SetPermissionsInput is the validated input for setting entity permissions.
// Replaces all existing grants for the entity.
type SetPermissionsInput struct {
	Visibility  VisibilityMode     `json:"visibility"`
	IsPrivate   bool               `json:"is_private"`   // Used when visibility=default.
	Permissions []PermissionGrant  `json:"permissions"`   // Used when visibility=custom.
}

// PermissionGrant is a single grant in a SetPermissionsInput request.
type PermissionGrant struct {
	SubjectType SubjectType `json:"subject_type"`
	SubjectID   string      `json:"subject_id"`
	Permission  Permission  `json:"permission"`
}

// ValidSubjectType returns true if s is a recognized subject type.
func ValidSubjectType(s SubjectType) bool {
	return s == SubjectRole || s == SubjectUser || s == SubjectGroup
}

// ValidPermission returns true if p is a recognized permission level.
func ValidPermission(p Permission) bool {
	return p == PermView || p == PermEdit
}

// --- Auto-Linking ---

// EntityNameEntry is a lightweight entity record for auto-linking.
// Contains just enough data to detect entity names in editor text and
// create links. Sorted by name length descending so longer names match first.
// Alias entries appear as separate rows with IsAlias=true, pointing to the
// same entity ID so the auto-linker matches them naturally.
type EntityNameEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	TypeName string `json:"type_name"`
	TypeIcon string `json:"type_icon"`
	TypeSlug string `json:"type_slug"`
	IsAlias  bool   `json:"is_alias,omitempty"`
}

// --- Entity Aliases ---

// EntityAlias represents an alternative name for an entity. Aliases appear
// in auto-linking, search, and @mention results alongside the primary name.
type EntityAlias struct {
	ID        int       `json:"id"`
	EntityID  string    `json:"entity_id"`
	Alias     string    `json:"alias"`
	CreatedAt time.Time `json:"created_at"`
}

// SetAliasesInput is the request body for replacing an entity's aliases.
type SetAliasesInput struct {
	Aliases []string `json:"aliases"`
}

// MaxAliasesPerEntity is the maximum number of aliases allowed per entity.
const MaxAliasesPerEntity = 10

// MinAliasLength is the minimum character length for an alias.
const MinAliasLength = 2

// MaxAliasLength is the maximum character length for an alias.
const MaxAliasLength = 200

// --- Backlinks ---

// BacklinkEntry pairs an entity that references the current entity with
// a text snippet showing the context around the @mention.
type BacklinkEntry struct {
	Entity  Entity `json:"entity"`
	Snippet string `json:"snippet"`
}

// --- Mention Links (for graph visualization) ---

// MentionLink represents a directional @mention reference from one entity to
// another, extracted from entry_html. Used by the relations graph to show
// mention-based edges alongside explicit relations.
type MentionLink struct {
	SourceEntityID string `json:"sourceEntityId"`
	TargetEntityID string `json:"targetEntityId"`
}

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

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/patch"
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
	Claimable       *bool             `json:"claimable,omitempty"`         // nil = unset (legacy heuristic); true/false = explicit Owner choice for player claiming.
	Description     *string           `json:"description,omitempty"`       // Rich text shown on category dashboard.
	PinnedEntityIDs []string          `json:"pinned_entity_ids,omitempty"` // Entity IDs pinned to dashboard top.
	DashboardLayout *string           `json:"dashboard_layout,omitempty"`  // JSON layout; nil = use hardcoded default.
	Fields          []FieldDefinition `json:"fields"`
	Layout          EntityTypeLayout  `json:"layout"`
	SortOrder       int               `json:"sort_order"`
	IsDefault       bool              `json:"is_default"`
	Enabled         bool              `json:"enabled"`
	ParentTypeName  *string           `json:"parent_type_name,omitempty"` // Joined field: parent type's name (not stored).
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

// CharacterLayout is the default page layout for player-character types: the
// dynamic character-sheet surface (the "big widget") full-width, then the
// player-notes block. Owners can customize it in the layout editor like any
// other layout. Visibility editing lives only in edit mode via form.templ's
// inline widget, never an auto-appended read-page row (ADR-057 decision 5).
func CharacterLayout() EntityTypeLayout {
	return EntityTypeLayout{
		Rows: []TemplateRow{
			{
				ID: "row-surface",
				Columns: []TemplateColumn{
					{
						ID:    "col-surface",
						Width: 12,
						Blocks: []TemplateBlock{
							{ID: "blk-character-surface", Type: "character_surface"},
						},
					},
				},
			},
			entityNotesRow("row-notes", "col-notes", "blk-notes"),
		},
	}
}

// DefaultLayout returns the standard two-column layout used for new entity types.
// It holds only the page's own content: player notes and the other page
// pieces are blocks the owner adds (only character pages start with player
// notes, in CharacterLayout). Visibility editing lives only in edit mode, not
// an auto-appended read-page row (ADR-057 decision 5).
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

// entityNotesRow builds a full-width row holding the player-notes
// (entity_notes) block for CharacterLayout. The block is gated by the
// "player-notes" addon at render time, so it renders nothing when the addon
// is off.
func entityNotesRow(rowID, colID, blockID string) TemplateRow {
	return TemplateRow{
		ID: rowID,
		Columns: []TemplateColumn{
			{
				ID:    colID,
				Width: 12,
				Blocks: []TemplateBlock{
					{ID: blockID, Type: "entity_notes"},
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
	if err := json.Unmarshal(raw, &layout); err == nil {
		if len(layout.Rows) > 0 {
			return layout
		}
		// Check if JSON explicitly has a "rows" key (new format, just empty).
		// Respect it rather than silently replacing with default layout.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err == nil {
			if _, hasRows := probe["rows"]; hasRows {
				return layout
			}
		}
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
	// GMOnly marks the field's VALUE as GM-only: it is stripped from
	// fields_data in entity API responses to non-GM (player / public)
	// callers, so GM secrets never reach a player's browser (server is the
	// authority; a client-side hide is not a fix). Absent/false =
	// player-visible. Populated from a system manifest's gm_only annotation
	// via preset application + EnsureFieldMetadataFromManifests.
	GMOnly bool `json:"gm_only,omitempty"`
	// OwnerOnly marks the field's VALUE as visible only to GM-tier roles
	// (same bar as GMOnly) AND the entity's own claimed owner
	// (Entity.OwnerUserID): it is stripped from fields_data for every other
	// Player-tier viewer, including fellow players who are not this entity's
	// owner. Unlike GMOnly (hidden even from the owner), OwnerOnly is for
	// content private between one player and the GM but not party-wide.
	OwnerOnly bool `json:"owner_only,omitempty"`
	// Choices names the system data file the field is picked from. Absent:
	// the picker falls back to the plural of the field key. The stored value
	// is still the picked entry's name as plain text.
	Choices string `json:"choices,omitempty"`
	// Play carries the system manifest's play-controls declaration for this
	// field, so whatever acts on play edits reads one stored source and core
	// never hard-codes a system's field names. Nil = not a play field.
	Play *FieldPlay `json:"play,omitempty"`
}

// FieldPlay mirrors the manifest's play block. It is a separate type from the
// systems package's because plugins do not import each other; the app layer
// copies between them.
type FieldPlay struct {
	Edit            string   `json:"edit"`                       // owner | gm | none.
	Kind            string   `json:"kind"`                       // counter | resource | conditions | choice | text.
	Min             *float64 `json:"min,omitempty"`              // Lower bound for counter/resource.
	Max             *float64 `json:"max,omitempty"`              // Literal upper bound.
	MaxField        string   `json:"max_field,omitempty"`        // Another number field whose value is the upper bound; wins over Max.
	Step            float64  `json:"step,omitempty"`             // UI nudge size.
	Options         []string `json:"options,omitempty"`          // Allowed values for conditions/choice.
	MaxLength       int      `json:"max_length,omitempty"`       // Bound for text/choice.
	ToFoundry       *bool    `json:"to_foundry,omitempty"`       // nil = follow the field's foundry_writable.
	CombatAuthority string   `json:"combat_authority,omitempty"` // foundry | chronicle | empty.
}

// Entity represents a single worldbuilding object — a character, location,
// item, or any other type defined in the campaign's entity types.
type Entity struct {
	ID              string          `json:"id"`
	CampaignID      string          `json:"campaign_id"`
	EntityTypeID    int             `json:"entity_type_id"`
	Name            string          `json:"name"`
	Slug            string          `json:"slug"`
	Entry           *string         `json:"entry,omitempty"`             // TipTap/ProseMirror JSON document.
	EntryHTML       *string         `json:"entry_html,omitempty"`        // Pre-rendered HTML from entry.
	PlayerNotes     *string         `json:"player_notes,omitempty"`      // Player-facing ProseMirror JSON (synced as a player-visible Foundry page).
	PlayerNotesHTML *string         `json:"player_notes_html,omitempty"` // Pre-rendered HTML from player_notes.
	ImagePath       *string         `json:"image_path,omitempty"`
	CoverImagePath  *string         `json:"cover_image_path,omitempty"` // Full-width banner image.
	ParentID        *string         `json:"parent_id,omitempty"`        // Parent entity ID (hierarchy). Mutually exclusive with ParentNodeID.
	ParentNodeID    *string         `json:"parent_node_id,omitempty"`   // Parent sidebar folder node ID. Mutually exclusive with ParentID.
	SortOrder       int             `json:"sort_order"`                 // Manual ordering within parent/category (0 = default).
	TypeLabel       *string         `json:"type_label,omitempty"`       // Freeform subtype (e.g., "City" for a Location).
	IsPrivate       bool            `json:"is_private"`
	Visibility      VisibilityMode  `json:"visibility"`
	IsTemplate      bool            `json:"is_template"`
	FieldsData      map[string]any  `json:"fields_data"`
	FieldOverrides  *FieldOverrides `json:"field_overrides,omitempty"` // Per-entity field customizations.
	PopupConfig     *PopupConfig    `json:"popup_config,omitempty"`    // Controls hover tooltip content.
	CreatedBy       string          `json:"created_by"`
	// OwnerUserID claims an entity for a player. Nullable: most entities
	// (locations, factions, lore) are not owned. Character-shaped entities
	// surface on the owner's "My Characters" landing page. Set by Foundry
	// sync (when the actor's owner maps to a chronicle user) or by the
	// player claim flow on entity show.
	OwnerUserID *string `json:"owner_user_id,omitempty"`
	// MapID points the entity at one of the campaign's maps and powers
	// the per-entity Map Editor block. Nullable — most entities aren't
	// "a map." FK enforces same-campaign integrity (added by the maps
	// plugin migration 005); ON DELETE SET NULL handles map deletion
	// gracefully (entity falls back to the picker / empty state).
	MapID     *string   `json:"map_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Joined fields from entity_types (populated by repository queries).
	TypeName       string `json:"type_name,omitempty"`
	TypeNamePlural string `json:"type_name_plural,omitempty"` // Joined entity_types.name_plural — used by breadcrumbs and category links.
	TypeIcon       string `json:"type_icon,omitempty"`
	TypeColor      string `json:"type_color,omitempty"`
	TypeSlug       string `json:"type_slug,omitempty"`

	// Tags is populated at the handler level via batch fetch, not by the repository.
	Tags []EntityTagInfo `json:"tags,omitempty"`
}

// IsOwnedBy reports whether userID is this entity's claimed owner (used to
// gate OwnerOnly field VALUES — see FilterRestrictedFields). False for an
// unclaimed entity, an empty/anonymous userID, or a nil receiver (so callers
// don't need a separate nil-entity check before asking).
func (e *Entity) IsOwnedBy(userID string) bool {
	return e != nil && userID != "" && e.OwnerUserID != nil && *e.OwnerUserID == userID
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
//
// is_private does not ride on this form: the permissions slide-in card owns
// that field and writes directly via /permissions. See
// UpdateEntityInput.IsPrivate.
type UpdateEntityRequest struct {
	Name              string     `json:"name" form:"name"`
	TypeLabel         string     `json:"type_label" form:"type_label"`
	ParentID          string     `json:"parent_id" form:"parent_id"`
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
	// OwnerUserID claims the entity for a player at create time. Optional;
	// nil means unclaimed. Only honored if the user is a member of the
	// target campaign — the service rejects cross-campaign assignments.
	// Foundry sync uses this to auto-claim character entities to the
	// chronicle user mapped from the Foundry actor's owner.
	OwnerUserID *string
}

// UpdateEntityInput is the validated input for updating an entity: a
// PARTIAL update where an absent field preserves, explicit null clears,
// and a present value replaces. ParentID/TypeLabel use patch.Field for
// this — a plain string with "" meaning "clear" can't express "absent"
// and silently detaches entities on a partial caller that omits it.
//
// IsPrivate stays a plain *bool (nil = don't change, non-nil = set) on the
// same three-state contract; the permissions widget is the sole
// authoritative writer, other handlers must not send it.
//
// Pictures are not set here: UpdateImage and UpdateCoverImage are their
// only writers, and they check the file belongs to the page's campaign.
type UpdateEntityInput struct {
	Name              patch.Field[string]
	TypeLabel         patch.Field[string] // absent = preserve; "" or null = clear.
	ParentID          patch.Field[string] // absent = preserve; "" or null = clear.
	IsPrivate         *bool               // nil = preserve current; non-nil = set to *IsPrivate.
	Entry             patch.Field[string] // absent or "" = preserve; null = clear (see service.Update).
	PlayerNotes       *string             // Player-facing content (nil = don't change).
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

	// PrivateOnly keeps only entities flagged is_private, so a caller that
	// wants the hidden ones (the DM Screen's reveal list) is not limited to
	// whatever happens to sit in the newest page of a mixed listing.
	PrivateOnly bool
}

// DefaultListOptions returns sensible defaults for pagination.
func DefaultListOptions() ListOptions {
	return ListOptions{Page: 1, PerPage: 24, Sort: "name"}
}

// OrderByClause returns a safe SQL ORDER BY clause based on the Sort field.
//
// Every branch ends with `e.id ASC`. None of the leading sort columns is
// unique, so without a tiebreaker MariaDB's plan can disagree between
// statements of one LIMIT/OFFSET walk about which tied rows land in a given
// window, causing pages to skip or repeat rows. The primary key makes each
// sort a total order, which OFFSET paging requires.
func (o ListOptions) OrderByClause() string {
	switch o.Sort {
	case "updated":
		return "ORDER BY e.updated_at DESC, e.id ASC"
	case "created":
		return "ORDER BY e.created_at DESC, e.id ASC"
	case "manual":
		return "ORDER BY e.sort_order ASC, e.name ASC, e.id ASC"
	default:
		return "ORDER BY e.name ASC, e.id ASC"
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
	Claimable    *bool  `json:"claimable,omitempty" form:"claimable"`
}

// UpdateEntityTypeRequest is the JSON body of PUT .../entity-types/:etid. It is
// a PARTIAL update: an absent key keeps the stored value, so a rename or a
// color change cannot reset the plural name or icon. Every column here is NOT
// NULL, so an explicit null also preserves.
type UpdateEntityTypeRequest struct {
	Name         patch.Field[string] `json:"name"`
	NamePlural   patch.Field[string] `json:"name_plural"`
	Icon         patch.Field[string] `json:"icon"`
	Color        patch.Field[string] `json:"color"`
	Fields       []FieldDefinition   `json:"fields"`
	ParentTypeID *int                `json:"parent_type_id"`      // New parent (nil = no change).
	ClearParent  bool                `json:"clear_parent"`        // Explicitly remove parent (make top-level).
	Claimable    *bool               `json:"claimable,omitempty"` // nil = no change; true/false = set the player-claim flag.
}

// --- Entity Type Service Input DTOs ---

// CreateEntityTypeInput is the validated input for creating an entity type.
type CreateEntityTypeInput struct {
	Name           string
	NamePlural     string
	Icon           string
	Color          string
	PresetCategory string            // Optional system preset category (e.g., "item", "character").
	ParentTypeID   *int              // Optional parent type for sub-type hierarchy.
	Claimable      *bool             // Optional explicit player-claim flag. nil = unset (service may default it for PC types).
	Fields         []FieldDefinition // Optional initial field schema (e.g. from a system preset). nil = none.
}

// UpdateEntityTypeInput is the validated input for updating an entity type.
type UpdateEntityTypeInput struct {
	Name         patch.Field[string]
	NamePlural   patch.Field[string]
	Icon         patch.Field[string]
	Color        patch.Field[string]
	Fields       []FieldDefinition
	ParentTypeID *int  // New parent type (nil = no change).
	ClearParent  bool  // Explicitly remove parent (make top-level).
	Claimable    *bool // New player-claim flag (nil = no change).
}

// PCSetupSnapshot is a read-only summary of a campaign's player-character
// category state, assembled by PlayerCharacterSetupSnapshot for the
// player-character extension settings page. Owner-facing — entity counts
// include all entities regardless of visibility.
type PCSetupSnapshot struct {
	// DefaultCharsParentID is the default top-level "Characters" category id, if present.
	DefaultCharsParentID *int
	// GenericPCTypes are generic premade player_character types (preset_category
	// "player_character" or slug "player-character"). Normally 0 or 1; >1 = ambiguous.
	GenericPCTypes []EntityType
	// SystemCharTypes are a game system's own claimable character types
	// (e.g. drawsteel-character). Normally 0 or 1; >1 = ambiguous.
	SystemCharTypes []EntityType
	// GenericPCCount / SystemCharCount are total entity counts across those types.
	GenericPCCount  int
	SystemCharCount int
	// SubCategoryCount is the number of sub-categories nested under "Characters".
	SubCategoryCount int
}

// MergeResult reports the outcome of MergeDuplicatePlayerCharacterType.
type MergeResult struct {
	Moved         int    // entities reassigned from the generic onto the system type
	RemovedTypeID int    // the deleted generic type's id (0 if nothing removed)
	TargetTypeID  int    // the surviving system character type's id (0 if no merge)
	TargetName    string // the surviving type's display name (e.g. "Heroes")
	NoOp          bool   // true when there was nothing to reconcile
}

// CharacterHomeMove describes moving a system character type's pages into
// "Characters": pending (from CharacterHomeCandidate) or done.
type CharacterHomeMove struct {
	FromTypeID int
	FromName   string // the system type's plural name, e.g. "Heroes"
	ToTypeID   int
	ToName     string // "Characters", or whatever the owner renamed it to
	Pages      int    // pages to move, or moved (Trash included)
	NoOp       bool   // nothing to move
}

// ClaimRoster carries the GM owner-overview data for a claimable category
// dashboard. It is assembled by the Index handler only for a Scribe+ viewer
// of a claimable entity type with the Player Character Claiming addon
// enabled; nil otherwise, in which case the roster panel is not rendered.
// Characters lists every entity of the category (not just the paginated
// dashboard page); Members is the set of assignable owners for the reassign
// dropdown; OwnerNames resolves an entity's owner_user_id to a display name.
type ClaimRoster struct {
	Characters []Entity                   // All characters of the category, by name.
	Members    []campaigns.CampaignMember // Campaign members assignable as owners.
	OwnerNames map[string]string          // owner_user_id -> display name.
}

// CastMember is one card on the Characters ("Cast") page: an entity plus the
// display flags the card needs. OwnerName is the resolved display name of a
// player character's owner (empty for NPCs); IsViewer marks the card as
// belonging to the viewing user so it can be highlighted and sorted first.
type CastMember struct {
	Entity    Entity
	OwnerName string
	IsViewer  bool
}

// CastView is the Characters page view-model. The page has addon-gated bands:
// Yours (the viewer's own characters), the Party (claimed player characters of
// the types the owner listed, shown when ShowPlayers, i.e. the
// player-character-claiming addon is on) and the NPCs band (NPCSection, a
// templ component contributed by the npcs plugin when its addon is on, nil
// otherwise). The page itself is only served when at least one of the two
// addons is enabled.
type CastView struct {
	Yours       []CastMember
	Party       []CastMember
	ShowPlayers bool
	// IsMember is false for signed-out and non-member viewers of a public
	// campaign; member-only links (claim hint) hide for them.
	IsMember bool
	// ClaimTypeID is the claimable type whose list the empty-party hint links
	// to; 0 means no link.
	ClaimTypeID int
	NPCSection  templ.Component

	// ShowNPCs is true when the npcs addon is on and its section is wired.
	ShowNPCs bool
	// PartyListed and NPCsListed say whether the owner has listed any page
	// type for that band. A band with none is hidden from everyone but the
	// owner, who sees the Add control instead.
	PartyListed bool
	NPCsListed  bool
	// PartyEditor and NPCEditor are the owner's controls; nil for everyone
	// else, so a player's page carries no controls at all.
	PartyEditor *CastBandEditor
	NPCEditor   *CastBandEditor
	// Landed is the type just added, flashed once on its chip.
	Landed int
	// Notice is a one-line confirmation shown as a toast after a change.
	Notice    string
	CSRFToken string
	// CanCreateHero shows the Create hero button: staff always, a player when
	// the owner lets players claim the hero's page type.
	CanCreateHero bool
	// CanManageEntries shows the Directors their "Your own entries" link.
	CanManageEntries bool
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
	Visibility  VisibilityMode    `json:"visibility"`
	IsPrivate   bool              `json:"is_private"`  // Used when visibility=default.
	Permissions []PermissionGrant `json:"permissions"` // Used when visibility=custom.
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

// BacklinkEntity is the safe, minimal view of a linking entity exposed by the
// backlinks API — enough to link to it and render its type icon, never its
// content. Unlike Entity, this type has no Entry/EntryHTML/FieldsData fields
// to accidentally serialize, so a new field added to Entity can't leak here.
type BacklinkEntity struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TypeName  string `json:"type_name"`
	TypeIcon  string `json:"type_icon"`
	TypeColor string `json:"type_color"`
}

// BacklinkEntry pairs a linking entity's safe summary with a text snippet
// showing the context around the @mention. For viewers below Scribe the
// snippet is built from GM-secret-stripped HTML (see GetBacklinksWithSnippets).
type BacklinkEntry struct {
	Entity  BacklinkEntity `json:"entity"`
	Snippet string         `json:"snippet"`
}

// --- Mention Links (for graph visualization) ---

// MentionLink represents a directional @mention reference from one entity to
// another, extracted from entry_html. Used by the relations graph to show
// mention-based edges alongside explicit relations.
type MentionLink struct {
	SourceEntityID string `json:"sourceEntityId"`
	TargetEntityID string `json:"targetEntityId"`
}

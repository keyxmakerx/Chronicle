// Package armory provides a gallery/hub view for item-type entities in a
// campaign. Items are entity types with preset_category "item" — their fields
// come from system manifests (D&D items, PF2e equipment, Draw Steel kits).
// This plugin is a view layer on top of the entities system — it does not
// own any database tables beyond the preset_category column on entity_types.
package armory

import "time"

// ItemListOptions controls filtering and pagination for the Armory gallery.
type ItemListOptions struct {
	Page       int    // 1-indexed page number.
	PerPage    int    // Items per page (default 24).
	Sort       string // "name" (default), "updated", "created".
	Search     string // Optional name search (prefix match).
	Tag        string // Optional tag slug filter.
	TypeID     int    // Optional entity type ID filter (0 = all item types).
	InstanceID int    // Optional inventory instance filter (0 = all items).
}

// DefaultItemListOptions returns sensible defaults for the Armory gallery.
func DefaultItemListOptions() ItemListOptions {
	return ItemListOptions{Page: 1, PerPage: 24, Sort: "name"}
}

// Offset returns the SQL offset for the current page.
func (o ItemListOptions) Offset() int {
	if o.Page < 1 {
		return 0
	}
	return (o.Page - 1) * o.PerPage
}

// OrderByClause returns a safe SQL ORDER BY clause based on the Sort field.
func (o ItemListOptions) OrderByClause() string {
	switch o.Sort {
	case "updated":
		return "ORDER BY e.updated_at DESC"
	case "created":
		return "ORDER BY e.created_at DESC"
	default:
		return "ORDER BY e.name ASC"
	}
}

// ItemCard is a lightweight view model for the Armory gallery grid.
// Fields are sourced from the entities table with joined entity_type info.
type ItemCard struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Slug      string         `json:"slug"`
	ImagePath *string        `json:"image_path,omitempty"`
	TypeLabel *string        `json:"type_label,omitempty"` // Freeform subtype (e.g., "Weapon", "Potion").
	TypeName  string         `json:"type_name"`
	TypeIcon  string         `json:"type_icon"`
	TypeColor string         `json:"type_color"`
	Fields    map[string]any `json:"fields_data"`
	IsPrivate bool           `json:"is_private"`
	Tags      []ItemTagInfo  `json:"tags,omitempty"`
}

// ItemTagInfo holds tag display info for item cards.
type ItemTagInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Color string `json:"color"`
}

// FieldString returns a string field value from the item's fields_data,
// or empty string if the key is missing or not a string.
func (c *ItemCard) FieldString(key string) string {
	if c.Fields == nil {
		return ""
	}
	v, ok := c.Fields[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// ItemTypeInfo holds basic info about an item-category entity type for
// the type filter dropdown.
type ItemTypeInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// InventoryInstance is a named item collection within a campaign.
// Standalone — not tied to a specific entity.
type InventoryInstance struct {
	ID          int       `json:"id"`
	CampaignID  string    `json:"campaign_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	SortOrder   int       `json:"sort_order"`
	ItemCount   int       `json:"item_count"` // Populated by COUNT join.
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateInstanceInput is the request payload for creating/updating an instance.
type CreateInstanceInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
}

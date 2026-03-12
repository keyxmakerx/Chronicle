// Package npcs provides a gallery/hub view for revealed NPCs in a campaign.
// NPCs are character-type entities that the DM has made visible to players
// ("revealed"). This plugin is a view layer on top of the entities system —
// it does not own any database tables.
package npcs

// NPCListOptions controls filtering and pagination for the NPC gallery.
type NPCListOptions struct {
	Page    int    // 1-indexed page number.
	PerPage int    // Items per page (default 24).
	Sort    string // "name" (default), "updated", "created".
	Search  string // Optional name search (prefix match).
	Tag     string // Optional tag slug filter.
}

// DefaultNPCListOptions returns sensible defaults for the NPC gallery.
func DefaultNPCListOptions() NPCListOptions {
	return NPCListOptions{Page: 1, PerPage: 24, Sort: "name"}
}

// Offset returns the SQL offset for the current page.
func (o NPCListOptions) Offset() int {
	if o.Page < 1 {
		return 0
	}
	return (o.Page - 1) * o.PerPage
}

// OrderByClause returns a safe SQL ORDER BY clause based on the Sort field.
func (o NPCListOptions) OrderByClause() string {
	switch o.Sort {
	case "updated":
		return "ORDER BY e.updated_at DESC"
	case "created":
		return "ORDER BY e.created_at DESC"
	default:
		return "ORDER BY e.name ASC"
	}
}

// NPCCard is a lightweight view model for the NPC gallery grid.
// Fields are sourced from the entities table with joined entity_type info.
type NPCCard struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Slug      string         `json:"slug"`
	ImagePath *string        `json:"image_path,omitempty"`
	TypeLabel *string        `json:"type_label,omitempty"` // Freeform subtype (e.g., "Innkeeper", "Guard").
	TypeName  string         `json:"type_name"`
	TypeIcon  string         `json:"type_icon"`
	TypeColor string         `json:"type_color"`
	Fields    map[string]any `json:"fields_data"`
	IsPrivate bool           `json:"is_private"`
	Tags      []NPCTagInfo  `json:"tags,omitempty"`
}

// NPCTagInfo holds tag display info for NPC cards.
type NPCTagInfo struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Color string `json:"color"`
}

// FieldString returns a string field value from the NPC's fields_data,
// or empty string if the key is missing or not a string.
func (c *NPCCard) FieldString(key string) string {
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

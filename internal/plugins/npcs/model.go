// Package npcs provides a gallery/hub view for revealed NPCs in a campaign.
// NPCs are character-type entities that the DM has made visible to players
// ("revealed"). This plugin is a view layer on top of the entities system —
// it does not own any database tables.
package npcs

import (
	"strconv"
	"strings"
)

// NPCListOptions controls filtering and pagination for the NPC gallery.
type NPCListOptions struct {
	Page    int    // 1-indexed page number.
	PerPage int    // Items per page (default 24).
	Sort    string // "name" (default), "updated", "created".
	Search  string // Optional name search (prefix match).
	Tag     string // Optional tag slug filter.

	// IncludeDmOnlyTags lets GM-only tags decorate cards. Off by default so a
	// player-facing list can never carry a dm_only tag name.
	IncludeDmOnlyTags bool
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

// NPCPageSize is the page size of the Characters page NPC list; the "Show more"
// button loads one more page of this size.
const NPCPageSize = 60

// NPCPager is the "Showing X of N" state of the cumulative NPC list: pages
// 1..Page have been loaded, so Shown is everything up to the end of Page.
type NPCPager struct {
	Shown    int
	Total    int
	HasMore  bool
	NextPage int
	// NextCount is how many cards the next click adds (the last page is short).
	NextCount int
}

// NewNPCPager derives the pager from the requested page and the filtered total.
// Total comes from the visibility-narrowed set, so the footer never discloses
// hidden NPCs.
func NewNPCPager(page, perPage, total int) NPCPager {
	if page < 1 {
		page = 1
	}
	shown := page * perPage
	if shown > total {
		shown = total
	}
	p := NPCPager{Shown: shown, Total: total}
	if remaining := total - shown; remaining > 0 {
		p.HasMore = true
		p.NextPage = page + 1
		p.NextCount = remaining
		if p.NextCount > perPage {
			p.NextCount = perPage
		}
	}
	return p
}

// NPCListView is everything the NPC list part renders: one page of cards plus
// the filter state, so the fragment endpoint and the initial section share it.
type NPCListView struct {
	Cards  []NPCCard
	Pager  NPCPager
	Search string
	Tag    string
	// Tags are the filter options (tags present on NPCs the viewer can see).
	Tags []NPCTagInfo
}

// Filtered reports whether a search or tag narrows the list.
func (v NPCListView) Filtered() bool { return v.Search != "" || v.Tag != "" }

// maxNPCSearchLen bounds the search term so a pasted blob can't become a huge LIKE.
const maxNPCSearchLen = 100

// maxNPCPage bounds the page parameter; 60 x 1000 is far past any real roster.
const maxNPCPage = 1000

// ParseNPCSectionQuery normalises the section endpoint's raw query values.
// tag is only honoured when it is one of allowedTags (the options this viewer
// is offered), so a hand-typed slug can't probe for GM-only tags.
func ParseNPCSectionQuery(q, tag, page string, allowedTags []NPCTagInfo) (search, tagSlug string, pageNum int) {
	search = strings.TrimSpace(q)
	if r := []rune(search); len(r) > maxNPCSearchLen {
		search = string(r[:maxNPCSearchLen])
	}
	for _, t := range allowedTags {
		if t.Slug == tag {
			tagSlug = tag
			break
		}
	}
	pageNum = 1
	if n, err := strconv.Atoi(page); err == nil && n >= 1 {
		pageNum = n
		if pageNum > maxNPCPage {
			pageNum = maxNPCPage
		}
	}
	return search, tagSlug, pageNum
}

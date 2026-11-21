// Package tags implements the tags widget for Chronicle. Tags are campaign-scoped
// labels that can be attached to entities for categorization and filtering.
// This widget provides CRUD operations for tags and the ability to associate
// tags with entities via a many-to-many join table (entity_tags).
//
// Tags are a Widget in Chronicle's three-tier extension architecture: they
// provide API endpoints for the frontend tag picker component and are mounted
// on entity profile pages.
package tags

import "time"

// Tag represents a campaign-scoped label that can be attached to entities.
// Tags have a unique slug within their campaign for URL-safe references.
// DmOnly tags are only visible to Scribes and Owners — Players cannot see them.
//
// A tag stores its own hex, or "" to follow the campaign's Tags and marks
// colour. Every tag the service returns has Color filled in with a real hex
// and OwnColor saying which of the two it is; StoredColor gives back what is
// stored.
type Tag struct {
	ID         int       `json:"id"`
	CampaignID string    `json:"campaignId"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Color      string    `json:"color"`
	OwnColor   bool      `json:"ownColor"`
	DmOnly     bool      `json:"dmOnly"`
	CreatedAt  time.Time `json:"createdAt"`
}

// EntityTag represents the many-to-many relationship between entities and tags.
// Each row in the entity_tags join table maps one entity to one tag.
type EntityTag struct {
	EntityID  string    `json:"entityId"`
	TagID     int       `json:"tagId"`
	CreatedAt time.Time `json:"createdAt"`
}

// --- Request DTOs (bound from HTTP requests) ---

// CreateTagRequest holds the data submitted when creating a new tag. An
// empty Color makes a tag that follows the campaign's colour.
type CreateTagRequest struct {
	Name   string `json:"name" form:"name"`
	Color  string `json:"color" form:"color"`
	DmOnly bool   `json:"dmOnly" form:"dmOnly"`
}

// UpdateTagRequest holds the data submitted when updating an existing tag.
//
// PARTIAL update: absent preserves, present replaces (ADR-056). Color and
// DmOnly are pointers so nil means "not sent" and a value (including the
// zero value) replaces — a rename that omits them must not clobber them,
// e.g. silently making a DM-only tag public. Name stays a required plain
// string: Update rejects the call with 400 when the merged name is blank,
// so an absent name fails loudly instead of overwriting silently. A Color
// sent as "" puts the tag back to following the campaign's colour.
type UpdateTagRequest struct {
	Name   string  `json:"name" form:"name"`
	Color  *string `json:"color" form:"color"`
	DmOnly *bool   `json:"dmOnly" form:"dmOnly"`
}

// UpdateTagInput is the service-layer form of UpdateTagRequest — identical
// shape, kept as a separate type so the service package boundary doesn't
// force callers to depend on a "Request"-named wire type. See
// UpdateTagRequest's doc comment for the partial-update contract.
type UpdateTagInput struct {
	Name   string
	Color  *string
	DmOnly *bool
}

// SetEntityTagsRequest holds the array of tag IDs to assign to an entity.
// This replaces all existing tags on the entity with the provided set.
type SetEntityTagsRequest struct {
	TagIDs []int `json:"tagIds"`
}

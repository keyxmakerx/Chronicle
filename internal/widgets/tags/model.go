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
type Tag struct {
	ID         int       `json:"id"`
	CampaignID string    `json:"campaignId"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Color      string    `json:"color"`
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

// CreateTagRequest holds the data submitted when creating a new tag.
type CreateTagRequest struct {
	Name  string `json:"name" form:"name"`
	Color string `json:"color" form:"color"`
}

// UpdateTagRequest holds the data submitted when updating an existing tag.
type UpdateTagRequest struct {
	Name  string `json:"name" form:"name"`
	Color string `json:"color" form:"color"`
}

// SetEntityTagsRequest holds the array of tag IDs to assign to an entity.
// This replaces all existing tags on the entity with the provided set.
type SetEntityTagsRequest struct {
	TagIDs []int `json:"tagIds"`
}

// Package posts implements the entity posts widget for Chronicle.
// Posts are named content sections attached to entities, displayed below
// the main entry on the entity show page. Each post has its own TipTap
// rich text content, visibility controls, and sort order for drag-to-reorder.
package posts

import (
	"encoding/json"
	"time"
)

// Post represents a single sub-note/post attached to an entity.
type Post struct {
	ID         string          `json:"id"`
	EntityID   string          `json:"entityId"`
	CampaignID string          `json:"campaignId"`
	Name       string          `json:"name"`
	Entry      json.RawMessage `json:"entry,omitempty"`
	EntryHTML  *string         `json:"entryHtml,omitempty"`
	IsPrivate  bool            `json:"isPrivate"`
	SortOrder  int             `json:"sortOrder"`
	CreatedBy  string          `json:"createdBy"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

// CreatePostRequest holds the data submitted when creating a new post.
type CreatePostRequest struct {
	Name      string          `json:"name"`
	Entry     json.RawMessage `json:"entry,omitempty"`
	EntryHTML *string         `json:"entryHtml,omitempty"`
	IsPrivate bool            `json:"isPrivate"`
}

// UpdatePostRequest holds the data submitted when updating a post.
type UpdatePostRequest struct {
	Name      *string         `json:"name,omitempty"`
	Entry     json.RawMessage `json:"entry,omitempty"`
	EntryHTML *string         `json:"entryHtml,omitempty"`
	IsPrivate *bool           `json:"isPrivate,omitempty"`
}

// ReorderPostsRequest holds the ordered list of post IDs for reordering.
type ReorderPostsRequest struct {
	PostIDs []string `json:"postIds"`
}

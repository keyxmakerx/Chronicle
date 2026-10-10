package entities

import "time"

// Place is one extra spot a page is listed in the page tree. The page keeps
// its single real parent (Entity.ParentID, which the breadcrumb follows); a
// Place only adds a second (or further) parent for the tree to show it under.
type Place struct {
	EntityID       string
	ParentEntityID string
	CampaignID     string
	SortOrder      int
	CreatedBy      string
	CreatedAt      time.Time
}

// PlaceLink names both ends of a Place with what a list needs to draw them,
// so no caller has to fetch each page again. It is read-only data; whether a
// viewer may see either page is decided by PlaceService, never by the reader.
type PlaceLink struct {
	EntityID       string
	EntityName     string
	EntityTypeIcon string
	EntityTypeName string
	ParentID       string
	ParentName     string
	SortOrder      int
}

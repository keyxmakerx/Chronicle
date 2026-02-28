// Package maps provides interactive map support for campaigns. Campaigns can
// have multiple maps (world, region, city, dungeon). Each map has a background
// image and positioned pin markers that optionally link to entities.
package maps

import "time"

// Map is an interactive map with a background image and positioned markers.
type Map struct {
	ID          string    `json:"id"`
	CampaignID  string    `json:"campaign_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	ImageID     *string   `json:"image_id,omitempty"`
	ImageWidth  int       `json:"image_width"`
	ImageHeight int       `json:"image_height"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Eager-loaded (populated by service, not every query).
	Markers []Marker `json:"markers,omitempty"`
}

// HasImage returns true if the map has a background image set.
func (m *Map) HasImage() bool {
	return m.ImageID != nil && *m.ImageID != ""
}

// Marker is a pin placed on a map at percentage coordinates (0-100).
// Optionally links to an entity and supports DM-only visibility.
type Marker struct {
	ID          string    `json:"id"`
	MapID       string    `json:"map_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	X           float64   `json:"x"`
	Y           float64   `json:"y"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	EntityID    *string   `json:"entity_id,omitempty"`
	Visibility  string    `json:"visibility"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Joined fields for display (populated by some queries).
	EntityName string `json:"entity_name,omitempty"`
	EntityIcon string `json:"entity_icon,omitempty"`
}

// IsDMOnly returns true if this marker is only visible to the DM.
func (m *Marker) IsDMOnly() bool {
	return m.Visibility == "dm_only"
}

// --- Request DTOs ---

// CreateMapInput is the validated input for creating a map.
type CreateMapInput struct {
	CampaignID string
	Name       string
	Description *string
	ImageID    *string
	ImageWidth  int
	ImageHeight int
}

// UpdateMapInput is the validated input for updating a map.
type UpdateMapInput struct {
	Name        string
	Description *string
	ImageID     *string
	ImageWidth  int
	ImageHeight int
}

// CreateMarkerInput is the validated input for placing a marker on a map.
type CreateMarkerInput struct {
	MapID      string
	Name       string
	Description *string
	X          float64
	Y          float64
	Icon       string
	Color      string
	EntityID   *string
	Visibility string
	CreatedBy  string
}

// UpdateMarkerInput is the validated input for updating a marker.
type UpdateMarkerInput struct {
	Name        string
	Description *string
	X           float64
	Y           float64
	Icon        string
	Color       string
	EntityID    *string
	Visibility  string
}

// MapViewData holds all data needed to render a single map page.
type MapViewData struct {
	CampaignID string
	Map        *Map
	Markers    []Marker
	IsScribe   bool
}

// MapListData holds all data needed to render the map list page.
type MapListData struct {
	CampaignID string
	Maps       []Map
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

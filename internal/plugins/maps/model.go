// Package maps provides interactive map support for campaigns. Campaigns can
// have multiple maps (world, region, city, dungeon). Each map has a background
// image and positioned pin markers that optionally link to entities.
package maps

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// VisibilityRules defines per-user visibility overrides for map content.
// Follows the same pattern as timelines and calendar events.
type VisibilityRules struct {
	AllowedUsers []string `json:"allowed_users,omitempty"`
	DeniedUsers  []string `json:"denied_users,omitempty"`
}

// ParseVisibilityRules parses the JSON visibility rules into a VisibilityRules struct.
func ParseVisibilityRules(raw *string) *VisibilityRules {
	if raw == nil || *raw == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*raw), &rules); err != nil {
		return nil
	}
	return &rules
}

// Map is an interactive map with a background image and positioned markers.
type Map struct {
	ID          string    `json:"id"`
	CampaignID  string    `json:"campaign_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	ImageID     *string   `json:"image_id,omitempty"`
	ImageWidth  int       `json:"image_width"`
	ImageHeight int       `json:"image_height"`
	// BackgroundColor optionally overrides the default theme-following
	// canvas color (bg-surface-alt, which adapts to dark/light via CSS
	// vars) with a fixed CSS color (e.g. "#000000"). Nil means "follow
	// theme" — the renderer falls back to the Tailwind class. Stored in
	// the maps.background_color VARCHAR(7) column that has existed since
	// migration 001 but was previously unused.
	BackgroundColor *string   `json:"background_color,omitempty"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Eager-loaded (populated by service, not every query).
	Markers []Marker `json:"markers,omitempty"`
}

// GetCampaignID returns the campaign this map belongs to. Implements
// middleware.CampaignScoped for generic IDOR protection.
func (m *Map) GetCampaignID() string { return m.CampaignID }

// HasImage returns true if the map has a background image set.
func (m *Map) HasImage() bool {
	return m.ImageID != nil && *m.ImageID != ""
}

// Marker is a pin placed on a map at percentage coordinates (0-100).
// Optionally links to an entity and supports per-player visibility via
// visibility_rules (same pattern as timelines/calendar events).
type Marker struct {
	ID              string    `json:"id"`
	MapID           string    `json:"map_id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	X               float64   `json:"x"`
	Y               float64   `json:"y"`
	Icon            string    `json:"icon"`
	Color           string    `json:"color"`
	PinCategory     *string   `json:"pin_category,omitempty"` // location, danger, treasure, quest, note.
	EntityID        *string   `json:"entity_id,omitempty"`
	Visibility      string    `json:"visibility"`
	VisibilityRules *string   `json:"visibility_rules,omitempty"`
	CreatedBy       *string   `json:"created_by,omitempty"`
	FoundryID       *string   `json:"foundry_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

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
// BackgroundColor is a tri-state: nil pointer = leave unchanged;
// pointer-to-empty-string = clear the override (revert to theme); any
// other CSS color string = set as the override.
//
// ExpectedUpdatedAt is the optional optimistic-concurrency token: when
// non-nil, the service rejects with 409 Conflict if the row's UpdatedAt
// has advanced past it. Omitting the field falls back to last-writer-wins
// for backwards compatibility — see internal/concurrency.Check.
type UpdateMapInput struct {
	Name              string
	Description       *string
	ImageID           *string
	ImageWidth        int
	ImageHeight       int
	BackgroundColor   *string
	ExpectedUpdatedAt *time.Time
}

// CreateMarkerInput is the validated input for placing a marker on a map.
type CreateMarkerInput struct {
	MapID           string
	Name            string
	Description     *string
	X               float64
	Y               float64
	Icon            string
	Color           string
	PinCategory     *string
	EntityID        *string
	Visibility      string
	VisibilityRules *string
	CreatedBy       string
	FoundryID       *string
}

// UpdateMarkerInput is the validated input for updating a marker.
// ExpectedUpdatedAt is the optimistic-concurrency token (optional) and is
// NOT a data field — it is the caller's last-known version, so it stays a
// plain pointer.
//
// Everything else is a patch.Field: this is a PARTIAL update under the
// contract ruled on 2026-08-07 (sweep R4) — an ABSENT key preserves the
// stored value, an EXPLICIT null clears it, a present value replaces it.
//
// Three losses were reproduced on this one struct. The Chronicle web edit
// form and the drag-end PUT send neither pin_category nor visibility_rules,
// so both were erased on every edit and every drag — and one of them is
// access-control data. Worse, the web request struct has no foundry_id
// member at all, so every web edit NULLed the marker's Foundry pairing key,
// which resurfaces later as DUPLICATE markers on the next sync.
//
// Absent-preserve resolves the fork the R3 booking could not: the web form
// still cannot set or clear a sync pairing key (it never sends foundry_id),
// while syncapi CAN still clear one by sending an explicit null.
type UpdateMarkerInput struct {
	Name              patch.Field[string]
	Description       patch.Field[string]
	X                 patch.Field[float64]
	Y                 patch.Field[float64]
	Icon              patch.Field[string]
	Color             patch.Field[string]
	PinCategory       patch.Field[string]
	EntityID          patch.Field[string]
	Visibility        patch.Field[string]
	VisibilityRules   patch.Field[string]
	FoundryID         patch.Field[string]
	ExpectedUpdatedAt *time.Time
}

// MapViewData holds all data needed to render a single map page.
type MapViewData struct {
	CampaignID string
	Map        *Map
	Markers    []Marker
	IsScribe   bool
}

// (FoundryPresenceView removed in NW-2.2 Chunk D2-cleanup. The "Connected
// to Foundry" pill lives in foundry_vtt now and is lazy-loaded by
// maps.templ via /foundry-vtt/presence-pill-fragment. The
// campaigns-side FoundryPresenceLookup interface + the live
// GET /campaigns/:id/foundry-presence JSON endpoint are unrelated and
// stay in place.)

// MapListData holds all data needed to render the map list page.
type MapListData struct {
	CampaignID string
	Maps       []Map
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
}

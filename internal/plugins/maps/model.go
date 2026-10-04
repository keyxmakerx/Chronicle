// Package maps provides interactive map support for campaigns. Campaigns can
// have multiple maps (world, region, city, dungeon). Each map has a background
// image and positioned pin markers that optionally link to entities.
package maps

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
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

// Allows reports whether userID may see content gated by these rules, under
// the non-owner branch of a visibility check — Owners bypass VisibilityRules
// entirely (ListMarkers/ListDrawings) and never call this. A nil receiver
// always allows.
//
// Must mirror the SQL predicate in repository.go's ListMarkers and
// drawing_repository.go's ListDrawings byte-for-byte, and the WebSocket
// hub's per-recipient gate (internal/websocket's Message.AudienceAllows,
// duplicated there since that package can't import a plugin's types) — all
// three must stay in lockstep or content becomes visible over one channel
// and not another.
//
// The default for a user named in NEITHER list depends on whether
// AllowedUsers is in use: empty means "everyone except DeniedUsers"
// (default-allow); non-empty is a strict allowlist (default-deny).
//
// A non-empty DeniedUsers also excludes an anonymous (empty) userID —
// permissions.DeniesAnonymous, ADR-049 — since a logged-out visitor can't be
// proven not to be the player the list names.
func (v *VisibilityRules) Allows(userID string) bool {
	if v == nil {
		return true
	}
	if permissions.DeniesAnonymous(v.DeniedUsers, userID) {
		return false
	}
	for _, id := range v.DeniedUsers {
		if id == userID {
			return false
		}
	}
	if len(v.AllowedUsers) == 0 {
		return true
	}
	for _, id := range v.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

// Map is an interactive map with a background image and positioned markers.
type Map struct {
	ID          string  `json:"id"`
	CampaignID  string  `json:"campaign_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ImageID     *string `json:"image_id,omitempty"`
	// PlayerImageURL replaces ImageID for a viewer who must not receive the
	// original (a map with a shadow): the address of the picture with the shadows
	// smudged in. Never stored; set only by MapService.ForViewer.
	PlayerImageURL string `json:"image_url,omitempty"`
	// PlayerImageAPIURL is the sync API address of the player copy, set on maps
	// the sync API returns when the map has a shadow, for every key: an owner key
	// still reads the original, but must hand players this copy instead.
	PlayerImageAPIURL string `json:"player_image_url,omitempty"`
	ImageWidth        int    `json:"image_width"`
	ImageHeight       int    `json:"image_height"`
	// BackgroundColor optionally overrides the default theme-following
	// canvas color (bg-surface-alt, which adapts to dark/light via CSS
	// vars) with a fixed CSS color (e.g. "#000000"). Nil means "follow
	// theme" — the renderer falls back to the Tailwind class.
	BackgroundColor *string `json:"background_color,omitempty"`
	// Display is the per-map display settings (frame override, pins, grid,
	// opening view, draw gate); nil means every default. See display_settings.go.
	Display   *DisplaySettings `json:"display_settings,omitempty"`
	SortOrder int              `json:"sort_order"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`

	// Eager-loaded (populated by service, not every query).
	Markers []Marker `json:"markers,omitempty"`
}

// GetCampaignID returns the campaign this map belongs to. Implements
// middleware.CampaignScoped for generic IDOR protection.
func (m *Map) GetCampaignID() string { return m.CampaignID }

// HasImage returns true if the map has a background image set, either the
// original or the player copy that stands in for it.
func (m *Map) HasImage() bool {
	return (m.ImageID != nil && *m.ImageID != "") || m.PlayerImageURL != ""
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
	CampaignID  string
	Name        string
	Description *string
	ImageID     *string
	ImageWidth  int
	ImageHeight int
}

// UpdateMapInput is the validated input for updating a map.
//
// PARTIAL update: absent preserves, explicit null clears, present replaces
// (see .ai/conventions.md). ImageID, ImageWidth, ImageHeight and
// Description use patch.Field[T], not a plain *T, so a rename-only PUT
// can't unlink the image or wipe the description.
//
// BackgroundColor stays a plain *string: nil = unchanged, pointer-to-empty
// = clear the override, any other value = set it — that sentinel, not a
// JSON null, is the contract the caller and service use.
//
// DisplaySettings is group-level partial too (MergeDisplaySettings): the whole
// key absent preserves, null clears every group, and an object replaces only
// the groups it names (a group set to null clears just that group).
//
// Name is a plain string; UpdateMap 400s on a blank merged name so an
// absent name fails loudly rather than overwriting silently.
//
// ExpectedUpdatedAt is the optional optimistic-concurrency token: non-nil
// rejects with 409 if UpdatedAt has advanced past it (internal/concurrency.Check);
// omitted falls back to last-writer-wins. Not a data field, so plain pointer.
type UpdateMapInput struct {
	Name              string
	Description       patch.Field[string]
	ImageID           patch.Field[string]
	ImageWidth        patch.Field[int]
	ImageHeight       patch.Field[int]
	BackgroundColor   *string
	DisplaySettings   patch.Field[json.RawMessage]
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
// Everything else is a patch.Field: this is a PARTIAL update (see
// .ai/conventions.md) — an ABSENT key preserves the stored value, an
// EXPLICIT null clears it, a present value replaces it. pin_category and
// visibility_rules (access-control data) must survive an edit or drag PUT
// that omits them, and foundry_id must survive a web edit that omits it —
// the web form never sends foundry_id, but syncapi can still clear it via
// an explicit null.
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
	// IsOwner gates the Map settings sheet: the PUT behind it is Owner-only,
	// so offering it to a scribe could only end in a refusal.
	IsOwner bool
	// IsDM is the owner or a co-DM. Only they see under shadows and get the
	// shadow tool; a scribe is hidden like a player.
	IsDM bool
	// UserID is the viewer's own id (empty when anonymous). The page uses it
	// only to key the per-person "where I left off" view in the browser.
	UserID string
	// Display is the resolved look of this map. The zero value is replaced by
	// defaults at render (see DisplayOrDefault), so a caller that builds
	// MapViewData by hand never has to fill it.
	Display ResolvedDisplay
}

// DisplayOrDefault returns Display, or the all-defaults resolution when the
// caller did not set one.
func (d MapViewData) DisplayOrDefault() ResolvedDisplay {
	if d.Display.Frame == "" {
		return ResolveDisplay(d.Map, "")
	}
	return d.Display
}

// MapListData holds all data needed to render the map list page.
type MapListData struct {
	CampaignID string
	Maps       []Map
	IsOwner    bool
	IsScribe   bool
	CSRFToken  string
	// CampaignFrame is the campaign-wide frame the list cards wear in small form.
	CampaignFrame string
}

// CanShadow reports whether this viewer gets the "Hide under shadow" tool: a
// DM who can draw on this map. Offering only; DrawingService enforces it.
func (d MapViewData) CanShadow() bool {
	return d.IsDM && d.CanDraw()
}

// CanPaintHexes reports whether this viewer is offered Paint in the Hexes
// tool. It mirrors HexService.requireWriter exactly: an owner or DM grant
// always, a scribe only when the map lets scribes draw, a player never. It is
// not CanDraw because a DM-granted player is below scribe, so CanDraw hides a
// tool the server would let them use.
func (d MapViewData) CanPaintHexes() bool {
	if d.IsDM {
		return true
	}
	if !d.IsScribe {
		return false
	}
	return d.DisplayOrDefault().DrawWho != DrawWhoOwners
}

// CanDraw reports whether this viewer gets the drawing tools: a scribe or
// above, and an owner when the map restricts drawing to owners. This only
// decides what is offered; DrawingService enforces the rule on the server.
func (d MapViewData) CanDraw() bool {
	if !d.IsScribe {
		return false
	}
	if d.DisplayOrDefault().DrawWho == DrawWhoOwners {
		return d.IsOwner
	}
	return true
}

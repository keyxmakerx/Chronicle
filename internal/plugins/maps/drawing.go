package maps

import (
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// Drawing represents a freehand drawing, shape, or text annotation on a map.
// Coordinates use percentage-based positioning (0-100) for resolution independence.
// A "shadow" drawing is not a visible shape for players: it is an area whose
// contents the server withholds from them (see shadow.go).
type Drawing struct {
	ID          string          `json:"id"`
	MapID       string          `json:"map_id"`
	LayerID     *string         `json:"layer_id,omitempty"`
	DrawingType string          `json:"drawing_type"` // freehand, rectangle, ellipse, polygon, text, shadow (two corners; fill_alpha is its strength, see shadow.go)
	Points      json.RawMessage `json:"points"`       // Array of {x, y} coordinate pairs.
	StrokeColor string          `json:"stroke_color"`
	StrokeWidth float64         `json:"stroke_width"`
	FillColor   *string         `json:"fill_color,omitempty"`
	FillAlpha   float64         `json:"fill_alpha"`
	TextContent *string         `json:"text_content,omitempty"`
	FontSize    *int            `json:"font_size,omitempty"`
	Rotation    float64         `json:"rotation"`
	Visibility  string          `json:"visibility"` // everyone, dm_only
	// VisibilityRules mirrors Marker.VisibilityRules — per-player
	// allow/deny overrides. Must be selected and enforced by ListDrawings
	// (drawing_repository.go, matching ListMarkers) and by the WS publisher
	// (routes.go's mapEventPublisherAdapter), or a rule set on a drawing
	// silently does nothing.
	VisibilityRules *string   `json:"visibility_rules,omitempty"`
	CreatedBy       *string   `json:"created_by,omitempty"`
	FoundryID       *string   `json:"foundry_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateDrawingInput is the validated input for creating a drawing.
type CreateDrawingInput struct {
	MapID       string
	LayerID     *string
	DrawingType string
	Points      json.RawMessage
	StrokeColor string
	StrokeWidth float64
	FillColor   *string
	FillAlpha   float64
	TextContent *string
	FontSize    *int
	Rotation    float64
	Visibility  string
	CreatedBy   string
	FoundryID   *string
	// CallerRole is the caller's campaign role (permissions.Role*), used for
	// the map's "who can draw" gate. Not a data field; 0 is refused.
	CallerRole int
	// CallerIsDM is true for an owner or co-DM. Only they may create a shadow;
	// it is not a data field.
	CallerIsDM bool
}

// UpdateDrawingInput is the validated input for updating a drawing.
// ExpectedUpdatedAt is the optimistic-concurrency token (optional) and is
// NOT a data field, so it stays a plain pointer.
//
// PARTIAL update: absent preserves, explicit null clears, present replaces
// (see .ai/conventions.md's partial-update contract; same shape as
// UpdateTokenInput). A caller sending only {points} to reshape a freehand
// stroke must not wipe fill, text content, font size or rotation.
type UpdateDrawingInput struct {
	Points            patch.Field[json.RawMessage]
	StrokeColor       patch.Field[string]
	StrokeWidth       patch.Field[float64]
	FillColor         patch.Field[string]
	FillAlpha         patch.Field[float64]
	TextContent       patch.Field[string]
	FontSize          patch.Field[int]
	Rotation          patch.Field[float64]
	Visibility        patch.Field[string]
	ExpectedUpdatedAt *time.Time
}

// Token represents a character, NPC, or object placed on a map.
// Tokens optionally link to Chronicle entities for cross-referencing.
type Token struct {
	ID             string          `json:"id"`
	MapID          string          `json:"map_id"`
	LayerID        *string         `json:"layer_id,omitempty"`
	EntityID       *string         `json:"entity_id,omitempty"`
	Name           string          `json:"name"`
	ImagePath      *string         `json:"image_path,omitempty"`
	X              float64         `json:"x"` // Percentage 0-100.
	Y              float64         `json:"y"`
	Width          float64         `json:"width"` // Grid units.
	Height         float64         `json:"height"`
	Rotation       float64         `json:"rotation"`
	Scale          float64         `json:"scale"`
	IsHidden       bool            `json:"is_hidden"` // GM-only visibility.
	IsLocked       bool            `json:"is_locked"`
	Bar1Value      *int            `json:"bar1_value,omitempty"`
	Bar1Max        *int            `json:"bar1_max,omitempty"`
	Bar2Value      *int            `json:"bar2_value,omitempty"`
	Bar2Max        *int            `json:"bar2_max,omitempty"`
	AuraRadius     *float64        `json:"aura_radius,omitempty"`
	AuraColor      *string         `json:"aura_color,omitempty"`
	LightRadius    *float64        `json:"light_radius,omitempty"`
	LightDimRadius *float64        `json:"light_dim_radius,omitempty"`
	LightColor     *string         `json:"light_color,omitempty"`
	VisionEnabled  bool            `json:"vision_enabled"`
	VisionRange    *float64        `json:"vision_range,omitempty"`
	Elevation      int             `json:"elevation"`
	SortOrder      int             `json:"sort_order"`
	StatusEffects  json.RawMessage `json:"status_effects,omitempty"`
	Flags          json.RawMessage `json:"flags,omitempty"`
	FoundryID      *string         `json:"foundry_id,omitempty"`
	CreatedBy      *string         `json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CreateTokenInput is the validated input for placing a token on a map.
type CreateTokenInput struct {
	MapID          string
	LayerID        *string
	EntityID       *string
	Name           string
	ImagePath      *string
	X              float64
	Y              float64
	Width          float64
	Height         float64
	Rotation       float64
	Scale          float64
	IsHidden       bool
	IsLocked       bool
	Bar1Value      *int
	Bar1Max        *int
	Bar2Value      *int
	Bar2Max        *int
	AuraRadius     *float64
	AuraColor      *string
	LightRadius    *float64
	LightDimRadius *float64
	LightColor     *string
	VisionEnabled  bool
	VisionRange    *float64
	Elevation      int
	StatusEffects  json.RawMessage
	Flags          json.RawMessage
	CreatedBy      string
	FoundryID      *string
}

// UpdateTokenInput is the validated input for updating a token.
// ExpectedUpdatedAt is the optimistic-concurrency token (optional) and is
// NOT a data field, so it stays a plain pointer.
//
// PARTIAL update: absent preserves, explicit null clears, present replaces
// (see .ai/conventions.md's partial-update contract). Every field uses
// patch.Field[T] rather than a plain *T, since a plain pointer bound from
// JSON can't distinguish "key omitted" from "key sent null" — a drag PUT
// carrying only {x, y} must not zero IsHidden, HP bars, or aura/light/vision.
//
// Name is the one field deliberately left a plain string: UpdateToken only
// assigns it when the caller sends a non-empty value, so an absent/blank
// name is already preserved without patch.Field.
type UpdateTokenInput struct {
	Name              string
	ImagePath         patch.Field[string]
	X                 patch.Field[float64]
	Y                 patch.Field[float64]
	Width             patch.Field[float64]
	Height            patch.Field[float64]
	Rotation          patch.Field[float64]
	Scale             patch.Field[float64]
	IsHidden          patch.Field[bool]
	IsLocked          patch.Field[bool]
	Bar1Value         patch.Field[int]
	Bar1Max           patch.Field[int]
	Bar2Value         patch.Field[int]
	Bar2Max           patch.Field[int]
	AuraRadius        patch.Field[float64]
	AuraColor         patch.Field[string]
	LightRadius       patch.Field[float64]
	LightDimRadius    patch.Field[float64]
	LightColor        patch.Field[string]
	VisionEnabled     patch.Field[bool]
	VisionRange       patch.Field[float64]
	Elevation         patch.Field[int]
	StatusEffects     patch.Field[json.RawMessage]
	Flags             patch.Field[json.RawMessage]
	ExpectedUpdatedAt *time.Time
}

// UpdateTokenPositionInput is a lightweight update for token position only.
// ExpectedUpdatedAt is the optimistic-concurrency token (optional). Drag
// pipelines that fire many position updates per second can simply omit it
// and accept last-writer-wins; deliberate "drop here" actions can include
// it to detect cross-user collisions.
type UpdateTokenPositionInput struct {
	X                 float64
	Y                 float64
	ExpectedUpdatedAt *time.Time
}

// Layer organizes map content into z-ordered groups (background, drawing, token, gm, fog).
type Layer struct {
	ID        string    `json:"id"`
	MapID     string    `json:"map_id"`
	Name      string    `json:"name"`
	LayerType string    `json:"layer_type"` // background, drawing, token, gm, fog
	SortOrder int       `json:"sort_order"`
	IsVisible bool      `json:"is_visible"`
	Opacity   float64   `json:"opacity"`
	IsLocked  bool      `json:"is_locked"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateLayerInput is the validated input for creating a layer.
type CreateLayerInput struct {
	MapID     string
	Name      string
	LayerType string
	SortOrder int
	IsVisible bool
	Opacity   float64
	IsLocked  bool
}

// UpdateLayerInput is the validated input for updating a layer.
// ExpectedUpdatedAt is the optimistic-concurrency token (optional) and is
// NOT a data field, so it stays a plain pointer.
//
// PARTIAL update: absent preserves, explicit null clears, present replaces
// (see .ai/conventions.md's partial-update contract). A SortOrder-only PUT
// must not silently reset the other patch.Field members' visibility/lock
// state.
//
// Name is deliberately left a plain string: UpdateLayer only ever assigns
// it when the caller sends a non-empty value, so an absent/blank name is
// already preserved.
type UpdateLayerInput struct {
	Name              string
	SortOrder         patch.Field[int]
	IsVisible         patch.Field[bool]
	Opacity           patch.Field[float64]
	IsLocked          patch.Field[bool]
	ExpectedUpdatedAt *time.Time
}

// FogRegion represents a revealed/hidden area of fog of war.
type FogRegion struct {
	ID         string          `json:"id"`
	MapID      string          `json:"map_id"`
	Points     json.RawMessage `json:"points"` // Polygon vertices.
	IsExplored bool            `json:"is_explored"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// CreateFogInput is the validated input for creating a fog region.
type CreateFogInput struct {
	MapID      string
	Points     json.RawMessage
	IsExplored bool
}

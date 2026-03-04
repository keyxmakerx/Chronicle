package maps

import (
	"encoding/json"
	"time"
)

// Drawing represents a freehand drawing, shape, or text annotation on a map.
// Coordinates use percentage-based positioning (0-100) for resolution independence.
type Drawing struct {
	ID          string          `json:"id"`
	MapID       string          `json:"map_id"`
	LayerID     *string         `json:"layer_id,omitempty"`
	DrawingType string          `json:"drawing_type"` // freehand, rectangle, ellipse, polygon, text
	Points      json.RawMessage `json:"points"`       // Array of {x, y} coordinate pairs.
	StrokeColor string          `json:"stroke_color"`
	StrokeWidth float64         `json:"stroke_width"`
	FillColor   *string         `json:"fill_color,omitempty"`
	FillAlpha   float64         `json:"fill_alpha"`
	TextContent *string         `json:"text_content,omitempty"`
	FontSize    *int            `json:"font_size,omitempty"`
	Rotation    float64         `json:"rotation"`
	Visibility  string          `json:"visibility"` // everyone, dm_only
	CreatedBy   *string         `json:"created_by,omitempty"`
	FoundryID   *string         `json:"foundry_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
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
}

// UpdateDrawingInput is the validated input for updating a drawing.
type UpdateDrawingInput struct {
	Points      json.RawMessage
	StrokeColor string
	StrokeWidth float64
	FillColor   *string
	FillAlpha   float64
	TextContent *string
	FontSize    *int
	Rotation    float64
	Visibility  string
}

// Token represents a character, NPC, or object placed on a map.
// Tokens optionally link to Chronicle entities for cross-referencing.
type Token struct {
	ID              string          `json:"id"`
	MapID           string          `json:"map_id"`
	LayerID         *string         `json:"layer_id,omitempty"`
	EntityID        *string         `json:"entity_id,omitempty"`
	Name            string          `json:"name"`
	ImagePath       *string         `json:"image_path,omitempty"`
	X               float64         `json:"x"` // Percentage 0-100.
	Y               float64         `json:"y"`
	Width           float64         `json:"width"`  // Grid units.
	Height          float64         `json:"height"`
	Rotation        float64         `json:"rotation"`
	Scale           float64         `json:"scale"`
	IsHidden        bool            `json:"is_hidden"` // GM-only visibility.
	IsLocked        bool            `json:"is_locked"`
	Bar1Value       *int            `json:"bar1_value,omitempty"`
	Bar1Max         *int            `json:"bar1_max,omitempty"`
	Bar2Value       *int            `json:"bar2_value,omitempty"`
	Bar2Max         *int            `json:"bar2_max,omitempty"`
	AuraRadius      *float64        `json:"aura_radius,omitempty"`
	AuraColor       *string         `json:"aura_color,omitempty"`
	LightRadius     *float64        `json:"light_radius,omitempty"`
	LightDimRadius  *float64        `json:"light_dim_radius,omitempty"`
	LightColor      *string         `json:"light_color,omitempty"`
	VisionEnabled   bool            `json:"vision_enabled"`
	VisionRange     *float64        `json:"vision_range,omitempty"`
	Elevation       int             `json:"elevation"`
	SortOrder       int             `json:"sort_order"`
	StatusEffects   json.RawMessage `json:"status_effects,omitempty"`
	Flags           json.RawMessage `json:"flags,omitempty"`
	FoundryID       *string         `json:"foundry_id,omitempty"`
	CreatedBy       *string         `json:"created_by,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
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
type UpdateTokenInput struct {
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
}

// UpdateTokenPositionInput is a lightweight update for token position only.
type UpdateTokenPositionInput struct {
	X float64
	Y float64
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
type UpdateLayerInput struct {
	Name      string
	SortOrder int
	IsVisible bool
	Opacity   float64
	IsLocked  bool
}

// FogRegion represents a revealed/hidden area of fog of war.
type FogRegion struct {
	ID         string          `json:"id"`
	MapID      string          `json:"map_id"`
	Points     json.RawMessage `json:"points"` // Polygon vertices.
	IsExplored bool            `json:"is_explored"`
	CreatedAt  time.Time       `json:"created_at"`
}

// CreateFogInput is the validated input for creating a fog region.
type CreateFogInput struct {
	MapID      string
	Points     json.RawMessage
	IsExplored bool
}

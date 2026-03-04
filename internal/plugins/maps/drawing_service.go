package maps

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// validDrawingTypes enumerates allowed drawing types.
var validDrawingTypes = map[string]bool{
	"freehand":  true,
	"rectangle": true,
	"ellipse":   true,
	"polygon":   true,
	"text":      true,
}

// validLayerTypes enumerates allowed layer types.
var validLayerTypes = map[string]bool{
	"background": true,
	"drawing":    true,
	"token":      true,
	"gm":         true,
	"fog":        true,
}

// DrawingService defines business logic for drawings, tokens, layers, and fog.
type DrawingService interface {
	// Drawing CRUD.
	CreateDrawing(ctx context.Context, input CreateDrawingInput) (*Drawing, error)
	GetDrawing(ctx context.Context, id string) (*Drawing, error)
	UpdateDrawing(ctx context.Context, id string, input UpdateDrawingInput) error
	DeleteDrawing(ctx context.Context, id string) error
	ListDrawings(ctx context.Context, mapID string, role int) ([]Drawing, error)

	// Token CRUD.
	CreateToken(ctx context.Context, input CreateTokenInput) (*Token, error)
	GetToken(ctx context.Context, id string) (*Token, error)
	UpdateToken(ctx context.Context, id string, input UpdateTokenInput) error
	UpdateTokenPosition(ctx context.Context, id string, input UpdateTokenPositionInput) error
	DeleteToken(ctx context.Context, id string) error
	ListTokens(ctx context.Context, mapID string, role int) ([]Token, error)

	// Layer CRUD.
	CreateLayer(ctx context.Context, input CreateLayerInput) (*Layer, error)
	GetLayer(ctx context.Context, id string) (*Layer, error)
	UpdateLayer(ctx context.Context, id string, input UpdateLayerInput) error
	DeleteLayer(ctx context.Context, id string) error
	ListLayers(ctx context.Context, mapID string) ([]Layer, error)

	// Fog CRUD.
	CreateFog(ctx context.Context, input CreateFogInput) (*FogRegion, error)
	DeleteFog(ctx context.Context, id string) error
	ListFog(ctx context.Context, mapID string) ([]FogRegion, error)
	ResetFog(ctx context.Context, mapID string) error
}

// drawingService implements DrawingService.
type drawingService struct {
	repo DrawingRepository
}

// NewDrawingService creates a new drawing service.
func NewDrawingService(repo DrawingRepository) DrawingService {
	return &drawingService{repo: repo}
}

// --- Drawing ---

// CreateDrawing validates input and creates a new drawing.
func (s *drawingService) CreateDrawing(ctx context.Context, input CreateDrawingInput) (*Drawing, error) {
	dt := strings.TrimSpace(input.DrawingType)
	if !validDrawingTypes[dt] {
		return nil, apperror.NewBadRequest("invalid drawing type: " + dt)
	}
	if len(input.Points) == 0 {
		return nil, apperror.NewBadRequest("points are required")
	}

	vis := input.Visibility
	if vis == "" {
		vis = "everyone"
	}

	d := &Drawing{
		ID:          generateID(),
		MapID:       input.MapID,
		LayerID:     input.LayerID,
		DrawingType: dt,
		Points:      input.Points,
		StrokeColor: input.StrokeColor,
		StrokeWidth: input.StrokeWidth,
		FillColor:   input.FillColor,
		FillAlpha:   input.FillAlpha,
		TextContent: input.TextContent,
		FontSize:    input.FontSize,
		Rotation:    input.Rotation,
		Visibility:  vis,
		CreatedBy:   &input.CreatedBy,
		FoundryID:   input.FoundryID,
	}

	if d.StrokeColor == "" {
		d.StrokeColor = "#000000"
	}
	if d.StrokeWidth <= 0 {
		d.StrokeWidth = 2.0
	}

	if err := s.repo.CreateDrawing(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// GetDrawing returns a drawing by ID.
func (s *drawingService) GetDrawing(ctx context.Context, id string) (*Drawing, error) {
	return s.repo.GetDrawing(ctx, id)
}

// UpdateDrawing validates input and updates a drawing.
func (s *drawingService) UpdateDrawing(ctx context.Context, id string, input UpdateDrawingInput) error {
	d, err := s.repo.GetDrawing(ctx, id)
	if err != nil {
		return err
	}

	if len(input.Points) > 0 {
		d.Points = input.Points
	}
	if input.StrokeColor != "" {
		d.StrokeColor = input.StrokeColor
	}
	if input.StrokeWidth > 0 {
		d.StrokeWidth = input.StrokeWidth
	}
	d.FillColor = input.FillColor
	d.FillAlpha = input.FillAlpha
	d.TextContent = input.TextContent
	d.FontSize = input.FontSize
	d.Rotation = input.Rotation
	if input.Visibility != "" {
		d.Visibility = input.Visibility
	}

	return s.repo.UpdateDrawing(ctx, d)
}

// DeleteDrawing removes a drawing.
func (s *drawingService) DeleteDrawing(ctx context.Context, id string) error {
	return s.repo.DeleteDrawing(ctx, id)
}

// ListDrawings returns all drawings for a map, filtered by role.
func (s *drawingService) ListDrawings(ctx context.Context, mapID string, role int) ([]Drawing, error) {
	return s.repo.ListDrawings(ctx, mapID, role)
}

// --- Token ---

// CreateToken validates input and creates a new token.
func (s *drawingService) CreateToken(ctx context.Context, input CreateTokenInput) (*Token, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("token name is required")
	}

	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return nil, apperror.NewBadRequest("token coordinates must be between 0 and 100")
	}

	t := &Token{
		ID:             generateID(),
		MapID:          input.MapID,
		LayerID:        input.LayerID,
		EntityID:       input.EntityID,
		Name:           name,
		ImagePath:      input.ImagePath,
		X:              input.X,
		Y:              input.Y,
		Width:          input.Width,
		Height:         input.Height,
		Rotation:       input.Rotation,
		Scale:          input.Scale,
		IsHidden:       input.IsHidden,
		IsLocked:       input.IsLocked,
		Bar1Value:      input.Bar1Value,
		Bar1Max:        input.Bar1Max,
		Bar2Value:      input.Bar2Value,
		Bar2Max:        input.Bar2Max,
		AuraRadius:     input.AuraRadius,
		AuraColor:      input.AuraColor,
		LightRadius:    input.LightRadius,
		LightDimRadius: input.LightDimRadius,
		LightColor:     input.LightColor,
		VisionEnabled:  input.VisionEnabled,
		VisionRange:    input.VisionRange,
		Elevation:      input.Elevation,
		StatusEffects:  input.StatusEffects,
		Flags:          input.Flags,
		CreatedBy:      &input.CreatedBy,
		FoundryID:      input.FoundryID,
	}

	if t.Width <= 0 {
		t.Width = 1.0
	}
	if t.Height <= 0 {
		t.Height = 1.0
	}
	if t.Scale <= 0 {
		t.Scale = 1.0
	}

	if err := s.repo.CreateToken(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetToken returns a token by ID.
func (s *drawingService) GetToken(ctx context.Context, id string) (*Token, error) {
	return s.repo.GetToken(ctx, id)
}

// UpdateToken validates input and updates a token.
func (s *drawingService) UpdateToken(ctx context.Context, id string, input UpdateTokenInput) error {
	t, err := s.repo.GetToken(ctx, id)
	if err != nil {
		return err
	}

	if input.Name != "" {
		t.Name = input.Name
	}
	t.ImagePath = input.ImagePath
	t.X = input.X
	t.Y = input.Y
	t.Width = input.Width
	t.Height = input.Height
	t.Rotation = input.Rotation
	t.Scale = input.Scale
	t.IsHidden = input.IsHidden
	t.IsLocked = input.IsLocked
	t.Bar1Value = input.Bar1Value
	t.Bar1Max = input.Bar1Max
	t.Bar2Value = input.Bar2Value
	t.Bar2Max = input.Bar2Max
	t.AuraRadius = input.AuraRadius
	t.AuraColor = input.AuraColor
	t.LightRadius = input.LightRadius
	t.LightDimRadius = input.LightDimRadius
	t.LightColor = input.LightColor
	t.VisionEnabled = input.VisionEnabled
	t.VisionRange = input.VisionRange
	t.Elevation = input.Elevation
	t.StatusEffects = input.StatusEffects
	t.Flags = input.Flags

	return s.repo.UpdateToken(ctx, t)
}

// UpdateTokenPosition updates only the position (optimized for drag).
func (s *drawingService) UpdateTokenPosition(ctx context.Context, id string, input UpdateTokenPositionInput) error {
	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return apperror.NewBadRequest("token coordinates must be between 0 and 100")
	}
	return s.repo.UpdateTokenPosition(ctx, id, input.X, input.Y)
}

// DeleteToken removes a token.
func (s *drawingService) DeleteToken(ctx context.Context, id string) error {
	return s.repo.DeleteToken(ctx, id)
}

// ListTokens returns all tokens for a map, filtered by role.
func (s *drawingService) ListTokens(ctx context.Context, mapID string, role int) ([]Token, error) {
	return s.repo.ListTokens(ctx, mapID, role)
}

// --- Layer ---

// CreateLayer validates input and creates a new layer.
func (s *drawingService) CreateLayer(ctx context.Context, input CreateLayerInput) (*Layer, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("layer name is required")
	}
	if !validLayerTypes[input.LayerType] {
		return nil, apperror.NewBadRequest("invalid layer type: " + input.LayerType)
	}

	l := &Layer{
		ID:        generateID(),
		MapID:     input.MapID,
		Name:      name,
		LayerType: input.LayerType,
		SortOrder: input.SortOrder,
		IsVisible: input.IsVisible,
		Opacity:   input.Opacity,
		IsLocked:  input.IsLocked,
	}
	if l.Opacity <= 0 {
		l.Opacity = 1.0
	}

	if err := s.repo.CreateLayer(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

// GetLayer returns a layer by ID.
func (s *drawingService) GetLayer(ctx context.Context, id string) (*Layer, error) {
	return s.repo.GetLayer(ctx, id)
}

// UpdateLayer validates input and updates a layer.
func (s *drawingService) UpdateLayer(ctx context.Context, id string, input UpdateLayerInput) error {
	l, err := s.repo.GetLayer(ctx, id)
	if err != nil {
		return err
	}

	if input.Name != "" {
		l.Name = input.Name
	}
	l.SortOrder = input.SortOrder
	l.IsVisible = input.IsVisible
	l.Opacity = input.Opacity
	l.IsLocked = input.IsLocked

	return s.repo.UpdateLayer(ctx, l)
}

// DeleteLayer removes a layer.
func (s *drawingService) DeleteLayer(ctx context.Context, id string) error {
	return s.repo.DeleteLayer(ctx, id)
}

// ListLayers returns all layers for a map.
func (s *drawingService) ListLayers(ctx context.Context, mapID string) ([]Layer, error) {
	return s.repo.ListLayers(ctx, mapID)
}

// --- Fog ---

// CreateFog validates input and creates a fog region.
func (s *drawingService) CreateFog(ctx context.Context, input CreateFogInput) (*FogRegion, error) {
	if len(input.Points) == 0 {
		return nil, apperror.NewBadRequest("fog points are required")
	}

	f := &FogRegion{
		ID:         generateID(),
		MapID:      input.MapID,
		Points:     input.Points,
		IsExplored: input.IsExplored,
	}

	if err := s.repo.CreateFog(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

// DeleteFog removes a fog region.
func (s *drawingService) DeleteFog(ctx context.Context, id string) error {
	return s.repo.DeleteFog(ctx, id)
}

// ListFog returns all fog regions for a map.
func (s *drawingService) ListFog(ctx context.Context, mapID string) ([]FogRegion, error) {
	return s.repo.ListFog(ctx, mapID)
}

// ResetFog removes all fog regions for a map.
func (s *drawingService) ResetFog(ctx context.Context, mapID string) error {
	return s.repo.ResetFog(ctx, mapID)
}

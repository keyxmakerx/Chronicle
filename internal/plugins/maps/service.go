package maps

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// generateID creates a random UUID v4 string.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// MapService defines business logic for the maps plugin.
type MapService interface {
	// Map CRUD.
	CreateMap(ctx context.Context, input CreateMapInput) (*Map, error)
	GetMap(ctx context.Context, id string) (*Map, error)
	UpdateMap(ctx context.Context, id string, input UpdateMapInput) error
	DeleteMap(ctx context.Context, id string) error
	ListMaps(ctx context.Context, campaignID string) ([]Map, error)

	// Marker CRUD.
	CreateMarker(ctx context.Context, input CreateMarkerInput) (*Marker, error)
	GetMarker(ctx context.Context, id string) (*Marker, error)
	UpdateMarker(ctx context.Context, id string, input UpdateMarkerInput) error
	DeleteMarker(ctx context.Context, id string) error
	ListMarkers(ctx context.Context, mapID string, role int) ([]Marker, error)
}

// mapService is the default MapService implementation.
type mapService struct {
	repo MapRepository
}

// NewMapService creates a MapService backed by the given repository.
func NewMapService(repo MapRepository) MapService {
	return &mapService{repo: repo}
}

// CreateMap creates a new map for a campaign.
func (s *mapService) CreateMap(ctx context.Context, input CreateMapInput) (*Map, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("map name is required")
	}
	if input.CampaignID == "" {
		return nil, apperror.NewValidation("campaign ID is required")
	}

	m := &Map{
		ID:          generateID(),
		CampaignID:  input.CampaignID,
		Name:        input.Name,
		Description: input.Description,
		ImageID:     input.ImageID,
		ImageWidth:  input.ImageWidth,
		ImageHeight: input.ImageHeight,
	}
	if err := s.repo.CreateMap(ctx, m); err != nil {
		return nil, fmt.Errorf("create map: %w", err)
	}
	return m, nil
}

// GetMap returns a map by ID, or a not-found error.
func (s *mapService) GetMap(ctx context.Context, id string) (*Map, error) {
	m, err := s.repo.GetMap(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get map: %w", err)
	}
	if m == nil {
		return nil, apperror.NewNotFound("map not found")
	}
	return m, nil
}

// UpdateMap modifies an existing map.
func (s *mapService) UpdateMap(ctx context.Context, id string, input UpdateMapInput) error {
	m, err := s.repo.GetMap(ctx, id)
	if err != nil {
		return fmt.Errorf("get map for update: %w", err)
	}
	if m == nil {
		return apperror.NewNotFound("map not found")
	}

	if input.Name == "" {
		return apperror.NewValidation("map name is required")
	}

	m.Name = input.Name
	m.Description = input.Description
	m.ImageID = input.ImageID
	m.ImageWidth = input.ImageWidth
	m.ImageHeight = input.ImageHeight

	if err := s.repo.UpdateMap(ctx, m); err != nil {
		return fmt.Errorf("update map: %w", err)
	}
	return nil
}

// DeleteMap removes a map and its markers.
func (s *mapService) DeleteMap(ctx context.Context, id string) error {
	m, err := s.repo.GetMap(ctx, id)
	if err != nil {
		return fmt.Errorf("get map for delete: %w", err)
	}
	if m == nil {
		return apperror.NewNotFound("map not found")
	}
	if err := s.repo.DeleteMap(ctx, id); err != nil {
		return fmt.Errorf("delete map: %w", err)
	}
	return nil
}

// ListMaps returns all maps for a campaign.
func (s *mapService) ListMaps(ctx context.Context, campaignID string) ([]Map, error) {
	maps, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list maps: %w", err)
	}
	return maps, nil
}

// CreateMarker places a new marker on a map.
func (s *mapService) CreateMarker(ctx context.Context, input CreateMarkerInput) (*Marker, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("marker name is required")
	}
	if input.MapID == "" {
		return nil, apperror.NewValidation("map ID is required")
	}
	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return nil, apperror.NewValidation("marker coordinates must be 0-100")
	}
	if input.Visibility == "" {
		input.Visibility = "everyone"
	}
	if input.Icon == "" {
		input.Icon = "fa-map-pin"
	}
	if input.Color == "" {
		input.Color = "#3b82f6"
	}

	mk := &Marker{
		ID:         generateID(),
		MapID:      input.MapID,
		Name:       input.Name,
		Description: input.Description,
		X:          input.X,
		Y:          input.Y,
		Icon:       input.Icon,
		Color:      input.Color,
		EntityID:   input.EntityID,
		Visibility: input.Visibility,
		CreatedBy:  &input.CreatedBy,
	}
	if err := s.repo.CreateMarker(ctx, mk); err != nil {
		return nil, fmt.Errorf("create marker: %w", err)
	}
	return mk, nil
}

// GetMarker returns a single marker by ID.
func (s *mapService) GetMarker(ctx context.Context, id string) (*Marker, error) {
	mk, err := s.repo.GetMarker(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get marker: %w", err)
	}
	if mk == nil {
		return nil, apperror.NewNotFound("marker not found")
	}
	return mk, nil
}

// UpdateMarker modifies an existing marker.
func (s *mapService) UpdateMarker(ctx context.Context, id string, input UpdateMarkerInput) error {
	mk, err := s.repo.GetMarker(ctx, id)
	if err != nil {
		return fmt.Errorf("get marker for update: %w", err)
	}
	if mk == nil {
		return apperror.NewNotFound("marker not found")
	}

	if input.Name == "" {
		return apperror.NewValidation("marker name is required")
	}
	if input.X < 0 || input.X > 100 || input.Y < 0 || input.Y > 100 {
		return apperror.NewValidation("marker coordinates must be 0-100")
	}

	mk.Name = input.Name
	mk.Description = input.Description
	mk.X = input.X
	mk.Y = input.Y
	mk.Icon = input.Icon
	mk.Color = input.Color
	mk.EntityID = input.EntityID
	mk.Visibility = input.Visibility

	if err := s.repo.UpdateMarker(ctx, mk); err != nil {
		return fmt.Errorf("update marker: %w", err)
	}
	return nil
}

// DeleteMarker removes a marker.
func (s *mapService) DeleteMarker(ctx context.Context, id string) error {
	mk, err := s.repo.GetMarker(ctx, id)
	if err != nil {
		return fmt.Errorf("get marker for delete: %w", err)
	}
	if mk == nil {
		return apperror.NewNotFound("marker not found")
	}
	if err := s.repo.DeleteMarker(ctx, id); err != nil {
		return fmt.Errorf("delete marker: %w", err)
	}
	return nil
}

// ListMarkers returns all markers for a map, filtered by role.
func (s *mapService) ListMarkers(ctx context.Context, mapID string, role int) ([]Marker, error) {
	markers, err := s.repo.ListMarkers(ctx, mapID, role)
	if err != nil {
		return nil, fmt.Errorf("list markers: %w", err)
	}
	return markers, nil
}

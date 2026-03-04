package syncapi

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// validChronicleTypes enumerates allowed sync mapping types.
var validChronicleTypes = map[string]bool{
	"entity":         true,
	"map":            true,
	"marker":         true,
	"drawing":        true,
	"token":          true,
	"calendar_event": true,
	"fog":            true,
	"layer":          true,
}

// validExternalSystems enumerates allowed external systems.
var validExternalSystems = map[string]bool{
	"foundry": true,
}

// validSyncDirections enumerates allowed sync directions.
var validSyncDirections = map[string]bool{
	"both": true,
	"push": true,
	"pull": true,
}

// SyncMappingService handles sync mapping business logic.
type SyncMappingService interface {
	CreateMapping(ctx context.Context, campaignID string, input CreateSyncMappingInput) (*SyncMapping, error)
	GetMapping(ctx context.Context, id string) (*SyncMapping, error)
	GetMappingByChronicle(ctx context.Context, campaignID, chronicleType, chronicleID, externalSystem string) (*SyncMapping, error)
	GetMappingByExternal(ctx context.Context, campaignID, externalSystem, externalID string) (*SyncMapping, error)
	ListMappings(ctx context.Context, campaignID string, limit, offset int) ([]SyncMapping, int, error)
	ListMappingsByType(ctx context.Context, campaignID, chronicleType string) ([]SyncMapping, error)
	BumpVersion(ctx context.Context, id string) error
	DeleteMapping(ctx context.Context, id string) error
	DeleteAllMappings(ctx context.Context, campaignID string) error
	PullModified(ctx context.Context, campaignID string, since time.Time, limit int) (*SyncPullResponse, error)
}

// syncMappingService implements SyncMappingService.
type syncMappingService struct {
	repo SyncMappingRepository
}

// NewSyncMappingService creates a sync mapping service.
func NewSyncMappingService(repo SyncMappingRepository) SyncMappingService {
	return &syncMappingService{repo: repo}
}

// CreateMapping validates input and creates a new sync mapping.
func (s *syncMappingService) CreateMapping(ctx context.Context, campaignID string, input CreateSyncMappingInput) (*SyncMapping, error) {
	// Validate chronicle type.
	ct := strings.TrimSpace(input.ChronicleType)
	if !validChronicleTypes[ct] {
		return nil, apperror.NewBadRequest(fmt.Sprintf("invalid chronicle_type: %s", ct))
	}

	// Validate external system.
	es := strings.TrimSpace(input.ExternalSystem)
	if !validExternalSystems[es] {
		return nil, apperror.NewBadRequest(fmt.Sprintf("invalid external_system: %s", es))
	}

	// Validate IDs.
	if strings.TrimSpace(input.ChronicleID) == "" {
		return nil, apperror.NewBadRequest("chronicle_id is required")
	}
	if strings.TrimSpace(input.ExternalID) == "" {
		return nil, apperror.NewBadRequest("external_id is required")
	}

	// Validate direction.
	dir := strings.TrimSpace(input.SyncDirection)
	if dir == "" {
		dir = "both"
	}
	if !validSyncDirections[dir] {
		return nil, apperror.NewBadRequest(fmt.Sprintf("invalid sync_direction: %s", dir))
	}

	// Check for existing mapping.
	existing, _ := s.repo.FindByChronicle(ctx, campaignID, ct, input.ChronicleID, es)
	if existing != nil {
		return nil, apperror.NewConflict("sync mapping already exists for this object")
	}

	mapping := &SyncMapping{
		ID:             uuid.New().String(),
		CampaignID:     campaignID,
		ChronicleType:  ct,
		ChronicleID:    input.ChronicleID,
		ExternalSystem: es,
		ExternalID:     input.ExternalID,
		SyncVersion:    1,
		LastSyncedAt:   time.Now().UTC(),
		SyncDirection:  dir,
		SyncMetadata:   input.SyncMetadata,
	}

	if err := s.repo.Create(ctx, mapping); err != nil {
		return nil, err
	}

	slog.Info("sync mapping created",
		slog.String("id", mapping.ID),
		slog.String("campaign", campaignID),
		slog.String("type", ct),
		slog.String("chronicle_id", input.ChronicleID),
		slog.String("external_id", input.ExternalID),
	)

	return mapping, nil
}

// GetMapping returns a sync mapping by ID.
func (s *syncMappingService) GetMapping(ctx context.Context, id string) (*SyncMapping, error) {
	return s.repo.FindByID(ctx, id)
}

// GetMappingByChronicle looks up a mapping by Chronicle object identity.
func (s *syncMappingService) GetMappingByChronicle(ctx context.Context, campaignID, chronicleType, chronicleID, externalSystem string) (*SyncMapping, error) {
	return s.repo.FindByChronicle(ctx, campaignID, chronicleType, chronicleID, externalSystem)
}

// GetMappingByExternal looks up a mapping by external system identity.
func (s *syncMappingService) GetMappingByExternal(ctx context.Context, campaignID, externalSystem, externalID string) (*SyncMapping, error) {
	return s.repo.FindByExternal(ctx, campaignID, externalSystem, externalID)
}

// ListMappings returns all mappings for a campaign with pagination.
func (s *syncMappingService) ListMappings(ctx context.Context, campaignID string, limit, offset int) ([]SyncMapping, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListByCampaign(ctx, campaignID, limit, offset)
}

// ListMappingsByType returns mappings filtered by Chronicle type.
func (s *syncMappingService) ListMappingsByType(ctx context.Context, campaignID, chronicleType string) ([]SyncMapping, error) {
	return s.repo.ListByType(ctx, campaignID, chronicleType)
}

// BumpVersion increments the sync version for a mapping.
func (s *syncMappingService) BumpVersion(ctx context.Context, id string) error {
	mapping, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return s.repo.UpdateVersion(ctx, id, mapping.SyncVersion+1)
}

// DeleteMapping removes a sync mapping.
func (s *syncMappingService) DeleteMapping(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// DeleteAllMappings removes all sync mappings for a campaign.
func (s *syncMappingService) DeleteAllMappings(ctx context.Context, campaignID string) error {
	return s.repo.DeleteByCampaign(ctx, campaignID)
}

// PullModified returns mappings updated since a given timestamp.
func (s *syncMappingService) PullModified(ctx context.Context, campaignID string, since time.Time, limit int) (*SyncPullResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	mappings, err := s.repo.ListModifiedSince(ctx, campaignID, since, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(mappings) > limit
	if hasMore {
		mappings = mappings[:limit]
	}

	return &SyncPullResponse{
		ServerTime: time.Now().UTC(),
		Mappings:   mappings,
		HasMore:    hasMore,
	}, nil
}

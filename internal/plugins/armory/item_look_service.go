package armory

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// ItemEntityChecker reports whether an entity is an item in the given
// campaign. Implemented in internal/app over the entities service. A missing
// entity, one in another campaign, or one of another type is a clean false.
type ItemEntityChecker interface {
	IsItemInCampaign(ctx context.Context, campaignID, entityID string) (bool, error)
}

// ItemLookService reads and saves item looks.
type ItemLookService interface {
	// GetLook returns the stored look for an item the viewer can see, or
	// (nil, nil) when it has none. An item that is missing, not an item, or
	// hidden from the viewer is NotFound, so a hidden item is not confirmed.
	GetLook(ctx context.Context, campaignID, itemEntityID string, role int, userID string) (json.RawMessage, error)
	// SaveLook validates raw and stores it, or clears the look when raw names
	// none, returning what is now stored (nil when cleared). Who may save is
	// decided by the route (Owner).
	SaveLook(ctx context.Context, campaignID, itemEntityID, userID string, raw []byte) (json.RawMessage, error)
}

type itemLookService struct {
	repo       ItemLookRepository
	items      ItemEntityChecker
	visibility EntityVisibilityFilter
}

// NewItemLookService creates the service. With a nil visibility filter every
// non-Owner viewer is treated as unable to see the item (fail closed).
func NewItemLookService(repo ItemLookRepository, items ItemEntityChecker, visibility EntityVisibilityFilter) ItemLookService {
	return &itemLookService{repo: repo, items: items, visibility: visibility}
}

func (s *itemLookService) requireItem(ctx context.Context, campaignID, itemEntityID string) error {
	ok, err := s.items.IsItemInCampaign(ctx, campaignID, itemEntityID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("item")
	}
	return nil
}

func (s *itemLookService) GetLook(ctx context.Context, campaignID, itemEntityID string, role int, userID string) (json.RawMessage, error) {
	if err := s.requireItem(ctx, campaignID, itemEntityID); err != nil {
		return nil, err
	}
	if role < permissions.RoleOwner {
		if s.visibility == nil {
			return nil, apperror.NewNotFound("item")
		}
		viewable, err := s.visibility.FilterViewableEntityIDs(ctx, campaignID, []string{itemEntityID}, role, userID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if !viewable[itemEntityID] {
			return nil, apperror.NewNotFound("item")
		}
	}
	return s.repo.Get(ctx, campaignID, itemEntityID)
}

func (s *itemLookService) SaveLook(ctx context.Context, campaignID, itemEntityID, userID string, raw []byte) (json.RawMessage, error) {
	if err := s.requireItem(ctx, campaignID, itemEntityID); err != nil {
		return nil, err
	}
	look, cleared, err := NormalizeItemLook(raw)
	if err != nil {
		return nil, err
	}
	if cleared {
		return nil, s.repo.Delete(ctx, campaignID, itemEntityID)
	}
	if err := s.repo.Upsert(ctx, campaignID, itemEntityID, userID, look); err != nil {
		return nil, err
	}
	return look, nil
}

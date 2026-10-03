package armory

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// ShopEntityChecker reports whether an entity is a shop in the given
// campaign. Implemented in internal/app over the entities service so this
// plugin never touches the entities repository. A missing entity, one in
// another campaign, or one of another type is a clean false, not an error.
type ShopEntityChecker interface {
	IsShopInCampaign(ctx context.Context, campaignID, entityID string) (bool, error)
}

// ShopRoomService reads and saves shop room layouts.
type ShopRoomService interface {
	// GetRoom returns the stored layout for a shop the viewer can see, or
	// (nil, nil) when none is saved yet. A shop that is missing, not a shop,
	// or hidden from the viewer is NotFound, so a hidden shop is not confirmed.
	GetRoom(ctx context.Context, campaignID, shopEntityID string, role int, userID string) (json.RawMessage, error)
	// SaveRoom validates raw, stores the normalized layout and returns it.
	// Who may save is decided by the route (Owner); the service only checks
	// the shop belongs to the campaign.
	SaveRoom(ctx context.Context, campaignID, shopEntityID, userID string, raw []byte) (json.RawMessage, error)
}

type shopRoomService struct {
	repo       ShopRoomRepository
	shops      ShopEntityChecker
	visibility EntityVisibilityFilter
}

// NewShopRoomService creates the service. With a nil visibility filter every
// non-Owner viewer is treated as unable to see the shop (fail closed).
func NewShopRoomService(repo ShopRoomRepository, shops ShopEntityChecker, visibility EntityVisibilityFilter) ShopRoomService {
	return &shopRoomService{repo: repo, shops: shops, visibility: visibility}
}

func (s *shopRoomService) requireShop(ctx context.Context, campaignID, shopEntityID string) error {
	ok, err := s.shops.IsShopInCampaign(ctx, campaignID, shopEntityID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewNotFound("shop")
	}
	return nil
}

func (s *shopRoomService) GetRoom(ctx context.Context, campaignID, shopEntityID string, role int, userID string) (json.RawMessage, error) {
	if err := s.requireShop(ctx, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	// Owner visibility sees every entity, as in the transaction reader.
	if role < permissions.RoleOwner {
		if s.visibility == nil {
			return nil, apperror.NewNotFound("shop")
		}
		viewable, err := s.visibility.FilterViewableEntityIDs(ctx, campaignID, []string{shopEntityID}, role, userID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if !viewable[shopEntityID] {
			return nil, apperror.NewNotFound("shop")
		}
	}
	return s.repo.Get(ctx, campaignID, shopEntityID)
}

// SaveRoom replaces the whole layout by design: the document is one unit the
// widget always sends complete, so it is not a partial-update endpoint.
func (s *shopRoomService) SaveRoom(ctx context.Context, campaignID, shopEntityID, userID string, raw []byte) (json.RawMessage, error) {
	if err := s.requireShop(ctx, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	layout, err := NormalizeShopRoomLayout(raw)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, campaignID, shopEntityID, userID, layout); err != nil {
		return nil, err
	}
	return layout, nil
}

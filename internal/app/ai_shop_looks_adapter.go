package app

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
)

// aiShopLooksAdapter implements records.ShopLooksAPI over the shop room and
// Shops street services, whose normalizers keep every check. Saving is the
// owner's, as on the routes; the kinds check that before calling.
type aiShopLooksAdapter struct {
	rooms  armory.ShopRoomService
	fronts armory.ShopStreetService
}

var _ records.ShopLooksAPI = aiShopLooksAdapter{}

// The shop room and storefront follow the shop stock in the prompt.
func init() {
	addAIKinds(205, func(d *aiDeps) []records.Kind {
		if d.ShopLooks == nil {
			return nil
		}
		owners := aiMenuAdapter{campaigns: d.Campaigns, types: d.Entities}
		return []records.Kind{
			records.NewShopRoomKind(d.ShopLooks, d.Entities, owners),
			records.NewStorefrontKind(d.ShopLooks, d.Entities, owners),
		}
	})
}

func (s aiShopLooksAdapter) Room(ctx context.Context, campaignID, shopID string, a records.Actor) (json.RawMessage, error) {
	return s.rooms.GetRoom(ctx, campaignID, shopID, a.Role, a.UserID)
}

func (s aiShopLooksAdapter) SaveRoom(ctx context.Context, campaignID, shopID string, a records.Actor, layout json.RawMessage, check bool) error {
	if check {
		_, err := armory.NormalizeShopRoomLayout(layout)
		return err
	}
	_, err := s.rooms.SaveRoom(ctx, campaignID, shopID, a.UserID, layout)
	return err
}

func (s aiShopLooksAdapter) Front(ctx context.Context, campaignID, shopID string, _ records.Actor) (json.RawMessage, bool, error) {
	return s.fronts.Front(ctx, campaignID, shopID)
}

func (s aiShopLooksAdapter) SaveFront(ctx context.Context, campaignID, shopID string, a records.Actor, front json.RawMessage, check bool) error {
	if check {
		_, err := armory.NormalizeShopFront(front)
		return err
	}
	_, err := s.fronts.SaveFront(ctx, campaignID, shopID, a.UserID, front)
	return err
}

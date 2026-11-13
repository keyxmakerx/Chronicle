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
	today  armory.ShopTodayService // nil leaves today's shelf out
}

var (
	_ records.ShopLooksAPI = aiShopLooksAdapter{}
	_ records.ShopShelfAPI = aiShopLooksAdapter{}
)

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

// CounterStarter is the armory's starter, so the AI edits what the widget draws.
func (aiShopLooksAdapter) CounterStarter(roomType string) json.RawMessage {
	return armory.ShopRoomCounterStarter(roomType)
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

// Shelf reads today's shelf as a may see it: goods a can't see are left out
// of the pool, and the hours to dawn only when the calendar shows a.
func (s aiShopLooksAdapter) Shelf(ctx context.Context, campaignID, shopID string, a records.Actor) (*records.ShopShelf, error) {
	if s.today == nil {
		return nil, nil
	}
	v, err := s.today.Today(ctx, campaignID, shopID, armory.Actor{UserID: a.UserID, Role: a.Role})
	if err != nil || v == nil {
		return nil, err
	}
	out := &records.ShopShelf{Places: v.Spots, HoursToDawn: -1, Pool: map[int]bool{}, Today: map[int]bool{}}
	if v.HoursToDawn != nil {
		out.HoursToDawn = *v.HoursToDawn
	}
	for _, id := range v.Pool {
		out.Pool[id] = true
	}
	for _, id := range v.Today {
		out.Today[id] = true
	}
	return out, nil
}

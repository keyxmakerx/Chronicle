package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// ShopRoomReader reads a shop's saved room layout. Satisfied by the armory
// plugin's ShopRoomService; nil layout means "none saved, generate one".
type ShopRoomReader interface {
	GetRoom(ctx context.Context, campaignID, shopEntityID string, role int, userID string) (json.RawMessage, error)
}

// SetShopRoomReader wires the armory room reader and the addon that gates
// it. The slug comes from the app wiring so this plugin names no other
// plugin. Unwired, the route refuses every call: the addon check fails
// closed on an empty slug and the handler answers 404.
func (h *APIHandler) SetShopRoomReader(r ShopRoomReader, addonSlug string) {
	h.shopRoomReader = r
	h.shopRoomAddon = addonSlug
}

// shopRoomAPIResponse is what the Foundry module shows to its players: the
// room layout and the goods a plain player may see.
type shopRoomAPIResponse struct {
	Layout json.RawMessage      `json:"layout"`
	Goods  []relations.Relation `json:"goods"`
}

// GetShopRoom returns a shop's room and its goods as players see them.
// GET /api/v1/campaigns/:id/armory/shops/:eid/room
//
// The module shows this room to every player in its world at once, so the
// goods are filtered for a plain Player with no per-user grants: dm_only
// relations and goods whose item page is hidden from players are left out.
// That is never more than any one player sees on the shop page.
func (h *APIHandler) GetShopRoom(c echo.Context) error {
	if h.shopRoomReader == nil {
		return apperror.NewNotFound("shop")
	}
	ctx := c.Request().Context()
	campaignID := c.Param("id")
	shopID := c.Param("eid")

	// The key's own visibility decides whether the shop exists for it at all.
	userID := h.resolveUserID(c)
	role := h.visibilityRoleFor(ctx, campaignID, userID, h.resolveRole(c))
	layout, err := h.shopRoomReader.GetRoom(ctx, campaignID, shopID, role, userID)
	if err != nil {
		return err
	}

	rels, err := h.relationSvc.ListByEntity(ctx, campaignID, shopID)
	if err != nil {
		slog.Error("api: list shop goods failed", slog.String("entity_id", shopID), slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list shop goods"))
	}
	goods, err := h.playerVisibleGoods(ctx, campaignID, rels)
	if err != nil {
		slog.Error("api: filter shop goods failed", slog.String("entity_id", shopID), slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list shop goods"))
	}
	if layout == nil {
		layout = json.RawMessage("null")
	}
	return c.JSON(http.StatusOK, shopRoomAPIResponse{Layout: layout, Goods: goods})
}

// playerVisibleGoods keeps the "sells" relations a plain player can see, with
// one batched visibility check for all item pages.
func (h *APIHandler) playerVisibleGoods(ctx context.Context, campaignID string, rels []relations.Relation) ([]relations.Relation, error) {
	sells := make([]relations.Relation, 0, len(rels))
	ids := make([]string, 0, len(rels))
	for _, r := range rels {
		if r.RelationType != "sells" || r.DmOnly {
			continue
		}
		sells = append(sells, r)
		ids = append(ids, r.TargetEntityID)
	}
	if len(sells) == 0 {
		return sells, nil
	}
	viewable, err := h.entitySvc.FilterViewableEntityIDs(ctx, campaignID, ids, int(campaigns.RolePlayer), "")
	if err != nil {
		return nil, err
	}
	out := make([]relations.Relation, 0, len(sells))
	for _, r := range sells {
		if viewable[r.TargetEntityID] {
			out = append(out, r)
		}
	}
	return out, nil
}

// ShopBuyAPIService is shop buying as the sync API needs it. The
// implementation lives behind internal/app so this plugin names no other
// plugin's types; answers are opaque and serialised as given.
//
// Like the stash calls, every call names the key holder and, optionally, the
// member it is made for, and is answered under that member's rules: who they
// may buy for, whether downtime lets them buy now, and which shops and goods
// they can see. Naming a member only ever narrows the key's own power.
type ShopBuyAPIService interface {
	Buyers(ctx context.Context, campaignID, keyUserID, actingUserID, shopEntityID string) (any, error)
	// Buy applies a basket. body is the request body as sent; the service
	// decodes the basket from it so its own checks stay the only ones.
	Buy(ctx context.Context, campaignID, keyUserID, actingUserID, shopEntityID string, body json.RawMessage) (any, error)
}

// maxShopBuyBodyBytes bounds a basket body, matching the web buy route.
const maxShopBuyBodyBytes = 32 << 10

// SetShopBuyer wires shop buying. It shares the shop room's addon gate.
// Unwired, the buy routes answer 404.
func (h *APIHandler) SetShopBuyer(s ShopBuyAPIService) {
	h.shopBuyer = s
}

// GetShopBuyers lists the characters the acting member may buy for at a shop.
// GET /api/v1/campaigns/:id/armory/shops/:eid/buyers?actingUserId=
func (h *APIHandler) GetShopBuyers(c echo.Context) error {
	if h.shopBuyer == nil {
		return apperror.NewNotFound("shop")
	}
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	out, err := h.shopBuyer.Buyers(c.Request().Context(), c.Param("id"), uid, c.QueryParam("actingUserId"), c.Param("eid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// BuyFromShop buys a basket as the acting member.
// POST /api/v1/campaigns/:id/armory/shops/:eid/buy
// Body: {"actingUserId"?, "buyerEntityId", "items":[{"relationId","quantity"}]}
func (h *APIHandler) BuyFromShop(c echo.Context) error {
	if h.shopBuyer == nil {
		return apperror.NewNotFound("shop")
	}
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	// Read one byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxShopBuyBodyBytes+1))
	if err != nil || len(raw) > maxShopBuyBodyBytes {
		return apperror.NewBadRequest("invalid request body")
	}
	var acting actingBody
	if err := json.Unmarshal(raw, &acting); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	out, err := h.shopBuyer.Buy(c.Request().Context(), c.Param("id"), uid, acting.ActingUserID, c.Param("eid"), raw)
	if err != nil {
		return err
	}
	noteSyncActor(c, acting.ActingUserID)
	return c.JSON(http.StatusOK, out)
}

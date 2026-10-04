package armory

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ShopRoomHandler serves the shop room layout endpoints. Thin: the service
// owns validation and visibility.
type ShopRoomHandler struct {
	svc ShopRoomService
}

// NewShopRoomHandler creates the handler.
func NewShopRoomHandler(svc ShopRoomService) *ShopRoomHandler {
	return &ShopRoomHandler{svc: svc}
}

// roomResponse is the wire form of both endpoints; layout is null when the
// shop has no saved room.
type roomResponse struct {
	Layout json.RawMessage `json:"layout"`
}

// Get handles GET /campaigns/:id/armory/shops/:eid/room.
func (h *ShopRoomHandler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// VisibilityRole so a DM-granted co-DM sees what the Owner sees.
	layout, err := h.svc.GetRoom(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, roomResponse{Layout: layout})
}

// Put handles PUT /campaigns/:id/armory/shops/:eid/room.
func (h *ShopRoomHandler) Put(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// Read one byte past the cap so an oversize body is detected, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxShopRoomBytes+1))
	if err != nil {
		return apperror.NewBadRequest("could not read request body")
	}
	layout, err := h.svc.SaveRoom(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), auth.GetUserID(c), raw)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return err
		}
		return apperror.NewInternal(err)
	}
	return c.JSON(http.StatusOK, roomResponse{Layout: layout})
}

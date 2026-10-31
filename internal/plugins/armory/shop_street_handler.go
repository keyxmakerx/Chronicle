package armory

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ShopStreetHandler serves the Shops street and storefront saves. Thin: the
// service owns validation and visibility.
type ShopStreetHandler struct {
	svc ShopStreetService
}

// NewShopStreetHandler creates the handler.
func NewShopStreetHandler(svc ShopStreetService) *ShopStreetHandler {
	return &ShopStreetHandler{svc: svc}
}

// Street handles GET /campaigns/:id/armory/shops/street.
func (h *ShopStreetHandler) Street(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.svc.Street(c.Request().Context(), cc.Campaign.ID, cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, v)
}

// frontResponse is the wire form of a saved storefront.
type frontResponse struct {
	Front json.RawMessage `json:"front"`
}

// PutFront handles PUT /campaigns/:id/armory/shops/:eid/front.
func (h *ShopStreetHandler) PutFront(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxShopFrontBytes+1))
	if err != nil {
		return apperror.NewBadRequest("could not read request body")
	}
	front, err := h.svc.SaveFront(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), auth.GetUserID(c), raw)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, frontResponse{Front: front})
}

package armory

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ShopTodayHandler serves today's shelf. Thin: the service owns the pick and
// visibility.
type ShopTodayHandler struct {
	svc ShopTodayService
}

// NewShopTodayHandler creates the handler.
func NewShopTodayHandler(svc ShopTodayService) *ShopTodayHandler {
	return &ShopTodayHandler{svc: svc}
}

// Get handles GET /campaigns/:id/armory/shops/:eid/today.
func (h *ShopTodayHandler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	a := Actor{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}
	v, err := h.svc.Today(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), a)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, v)
}

// spotsRequest is the body of a save of the number of places.
type spotsRequest struct {
	Spots *int `json:"spots"`
}

// Put handles PUT /campaigns/:id/armory/shops/:eid/today.
func (h *ShopTodayHandler) Put(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req spotsRequest
	if err := c.Bind(&req); err != nil || req.Spots == nil {
		return apperror.NewBadRequest("Say how many places today's shelf has.")
	}
	v, err := h.svc.SaveSpots(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), auth.GetUserID(c), *req.Spots)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, v)
}

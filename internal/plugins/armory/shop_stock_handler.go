package armory

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ShopStockHandler serves Fill the shelves. Thin: the service owns every rule.
type ShopStockHandler struct {
	svc ShopStockService
}

// NewShopStockHandler creates the handler.
func NewShopStockHandler(svc ShopStockService) *ShopStockHandler {
	return &ShopStockHandler{svc: svc}
}

func stockActor(c *echo.Context, cc *campaigns.CampaignContext) Actor {
	return Actor{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}
}

// Sources handles GET /campaigns/:id/armory/shops/:eid/fill-sources.
func (h *ShopStockHandler) Sources(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.svc.Sources(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), stockActor(c, cc))
	if err != nil {
		return listError(err)
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, v)
}

// Keep handles POST /campaigns/:id/armory/shops/:eid/stock.
func (h *ShopStockHandler) Keep(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in KeepInput
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("The draft couldn't be read.")
	}
	v, err := h.svc.Keep(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), stockActor(c, cc), in)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, v)
}

type undoRequest struct {
	Batch     string `json:"batch"`
	Relations []int  `json:"relations"`
}

// Undo handles POST /campaigns/:id/armory/shops/:eid/stock/undo.
func (h *ShopStockHandler) Undo(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in undoRequest
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("There is nothing to undo.")
	}
	n, err := h.svc.Undo(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), stockActor(c, cc), in.Batch, in.Relations)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, map[string]int{"removed": n})
}

type foodPriceRequest struct {
	FoodPrice *float64 `json:"foodPrice"`
}

// PutFoodPrice handles PUT /campaigns/:id/armory/food-price.
func (h *ShopStockHandler) PutFoodPrice(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in foodPriceRequest
	if err := c.Bind(&in); err != nil || in.FoodPrice == nil {
		return apperror.NewBadRequest("Say what a day's food costs.")
	}
	p, err := h.svc.SaveFoodPrice(c.Request().Context(), cc.Campaign.ID, stockActor(c, cc), *in.FoodPrice)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, map[string]float64{"foodPrice": p})
}

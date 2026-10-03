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

// maxBuyBodyBytes bounds a basket: 50 lines are far below this.
const maxBuyBodyBytes = 32 << 10

// ShopBuyHandler serves the shop-room buy endpoints. Thin: the service owns
// every rule.
type ShopBuyHandler struct {
	svc ShopBuyService
}

// NewShopBuyHandler creates the handler.
func NewShopBuyHandler(svc ShopBuyService) *ShopBuyHandler {
	return &ShopBuyHandler{svc: svc}
}

// buyActor builds the caller from the request. VisibilityRole so a DM-granted
// co-DM is treated as the Owner, as on every other armory path.
func buyActor(c echo.Context, cc *campaigns.CampaignContext) Actor {
	return Actor{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}
}

// Buyers handles GET /campaigns/:id/armory/shops/:eid/buyers.
func (h *ShopBuyHandler) Buyers(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	view, err := h.svc.Buyers(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), buyActor(c, cc))
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, view)
}

// Buy handles POST /campaigns/:id/armory/shops/:eid/buy.
func (h *ShopBuyHandler) Buy(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// Read one byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBuyBodyBytes+1))
	if err != nil || len(raw) > maxBuyBodyBytes {
		return apperror.NewBadRequest("invalid request body")
	}
	var in BuyInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	res, err := h.svc.Buy(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), buyActor(c, cc), in)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, res)
}

package armory

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// maxRollBodyBytes bounds a roll or a note body, both far smaller.
const maxRollBodyBytes = 8 << 10

// ShopRollHandler serves the counter roll endpoints and the owner's hidden
// notes. Thin: the service owns every rule.
type ShopRollHandler struct {
	svc ShopRollService
}

// NewShopRollHandler creates the handler.
func NewShopRollHandler(svc ShopRollService) *ShopRollHandler {
	return &ShopRollHandler{svc: svc}
}

func readRollBody(c *echo.Context, into any) error {
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxRollBodyBytes+1))
	if err != nil || len(raw) > maxRollBodyBytes {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	return nil
}

// Rolls handles GET /campaigns/:id/armory/shops/:eid/rolls?character=<id>.
func (h *ShopRollHandler) Rolls(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.svc.Rolls(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), buyActor(c, cc), c.QueryParam("character"))
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, v)
}

// Roll handles POST /campaigns/:id/armory/shops/:eid/rolls.
func (h *ShopRollHandler) Roll(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in RollInput
	if err := readRollBody(c, &in); err != nil {
		return err
	}
	res, err := h.svc.Roll(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), buyActor(c, cc), in)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, res)
}

// Secrets handles GET /campaigns/:id/armory/shops/:eid/secrets.
func (h *ShopRollHandler) Secrets(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.svc.Secrets(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), buyActor(c, cc))
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, v)
}

// PutSecret handles PUT /campaigns/:id/armory/shops/:eid/secrets/:rel.
func (h *ShopRollHandler) PutSecret(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !validRelationKey(c.Param("rel")) {
		return apperror.NewBadRequest("invalid item")
	}
	rel, _ := strconv.Atoi(c.Param("rel"))
	// Both fields are written together: this is the whole note, not a patch.
	var in GoodSecret
	if err := readRollBody(c, &in); err != nil {
		return err
	}
	out, err := h.svc.SaveSecret(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), rel, buyActor(c, cc), in)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, out)
}

package armory

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ItemLookHandler serves the look endpoints, which answer
// {"look": …, "source": …} (see LookView). Thin: the service owns
// validation and visibility.
type ItemLookHandler struct {
	svc ItemLookService
}

// NewItemLookHandler creates the handler.
func NewItemLookHandler(svc ItemLookService) *ItemLookHandler {
	return &ItemLookHandler{svc: svc}
}

// Get handles GET /campaigns/:id/armory/items/:eid/look.
func (h *ItemLookHandler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	view, err := h.svc.GetLook(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, view)
}

// Put handles PUT /campaigns/:id/armory/items/:eid/look.
func (h *ItemLookHandler) Put(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// Read one byte past the cap so an oversize body is detected, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxItemLookBytes+1))
	if err != nil {
		return apperror.NewBadRequest("could not read request body")
	}
	view, err := h.svc.SaveLook(c.Request().Context(), cc.Campaign.ID, c.Param("eid"), auth.GetUserID(c), raw)
	if err != nil {
		return listError(err)
	}
	return c.JSON(http.StatusOK, view)
}

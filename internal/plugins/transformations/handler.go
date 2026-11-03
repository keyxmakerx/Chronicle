// The browser HTTP surface for layers. Handlers bind, call the service and
// shape the response; the service owns every rule, so a mis-wired route
// cannot widen access.
package transformations

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the layer routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func viewerFrom(c echo.Context) (campaignID string, v Viewer, err error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{UserID: auth.GetUserID(c), VisibilityRole: cc.VisibilityRole()}, nil
}

// List handles GET /campaigns/:id/pages/:eid/layers.
func (h *Handler) List(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.ListForPage(c.Request().Context(), cid, c.Param("eid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Add handles POST /campaigns/:id/pages/:eid/layers.
func (h *Handler) Add(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	var in AddInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	out, err := h.svc.Add(c.Request().Context(), cid, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Remove handles DELETE /campaigns/:id/layers/:lid.
func (h *Handler) Remove(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Remove(c.Request().Context(), cid, c.Param("lid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

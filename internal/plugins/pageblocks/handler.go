// The browser HTTP surface for page blocks. Handlers bind, call the service
// and shape the response; the service owns every rule, so a mis-wired route
// cannot widen access.
package pageblocks

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the block routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// viewerFrom builds the Viewer from the campaign context.
func viewerFrom(c echo.Context) (campaignID string, v Viewer, err error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{
		UserID:         auth.GetUserID(c),
		MemberRole:     int(cc.MemberRole),
		VisibilityRole: cc.VisibilityRole(),
	}, nil
}

// Get handles GET /campaigns/:id/page-blocks/:eid/:bid.
func (h *Handler) Get(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), cid, c.Param("eid"), c.Param("bid"), c.QueryParam("kind"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Put handles PUT /campaigns/:id/page-blocks/:eid/:bid.
func (h *Handler) Put(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, rerr := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if rerr != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	var in SaveInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	out, err := h.svc.Put(c.Request().Context(), cid, c.Param("eid"), c.Param("bid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

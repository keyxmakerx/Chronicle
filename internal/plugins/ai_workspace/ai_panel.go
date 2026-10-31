package ai_workspace

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ChipCounts is what the top bar's AI button shows: requests waiting for the
// owner, open warnings about their keys, and whether AI is stopped here.
type ChipCounts struct {
	Waiting  int  `json:"waiting"`
	Warnings int  `json:"warnings"`
	Stopped  bool `json:"stopped"`
}

// AIChip answers the AI button on every campaign page. It is JSON, not a
// fragment, so the button can tell when the count goes up and drop a notice.
func (h *Handler) AIChip(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var out ChipCounts
	if h.connector != nil {
		ctx := c.Request().Context()
		reqs, err := h.connector.Waiting(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
		out.Waiting = len(reqs)
		if h.connector.Watching() {
			alerts, err := h.connector.Alerts(ctx, cc.Campaign.ID)
			if err != nil {
				return err
			}
			out.Warnings = len(alerts)
			sw, err := h.connector.CampaignSwitches(ctx, cc.Campaign.ID)
			if err != nil {
				return err
			}
			out.Stopped = sw.CutOffAt != nil
		}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, out)
}

// AIPanel is the panel the AI button opens over any page: the AI's waiting
// requests, answered in place, and copy and paste.
func (h *Handler) AIPanel(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, aiPanelBody(cc, v))
}

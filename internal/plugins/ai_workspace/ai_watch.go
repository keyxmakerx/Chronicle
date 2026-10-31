package ai_workspace

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// AnswerAIAlert answers one warning about the campaign's AI key: how=mine
// says the owner did it, how=paused also pauses the key.
func (h *Handler) AnswerAIAlert(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	id, err := strconv.ParseInt(c.Param("aid"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("that warning isn't one of this campaign's")
	}
	how := c.FormValue("how")
	ctx := c.Request().Context()
	if err := h.connector.SettleOwnAlert(ctx, cc.Campaign.ID, auth.GetUserID(c), id, how); err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), "campaign.ai_alert.answered", map[string]any{"alert_id": id, "how": how})
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiConnectSection(v))
}

// CutOffAI stops every AI in the campaign (on=1) or lets it back (on=0).
// Keys keep working once it is back on; nothing they asked for is lost.
func (h *Handler) CutOffAI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	on := c.FormValue("on") == "1"
	ctx := c.Request().Context()
	if err := h.connector.SetCutOff(ctx, cc.Campaign.ID, auth.GetUserID(c), on); err != nil {
		return err
	}
	if h.audit != nil {
		event := "campaign.ai.cut_off"
		if !on {
			event = "campaign.ai.back_on"
		}
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), event, nil)
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiConnectSection(v))
}

// alertTitle is a warning's headline on the owner's page.
func alertTitle(a connector.Alert) string {
	switch a.Kind {
	case connector.AlertNewApp:
		return "Your key was used from a new app"
	case connector.AlertLimit:
		return "Your key keeps hitting its limit"
	}
	return "Something about your key looks wrong"
}

// alertDetail is the sentence after the title: the stored detail, closed with
// one full stop, and how many times it happened when more than once.
func alertDetail(a connector.Alert) string {
	d := strings.TrimRight(strings.TrimSpace(a.Detail), ".")
	if d != "" {
		d += "."
	}
	if a.Hits > 1 {
		d = strings.TrimSpace(d + " " + strconv.Itoa(a.Hits) + " times so far.")
	}
	return d
}

// ai_players.go is the owner's side of players' AI on the Your AI page: the
// switch that lets players use their own AI, and each player's key with a
// way to pause or disconnect it. The owner never sees what a player's AI
// read or asked for; that is the player's.

package ai_workspace

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PlayersView is the players panel on the owner's Your AI page.
type PlayersView struct {
	CampaignID string
	// Blocked is true while the site admin doesn't let players use AI; the
	// switch is then not offered.
	Blocked bool
	Allowed bool
	Keys    []connector.KeyRow
}

// fillPlayers reads the players panel for the owner.
func (h *Handler) fillPlayers(c echo.Context, cc *campaigns.CampaignContext, v *ConnectView) error {
	if h.connector == nil || !h.connector.Watching() {
		return nil
	}
	ctx := c.Request().Context()
	p := &PlayersView{CampaignID: cc.Campaign.ID}
	sa, err := h.connector.ServerSwitches(ctx)
	if err != nil {
		return err
	}
	p.Blocked = sa.PlayersBlocked
	sw, err := h.connector.CampaignSwitches(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	p.Allowed = sw.PlayersAllowed
	if p.Keys, err = h.connector.PlayerKeys(ctx, cc.Campaign.ID, auth.GetUserID(c)); err != nil {
		return err
	}
	v.Players = p
	return nil
}

// SetPlayersAI turns players' AI on or off in the campaign.
func (h *Handler) SetPlayersAI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("using an AI isn't available on this Chronicle site")
	}
	on := c.FormValue("allowed") == "on"
	ctx := c.Request().Context()
	if err := h.connector.SetPlayersAllowed(ctx, cc.Campaign.ID, auth.GetUserID(c), on); err != nil {
		return err
	}
	if h.audit != nil {
		event := "campaign.ai_players.turned_off"
		if on {
			event = "campaign.ai_players.turned_on"
		}
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), event, nil)
	}
	return h.renderPlayers(c, cc)
}

// PlayerKeyAction pauses, resumes or disconnects one player's key.
func (h *Handler) PlayerKeyAction(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("using an AI isn't available on this Chronicle site")
	}
	id, err := strconv.ParseInt(c.Param("kid"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewNotFound("that key was ended or has run out; reload the page")
	}
	ctx := c.Request().Context()
	act := c.FormValue("act")
	var key *connector.Connection
	switch act {
	case "pause", "resume":
		key, err = h.connector.StopPlayerKey(ctx, cc.Campaign.ID, auth.GetUserID(c), id, act == "pause")
	case "end":
		key, err = h.connector.EndPlayerKey(ctx, cc.Campaign.ID, auth.GetUserID(c), id)
	default:
		return apperror.NewBadRequest("pick pause, resume or disconnect")
	}
	if err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), "campaign.ai_player_key."+act,
			map[string]any{"connection_id": id, "player_id": key.UserID})
	}
	return h.renderPlayers(c, cc)
}

func (h *Handler) renderPlayers(c echo.Context, cc *campaigns.CampaignContext) error {
	var v ConnectView
	if err := h.fillPlayers(c, cc, &v); err != nil {
		return err
	}
	if v.Players == nil {
		return apperror.NewBadRequest("players' AI isn't available on this Chronicle site")
	}
	return middleware.Render(c, http.StatusOK, aiPlayersPanel(*v.Players))
}

// playerKeyLine says which app a player's key is and how it stands.
func playerKeyLine(k connector.KeyRow) string {
	app := appName(&k.Connection)
	switch {
	case k.PausedAt != nil:
		return app + " · paused by you"
	case k.LastUsedAt == nil:
		return app + " · not used yet"
	}
	return fmt.Sprintf("%s · %s today", app, countOf(k.CallsToday, "call", "calls"))
}

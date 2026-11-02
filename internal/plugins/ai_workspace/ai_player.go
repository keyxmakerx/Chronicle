// ai_player.go is a player's AI: the AI button and panel on a player's
// screen, their own key, and the changes their AI asks for. The owner only
// sets whether players may use it (ai_players.go); each player answers their
// own AI. What a player's AI may change is decided in player_changes.go.

package ai_workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// playerPasteMax caps a pasted answer, as a request's blocks are capped.
const playerPasteMax = 256 * 1024

// PlayerAIView is a player's AI panel.
type PlayerAIView struct {
	CampaignID string
	// Why says why the player can't use AI here now; "" when they can.
	Why  string
	Conn *connector.Connection
	// NewKey is the key just made, shown this once and never again.
	NewKey   string
	Endpoint string
	Requests []connector.Request
	// Menu opens the connect menu, as it is after making a key.
	Menu bool
	// Done is the line after Apply saying what changed.
	Done string
	Now  time.Time
}

// PlayerReview is what a player's request or paste would change.
type PlayerReview struct {
	CampaignID string
	// RequestID is the request under review; 0 for a paste, whose text
	// travels with the form instead.
	RequestID int64
	Text      string
	Changes   []PlayerChange
}

// Kept is the changes that can be applied.
func (r PlayerReview) Kept() []PlayerChange {
	var out []PlayerChange
	for _, c := range r.Changes {
		if c.Held == "" {
			out = append(out, c)
		}
	}
	return out
}

// LeftOut is the changes held back, with why.
func (r PlayerReview) LeftOut() []PlayerChange {
	var out []PlayerChange
	for _, c := range r.Changes {
		if c.Held != "" {
			out = append(out, c)
		}
	}
	return out
}

// playersWhy is the plain reason a player can't use AI here now, or "".
func (h *Handler) playersWhy(ctx context.Context, campaignID string) string {
	if h.connector == nil {
		return "Using an AI isn't available on this Chronicle site."
	}
	err := h.connector.PlayersMayUseAI(ctx, campaignID)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, connector.ErrPlayersBlocked):
		return "The server's admin hasn't let players use AI."
	case errors.Is(err, connector.ErrCutOff):
		return "The campaign's owner has stopped AI in this campaign for now."
	case errors.Is(err, connector.ErrAIOff):
		return "AI is turned off on this server for now."
	}
	return "The campaign's owner hasn't let players use AI here."
}

// playerView reads a player's panel.
func (h *Handler) playerView(c echo.Context, cc *campaigns.CampaignContext) (PlayerAIView, error) {
	ctx := c.Request().Context()
	v := PlayerAIView{CampaignID: cc.Campaign.ID, Now: time.Now()}
	if v.Why = h.playersWhy(ctx, cc.Campaign.ID); v.Why != "" {
		return v, nil
	}
	v.Endpoint = connector.EndpointURL(h.baseURL, cc.Campaign.ID)
	var err error
	if v.Conn, err = h.connector.Current(ctx, cc.Campaign.ID, auth.GetUserID(c)); err != nil {
		return v, err
	}
	if v.Requests, err = h.connector.Waiting(ctx, cc.Campaign.ID, auth.GetUserID(c)); err != nil {
		return v, err
	}
	return v, nil
}

// PlayerChip answers the AI button on a player's screen: how many of their
// AI's requests wait, or off when they can't use AI here, which hides it.
func (h *Handler) PlayerChip(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	out := struct {
		Waiting int  `json:"waiting"`
		Off     bool `json:"off"`
	}{}
	ctx := c.Request().Context()
	if h.playersWhy(ctx, cc.Campaign.ID) != "" {
		out.Off = true
	} else {
		reqs, err := h.connector.Waiting(ctx, cc.Campaign.ID, auth.GetUserID(c))
		if err != nil {
			return err
		}
		out.Waiting = len(reqs)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, out)
}

// PlayerPanel is the panel a player's AI button opens.
func (h *Handler) PlayerPanel(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.playerView(c, cc)
	if err != nil {
		return err
	}
	v.Menu = v.Menu || c.QueryParam("menu") == "1"
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, playerPanelBody(v))
}

// PlayerBubble is the small bubble a player's AI button drops: copy and
// paste in one thin row, then their AI's waiting requests.
func (h *Handler) PlayerBubble(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.playerView(c, cc)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, playerBubbleBody(v))
}

// PlayerConnect makes the player's own key, revoking their earlier one, and
// shows it once. A player's key always reads as them and may ask for
// changes to their own character.
func (h *Handler) PlayerConnect(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if why := h.playersWhy(c.Request().Context(), cc.Campaign.ID); why != "" {
		return apperror.NewForbidden(strings.TrimRight(why, "."))
	}
	ctx := c.Request().Context()
	raw, conn, err := h.connector.Connect(ctx, cc.Campaign.ID, auth.GetUserID(c), aiexport.PrivacyModeSafe, true)
	if err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), "campaign.ai_connection.created",
			map[string]any{"for_player": conn.ForPlayer, "expires_at": conn.ExpiresAt.Format(time.RFC3339)})
	}
	v, err := h.playerView(c, cc)
	if err != nil {
		return err
	}
	v.Conn, v.NewKey, v.Menu = conn, raw, true
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, playerPanelBody(v))
}

// PlayerDisconnect revokes the player's own key.
func (h *Handler) PlayerDisconnect(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("using an AI isn't available on this Chronicle site")
	}
	ctx := c.Request().Context()
	if err := h.connector.Disconnect(ctx, cc.Campaign.ID, auth.GetUserID(c)); err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), "campaign.ai_connection.disconnected", nil)
	}
	v, err := h.playerView(c, cc)
	if err != nil {
		return err
	}
	v.Menu = true
	return middleware.Render(c, http.StatusOK, playerPanelBody(v))
}

// PlayerCopy is the text a player copies for an AI app they paste into:
// their characters, what each may change, and how to write a change.
func (h *Handler) PlayerCopy(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	if why := h.playersWhy(ctx, cc.Campaign.ID); why != "" {
		return apperror.NewForbidden(strings.TrimRight(why, "."))
	}
	var sheets []PlayerSheet
	if h.playerSheets != nil {
		var err error
		if sheets, err = h.playerSheets.Sheets(ctx, cc.Campaign.ID, auth.GetUserID(c)); err != nil {
			return err
		}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.String(http.StatusOK, playerInstructions(cc.Campaign.Name, sheets))
}

// PlayerPaste reviews an answer the player pasted.
func (h *Handler) PlayerPaste(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	text := strings.TrimSpace(c.FormValue("text"))
	if text == "" {
		return apperror.NewBadRequest("paste your AI's answer first")
	}
	if len(text) > playerPasteMax {
		return apperror.NewBadRequest("that answer is too long; ask your AI for fewer changes at once")
	}
	return h.renderPlayerReview(c, cc, 0, text)
}

// PlayerReviewRequest reviews one of the player's waiting requests.
func (h *Handler) PlayerReviewRequest(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	q, err := h.playerRequest(c, cc)
	if err != nil {
		return err
	}
	return h.renderPlayerReview(c, cc, q.ID, q.Body)
}

func (h *Handler) playerRequest(c echo.Context, cc *campaigns.CampaignContext) (*connector.Request, error) {
	if h.connector == nil {
		return nil, apperror.NewBadRequest("using an AI isn't available on this Chronicle site")
	}
	id, err := strconv.ParseInt(c.Param("rid"), 10, 64)
	if err != nil || id <= 0 {
		return nil, apperror.NewNotFound("that request isn't here any more")
	}
	return h.connector.WaitingRequest(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), id)
}

func (h *Handler) renderPlayerReview(c echo.Context, cc *campaigns.CampaignContext, rid int64, text string) error {
	ctx := c.Request().Context()
	if why := h.playersWhy(ctx, cc.Campaign.ID); why != "" {
		return apperror.NewForbidden(strings.TrimRight(why, "."))
	}
	changes, err := planPlayerChanges(ctx, h.playerSheets, cc.Campaign.ID, auth.GetUserID(c), text)
	if err != nil {
		return err
	}
	r := PlayerReview{CampaignID: cc.Campaign.ID, RequestID: rid, Changes: changes}
	if rid == 0 {
		r.Text = text
	}
	return middleware.Render(c, http.StatusOK, playerReview(r))
}

// PlayerApply applies the changes the player said yes to. The blocks are
// read again from the request or the paste, and each is checked again as it
// is written, so the form can only choose among what the player may do.
func (h *Handler) PlayerApply(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	if why := h.playersWhy(ctx, cc.Campaign.ID); why != "" {
		return apperror.NewForbidden(strings.TrimRight(why, "."))
	}
	userID := auth.GetUserID(c)
	var rid int64
	text, label := strings.TrimSpace(c.FormValue("text")), "copy and paste"
	if s := c.FormValue("request"); s != "" && s != "0" {
		var err error
		if rid, err = strconv.ParseInt(s, 10, 64); err != nil || rid <= 0 {
			return apperror.NewNotFound("that request isn't here any more")
		}
		q, err := h.connector.WaitingRequest(ctx, cc.Campaign.ID, userID, rid)
		if err != nil {
			return err
		}
		text, label = q.Body, q.AppName
	}
	if text == "" || len(text) > playerPasteMax {
		return apperror.NewBadRequest("paste your AI's answer first")
	}
	yes := map[int]bool{}
	for _, s := range c.Request().Form["yes"] {
		if n, err := strconv.Atoi(s); err == nil {
			yes[n] = true
		}
	}
	changes, err := planPlayerChanges(ctx, h.playerSheets, cc.Campaign.ID, userID, text)
	if err != nil {
		return err
	}
	applied, failed := 0, 0
	var names []string
	for _, ch := range changes {
		if ch.Held != "" || !yes[ch.Block] {
			continue
		}
		if err := h.playerSheets.PatchSheet(ctx, cc.Campaign.ID, userID, ch.SheetID, ch.Fields, label, true); err != nil {
			failed++
			continue
		}
		applied++
		names = append(names, ch.Page)
	}
	outcome := fmt.Sprintf("%d applied", applied)
	if failed > 0 {
		outcome += fmt.Sprintf(", %d couldn't be", failed)
	}
	if rid > 0 {
		if err := h.connector.Answered(ctx, cc.Campaign.ID, userID, rid, outcome); err != nil {
			return err
		}
	}
	if h.audit != nil && applied > 0 {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, userID, "campaign.ai_player_change.applied",
			map[string]any{"request_id": rid, "applied": applied, "pages": strings.Join(names, ", ")})
	}
	v, err := h.playerView(c, cc)
	if err != nil {
		return err
	}
	v.Done = playerAppliedLine(applied, failed, names)
	return middleware.Render(c, http.StatusOK, playerPanelBody(v))
}

// PlayerTurnDown answers one of the player's requests No as a whole.
func (h *Handler) PlayerTurnDown(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	q, err := h.playerRequest(c, cc)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := h.connector.TurnDown(ctx, cc.Campaign.ID, auth.GetUserID(c), q.ID); err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, auth.GetUserID(c), "campaign.ai_request.turned_down", map[string]any{"request_id": q.ID})
	}
	v, err := h.playerView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, playerRequests(v, false))
}

// playerConnected reports a live key the player made.
func playerConnected(v PlayerAIView) bool {
	return v.Conn != nil && v.Conn.Live(v.Now)
}

// playerConnState is the dot's state: paused or connected.
func playerConnState(v PlayerAIView) string {
	if v.Conn != nil && v.Conn.PausedAt != nil {
		return "paused"
	}
	return "connected"
}

// playerConnLine says how the player's key stands.
func playerConnLine(v PlayerAIView) string {
	app := appName(v.Conn)
	if v.Conn.PausedAt != nil {
		return app + " is paused by the campaign's owner. Its calls are refused until they let it back."
	}
	if v.Conn.LastUsedAt == nil {
		return "Your key is made but your AI hasn't used it yet. It sees what you see."
	}
	return app + " is connected. It sees what you see."
}

// playerRequestLine is the line under a request: the AI's words, what
// Chronicle reads it as, and when.
func playerRequestLine(v PlayerAIView, q connector.Request) string {
	var parts []string
	if q.Summary != "" {
		parts = append(parts, "“"+q.Summary+"”")
	}
	parts = append(parts, agoText(v.Now, q.CreatedAt))
	return strings.Join(parts, " · ") + ". Nothing has changed yet."
}

// playerValue is a new value as the review shows it.
func playerValue(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(cleared)"
	}
	return s
}

// playerPageName names a held-back block.
func playerPageName(c PlayerChange) string {
	if strings.TrimSpace(c.Page) != "" {
		return c.Page
	}
	return fmt.Sprintf("Block %d", c.Block)
}

// playerAppliedLine says what Apply did.
func playerAppliedLine(applied, failed int, names []string) string {
	line := "Changed " + strings.Join(names, ", ") + "."
	if applied == 0 {
		line = "Nothing changed."
	}
	if failed > 0 {
		line += fmt.Sprintf(" %s couldn't be made; try reviewing again.", countOf(failed, "change", "changes"))
	}
	return line
}

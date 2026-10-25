// ai_page.go is the campaign's "Your AI" page (owner only): connecting an AI
// app straight to the campaign, what it asked for, what it did, and the
// copy-and-paste tool for every other AI.

package ai_workspace

import (
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

// ConnectView is the connect section: no connection, a key just made (shown
// once), or the connection and what it did.
type ConnectView struct {
	CampaignID string
	// Available is false when the connector isn't wired (its tables did
	// not migrate); the page then offers copy and paste only.
	Available bool
	Conn      *connector.Connection
	// NewKey is the key just made, shown this once and never again.
	NewKey   string
	Endpoint string
	Calls    []connector.Call
	// Requests are what the AI asked for, waiting for the owner.
	Requests []connector.Request
	Now      time.Time
}

// SetConnector wires the connector service and the site's public address,
// which the page shows as the connector's address.
func (h *Handler) SetConnector(svc *connector.Service, baseURL string) {
	h.connector = svc
	h.baseURL = baseURL
}

// connectView reads the campaign's connection and recent calls.
func (h *Handler) connectView(c echo.Context, cc *campaigns.CampaignContext) (ConnectView, error) {
	v := ConnectView{CampaignID: cc.Campaign.ID, Now: time.Now()}
	if h.connector == nil {
		return v, nil
	}
	v.Available = true
	v.Endpoint = connector.EndpointURL(h.baseURL, cc.Campaign.ID)
	ctx := c.Request().Context()
	conn, err := h.connector.Current(ctx, cc.Campaign.ID)
	if err != nil {
		return v, err
	}
	v.Conn = conn
	if v.Calls, err = h.connector.Activity(ctx, cc.Campaign.ID); err != nil {
		return v, err
	}
	if v.Requests, err = h.connector.Waiting(ctx, cc.Campaign.ID); err != nil {
		return v, err
	}
	return v, nil
}

// AIPage renders the Your AI page.
func (h *Handler) AIPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	// The key is swapped into this page once; no-store keeps the browser's
	// back button from bringing it back.
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, AIPageView(cc, v))
}

// AIActivity re-renders "What it did", which the page refreshes while open.
func (h *Handler) AIActivity(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiActivityList(v))
}

// AIRequests re-renders the waiting requests, which the page asks for while
// open. seen is what the page shows now (requestsSeen); when nothing changed
// the answer is 204, so htmx swaps nothing.
func (h *Handler) AIRequests(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	if c.QueryParam("seen") == requestsSeen(v.Requests) {
		return c.NoContent(http.StatusNoContent)
	}
	return middleware.Render(c, http.StatusOK, aiRequests(v, false))
}

// TurnDownRequest answers a waiting request No as a whole: nothing applies.
func (h *Handler) TurnDownRequest(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	id, err := strconv.ParseInt(c.Param("rid"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("that request isn't here any more")
	}
	if err := h.connector.TurnDown(c.Request().Context(), cc.Campaign.ID, id); err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(), cc.Campaign.ID, "campaign.ai_request.turned_down", map[string]any{"request_id": id})
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiRequests(v, false))
}

// requestsSeen names the waiting requests a page shows: how many, and the
// newest one's id.
func requestsSeen(qs []connector.Request) string {
	if len(qs) == 0 {
		return "0"
	}
	return fmt.Sprintf("%d-%d", len(qs), qs[0].ID)
}

// requestHeading is a request card's title: "Claude asks for 6 changes".
func requestHeading(q connector.Request) string {
	app := appName(&connector.Connection{ClientName: q.AppName})
	word := "changes"
	if q.Blocks == 1 {
		word = "change"
	}
	return fmt.Sprintf("%s asks for %d %s", app, q.Blocks, word)
}

// requestLine is the line under it: the AI's own words in quotes (never
// Chronicle's), what the blocks would do, and when.
func requestLine(v ConnectView, q connector.Request) string {
	var parts []string
	if q.Summary != "" {
		parts = append(parts, "“"+q.Summary+"”")
	}
	parts = append(parts, q.Counts, agoText(v.Now, q.CreatedAt))
	return strings.Join(parts, " · ") + ". Nothing has changed yet."
}

// ConnectAI makes a new connection key (revoking any earlier one) and shows
// it once. sees is "players" or "owner"; change is "ask" or "look".
func (h *Handler) ConnectAI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	privacy := aiexport.PrivacyModeSafe
	switch c.FormValue("sees") {
	case "players":
	case "owner":
		privacy = aiexport.PrivacyModePermitted
	default:
		return apperror.NewBadRequest("pick what your AI may see")
	}
	var canAsk bool
	switch c.FormValue("change") {
	case "ask":
		canAsk = true
	case "look":
	default:
		return apperror.NewBadRequest("pick whether your AI may ask for changes")
	}
	raw, conn, err := h.connector.Connect(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), privacy, canAsk)
	if err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(), cc.Campaign.ID, "campaign.ai_connection.created",
			map[string]any{"privacy": privacy.String(), "can_ask": canAsk, "expires_at": conn.ExpiresAt.Format(time.RFC3339)})
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	// The connection just made, never a re-read that a second request
	// could have replaced.
	v.Conn, v.NewKey = conn, raw
	c.Response().Header().Set("Cache-Control", "no-store")
	return middleware.Render(c, http.StatusOK, aiConnectSection(v))
}

// DisconnectAI revokes the campaign's connection key at once.
func (h *Handler) DisconnectAI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	if err := h.connector.Disconnect(c.Request().Context(), cc.Campaign.ID); err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(), cc.Campaign.ID, "campaign.ai_connection.disconnected", nil)
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiConnectSection(v))
}

// appName is what the page calls the connected app: its own name when it
// gave a well-known one, else "Your AI".
func appName(conn *connector.Connection) string {
	if conn == nil {
		return "Your AI"
	}
	n := strings.ToLower(conn.ClientName)
	switch {
	case strings.Contains(n, "claude code"), strings.Contains(n, "claude-code"):
		return "Claude Code"
	case strings.Contains(n, "claude"):
		return "Claude"
	case strings.Contains(n, "chatgpt"), strings.Contains(n, "openai"):
		return "ChatGPT"
	case strings.Contains(n, "open webui"), strings.Contains(n, "open-webui"), strings.Contains(n, "openwebui"):
		return "Open WebUI"
	case strings.Contains(n, "lm studio"), strings.Contains(n, "lmstudio"):
		return "LM Studio"
	}
	return "Your AI"
}

// seesLabel says what a connection's answers show, in the page's words.
func seesLabel(p aiexport.PrivacyMode) string {
	if p == aiexport.PrivacyModePermitted {
		return "sees what you see"
	}
	return "sees what players see"
}

// changeLabel says what a connection may change, in the page's words.
func changeLabel(canAsk bool) string {
	if canAsk {
		return "asks for changes"
	}
	return "only looks"
}

// agoText is a plain "5 minutes ago".
func agoText(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 60*24*time.Hour:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
	return t.Format("2 January 2006")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// connStatus is the status line's state: waiting (never used), connected,
// or ended (ran out).
func connStatus(v ConnectView) string {
	switch {
	case v.Conn == nil:
		return "none"
	case v.Conn.Expired(v.Now):
		return "ended"
	case v.Conn.LastUsedAt == nil:
		return "waiting"
	}
	return "connected"
}

// claudeHeaderValue is what goes in Claude's Authorization header box.
func claudeHeaderValue(key string) string { return "Bearer " + key }

// claudeCodeCommand is the one line that adds the connector to Claude Code.
func claudeCodeCommand(v ConnectView) string {
	return fmt.Sprintf(`claude mcp add --transport http chronicle %s --header "Authorization: Bearer %s"`, v.Endpoint, v.NewKey)
}

// statusLine is the line under the connection's name.
func statusLine(v ConnectView) string {
	c := v.Conn
	if c == nil {
		return ""
	}
	parts := []string{"key made " + agoText(v.Now, c.CreatedAt), seesLabel(c.Privacy), changeLabel(c.CanAsk)}
	if c.LastUsedAt != nil {
		parts = append(parts, "last used "+agoText(v.Now, *c.LastUsedAt))
	}
	parts = append(parts, "key ends in "+c.KeyHint, "runs out on "+c.ExpiresAt.UTC().Format("2 January 2006"))
	return strings.Join(parts, " · ")
}

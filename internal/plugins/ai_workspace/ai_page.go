// ai_page.go is the campaign's "Your AI" page (owner only): connecting an AI
// app straight to the campaign, what it asked for, what it did, and the
// copy-and-paste tool for every other AI.

package ai_workspace

import (
	"context"
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
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
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
	// Sittings group Calls; Shown is how many the page lists.
	Sittings []connector.Sitting
	Shown    int
	Now      time.Time
	// Loc is the owner's time zone for the sittings' times (UTC when unset).
	Loc *time.Location
}

// sittingsStep is how many sittings the page lists at first, and how many
// more each "Show older sittings" adds.
const sittingsStep = 8

// SetZone wires the owner's time zone, for the times of what their AI did.
// Optional: unwired, times are UTC.
func (h *Handler) SetZone(f func(ctx context.Context, userID string) *time.Location) { h.zone = f }

// SetConnector wires the connector service and the site's public address,
// which the page shows as the connector's address.
func (h *Handler) SetConnector(svc *connector.Service, baseURL string) {
	h.connector = svc
	h.baseURL = baseURL
}

// connectView reads the campaign's connection and recent calls.
func (h *Handler) connectView(c echo.Context, cc *campaigns.CampaignContext) (ConnectView, error) {
	v := ConnectView{CampaignID: cc.Campaign.ID, Now: time.Now(), Shown: sittingsStep, Loc: time.UTC}
	if h.zone != nil {
		if loc := h.zone(c.Request().Context(), auth.GetUserID(c)); loc != nil {
			v.Loc = loc
		}
	}
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
	v.Sittings = connector.Sittings(v.Calls)
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
	if n, err := strconv.Atoi(c.QueryParam("shown")); err == nil && n > sittingsStep {
		v.Shown = min(n, len(v.Sittings)+sittingsStep)
	}
	// The page asks every 30 seconds; only new calls redraw it, so the
	// sittings the owner opened stay open.
	if seen := c.QueryParam("seen"); seen != "" && seen == activitySeen(v) {
		return c.NoContent(http.StatusNoContent)
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
	privacy, canAsk, err := accessForm(c)
	if err != nil {
		return err
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

// accessForm reads what the AI may do from a form: sees is "players" or
// "owner", change is "ask" or "look".
func accessForm(c echo.Context) (aiexport.PrivacyMode, bool, error) {
	privacy := aiexport.PrivacyModeSafe
	switch c.FormValue("sees") {
	case "players":
	case "owner":
		privacy = aiexport.PrivacyModePermitted
	default:
		return privacy, false, apperror.NewBadRequest("pick what your AI may see")
	}
	switch c.FormValue("change") {
	case "ask":
		return privacy, true, nil
	case "look":
		return privacy, false, nil
	}
	return privacy, false, apperror.NewBadRequest("pick whether your AI may ask for changes")
}

// connectionID is the key the page shows, which a pause or change names so
// it never lands on a newer key made elsewhere since.
func connectionID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.FormValue("connection"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperror.NewBadRequest("that key isn't this campaign's key any more; reload the page")
	}
	return id, nil
}

// PauseAI stops the campaign's key at once; what it asked for still waits.
func (h *Handler) PauseAI(c echo.Context) error { return h.setPaused(c, true) }

// ResumeAI lets a paused key work again.
func (h *Handler) ResumeAI(c echo.Context) error { return h.setPaused(c, false) }

func (h *Handler) setPaused(c echo.Context, pause bool) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	event := "campaign.ai_connection.resumed"
	if pause {
		event = "campaign.ai_connection.paused"
		err = h.connector.Pause(ctx, cc.Campaign.ID, auth.GetUserID(c), id)
	} else {
		err = h.connector.Resume(ctx, cc.Campaign.ID, auth.GetUserID(c), id)
	}
	if err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, event, map[string]any{"connection_id": id})
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiConnectSection(v))
}

// ChangeAIAccess changes what the key may see and whether it may ask, from
// its next call. Letting it see what the owner sees needs widen=yes, which
// the page sends only after the owner confirms.
func (h *Handler) ChangeAIAccess(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.connector == nil {
		return apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	privacy, canAsk, err := accessForm(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	cur, err := h.connector.Current(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if cur != nil && cur.ID == id && widens(cur.Privacy, privacy) && c.FormValue("widen") != "yes" {
		return apperror.NewBadRequest("confirm that your AI may see what you see")
	}
	was, err := h.connector.ChangeAccess(ctx, cc.Campaign.ID, auth.GetUserID(c), id, privacy, canAsk)
	if err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(ctx, cc.Campaign.ID, "campaign.ai_connection.changed", map[string]any{
			"connection_id": id,
			"privacy_from":  was.Privacy.String(), "privacy_to": privacy.String(),
			"can_ask_from": was.CanAsk, "can_ask_to": canAsk,
		})
	}
	v, err := h.connectView(c, cc)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, aiConnectSection(v))
}

// widens reports whether a change lets the AI see more than it does.
func widens(from, to aiexport.PrivacyMode) bool {
	return from == aiexport.PrivacyModeSafe && to != aiexport.PrivacyModeSafe
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

// connStatus is the connection's state: none, ended (ran out), paused,
// waiting (never used) or connected.
func connStatus(v ConnectView) string {
	switch {
	case v.Conn == nil:
		return "none"
	case v.Conn.Expired(v.Now):
		return "ended"
	case v.Conn.PausedAt != nil:
		return "paused"
	case v.Conn.LastUsedAt == nil:
		return "waiting"
	}
	return "connected"
}

// hasKey reports whether the campaign has a key that still works or is
// paused, and no new key is being shown.
func hasKey(v ConnectView) bool {
	if !v.Available || v.NewKey != "" {
		return false
	}
	switch connStatus(v) {
	case "paused", "waiting", "connected":
		return true
	}
	return false
}

// statusTitle is the connection panel's heading.
func statusTitle(v ConnectView) string {
	switch connStatus(v) {
	case "paused":
		return appName(v.Conn) + " is paused"
	case "waiting":
		return "Waiting for your AI to connect"
	}
	return appName(v.Conn) + " is connected"
}

// seesValue and changeValue are a connection's settings as the forms write
// them.
func seesValue(c *connector.Connection) string {
	if c != nil && c.Privacy != aiexport.PrivacyModeSafe {
		return "owner"
	}
	return "players"
}

func changeValue(c *connector.Connection) string {
	if c != nil && c.CanAsk {
		return "ask"
	}
	return "look"
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

// activitySeen names what "What it did" shows: the newest call and how many
// sittings are listed.
func activitySeen(v ConnectView) string {
	if len(v.Calls) == 0 {
		return fmt.Sprintf("0-%d", v.Shown)
	}
	return fmt.Sprintf("%d-%d", v.Calls[0].ID, v.Shown)
}

// shownSittings is the sittings the page lists now.
func shownSittings(v ConnectView) []connector.Sitting {
	if len(v.Sittings) > v.Shown {
		return v.Sittings[:v.Shown]
	}
	return v.Sittings
}

// sittingSummary says in one line what an AI did in a sitting.
func sittingSummary(s connector.Sitting) string {
	var parts []string
	if s.Reads > 0 {
		parts = append(parts, "read "+plural(s.Reads, "page"))
	}
	if s.Lookups > 0 {
		parts = append(parts, "looked up "+plural(s.Lookups, "thing"))
	}
	switch s.Asks {
	case 0:
	case 1:
		parts = append(parts, "asked for changes once")
	case 2:
		parts = append(parts, "asked for changes twice")
	default:
		parts = append(parts, fmt.Sprintf("asked for changes %d times", s.Asks))
	}
	if s.Refused > 0 {
		parts = append(parts, fmt.Sprintf("%d not answered", s.Refused))
	}
	line := strings.Join(parts, ", ")
	if line == "" {
		return "Connected"
	}
	return strings.ToUpper(line[:1]) + line[1:]
}

// sittingWaiting counts the waiting requests an AI sent in a sitting.
func sittingWaiting(v ConnectView, s connector.Sitting) int {
	n := 0
	for _, q := range v.Requests {
		if !q.CreatedAt.Before(s.Start.Add(-time.Minute)) && !q.CreatedAt.After(s.End.Add(time.Minute)) {
			n++
		}
	}
	return n
}

// sittingTag is the short word at the end of a sitting's line.
func sittingTag(v ConnectView, s connector.Sitting) string {
	if n := sittingWaiting(v, s); n > 0 {
		return fmt.Sprintf("%d waiting for you", n)
	}
	if s.Asks == 0 {
		return "only looked"
	}
	return ""
}

// sittingWhen is when a sitting started, in the owner's time zone: "Today,
// 2:44 pm", "Yesterday, …", a weekday within the week, else the date.
func sittingWhen(v ConnectView, t time.Time) string {
	now, t := v.Now.In(v.Loc), t.In(v.Loc)
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, v.Loc) }
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	var d string
	switch {
	case days <= 0:
		d = "Today"
	case days == 1:
		d = "Yesterday"
	case days < 7:
		d = t.Format("Monday")
	case t.Year() == now.Year():
		d = t.Format("Monday 2 January")
	default:
		d = t.Format("2 January 2006")
	}
	return d + ", " + clockText(v, t)
}

// clockText is a time of day in the owner's zone: "2:44 pm".
func clockText(v ConnectView, t time.Time) string {
	return strings.ToLower(t.In(v.Loc).Format("3:04 PM"))
}

// doneLine says what a request's Apply made: "Made 5 of 6 changes. You
// left 1 out."
func doneLine(r importer.CommitResult) string {
	made := r.Created + r.Renamed + r.Updated + r.Deleted
	total := made + r.Skipped + r.Failed
	line := fmt.Sprintf("Made %d of %s.", made, plural(total, "change"))
	if r.Skipped > 0 {
		line += fmt.Sprintf(" You left %d out.", r.Skipped)
	}
	if r.Failed > 0 {
		line += fmt.Sprintf(" %d couldn't be made; see what changed.", r.Failed)
	}
	return line
}

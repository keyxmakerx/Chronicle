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
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// ActivityRecorder is the slice of the admin change log Site admin ▸ AI
// writes to. Declared here so this plugin never imports admin; app wiring
// supplies an adapter that stamps the signed-in admin. Best-effort.
type ActivityRecorder interface {
	RecordAdminChange(c echo.Context, action, targetType, targetID, label string)
}

// AdminAIHandler is Site admin ▸ AI: every AI key on the server, what looks
// wrong, and the server's switches.
type AdminAIHandler struct {
	svc      *connector.Service
	activity ActivityRecorder
	zone     func(echo.Context) *time.Location
}

// NewAdminAIHandler creates Site admin ▸ AI over the connector's service.
func NewAdminAIHandler(svc *connector.Service) *AdminAIHandler { return &AdminAIHandler{svc: svc} }

// SetActivityRecorder wires the admin change log. Optional.
func (h *AdminAIHandler) SetActivityRecorder(r ActivityRecorder) { h.activity = r }

// SetZone wires the admin's time zone, for the times on the page. Optional:
// unwired, times are UTC.
func (h *AdminAIHandler) SetZone(f func(echo.Context) *time.Location) { h.zone = f }

func (h *AdminAIHandler) record(c echo.Context, action, targetType, targetID, label string) {
	if h.activity != nil {
		h.activity.RecordAdminChange(c, action, targetType, targetID, label)
	}
}

// RegisterAdminRoutes mounts Site admin ▸ AI on the admin group (already
// sign-in and site-admin gated). Turning AI off, changing the players
// switch and ending a key also ask for the admin's password again.
func RegisterAdminRoutes(admin *echo.Group, h *AdminAIHandler, reauth echo.MiddlewareFunc) {
	admin.GET("/ai", h.Page)
	admin.POST("/ai/access", h.SetAccess, reauth)
	admin.POST("/ai/keys/:kid/:act", h.Key, reauth)
	admin.POST("/ai/alerts/:aid/settle", h.Settle)
}

// AdminAIView is what the page renders.
type AdminAIView struct {
	*connector.AdminView
	Now  time.Time
	Loc  *time.Location
	// Only lists only the keys with an open warning.
	Only bool
	Note string
}

func (h *AdminAIHandler) view(c echo.Context, note string) (*AdminAIView, error) {
	v, err := h.svc.Admin(c.Request().Context())
	if err != nil {
		return nil, err
	}
	out := &AdminAIView{AdminView: v, Now: time.Now(), Loc: time.UTC, Only: c.QueryParam("only") == "flagged" || c.FormValue("only") == "flagged", Note: note}
	if h.zone != nil {
		if loc := h.zone(c); loc != nil {
			out.Loc = loc
		}
	}
	return out, nil
}

// Page renders Site admin ▸ AI.
func (h *AdminAIHandler) Page(c echo.Context) error {
	v, err := h.view(c, "")
	if err != nil {
		return err
	}
	if middleware.IsHTMX(c) && c.Request().Header.Get("HX-Target") == "admin-ai" {
		return middleware.Render(c, http.StatusOK, adminAIBody(v))
	}
	return middleware.Render(c, http.StatusOK, AdminAIPage(v))
}

// SetAccess saves the server's switches: ai_off=1 stops every key,
// players=1 lets owners turn on players' AI.
func (h *AdminAIHandler) SetAccess(c echo.Context) error {
	a := connector.ServerAccess{AIOff: c.FormValue("ai_off") == "1", PlayersBlocked: c.FormValue("players") != "1"}
	if err := h.svc.SetServerAccess(c.Request().Context(), a, auth.GetUserID(c)); err != nil {
		return err
	}
	label := "AI on"
	if a.AIOff {
		label = "AI off"
	}
	if a.PlayersBlocked {
		label += ", players' AI not allowed"
	} else {
		label += ", players' AI allowed"
	}
	h.record(c, "ai.access_changed", "ai", "server", label)
	v, err := h.view(c, "Saved.")
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, adminAIBody(v))
}

// Key pauses, resumes or ends one key.
func (h *AdminAIHandler) Key(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("kid"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("that isn't a key")
	}
	act := c.Param("act")
	k, err := h.svc.AdminKey(c.Request().Context(), id, act)
	if err != nil {
		return err
	}
	h.record(c, "ai.key_"+act, "ai_key", strconv.FormatInt(id, 10), "key ending "+k.KeyHint)
	note := map[string]string{"pause": "Key paused. Its owner sees it paused on their Your AI page.", "resume": "Key resumed.", "end": "Key ended. Its owner can make a new one."}[act]
	v, err := h.view(c, note)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, adminAIBody(v))
}

// Settle closes a warning; for a wrong-key flood it also stops turning the
// address away.
func (h *AdminAIHandler) Settle(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("aid"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("that isn't a warning")
	}
	a, err := h.svc.AdminSettle(c.Request().Context(), id)
	if err != nil {
		return err
	}
	note := "Marked as looked at."
	if a.Kind == connector.AlertWrongKeys {
		note = "Wrong keys from " + a.Address + " are no longer turned away."
	}
	h.record(c, "ai.alert_settled", "ai_alert", strconv.FormatInt(id, 10), a.Kind)
	v, err := h.view(c, note)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, adminAIBody(v))
}

// --- what the page says ---

func (v *AdminAIView) liveKeys() int { return len(v.Keys) }

func (v *AdminAIView) needsALook() int {
	n := len(v.Floods)
	for _, l := range v.Alerts {
		if len(l) > 0 {
			n++
		}
	}
	return n
}

func (v *AdminAIView) shownKeys() []connector.KeyRow {
	if !v.Only {
		return v.Keys
	}
	var out []connector.KeyRow
	for _, k := range v.Keys {
		if len(v.Alerts[k.ID]) > 0 {
			out = append(out, k)
		}
	}
	return out
}

func (v *AdminAIView) clock(t time.Time) string {
	t = t.In(v.Loc)
	y1, m1, d1 := t.Date()
	y2, m2, d2 := v.Now.In(v.Loc).Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return t.Format("3:04 pm")
	}
	return t.Format("2 Jan, 3:04 pm")
}

func (v *AdminAIView) lastUsed(k connector.KeyRow) string {
	if k.LastUsedAt == nil {
		return "never"
	}
	return v.clock(*k.LastUsedAt)
}

func (v *AdminAIView) sinceText() string {
	since := v.Today.Since.In(v.Loc)
	if since.Hour() == 0 && since.Minute() == 0 {
		return "since midnight"
	}
	return "since " + v.clock(v.Today.Since) + ", when the server started"
}

// keyApp is the apps a key has connected as, or its last name.
func keyApp(k connector.KeyRow) string {
	if k.ClientName == "" {
		return "not connected yet"
	}
	return k.ClientName
}

// keyWho says whose key it is: the owner's, or a player's.
func keyWho(k connector.KeyRow) string {
	if strings.TrimSpace(k.PersonName) == "" {
		return "a person who has left"
	}
	return k.PersonName
}

func keyState(k connector.KeyRow) string {
	if k.PausedAt != nil {
		return "Paused"
	}
	return ""
}

// countOf is "1 key" or "3 keys".
func countOf(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// hitsSuffix adds how many times a repeating warning happened.
func hitsSuffix(a connector.Alert) string {
	if a.Hits <= 1 {
		return ""
	}
	return fmt.Sprintf(" (%d times)", a.Hits)
}

// feedLine is one line of the last day's feed.
func feedLine(a connector.Alert) string {
	line := a.Detail + hitsSuffix(a) + "."
	if a.SettledAt != nil {
		line += " " + map[string]string{
			connector.SettledMine: "The owner said it was them.", connector.SettledPaused: "The key was paused.",
			connector.SettledEnded: "The key was ended.", connector.SettledSeen: "Marked as looked at.",
		}[a.SettledHow]
	}
	return line
}

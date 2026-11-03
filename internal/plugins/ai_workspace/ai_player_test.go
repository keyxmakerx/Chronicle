package ai_workspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// playerOwners: owner-1 owns camp-1, player-1 is a player in it.
type playerOwners struct{}

func (playerOwners) Check(_ context.Context, _, userID string) (string, error) {
	if userID == "owner-1" {
		return "Ashfall", nil
	}
	return "", connector.ErrNotOwner
}
func (playerOwners) CheckMember(_ context.Context, _, userID string) (string, error) {
	if userID == "owner-1" || userID == "player-1" {
		return "Ashfall", nil
	}
	return "", connector.ErrNotMember
}

// switchWatch is just the switches of the connector's watch.
type switchWatch struct {
	server  connector.ServerAccess
	allowed bool
	conns   map[int64]*connector.Connection
}

func (w *switchWatch) ServerAccess(context.Context) (connector.ServerAccess, error) {
	return w.server, nil
}
func (w *switchWatch) SetServerAccess(context.Context, connector.ServerAccess, string, time.Time) error {
	return nil
}
func (w *switchWatch) CampaignAccess(context.Context, string) (connector.CampaignAccess, error) {
	return connector.CampaignAccess{PlayersAllowed: w.allowed}, nil
}
func (w *switchWatch) SetCutOff(context.Context, string, *time.Time, time.Time) error { return nil }
func (w *switchWatch) SetPlayersAllowed(_ context.Context, _ string, on bool, _ time.Time) error {
	w.allowed = on
	return nil
}
func (w *switchWatch) NoteApp(context.Context, int64, string) (bool, error) { return false, nil }
func (w *switchWatch) RaiseAlert(context.Context, *connector.Alert) error   { return nil }
func (w *switchWatch) OpenAlerts(context.Context, string, int) ([]connector.Alert, error) {
	return nil, nil
}
func (w *switchWatch) RecentAlerts(context.Context, time.Time, int) ([]connector.Alert, error) {
	return nil, nil
}
func (w *switchWatch) SettleAlert(context.Context, string, int64, string, time.Time) (*connector.Alert, error) {
	return nil, nil
}
func (w *switchWatch) ConnectionByID(_ context.Context, id int64) (*connector.Connection, error) {
	return w.conns[id], nil
}
func (w *switchWatch) LiveKeys(context.Context, time.Time, time.Time) ([]connector.KeyRow, error) {
	return nil, nil
}
func (w *switchWatch) CallsSince(context.Context, time.Time) (int, error) { return 0, nil }

// playerRig is a handler where player-1 has a key and Tyne, and players'
// AI is off until a test turns it on.
func playerRig(t *testing.T) (*Handler, *reqRepo, *switchWatch, *fakeSheets) {
	t.Helper()
	h := recordsHandler(&memTables{})
	conn := &connector.Connection{ID: 1, CampaignID: "camp-1", UserID: "player-1", ForPlayer: true, CanAsk: true,
		ClientName: "Claude", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	repo := &reqRepo{conn: conn}
	svc := connector.NewService(repo, playerOwners{})
	w := &switchWatch{conns: map[int64]*connector.Connection{1: conn}}
	svc.SetWatch(w)
	h.SetConnector(svc, "https://c.example")
	sheets := tyneSheets()
	h.SetPlayerSheets(sheets)
	return h, repo, w, sheets
}

func playerCtx(c echo.Context) {
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Ashfall"}, MemberRole: campaigns.RolePlayer,
	})
	c.Set("auth_user_id", "player-1")
}

func asPlayer(t *testing.T, h func(echo.Context) error, method, path string, form url.Values, rid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if method == http.MethodPost {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	playerCtx(c)
	if rid != "" {
		c.SetParamNames("id", "rid")
		c.SetParamValues("camp-1", rid)
	}
	if err := h(c); err != nil {
		rec.Code = -1
		rec.Body.WriteString(err.Error())
	}
	return rec
}

// Off until the owner turns it on: the player's button hides, the panel
// says why, and nothing can be made, copied or applied.
func TestPlayerAI_OffUntilAllowed(t *testing.T) {
	h, _, w, _ := playerRig(t)
	rec := asPlayer(t, h.PlayerChip, http.MethodGet, "/campaigns/camp-1/ai/me/chip", nil, "")
	var chip struct {
		Waiting int  `json:"waiting"`
		Off     bool `json:"off"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &chip); err != nil || !chip.Off {
		t.Fatalf("chip = %s, want off", rec.Body)
	}
	panel := asPlayer(t, h.PlayerPanel, http.MethodGet, "/campaigns/camp-1/ai/me/panel", nil, "").Body.String()
	if !strings.Contains(panel, "hasn&#39;t let players use AI") || strings.Contains(panel, "ai-import-paste-area") {
		t.Fatalf("off panel = %s", panel)
	}
	for name, call := range map[string]func(echo.Context) error{"key": h.PlayerConnect, "copy": h.PlayerCopy, "paste": h.PlayerPaste, "apply": h.PlayerApply} {
		if rec := asPlayer(t, call, http.MethodPost, "/x", url.Values{"text": {"x"}}, ""); rec.Code != -1 {
			t.Errorf("%s worked while players' AI is off: %d", name, rec.Code)
		}
	}
	w.allowed = true
	w.server.PlayersBlocked = true
	if panel := asPlayer(t, h.PlayerPanel, http.MethodGet, "/x", nil, "").Body.String(); !strings.Contains(panel, "server&#39;s admin") {
		t.Errorf("blocked by the admin: %s", panel)
	}
}

// A paste is reviewed as the player's own changes: their character's keys
// are offered, everything else is left out with the reason, and Apply
// writes only what was offered and said yes to, however the form is forged.
func TestPlayerAI_PasteReviewApply(t *testing.T) {
	h, _, w, sheets := playerRig(t)
	w.allowed = true
	text := "---\naction: update\nname: Tyne\nfields:\n  class: Fury\n---\n" +
		"---\naction: update\nname: Brann\nfields:\n  pronouns: he/him\n---\n" +
		"---\nname: Sword of Dawn\n---\nA new item.\n"
	rec := asPlayer(t, h.PlayerPaste, http.MethodPost, "/x", url.Values{"text": {text}}, "")
	html := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("paste: %d %s", rec.Code, html)
	}
	for _, want := range []string{"Tyne", "Fury", `name="yes" value="1"`, "Left out", "Brann", "isn&#39;t a character you claimed", "can&#39;t add or remove pages"} {
		if !strings.Contains(html, want) {
			t.Errorf("review missing %q", want)
		}
	}
	if strings.Contains(html, `name="yes" value="2"`) || strings.Contains(html, `name="yes" value="3"`) {
		t.Error("a held-back block is offered as a choice")
	}

	// Saying yes to the held-back blocks too changes nothing more.
	rec = asPlayer(t, h.PlayerApply, http.MethodPost, "/x", url.Values{"text": {text}, "yes": {"1", "2", "3"}}, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Changed Tyne.") {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	if len(sheets.applied) != 1 || sheets.applied["e-tyne"]["class"] != "Fury" {
		t.Fatalf("applied = %+v", sheets.applied)
	}

	// No yes, nothing written.
	sheets.applied = nil
	asPlayer(t, h.PlayerApply, http.MethodPost, "/x", url.Values{"text": {text}}, "")
	if sheets.applied != nil {
		t.Fatalf("applied without a yes: %+v", sheets.applied)
	}
}

// The player's AI's request waits for the player, is reviewed and applied
// by them, and is then answered.
func TestPlayerAI_Request(t *testing.T) {
	h, repo, w, sheets := playerRig(t)
	w.allowed = true
	q, err := h.connector.Ask(context.Background(), repo.conn, "Pick my class", "---\naction: update\nname: Tyne\nfields:\n  class: Tactician\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	if chip := asPlayer(t, h.PlayerChip, http.MethodGet, "/x", nil, "").Body.String(); !strings.Contains(chip, `"waiting":1`) {
		t.Errorf("chip = %s", chip)
	}
	rid := "1"
	if review := asPlayer(t, h.PlayerReviewRequest, http.MethodGet, "/x", nil, rid).Body.String(); !strings.Contains(review, "Tactician") || strings.Contains(review, `name="text"`) {
		t.Fatalf("review = %s", review)
	}
	rec := asPlayer(t, h.PlayerApply, http.MethodPost, "/x", url.Values{"request": {rid}, "yes": {"1"}}, "")
	if rec.Code != http.StatusOK || sheets.applied["e-tyne"]["class"] != "Tactician" {
		t.Fatalf("apply: %d %s %+v", rec.Code, rec.Body, sheets.applied)
	}
	if left, _ := h.connector.Waiting(context.Background(), "camp-1", "player-1"); len(left) != 0 {
		t.Errorf("request %d still waits", q.ID)
	}
}

// The text copied for an AI app names only the player's characters.
func TestPlayerAI_Copy(t *testing.T) {
	h, _, w, _ := playerRig(t)
	w.allowed = true
	rec := asPlayer(t, h.PlayerCopy, http.MethodGet, "/x", nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "### Tyne") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
}

func asOwner1(t *testing.T, h func(echo.Context) error, form url.Values, kid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Ashfall"}, MemberRole: campaigns.RoleOwner})
	c.Set("auth_user_id", "owner-1")
	c.SetParamNames("id", "kid")
	c.SetParamValues("camp-1", kid)
	if err := h(c); err != nil {
		rec.Code = -1
		rec.Body.WriteString(err.Error())
	}
	return rec
}

// The owner turns players' AI on and pauses one player's key from the
// players panel; the panel comes back in its new state.
func TestPlayersPanel_SwitchAndPause(t *testing.T) {
	h, repo, w, _ := playerRig(t)
	rec := asOwner1(t, h.SetPlayersAI, url.Values{"allowed": {"on"}}, "")
	if rec.Code != http.StatusOK || !w.allowed || !strings.Contains(rec.Body.String(), `id="ai-players"`) {
		t.Fatalf("switch on: %d %s", rec.Code, rec.Body)
	}
	if rec := asOwner1(t, h.PlayerKeyAction, url.Values{"act": {"pause"}}, "1"); rec.Code != http.StatusOK || repo.conn.PausedAt == nil {
		t.Fatalf("pause: %d %s", rec.Code, rec.Body)
	}
	if rec := asOwner1(t, h.PlayerKeyAction, url.Values{"act": {"explode"}}, "1"); rec.Code != -1 {
		t.Error("an unknown action was accepted")
	}
	// A player can't reach the owner's switch.
	if rec := asPlayer(t, h.SetPlayersAI, http.MethodPost, "/x", url.Values{"allowed": {""}}, ""); rec.Code != -1 || !w.allowed {
		t.Error("a player turned players' AI off")
	}
}

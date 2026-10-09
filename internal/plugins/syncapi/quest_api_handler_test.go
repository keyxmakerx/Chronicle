package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeQuestAPI records what the handler passed on.
type fakeQuestAPI struct {
	players bool
	home    QuestHome
	body    string
	pay     QuestPay
	called  string
}

func (f *fakeQuestAPI) Homes(_ context.Context, _, _ string, players bool) (any, error) {
	f.called, f.players = "homes", players
	return []string{}, nil
}
func (f *fakeQuestAPI) Boards(_ context.Context, _, _ string, h QuestHome, players bool) (any, error) {
	f.called, f.home, f.players = "boards", h, players
	return map[string]any{}, nil
}
func (f *fakeQuestAPI) Quest(_ context.Context, _, _, _ string, players bool) (any, error) {
	f.called, f.players = "quest", players
	return map[string]any{}, nil
}
func (f *fakeQuestAPI) PutQuest(_ context.Context, _, _, _ string, body []byte) (any, error) {
	f.called, f.body = "put", string(body)
	return map[string]any{}, nil
}
func (f *fakeQuestAPI) Party(context.Context, string, string) (any, error) {
	f.called = "party"
	return []string{}, nil
}
func (f *fakeQuestAPI) Pay(_ context.Context, _, _ string, in QuestPay) (any, error) {
	f.called, f.pay = "pay", in
	return map[string]any{}, nil
}
func (f *fakeQuestAPI) Give(context.Context, string, string, QuestGive) (any, error) {
	f.called = "give"
	return map[string]any{}, nil
}

func TestQuestAPIHandler(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		granted    bool
		noKey      bool
		run        func(*QuestAPIHandler) echo.HandlerFunc
		method     string
		query      string
		body       string
		wantStatus int
		check      func(*testing.T, *fakeQuestAPI)
	}{
		{name: "owner reads homes", role: campaigns.RoleOwner, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Homes }, wantStatus: 200,
			check: func(t *testing.T, f *fakeQuestAPI) {
				if f.called != "homes" || f.players {
					t.Errorf("got %q players=%v", f.called, f.players)
				}
			}},
		{name: "co-DM asks for the players view", role: campaigns.RolePlayer, granted: true, query: "audience=players", run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Homes }, wantStatus: 200,
			check: func(t *testing.T, f *fakeQuestAPI) {
				if !f.players {
					t.Error("players view not passed on")
				}
			}},
		{name: "player key refused", role: campaigns.RolePlayer, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Homes }, wantStatus: 403},
		{name: "scribe key refused", role: campaigns.RoleScribe, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Pay }, method: http.MethodPost, body: `{"characterId":"c","amount":1}`, wantStatus: 403},
		{name: "no key", noKey: true, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Homes }, wantStatus: 401},
		{name: "category board", role: campaigns.RoleOwner, query: "category=13", run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Boards }, wantStatus: 200,
			check: func(t *testing.T, f *fakeQuestAPI) {
				if f.home != (QuestHome{Kind: "category", ID: "13"}) {
					t.Errorf("home %+v", f.home)
				}
			}},
		{name: "board needs exactly one home", role: campaigns.RoleOwner, query: "category=13&page=p", run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Boards }, wantStatus: 400},
		{name: "board needs a home", role: campaigns.RoleOwner, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Boards }, wantStatus: 400},
		{name: "put passes the body on", role: campaigns.RoleOwner, method: http.MethodPut, body: `{"version":3,"handedOut":true}`, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.PutQuest }, wantStatus: 200,
			check: func(t *testing.T, f *fakeQuestAPI) {
				if f.body != `{"version":3,"handedOut":true}` {
					t.Errorf("body %q", f.body)
				}
			}},
		{name: "empty put refused", role: campaigns.RoleOwner, method: http.MethodPut, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.PutQuest }, wantStatus: 400},
		{name: "oversize put refused", role: campaigns.RoleOwner, method: http.MethodPut, body: `{"x":"` + strings.Repeat("a", questBodyLimit) + `"}`, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.PutQuest }, wantStatus: 400},
		{name: "pay decodes", role: campaigns.RoleOwner, method: http.MethodPost, body: `{"characterId":"c1","amount":40.5,"reason":"as a reward"}`, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Pay }, wantStatus: 200,
			check: func(t *testing.T, f *fakeQuestAPI) {
				if f.pay != (QuestPay{CharacterID: "c1", Amount: 40.5, Reason: "as a reward"}) {
					t.Errorf("pay %+v", f.pay)
				}
			}},
		{name: "party has no players view", role: campaigns.RoleOwner, query: "audience=players", run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Party }, wantStatus: 403,
			check: func(t *testing.T, f *fakeQuestAPI) {
				if f.called != "" {
					t.Errorf("service called: %q", f.called)
				}
			}},
		{name: "pay with a bad body", role: campaigns.RoleOwner, method: http.MethodPost, body: `{`, run: func(h *QuestAPIHandler) echo.HandlerFunc { return h.Pay }, wantStatus: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeQuestAPI{}
			h := NewQuestAPIHandler(f, &changesCampaignSvc{role: tt.role, granted: tt.granted}, "reward-addon")
			e := echo.New()
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "/?"+tt.query, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "entityID")
			c.SetParamValues("camp-1", "q1")
			if !tt.noKey {
				c.Set(apiKeyContextKey, &APIKey{ID: 1, CampaignID: "camp-1", UserID: "u1"})
			}
			var status int
			if err := tt.run(h)(c); err != nil {
				status = apperror.SafeCode(err)
			} else {
				status = rec.Code
			}
			if status != tt.wantStatus {
				t.Fatalf("status %d, want %d", status, tt.wantStatus)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

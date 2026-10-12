package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeEncounterAPI records what the handler passed on.
type fakeEncounterAPI struct {
	called, q, id string
	limit         int
}

func (f *fakeEncounterAPI) Search(_ context.Context, _, _, q string, limit int) (any, int, error) {
	f.called, f.q, f.limit = "search", q, limit
	return []string{"a"}, 7, nil
}

func (f *fakeEncounterAPI) Get(_ context.Context, _, _, id string) (any, error) {
	f.called, f.id = "get", id
	return map[string]any{"id": id}, nil
}

func TestEncounterAPIHandler(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		granted    bool
		noKey      bool
		get        bool
		query      string
		wantStatus int
		check      func(*testing.T, *fakeEncounterAPI, *httptest.ResponseRecorder)
	}{
		{name: "owner lists, in the envelope", role: campaigns.RoleOwner, query: "q=ramp&limit=20", wantStatus: 200,
			check: func(t *testing.T, f *fakeEncounterAPI, rec *httptest.ResponseRecorder) {
				if f.called != "search" || f.q != "ramp" || f.limit != 20 {
					t.Errorf("passed on %+v", f)
				}
				var env struct {
					Data  []string `json:"data"`
					Total int      `json:"total"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Total != 7 || len(env.Data) != 1 {
					t.Errorf("envelope %s", rec.Body.String())
				}
			}},
		{name: "co-DM gets one", role: campaigns.RolePlayer, granted: true, get: true, wantStatus: 200,
			check: func(t *testing.T, f *fakeEncounterAPI, _ *httptest.ResponseRecorder) {
				if f.called != "get" || f.id != "enc-1" {
					t.Errorf("passed on %+v", f)
				}
			}},
		{name: "player key refused", role: campaigns.RolePlayer, wantStatus: 403},
		{name: "scribe key refused", role: campaigns.RoleScribe, get: true, wantStatus: 403},
		{name: "no key", noKey: true, wantStatus: 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeEncounterAPI{}
			h := NewEncounterAPIHandler(f, &changesCampaignSvc{role: tt.role, granted: tt.granted})
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "encounterID")
			c.SetParamValues("camp-1", "enc-1")
			if !tt.noKey {
				c.Set(apiKeyContextKey, &APIKey{ID: 1, CampaignID: "camp-1", UserID: "u1"})
			}
			run := h.List
			if tt.get {
				run = h.Get
			}
			var status int
			if err := run(c); err != nil {
				status = apperror.SafeCode(err)
			} else {
				status = rec.Code
			}
			if status != tt.wantStatus {
				t.Fatalf("status %d, want %d", status, tt.wantStatus)
			}
			if tt.wantStatus >= 400 && f.called != "" {
				t.Errorf("service called on a refused request: %q", f.called)
			}
			if tt.check != nil {
				tt.check(t, f, rec)
			}
		})
	}
}

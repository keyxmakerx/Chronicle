package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

const site = "https://chronicle.example"

type endpointRig struct {
	ep   *Endpoint
	svc  *Service
	repo *memRepo
	key  string
}

func newRig(t *testing.T, p aiexport.PrivacyMode) *endpointRig {
	t.Helper()
	svc, repo, _, _ := newTestService()
	key, _, err := svc.Connect(context.Background(), "camp-1", "owner-1", p, false)
	if err != nil {
		t.Fatal(err)
	}
	ep := NewEndpoint(svc, site+"/")
	ep.SetLookups(&records.Lookups{Tables: lootTables()})
	return &endpointRig{ep: ep, svc: svc, repo: repo, key: key}
}

// post sends body to campaign's endpoint; mod adjusts the request first.
func (r *endpointRig) post(campaign, body string, mod func(req *http.Request, key string)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/ai/campaigns/"+campaign+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.key)
	if mod != nil {
		mod(req, r.key)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(campaign)
	if err := r.ep.Serve(c); err != nil {
		panic(err)
	}
	return rec
}

const pingBody = `{"jsonrpc":"2.0","id":1,"method":"ping"}`

func TestEndpointURL(t *testing.T) {
	if got := EndpointURL(site+"/", "c 1"); got != site+"/api/ai/campaigns/c%201/mcp" {
		t.Errorf("got %q", got)
	}
}

func TestServe_Refusals(t *testing.T) {
	cases := []struct {
		name     string
		campaign string
		body     string
		mod      func(req *http.Request, key string)
		want     int
	}{
		{"no key", "camp-1", pingBody, func(r *http.Request, _ string) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
		{"a wrong key", "camp-1", pingBody, func(r *http.Request, _ string) { r.Header.Set("Authorization", "Bearer chai_"+strings.Repeat("A", 43)) }, http.StatusUnauthorized},
		{"not a bearer key", "camp-1", pingBody, func(r *http.Request, _ string) { r.Header.Set("Authorization", "Basic abc") }, http.StatusUnauthorized},
		{"the key in the address only", "camp-1", pingBody, func(r *http.Request, key string) {
			r.Header.Del("Authorization")
			r.URL.RawQuery = "key=" + key
		}, http.StatusUnauthorized},
		{"another campaign", "camp-2", pingBody, nil, http.StatusUnauthorized},
		{"a web page", "camp-1", pingBody, func(r *http.Request, _ string) { r.Header.Set("Origin", "https://evil.example") }, http.StatusForbidden},
		{"not JSON", "camp-1", pingBody, func(r *http.Request, _ string) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"too large", "camp-1", `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"` + strings.Repeat("a", maxBody) + `"}}`, nil, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, aiexport.PrivacyModeSafe)
			rec := r.post(tc.campaign, tc.body, tc.mod)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("a refusal may be cached")
			}
			if strings.Contains(rec.Body.String(), r.key) {
				t.Error("the refusal echoed the key")
			}
			if len(r.repo.calls) != 0 {
				t.Errorf("a refused request was logged: %+v", r.repo.calls)
			}
		})
	}
}

func TestServe_SameOriginAndXAPIKeyAreAccepted(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModeSafe)
	rec := r.post("camp-1", pingBody, func(req *http.Request, key string) {
		req.Header.Set("Origin", site)
		req.Header.Del("Authorization")
		req.Header.Set("X-API-Key", key)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
}

func TestServe_RateLimited(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModeSafe)
	for i := 0; i < callsPerMinute; i++ {
		if rec := r.post("camp-1", pingBody, nil); rec.Code != http.StatusOK {
			t.Fatalf("call %d: %d", i+1, rec.Code)
		}
	}
	rec := r.post("camp-1", pingBody, nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("status = %d, Retry-After = %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

// Connecting and each tool call show in "What it did"; the first connect
// stamps the connection as used.
func TestServe_LogsWhatItDid(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModePermitted)
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude <b>"}}}`
	if rec := r.post("camp-1", init, nil); rec.Code != http.StatusOK {
		t.Fatalf("initialize: %d %s", rec.Code, rec.Body)
	}
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"rolling_tables","arguments":{"name":"Loot"}}}`
	rec := r.post("camp-1", call, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Gem") {
		t.Fatalf("call: %d %s", rec.Code, rec.Body)
	}
	calls, _ := r.svc.Activity(context.Background(), "camp-1")
	if len(calls) != 2 {
		t.Fatalf("logged %d calls: %+v", len(calls), calls)
	}
	if calls[1].Summary != "Connected" || calls[1].Note != "Claude b" {
		t.Errorf("connect line = %+v", calls[1])
	}
	if calls[0].Tool != "rolling_tables" || calls[0].Summary != "Read rolling tables: “Loot”" || !calls[0].OK {
		t.Errorf("call line = %+v", calls[0])
	}
	conn, _ := r.svc.Current(context.Background(), "camp-1")
	if conn.LastUsedAt == nil || conn.ClientName != "Claude b" {
		t.Errorf("connection = %+v", conn)
	}
}

func TestNoStream(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModeSafe)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/campaigns/camp-1/mcp", nil)
	rec := httptest.NewRecorder()
	if err := r.ep.NoStream(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
		t.Errorf("got %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}
}

// Wrong keys from one address are cut off; a good key from the same address
// (a hosted AI app's shared one) still gets through.
func TestServe_FailedKeysLimitedPerAddress(t *testing.T) {
	r := newRig(t, aiexport.PrivacyModeSafe)
	wrong := func(req *http.Request, _ string) {
		req.Header.Set("Authorization", "Bearer chai_"+strings.Repeat("B", 43))
	}
	var last int
	for i := 0; i <= failuresPerMinute; i++ {
		last = r.post("camp-1", pingBody, wrong).Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("after %d wrong keys: %d, want 429", failuresPerMinute+1, last)
	}
	if rec := r.post("camp-1", pingBody, nil); rec.Code != http.StatusOK {
		t.Errorf("a good key from the same address: %d", rec.Code)
	}
}

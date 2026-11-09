package maintenance

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func hashOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func gateEcho(s *State) *echo.Echo {
	e := echo.New()
	e.Use(s.Gate())
	ok := func(c *echo.Context) error { return c.String(http.StatusOK, "through") }
	for _, p := range []string{"/", "/healthz", "/health", "/static/app.css", "/api/v1/x", "/campaigns", "/admin/restore", "/admin/other", "/ws"} {
		e.GET(p, ok)
	}
	e.POST("/campaigns", ok)
	return e
}

// TestGateWhoGetsThrough is the contract: once closed, only probes, static
// files and the owning admin's own pages get past.
func TestGateWhoGetsThrough(t *testing.T) {
	const ownerToken = "owner-token-value"
	tests := []struct {
		name    string
		active  bool
		method  string
		path    string
		cookie  string
		accept  string
		want    int
		wantCT  string
		retry   bool
		wantOwn bool
	}{
		{name: "open site serves everyone", active: false, method: "GET", path: "/campaigns", want: 200},
		{name: "closed: page gets the plain page", active: true, method: "GET", path: "/campaigns", want: 503, wantCT: "text/html", retry: true},
		{name: "closed: POST refused", active: true, method: "POST", path: "/campaigns", want: 503, retry: true},
		{name: "closed: api path gets json", active: true, method: "GET", path: "/api/v1/x", want: 503, wantCT: "application/json", retry: true},
		{name: "closed: json accept gets json", active: true, method: "GET", path: "/campaigns", accept: "application/json", want: 503, wantCT: "application/json", retry: true},
		{name: "closed: browser accept with json fallback still gets the page", active: true, method: "GET", path: "/campaigns", accept: "text/html,application/json", want: 503, wantCT: "text/html"},
		{name: "closed: websocket refused", active: true, method: "GET", path: "/ws", want: 503, wantCT: "application/json", retry: true},
		{name: "closed: healthz passes", active: true, method: "GET", path: "/healthz", want: 200},
		{name: "closed: health alias passes", active: true, method: "GET", path: "/health", want: 200},
		{name: "closed: static GET passes", active: true, method: "GET", path: "/static/app.css", want: 200},
		{name: "closed: owner reaches its page", active: true, method: "GET", path: "/admin/restore", cookie: ownerToken, want: 200, wantOwn: true},
		{name: "closed: owner is still refused elsewhere", active: true, method: "GET", path: "/campaigns", cookie: ownerToken, want: 503},
		{name: "closed: owner is refused on an admin path the handler does not claim", active: true, method: "GET", path: "/admin/other", cookie: ownerToken, want: 503},
		{name: "closed: another session cannot use the admin page", active: true, method: "GET", path: "/admin/restore", cookie: "someone-else", want: 503},
		{name: "closed: no cookie cannot use the admin page", active: true, method: "GET", path: "/admin/restore", want: 503},
		{name: "closed: the hash itself is not a credential", active: true, method: "GET", path: "/admin/restore", cookie: hashOf(ownerToken), want: 503},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestState(t)
			served := false
			s.SetOwnerHandler(func(c *echo.Context) (bool, error) {
				if c.Request().URL.Path != "/admin/restore" {
					return false, nil
				}
				served = true
				return true, c.String(http.StatusOK, "owner")
			})
			if tc.active {
				if err := s.Enter(hashOf(ownerToken), "u1", "m.txt"); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			gateEcho(s).ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.wantCT != "" && !strings.Contains(rec.Header().Get("Content-Type"), tc.wantCT) {
				t.Errorf("content-type = %q, want %q", rec.Header().Get("Content-Type"), tc.wantCT)
			}
			if tc.retry && rec.Header().Get("Retry-After") == "" {
				t.Error("503 without Retry-After")
			}
			if served != tc.wantOwn {
				t.Errorf("owner handler served = %v, want %v", served, tc.wantOwn)
			}
		})
	}
}

// TestGateOwnerEndsWithRestore pins that the owner bypass lasts exactly as
// long as the gate is closed.
func TestGateOwnerEndsWithRestore(t *testing.T) {
	s, _ := newTestState(t)
	_ = s.Enter(hashOf("tok"), "u1", "m.txt")
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "tok"})
	if !s.IsOwner(req) {
		t.Fatal("owner not recognised while closed")
	}
	s.Succeed("ok")
	if s.IsOwner(req) {
		t.Error("owner bypass survived the end of the restore")
	}
}

// TestWaitIdle pins the drain used before the script starts.
func TestWaitIdle(t *testing.T) {
	s, _ := newTestState(t)
	if !s.WaitIdle(50 * time.Millisecond) {
		t.Error("idle state reported busy")
	}
	done := s.Track()
	if s.WaitIdle(60 * time.Millisecond) {
		t.Error("busy state reported idle")
	}
	done()
	if !s.WaitIdle(50 * time.Millisecond) {
		t.Error("state still busy after the request ended")
	}
}

// TestSessionHashReadsEitherCookie pins the cookie names the gate reads.
func TestSessionHashReadsEitherCookie(t *testing.T) {
	for _, name := range SessionCookieNames() {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: name, Value: "t"})
		if SessionHash(req) != hashOf("t") {
			t.Errorf("cookie %q not read", name)
		}
	}
	if SessionHash(httptest.NewRequest("GET", "/", nil)) != "" {
		t.Error("hash without a cookie")
	}
}

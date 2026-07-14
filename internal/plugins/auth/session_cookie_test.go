// session_cookie_test.go — the __Host- session cookie prefix (beta hardening):
// over HTTPS the session cookie is __Host--prefixed + Secure and a legacy bare
// cookie is ignored (the one-time re-login); over plain HTTP the bare name is
// kept so local dev works.
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func newCookieCtx(secure bool) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if secure {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSessionCookie_HostPrefix(t *testing.T) {
	t.Run("HTTPS sets the __Host- name, Secure, Path=/", func(t *testing.T) {
		c, rec := newCookieCtx(true)
		setSessionCookie(c, "tok", time.Hour)
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("expected 1 Set-Cookie, got %d", len(cookies))
		}
		ck := cookies[0]
		if ck.Name != "__Host-chronicle_session" {
			t.Errorf("HTTPS cookie name = %q, want __Host-chronicle_session", ck.Name)
		}
		if !ck.Secure {
			t.Error("__Host- cookie must be Secure")
		}
		if ck.Path != "/" {
			t.Errorf("__Host- cookie must have Path=/, got %q", ck.Path)
		}
		if ck.Domain != "" {
			t.Errorf("__Host- cookie must have no Domain, got %q", ck.Domain)
		}
	})

	t.Run("HTTP keeps the bare name, not Secure (dev fallback)", func(t *testing.T) {
		c, rec := newCookieCtx(false)
		setSessionCookie(c, "tok", time.Hour)
		ck := rec.Result().Cookies()[0]
		if ck.Name != "chronicle_session" {
			t.Errorf("HTTP cookie name = %q, want chronicle_session", ck.Name)
		}
		if ck.Secure {
			t.Error("bare dev cookie must not be Secure (would be dropped over http://)")
		}
	})

	t.Run("HTTPS read PREFERS __Host- over a forged/legacy bare cookie", func(t *testing.T) {
		c, _ := newCookieCtx(true)
		c.Request().AddCookie(&http.Cookie{Name: "chronicle_session", Value: "forged"})
		c.Request().AddCookie(&http.Cookie{Name: "__Host-chronicle_session", Value: "real"})
		if got := getSessionToken(c); got != "real" {
			t.Errorf("over HTTPS the __Host- cookie must win over a bare one, got %q", got)
		}
	})

	t.Run("HTTPS falls back to the bare cookie only when no __Host- is present", func(t *testing.T) {
		// A pre-upgrade session (bare only) stays valid over HTTPS via the
		// fallback — no forced re-login — while a logged-in user (who has a
		// __Host- cookie) is unaffected by the fallback.
		c, _ := newCookieCtx(true)
		c.Request().AddCookie(&http.Cookie{Name: "chronicle_session", Value: "legacy"})
		if got := getSessionToken(c); got != "legacy" {
			t.Errorf("HTTPS must fall back to the bare cookie when no __Host- exists, got %q", got)
		}
	})

	t.Run("HTTP read still uses the bare cookie", func(t *testing.T) {
		c, _ := newCookieCtx(false)
		c.Request().AddCookie(&http.Cookie{Name: "chronicle_session", Value: "dev"})
		if got := getSessionToken(c); got != "dev" {
			t.Errorf("over HTTP must read the bare cookie, got %q", got)
		}
	})

	t.Run("logout over HTTPS clears both names", func(t *testing.T) {
		c, rec := newCookieCtx(true)
		clearSessionCookie(c)
		cleared := map[string]bool{}
		for _, ck := range rec.Result().Cookies() {
			if ck.MaxAge < 0 {
				cleared[ck.Name] = true
			}
		}
		if !cleared["__Host-chronicle_session"] || !cleared["chronicle_session"] {
			t.Errorf("logout over HTTPS must expire both cookie names, cleared: %v", cleared)
		}
	})
}

// csrf_test.go — C-AUTH-LOGIN-CSRF-FIX. Pins the double-submit cookie
// lifecycle, the proxy/scheme-flip root-cause fix (the cookie is found under
// either name regardless of how the scheme is derived on the validating
// request), the friendly (no-jargon) 403, and the login auto-recovery path.
package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// runCSRF drives the middleware once and reports whether next() ran, plus the
// recorder + any returned error.
func runCSRF(t *testing.T, req *http.Request) (nextCalled bool, rec *httptest.ResponseRecorder, err error) {
	t.Helper()
	e := echo.New()
	rec = httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := CSRF()(func(c echo.Context) error {
		nextCalled = true
		return c.NoContent(http.StatusOK)
	})
	err = h(c)
	return nextCalled, rec, err
}

// setCookieValue extracts the token the middleware set on a GET, plus the
// cookie name it used.
func issuedCookie(t *testing.T, rec *httptest.ResponseRecorder) (name, value string) {
	t.Helper()
	for _, ck := range rec.Result().Cookies() {
		if strings.Contains(ck.Name, csrfCookieBaseName) {
			return ck.Name, ck.Value
		}
	}
	t.Fatalf("no CSRF cookie was set; headers: %v", rec.Header())
	return "", ""
}

func TestCSRF_GetIssuesCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	next, rec, err := runCSRF(t, req)
	if err != nil {
		t.Fatalf("GET errored: %v", err)
	}
	if !next {
		t.Fatalf("GET should pass through")
	}
	name, val := issuedCookie(t, rec)
	if name != csrfCookieBaseName {
		t.Errorf("plain HTTP should use the bare cookie name; got %q", name)
	}
	if len(val) == 0 {
		t.Errorf("issued cookie has no value")
	}
}

func TestCSRF_HTTPSUsesHostPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	_, rec, _ := runCSRF(t, req)
	name, _ := issuedCookie(t, rec)
	if name != csrfCookieSecureName {
		t.Errorf("HTTPS (via X-Forwarded-Proto) should use the __Host- prefix; got %q", name)
	}
}

func TestCSRF_SetThenSubmitRoundTrip(t *testing.T) {
	// Simulate the GET issuing a token, then a POST that submits it back.
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	_, getRec, _ := runCSRF(t, getReq)
	name, token := issuedCookie(t, getRec)

	form := url.Values{"csrf_token": {token}}
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: name, Value: token})

	next, _, err := runCSRF(t, postReq)
	if err != nil {
		t.Fatalf("valid round-trip errored: %v", err)
	}
	if !next {
		t.Fatalf("valid double-submit should pass")
	}
}

// THE root-cause regression: the GET set the cookie under the bare name (app
// saw HTTP), but the POST arrives with X-Forwarded-Proto=https (app now sees
// HTTPS and would pick the __Host- name). The cookie is still the bare one;
// the fix must find it under either name so the double-submit still matches.
func TestCSRF_SchemeFlip_BareCookie_HTTPSPost(t *testing.T) {
	token := "deadbeef" + strings.Repeat("0", 56)
	form := url.Values{"csrf_token": {token}}
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("X-Forwarded-Proto", "https") // POST sees HTTPS…
	postReq.AddCookie(&http.Cookie{Name: csrfCookieBaseName, Value: token}) // …but cookie is bare-named.

	next, _, err := runCSRF(t, postReq)
	if err != nil {
		t.Fatalf("scheme-flip (bare cookie, https post) should still validate; got: %v", err)
	}
	if !next {
		t.Fatalf("scheme-flip round-trip must pass — this is the login bug")
	}
}

// The reverse flip: cookie set under __Host- (GET saw HTTPS), POST seen as HTTP.
func TestCSRF_SchemeFlip_HostCookie_HTTPPost(t *testing.T) {
	token := "cafebabe" + strings.Repeat("1", 56)
	form := url.Values{"csrf_token": {token}}
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: csrfCookieSecureName, Value: token})

	next, _, err := runCSRF(t, postReq)
	if err != nil {
		t.Fatalf("reverse scheme-flip should validate; got: %v", err)
	}
	if !next {
		t.Fatalf("reverse scheme-flip round-trip must pass")
	}
}

func TestCSRF_XForwardedProtoList(t *testing.T) {
	// "https, http" (proxy chain) — left-most is the client scheme → secure.
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https, http")
	_, rec, _ := runCSRF(t, req)
	if name, _ := issuedCookie(t, rec); name != csrfCookieSecureName {
		t.Errorf("comma-list X-Forwarded-Proto should be treated as HTTPS; got %q", name)
	}
}

// Login POST with a stale/missing token self-heals: HTMX gets HX-Redirect, a
// plain browser gets a 303 — both back to the login GET (which re-issues).
func TestCSRF_LoginRecovery_HTMX(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set("HX-Request", "true")
	next, rec, err := runCSRF(t, req)
	if err != nil {
		t.Fatalf("recovery should not return an error: %v", err)
	}
	if next {
		t.Fatalf("invalid token must not reach the handler")
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/login?expired=1" {
		t.Errorf("HTMX login recovery should HX-Redirect to the form; got %q", got)
	}
}

func TestCSRF_LoginRecovery_PlainBrowser(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	next, rec, err := runCSRF(t, req)
	if err != nil {
		t.Fatalf("recovery should not error: %v", err)
	}
	if next {
		t.Fatalf("invalid token must not reach the handler")
	}
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?expired=1" {
		t.Errorf("plain login recovery should 303 → /login?expired=1; got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

// A non-recoverable mutating path with a bad token gets the friendly 403 — no
// "CSRF" jargon in the user-facing message (the reason stays in logs).
func TestCSRF_FriendlyForbiddenElsewhere(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/campaigns/c1/entities", nil)
	next, _, err := runCSRF(t, req)
	if next {
		t.Fatalf("invalid token must not reach the handler")
	}
	var appErr *apperror.AppError
	if err == nil || !asAppError(err, &appErr) {
		t.Fatalf("expected an AppError; got %v", err)
	}
	if appErr.Code != http.StatusForbidden {
		t.Errorf("expected 403; got %d", appErr.Code)
	}
	if strings.Contains(strings.ToLower(appErr.Message), "csrf") {
		t.Errorf("user-facing message must not leak 'CSRF' jargon: %q", appErr.Message)
	}
	if appErr.Message != CSRFFriendlyMessage {
		t.Errorf("expected the friendly message; got %q", appErr.Message)
	}
}

// API paths are skipped entirely (Bearer/API-key auth, not cookies).
func TestCSRF_SkipsAPIPaths(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maps", nil)
	next, _, err := runCSRF(t, req)
	if err != nil || !next {
		t.Errorf("API paths must skip CSRF; next=%v err=%v", next, err)
	}
}

// asAppError is a tiny errors.As shim kept local so the test file doesn't need
// the errors import just for one call.
func asAppError(err error, target **apperror.AppError) bool {
	if ae, ok := err.(*apperror.AppError); ok {
		*target = ae
		return true
	}
	return false
}

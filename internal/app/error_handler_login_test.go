package app

// error_handler_login_test.go — the 401→login translation rules
// (cordinator#30 r2). A 401 must redirect to /login ONLY when landing there
// helps the user navigate: direct browser requests and HTMX BOOSTED
// navigations. A lazily-loaded FRAGMENT that 401s (e.g. an owner-only
// widget on a public page viewed anonymously) must NOT hijack the whole
// page — it falls through to the toast/no-swap branch.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func runErrorHandler(t *testing.T, hdrs map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/some-fragment", nil)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	(&App{}).errorHandler(apperror.NewUnauthorized("auth required"), c)
	return rec
}

func TestErrorHandler401_FragmentDoesNotHijack(t *testing.T) {
	rec := runErrorHandler(t, map[string]string{"HX-Request": "true"})
	if got := rec.Header().Get("HX-Redirect"); got != "" {
		t.Fatalf("fragment 401 must not HX-Redirect (got %q) — it hijacks public pages", got)
	}
	if rec.Header().Get("HX-Trigger") == "" {
		t.Errorf("fragment 401 should surface as a toast (HX-Trigger missing)")
	}
	if rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("fragment 401 should not swap content (HX-Reswap=none missing)")
	}
}

func TestErrorHandler401_BoostedNavRedirects(t *testing.T) {
	rec := runErrorHandler(t, map[string]string{"HX-Request": "true", "HX-Boosted": "true"})
	if got := rec.Header().Get("HX-Redirect"); got != "/login" {
		t.Fatalf("boosted-nav 401 must HX-Redirect to /login, got %q", got)
	}
}

func TestErrorHandler401_BrowserRedirects(t *testing.T) {
	rec := runErrorHandler(t, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("direct browser 401 must 303 to /login, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

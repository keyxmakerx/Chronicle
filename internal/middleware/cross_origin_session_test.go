package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// An allowed outside origin may read Chronicle's answers, but never with the
// visitor's session cookie: no response tells the browser credentials are
// allowed, on API paths or page paths, for a preflight or a real request.
func TestCORS_NeverAllowsCredentials(t *testing.T) {
	e := echo.New()
	h := CORS(CORSConfig{
		AllowedOrigins: []string{"https://chronicle.example"},
		DynamicOrigins: func() []string { return []string{"https://foundry.example"} },
	})(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	for _, method := range []string{http.MethodOptions, http.MethodGet, http.MethodPost} {
		for _, path := range []string{"/api/v1/campaigns/c/entities", "/campaigns/c/settings", "/admin/users"} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Origin", "https://foundry.example")
			rec := httptest.NewRecorder()
			if err := h(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "https://foundry.example" {
				t.Fatalf("%s %s: the allowed origin lost its CORS answer", method, path)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Errorf("%s %s: Allow-Credentials = %q", method, path, got)
			}
		}
	}
}

// The token is taken from the request body or the header, never from the
// address, where it would be written to logs and sent on in Referer headers.
func TestCSRF_IgnoresTokenInQueryString(t *testing.T) {
	_, getRec, _ := runCSRF(t, httptest.NewRequest(http.MethodGet, "/login", nil))
	name, token := issuedCookie(t, getRec)

	req := httptest.NewRequest(http.MethodPost, "/campaigns/c/settings?csrf_token="+token, nil)
	req.AddCookie(&http.Cookie{Name: name, Value: token})
	next, _, _ := runCSRF(t, req)
	if next {
		t.Fatal("a token in the query string was accepted")
	}
}

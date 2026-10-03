package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// A browser sends a preflight before any request carrying a custom header, and
// refuses the real request unless the header is listed. The Foundry module
// adds X-Chronicle-Module-Version to every call, so leaving it off would break
// all sync from a browser-hosted Foundry.
func TestCORSPreflight_AllowsModuleVersionHeader(t *testing.T) {
	e := echo.New()
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://foundry.example"}})(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/campaigns/c/entities", nil)
	req.Header.Set("Origin", "https://foundry.example")
	rec := httptest.NewRecorder()
	if err := h(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Chronicle-Module-Version") {
		t.Errorf("Allow-Headers = %q, want it to include X-Chronicle-Module-Version", got)
	}
}

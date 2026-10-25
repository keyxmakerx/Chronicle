package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestSecurityHeadersFraming pins who may frame a page: only Chronicle's own
// pages, for a page opened beside another. Every other site, and a request
// that doesn't say where it comes from, is refused.
func TestSecurityHeadersFraming(t *testing.T) {
	cases := []struct {
		name, dest, site, ancestors, options string
	}{
		{"plain page load", "document", "same-origin", "frame-ancestors 'none'", "DENY"},
		{"no fetch headers", "", "", "frame-ancestors 'none'", "DENY"},
		{"framed by another site", "iframe", "cross-site", "frame-ancestors 'none'", "DENY"},
		{"framed by a subdomain", "iframe", "same-site", "frame-ancestors 'none'", "DENY"},
		{"framed by Chronicle itself", "iframe", "same-origin", "frame-ancestors 'self'", "SAMEORIGIN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
			if tc.dest != "" {
				req.Header.Set("Sec-Fetch-Dest", tc.dest)
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			rec := httptest.NewRecorder()
			h := SecurityHeaders()(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
			if err := h(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			csp := rec.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, tc.ancestors) {
				t.Errorf("CSP %q, want %q", csp, tc.ancestors)
			}
			if got := rec.Header().Get("X-Frame-Options"); got != tc.options {
				t.Errorf("X-Frame-Options %q, want %q", got, tc.options)
			}
			if rec.Header().Get("Vary") != "Sec-Fetch-Dest, Sec-Fetch-Site" {
				t.Errorf("Vary %q, want both fetch headers so a cache never serves one answer for the other", rec.Header().Get("Vary"))
			}
		})
	}
}

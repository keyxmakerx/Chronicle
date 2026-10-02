package packages

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestRoutesGuardCodeChangingWrites pins that the package writes which change
// what code the site runs, or delete package files, sit behind reauth.
func TestRoutesGuardCodeChangingWrites(t *testing.T) {
	e := echo.New()
	reauth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusForbidden) }
	}
	// A nil handler is safe: the stub refuses before any handler runs.
	RegisterRoutes(e.Group("/admin"), nil, reauth)

	tests := []struct{ method, path string }{
		{http.MethodPost, "/admin/packages"},
		{http.MethodDelete, "/admin/packages/pkg-1"},
		{http.MethodPut, "/admin/packages/pkg-1/repo"},
		{http.MethodDelete, "/admin/packages/prune"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403 from reauth", rec.Code)
			}
		})
	}
}

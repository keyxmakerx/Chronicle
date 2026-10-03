package syncapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestRoutesGuardSensitiveWrites pins that the writes that change who can reach the API sit behind reauth.
func TestRoutesGuardSensitiveWrites(t *testing.T) {
	e := echo.New()
	reauth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusForbidden) }
	}
	// A nil handler is safe: the stub refuses before any handler runs.
	RegisterAdminRoutes(e.Group("/admin"), nil, reauth)

	tests := []struct{ method, path string }{
		{http.MethodPost, "/admin/api/ip-blocks"},
		{http.MethodDelete, "/admin/api/ip-blocks/b1"},
		{http.MethodPut, "/admin/api/keys/k1/toggle"},
		{http.MethodDelete, "/admin/api/keys/k1"},
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

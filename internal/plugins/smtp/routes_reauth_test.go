package smtp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestRoutesGuardSensitiveWrites pins that the writes that repoint or send outgoing mail sit behind reauth.
func TestRoutesGuardSensitiveWrites(t *testing.T) {
	e := echo.New()
	reauth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusForbidden) }
	}
	// A nil handler is safe: the stub refuses before any handler runs.
	RegisterRoutes(e.Group("/admin"), nil, reauth)

	tests := []struct{ method, path string }{
		{http.MethodPut, "/admin/smtp"},
		{http.MethodPost, "/admin/smtp/send-test"},
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

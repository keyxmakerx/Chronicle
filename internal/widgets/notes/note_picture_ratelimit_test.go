package notes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// The picture route carries its own limit on top of the group's, for the site
// mount and the allowed-app mount alike: both go through registerNoteJSONRoutes.
func TestNotePictureRoute_IsRateLimited(t *testing.T) {
	pass := func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	mounts := []struct {
		name string
		path string
	}{
		{"site mount", "/campaigns/:id"},
		{"app mount", "/api/notes-app/campaigns/:id"},
	}
	for _, m := range mounts {
		t.Run(m.name, func(t *testing.T) {
			e := echo.New()
			registerNoteJSONRoutes(e.Group(m.path), &Handler{}, pass)
			url := m.path[:len(m.path)-len(":id")] + "c1/notes/pictures"

			// The unwired handler errors every time (no error handler here
			// maps it); what matters is that the 31st call is stopped first.
			for i := 1; i <= 31; i++ {
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, url, nil))
				if limited := rec.Code == http.StatusTooManyRequests; limited != (i == 31) {
					t.Fatalf("call %d: status %d, limited=%v", i, rec.Code, limited)
				}
			}
		})
	}
}

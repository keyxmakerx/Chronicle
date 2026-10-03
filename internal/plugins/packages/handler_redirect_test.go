package packages

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestOldPagesRedirectToTabs pins that the retired standalone pages send a
// browser visit to the matching tab. A nil service is safe: the redirect
// answers before any service call.
func TestOldPagesRedirectToTabs(t *testing.T) {
	h := &Handler{}
	tests := []struct {
		name    string
		handler func(echo.Context) error
		want    string
	}{
		{"pending", h.ListPendingSubmissions, "/admin/packages?tab=review"},
		{"settings", h.GetSecuritySettings, "/admin/packages?tab=settings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/admin/packages/"+tt.name, nil), rec)
			if err := tt.handler(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != tt.want {
				t.Errorf("Location = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWritesReturnToCurrentPanel pins that a write made from the open panel
// comes back to it, via HX-Redirect, and ignores a foreign address.
func TestWritesReturnToCurrentPanel(t *testing.T) {
	tests := []struct{ name, current, want string }{
		{"from the panel", "http://h/admin/packages?pkg=p1&ptab=settings", "/admin/packages?pkg=p1&ptab=settings"},
		{"foreign page", "http://h/somewhere/else", "/admin/packages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/admin/packages/p1/auto-update", nil)
			req.Header.Set("HX-Request", "true")
			req.Header.Set("HX-Current-URL", tt.current)
			rec := httptest.NewRecorder()
			if err := (&Handler{}).backToPage(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			if got := rec.Header().Get("HX-Redirect"); got != tt.want {
				t.Errorf("HX-Redirect = %q, want %q", got, tt.want)
			}
		})
	}
}

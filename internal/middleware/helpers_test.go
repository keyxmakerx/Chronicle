package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestIsHTMX pins which htmx requests get a fragment. A boosted navigation
// and a history restore (Back or Forward, which swaps the whole body) both
// need the full page.
func TestIsHTMX(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{"a plain page load", nil, false},
		{"an htmx request for a fragment", map[string]string{"HX-Request": "true"}, true},
		{"a boosted navigation", map[string]string{"HX-Request": "true", "HX-Boosted": "true"}, false},
		{"a history restore", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/campaigns/c1/maps", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			if got := IsHTMX(c); got != tt.want {
				t.Fatalf("IsHTMX = %v, want %v", got, tt.want)
			}
		})
	}
}

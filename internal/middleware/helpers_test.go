package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
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

func TestMediaContext(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	saved := MediaSigner
	t.Cleanup(func() { MediaSigner = saved })

	MediaSigner = nil
	if got := layouts.MediaURL(MediaContext(c), "abc"); got != "/media/abc" {
		t.Fatalf("unsigned fallback = %q", got)
	}

	MediaSigner = func(echo.Context) (layouts.MediaURLFunc, layouts.MediaThumbFunc) {
		return func(id string) string { return "/media/" + id + "?sig=1" },
			func(id, size string) string { return "/media/" + id + "/thumb/" + size + "?sig=1" }
	}
	if got := layouts.AvatarURL(MediaContext(c), "abc"); got != "/media/abc/thumb/300?sig=1" {
		t.Fatalf("signed avatar = %q", got)
	}
}

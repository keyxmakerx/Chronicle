package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/config"
)

// countingReader counts what the server actually read from a request body.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// TestSiteLookBodyLimit pins that the Site look form's size cap is enforced by
// middleware that runs before the CSRF check, which is the first thing to
// parse the multipart body.
func TestSiteLookBodyLimit(t *testing.T) {
	a := &App{Config: &config.Config{}, Echo: echo.New()}
	a.setupMiddleware()
	a.Echo.POST(siteLookPath, func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	a.Echo.POST("/login", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	const mb = 1 << 20
	body := func(n int) string { return strings.Repeat("a", n) }
	tests := []struct {
		name       string
		path       string
		size       int
		wantStatus int // 0: anything but 413.
	}{
		{"site look under its cap passes the limit (it then fails the CSRF check, not the size one)", siteLookPath, 4 * mb, 0},
		{"site look over its cap is refused up front", siteLookPath, 6 * mb, http.StatusRequestEntityTooLarge},
		{"another route keeps the global 2 MB cap", "/login", 3 * mb, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(body(tc.size)))
			req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
			rec := httptest.NewRecorder()
			a.Echo.ServeHTTP(rec, req)
			if tc.wantStatus == 0 {
				if rec.Code == http.StatusRequestEntityTooLarge {
					t.Errorf("a body under the cap was refused as too large")
				}
			} else if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}

	// Without a Content-Length the up-front check can't help; the reader
	// itself must stop at the cap rather than let the parse read it all.
	t.Run("an unsized body is cut off at the cap", func(t *testing.T) {
		cr := &countingReader{r: strings.NewReader(body(12 * mb))}
		req := httptest.NewRequest("POST", siteLookPath, cr)
		req.ContentLength = -1
		req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		a.Echo.ServeHTTP(httptest.NewRecorder(), req)
		if cr.n > 5*mb+64*1024 {
			t.Errorf("server read %d bytes, want about 5 MB at most", cr.n)
		}
	})
}

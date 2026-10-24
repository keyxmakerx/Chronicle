package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestStaticGzipWanted(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/static/js/boot.js", true},
		{"/static/css/app.css", true},
		{"/static/plugins/maps/js/map.MJS", true},
		{"/static/img/favicon.svg", true},
		{"/static/img/logo.png", false},
		{"/static/fonts/x.woff2", false},
		{"/campaigns/abc", false},
		{"/login", false},
		{"/api/v1/things.json", false},
		{"/staticfoo/app.js", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := staticGzipWanted("/static/", tt.path); got != tt.want {
				t.Errorf("staticGzipWanted(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestStaticGzip_CompressesScriptsNotPages(t *testing.T) {
	body := strings.Repeat("var chronicle = 1;\n", 200)
	e := echo.New()
	e.Use(StaticGzip("/static/"))
	e.GET("/static/js/app.js", func(c echo.Context) error { return c.String(http.StatusOK, body) })
	e.GET("/campaigns/x", func(c echo.Context) error { return c.HTML(http.StatusOK, body) })

	for _, tc := range []struct {
		path     string
		wantGzip bool
	}{{"/static/js/app.js", true}, {"/campaigns/x", false}} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		gz := rec.Header().Get("Content-Encoding") == "gzip"
		if gz != tc.wantGzip {
			t.Fatalf("%s: gzip = %v, want %v", tc.path, gz, tc.wantGzip)
		}
		if gz {
			r, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatalf("gzip reader: %v", err)
			}
			got, _ := io.ReadAll(r)
			if string(got) != body {
				t.Errorf("%s: decompressed body differs", tc.path)
			}
		}
	}
}

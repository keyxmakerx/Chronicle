package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestBuildHeader(t *testing.T) {
	tests := []struct {
		name, path string
	}{
		{"page", "/campaigns/c1/calendars/k1"},
		{"static asset", "/static/js/boot.js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, tt.path, nil), rec)
			h := BuildHeader("abc123")(func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
			if err := h(c); err != nil {
				t.Fatal(err)
			}
			if got := rec.Header().Get(BuildHeaderName); got != "abc123" {
				t.Fatalf("%s = %q, want abc123", BuildHeaderName, got)
			}
		})
	}
}

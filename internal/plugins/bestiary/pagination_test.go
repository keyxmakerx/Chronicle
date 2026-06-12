// pagination_test.go — per_page clamping at the handler boundary
// (audit-R2 Finding 3, unbounded pagination). A hostile `per_page` must be
// capped at maxPerPage before it reaches the service/query layer.
package bestiary

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestParsePagination_ClampsPerPage(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantPage    int
		wantPerPage int
	}{
		{"defaults when empty", "", 1, defaultPerPage},
		{"page floor", "page=0", 1, defaultPerPage},
		{"per_page floor", "per_page=0", 1, defaultPerPage},
		{"within bounds preserved", "page=3&per_page=25", 3, 25},
		{"per_page clamped at max", "per_page=1000", 1, maxPerPage},
		{"per_page overflow clamped", "per_page=2147483647", 1, maxPerPage},
		{"exact max preserved", "per_page=50", 1, maxPerPage},
	}

	e := echo.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			page, perPage := parsePagination(c)
			if page != tc.wantPage {
				t.Errorf("page = %d, want %d", page, tc.wantPage)
			}
			if perPage != tc.wantPerPage {
				t.Errorf("perPage = %d, want %d", perPage, tc.wantPerPage)
			}
		})
	}
}

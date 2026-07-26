package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func runStaticCache(t *testing.T, target string, seed func(c echo.Context)) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := StaticCache("/static/")(func(c echo.Context) error {
		if seed != nil {
			seed(c)
		}
		return c.String(http.StatusOK, "body")
	})
	if err := h(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	return rec
}

// TestStaticCache_VersionedIsImmutable: a `?v=` URL is content-addressed, so
// its bytes can never change — cache it hard and skip revalidation entirely.
func TestStaticCache_VersionedIsImmutable(t *testing.T) {
	rec := runStaticCache(t, "/static/css/app.css?v=deadbeef01", nil)
	got := rec.Header().Get(echo.HeaderCacheControl)
	if got != "public, max-age=31536000, immutable" {
		t.Errorf("versioned asset Cache-Control = %q; want the immutable policy", got)
	}
}

// TestStaticCache_UnversionedMustRevalidate is the safety arm: an asset whose
// template we forgot to convert must degrade to "always revalidated", never to
// Echo's default of no Cache-Control at all (which lets browsers apply
// heuristic freshness and serve a build-old copy for hours).
func TestStaticCache_UnversionedMustRevalidate(t *testing.T) {
	rec := runStaticCache(t, "/static/js/boot.js", nil)
	got := rec.Header().Get(echo.HeaderCacheControl)
	if got != "public, max-age=0, must-revalidate" {
		t.Errorf("unversioned asset Cache-Control = %q; want the revalidate policy", got)
	}
}

// TestStaticCache_IgnoresNonStaticPaths: the middleware is registered globally
// (it has to cover both the on-disk mount and every plugin embed mount), so it
// must be inert everywhere else.
func TestStaticCache_IgnoresNonStaticPaths(t *testing.T) {
	for _, target := range []string{"/campaigns/abc/calendar/v2", "/media/uploads/x.png?v=1", "/staticky/x.js"} {
		rec := runStaticCache(t, target, nil)
		if got := rec.Header().Get(echo.HeaderCacheControl); got != "" {
			t.Errorf("%s: Cache-Control = %q; want the middleware to stay out of it", target, got)
		}
	}
}

// TestStaticCache_HandlerPolicyWins: the middleware writes a DEFAULT before the
// handler runs, so a handler under /static with its own caching decision
// (package downloads, signed assets) overwrites it and wins.
func TestStaticCache_HandlerPolicyWins(t *testing.T) {
	rec := runStaticCache(t, "/static/plugins/calendar/js/event_grid.js", func(c echo.Context) {
		c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
	})
	if got := rec.Header().Get(echo.HeaderCacheControl); got != "no-store" {
		t.Errorf("Cache-Control = %q; want the handler's own no-store preserved", got)
	}
}

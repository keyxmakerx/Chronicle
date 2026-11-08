package middleware

import (
	"strings"

	"github.com/labstack/echo/v5"
)

// StaticCache sets an explicit Cache-Control policy on static asset
// responses. Echo's `e.Static` serves via `http.ServeContent`, which emits
// Last-Modified but no Cache-Control, letting browsers reuse a stale file
// for hours after a deploy. See layouts.AssetURL for the `?v=`
// cache-busting token this depends on: with `?v=` the URL is
// content-addressed, so cache hard and forever (`immutable`); without it
// (hand-typed URL, legacy link, forgotten conversion), allow caching but
// force revalidation so a stale copy is at worst one 304 away from correct.
//
// Applied as a default before the handler runs — requests outside urlPrefix
// pass through, and a handler's own Cache-Control wins afterwards.
// Registered globally, not on a route group, because static assets are
// mounted in two places (`/static` and each plugin's `/static/plugins/<slug>`).
func StaticCache(urlPrefix string) echo.MiddlewareFunc {
	const (
		immutablePolicy  = "public, max-age=31536000, immutable"
		revalidatePolicy = "public, max-age=0, must-revalidate"
	)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !strings.HasPrefix(c.Request().URL.Path, urlPrefix) {
				return next(c)
			}
			h := c.Response().Header()
			if h.Get(echo.HeaderCacheControl) == "" {
				if c.QueryParam("v") != "" {
					h.Set(echo.HeaderCacheControl, immutablePolicy)
				} else {
					h.Set(echo.HeaderCacheControl, revalidatePolicy)
				}
			}
			return next(c)
		}
	}
}

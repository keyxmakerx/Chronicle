package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// StaticCache sets an explicit Cache-Control policy on static asset responses.
//
// WHY. Echo's `e.Static` serves through `http.ServeContent`, which emits
// Last-Modified but no Cache-Control at all. With no explicit directive
// browsers apply HEURISTIC freshness (commonly ~10% of the resource's age),
// so a long-untouched file like `/static/js/boot.js` can be reused for hours
// without a single revalidation. That is the "the site looks old after a
// deploy" class of bug — see layouts.AssetURL's header comment for the
// concrete calendar failure it produced.
//
// The policy has two arms, and it is the `?v=` token from layouts.AssetURL
// that picks between them:
//
//   - `?v=` present → the URL is content-addressed, so the bytes behind it can
//     never change. Cache hard and forever: `immutable` additionally tells the
//     browser not to revalidate even on an explicit reload.
//   - no `?v=` → a hand-typed URL, a legacy link, or a direct fetch. Allow
//     caching but force revalidation on every use, so a stale copy is at worst
//     one conditional request (304, no body) away from correct. This is the
//     arm that makes the whole scheme SAFE: an asset whose template we forgot
//     to convert degrades to "always revalidated", never to "silently stale".
//
// The policy is applied as a DEFAULT, before the handler runs: requests outside
// urlPrefix pass straight through, and any handler that sets its own
// Cache-Control afterwards overwrites this one and wins.
//
// Registered globally rather than on a route group because static assets are
// mounted in two places — the on-disk `/static` root and each plugin's embed FS
// under `/static/plugins/<slug>` — and one prefix check covers both.
func StaticCache(urlPrefix string) echo.MiddlewareFunc {
	const (
		immutablePolicy  = "public, max-age=31536000, immutable"
		revalidatePolicy = "public, max-age=0, must-revalidate"
	)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
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

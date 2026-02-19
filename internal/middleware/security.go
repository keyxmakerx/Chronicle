package middleware

import (
	"github.com/labstack/echo/v4"
)

// SecurityHeaders returns middleware that sets security-related HTTP headers
// on every response. These headers protect against common web attacks even
// if application-level vulnerabilities exist.
//
// Since Chronicle runs behind Cosmos Cloud's reverse proxy, TLS is handled
// externally. These headers provide defense-in-depth at the application layer.
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()

			// Content-Security-Policy: restrict what resources the browser can load.
			// 'self' allows resources from the same origin only.
			// 'unsafe-inline' is needed for Alpine.js x-* attributes and inline styles.
			// 'unsafe-eval' is needed for Alpine.js expressions.
			// Google Fonts + Font Awesome CDN are explicitly allowed.
			h.Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
					"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com; "+
					"img-src 'self' data: blob:; "+
					"font-src 'self' https://fonts.gstatic.com https://cdnjs.cloudflare.com; "+
					"connect-src 'self'; "+
					"frame-ancestors 'none'; "+
					"base-uri 'self'; "+
					"form-action 'self'",
			)

			// Strict-Transport-Security: enforce HTTPS for 1 year including subdomains.
			// Chronicle runs behind a reverse proxy that terminates TLS; this header
			// tells browsers to always use HTTPS for subsequent requests.
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

			// X-Content-Type-Options: prevent MIME type sniffing.
			h.Set("X-Content-Type-Options", "nosniff")

			// X-Frame-Options: prevent clickjacking (redundant with CSP frame-ancestors
			// but some older browsers only support this header).
			h.Set("X-Frame-Options", "DENY")

			// Referrer-Policy: limit referrer information leaked to external sites.
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Permissions-Policy: disable browser features we don't use.
			h.Set("Permissions-Policy",
				"camera=(), microphone=(), geolocation=(), payment=()",
			)

			// X-XSS-Protection: legacy header for older browsers. Modern browsers
			// use CSP instead, but this doesn't hurt.
			h.Set("X-XSS-Protection", "1; mode=block")

			return next(c)
		}
	}
}

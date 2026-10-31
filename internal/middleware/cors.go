package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// CORSConfig holds configuration for the CORS middleware.
type CORSConfig struct {
	// AllowedOrigins is the list of origins permitted to make cross-origin
	// requests. Use ["*"] to allow all (not recommended for production).
	// Example: ["https://chronicle.yourdomain.com", "http://localhost:3000"]
	AllowedOrigins []string

	// DynamicOrigins is an optional function that returns additional allowed
	// origins at runtime. Called per-request to support admin-managed origin
	// whitelists stored in the database. May return nil.
	DynamicOrigins func() []string
}

// CORS returns middleware that handles Cross-Origin Resource Sharing headers.
// Primarily needed for the REST API (/api/v1/*) when external clients like
// the Foundry VTT module make requests from a different origin; the web UI
// is same-origin and doesn't need it.
//
// It never allows credentials: outside clients authenticate with a Bearer
// key, and a page on an allowed origin must not be able to read Chronicle
// pages, or post to them, with the visitor's session cookie.
func CORS(cfg CORSConfig) echo.MiddlewareFunc {
	// Build a set for fast origin lookup of static origins.
	allowAll := false
	staticOrigins := make(map[string]bool)
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		staticOrigins[o] = true
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()
			origin := req.Header.Get("Origin")

			// No Origin header means same-origin request -- skip CORS.
			if origin == "" {
				return next(c)
			}

			// Check if the origin is allowed: static list first, then dynamic.
			allowed := allowAll || staticOrigins[origin]
			if !allowed && cfg.DynamicOrigins != nil {
				for _, o := range cfg.DynamicOrigins() {
					if o == origin {
						allowed = true
						break
					}
				}
			}
			if !allowed {
				// Origin not in whitelist -- proceed without CORS headers.
				// The browser will block the response on the client side.
				return next(c)
			}

			// Set CORS response headers.
			res.Header().Set("Access-Control-Allow-Origin", origin)
			res.Header().Set("Vary", "Origin")

			// Handle preflight OPTIONS requests.
			if req.Method == http.MethodOptions {
				res.Header().Set("Access-Control-Allow-Methods",
					strings.Join([]string{
						http.MethodGet,
						http.MethodPost,
						http.MethodPut,
						http.MethodPatch,
						http.MethodDelete,
						http.MethodOptions,
					}, ", "))

				res.Header().Set("Access-Control-Allow-Headers",
					strings.Join([]string{
						"Content-Type",
						"Authorization",
						"X-Requested-With",
						"X-Device-Fingerprint",
						"X-Chronicle-Module-Version",
						"HX-Request",
						"HX-Current-URL",
						"HX-Target",
						"HX-Trigger",
					}, ", "))

				// Cache preflight response for 1 hour to reduce preflight requests.
				res.Header().Set("Access-Control-Max-Age", "3600")

				return c.NoContent(http.StatusNoContent)
			}

			// Expose specific headers so JS can read them from cross-origin responses.
			res.Header().Set("Access-Control-Expose-Headers",
				strings.Join([]string{
					"HX-Redirect",
					"HX-Refresh",
					"HX-Trigger",
					"HX-Trigger-After-Settle",
					// A Foundry world reads these to wait out a 429.
					"Retry-After",
					"X-RateLimit-Limit",
					"X-RateLimit-Remaining",
				}, ", "))

			return next(c)
		}
	}
}

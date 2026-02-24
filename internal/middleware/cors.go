package middleware

import (
	"log/slog"
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

	// AllowCredentials indicates whether the browser should include cookies
	// and auth headers in cross-origin requests. Required for session-based
	// auth from a different origin (e.g., Foundry VTT module).
	AllowCredentials bool
}

// CORS returns middleware that handles Cross-Origin Resource Sharing headers.
//
// This is primarily needed for the REST API (/api/v1/*) when external clients
// like the Foundry VTT module make requests from a different origin. The main
// web UI is same-origin and doesn't need CORS.
//
// For the web UI behind Cosmos reverse proxy, CORS is not strictly needed since
// all requests are same-origin. But the API must support cross-origin access for
// external integrations.
func CORS(cfg CORSConfig) echo.MiddlewareFunc {
	// Build a set for fast origin lookup.
	allowAll := false
	originSet := make(map[string]bool)
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		originSet[o] = true
	}

	// SECURITY: Wildcard origin with credentials is a dangerous misconfiguration.
	// It allows any website to make authenticated requests to the API. Refuse to
	// send credentials when the origin is a wildcard.
	if allowAll && cfg.AllowCredentials {
		slog.Warn("CORS misconfiguration: AllowedOrigins=['*'] with AllowCredentials=true is insecure — credentials will NOT be sent for wildcard origins. Specify explicit origins instead.")
		cfg.AllowCredentials = false
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

			// Check if the origin is allowed.
			allowed := allowAll || originSet[origin]
			if !allowed {
				// Origin not in whitelist -- proceed without CORS headers.
				// The browser will block the response on the client side.
				return next(c)
			}

			// Set CORS response headers.
			res.Header().Set("Access-Control-Allow-Origin", origin)
			res.Header().Set("Vary", "Origin")

			if cfg.AllowCredentials {
				res.Header().Set("Access-Control-Allow-Credentials", "true")
			}

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
				}, ", "))

			return next(c)
		}
	}
}

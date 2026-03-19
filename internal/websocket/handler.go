package websocket

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	gorillaWs "github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DynamicOrigins is an optional function that returns additional allowed
// origins at runtime (e.g., from the admin CORS whitelist in the database).
type DynamicOrigins func() []string

// newUpgrader creates a WebSocket upgrader that validates the Origin header
// against the configured allowed origins plus any dynamic origins. Requests
// authenticated with an API key (Foundry VTT) bypass origin checks since
// they already prove authorization.
func newUpgrader(allowedOrigins []string, dynamicOrigins DynamicOrigins) gorillaWs.Upgrader {
	// Pre-parse static allowed origins for efficient comparison.
	parsedOrigins := make([]string, 0, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			// Normalize to scheme + host (strip path/query).
			parsedOrigins = append(parsedOrigins, strings.ToLower(u.Scheme+"://"+u.Host))
		}
	}

	return gorillaWs.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			// API key auth (Foundry VTT) — origin varies by deployment, but
			// the token query param already proves authorization.
			if r.URL.Query().Get("token") != "" {
				return true
			}

			origin := r.Header.Get("Origin")
			if origin == "" {
				// No Origin header: same-origin request or non-browser client.
				return true
			}

			originLower := strings.ToLower(origin)
			for _, allowed := range parsedOrigins {
				if originLower == allowed {
					return true
				}
			}

			// Check dynamic origins (admin CORS whitelist from database).
			if dynamicOrigins != nil {
				for _, o := range dynamicOrigins() {
					if u, err := url.Parse(o); err == nil && u.Host != "" {
						if originLower == strings.ToLower(u.Scheme+"://"+u.Host) {
							return true
						}
					}
				}
			}

			slog.Warn("ws: origin rejected",
				slog.String("origin", origin),
				slog.String("remote", r.RemoteAddr),
			)
			return false
		},
	}
}

// Authenticator resolves a WebSocket upgrade request into campaign/user identity.
// Implemented by the syncapi plugin for API key auth and by the auth plugin
// for browser session auth.
type Authenticator interface {
	// AuthenticateWS extracts campaign ID, user ID, source, and role from
	// the upgrade request. Returns an error if authentication fails.
	AuthenticateWS(r *http.Request) (campaignID, userID, source string, role int, err error)
}

// HandleUpgrade returns an Echo handler that upgrades HTTP connections to WebSocket
// and registers them with the hub. Authentication is delegated to the Authenticator.
// The allowedOrigins parameter specifies which origins are permitted for browser
// clients (typically the app's BaseURL). The optional dynamicOrigins function
// provides additional origins from the admin CORS whitelist at runtime.
// API-key-authenticated connections bypass origin checks.
func HandleUpgrade(hub *Hub, auth Authenticator, allowedOrigins []string, dynamicOrigins DynamicOrigins) echo.HandlerFunc {
	upgrader := newUpgrader(allowedOrigins, dynamicOrigins)

	return func(c echo.Context) error {
		r := c.Request()

		campaignID, userID, source, role, err := auth.AuthenticateWS(r)
		if err != nil {
			slog.Warn("ws: auth failed",
				slog.Any("error", err),
				slog.String("remote", r.RemoteAddr),
			)
			return apperror.NewUnauthorized("authentication required")
		}

		conn, err := upgrader.Upgrade(c.Response(), r, nil)
		if err != nil {
			slog.Error("ws: upgrade failed",
				slog.Any("error", err),
				slog.String("remote", r.RemoteAddr),
			)
			return nil // Upgrade already sent HTTP response.
		}

		hub.RegisterClient(conn, campaignID, userID, source, role)
		return nil
	}
}

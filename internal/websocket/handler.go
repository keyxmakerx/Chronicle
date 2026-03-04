package websocket

import (
	"log/slog"
	"net/http"

	gorillaWs "github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// upgrader configures the WebSocket upgrade with permissive CORS for dev.
// In production, CheckOrigin should validate against the configured base URL.
var upgrader = gorillaWs.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now; Foundry VTT connects from different hosts.
		// API key auth on the connection provides security.
		return true
	},
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
func HandleUpgrade(hub *Hub, auth Authenticator) echo.HandlerFunc {
	return func(c echo.Context) error {
		r := c.Request()

		campaignID, userID, source, role, err := auth.AuthenticateWS(r)
		if err != nil {
			slog.Warn("ws: auth failed",
				slog.Any("error", err),
				slog.String("remote", r.RemoteAddr),
			)
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
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

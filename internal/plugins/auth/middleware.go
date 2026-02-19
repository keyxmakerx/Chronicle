package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Context keys for storing session data in Echo context. Other plugins
// use these keys (via the exported getter functions below) to access
// the authenticated user's information.
const (
	contextKeySession = "auth_session"
	contextKeyUserID  = "auth_user_id"
)

// RequireAuth returns middleware that validates the session cookie and
// injects session data into the request context. If the session is
// invalid or missing, it redirects browsers to /login or returns 401
// for API requests.
func RequireAuth(service AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := getSessionToken(c)
			if token == "" {
				return handleUnauthenticated(c)
			}

			session, err := service.ValidateSession(c.Request().Context(), token)
			if err != nil {
				// Invalid or expired session -- clear the stale cookie.
				clearSessionCookie(c)
				return handleUnauthenticated(c)
			}

			// Store session data in context for downstream handlers.
			c.Set(contextKeySession, session)
			c.Set(contextKeyUserID, session.UserID)

			return next(c)
		}
	}
}

// handleUnauthenticated returns the appropriate response for unauthenticated
// requests: redirect for browsers, 401 JSON for API clients.
func handleUnauthenticated(c echo.Context) error {
	// API requests get a JSON 401 response.
	if isAPIRequest(c) {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error":   "unauthorized",
			"message": "authentication required",
		})
	}

	// HTMX requests get a redirect header so the full page navigates.
	if isHTMXRequest(c) {
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.NoContent(http.StatusNoContent)
	}

	// Regular browser requests get a 303 redirect to login.
	return c.Redirect(http.StatusSeeOther, "/login")
}

// --- Exported getters for other plugins ---

// GetSession retrieves the authenticated session from the Echo context.
// Returns nil if the request is not authenticated (middleware not applied).
func GetSession(c echo.Context) *Session {
	session, ok := c.Get(contextKeySession).(*Session)
	if !ok {
		return nil
	}
	return session
}

// GetUserID retrieves the authenticated user's ID from the Echo context.
// Returns empty string if the request is not authenticated.
func GetUserID(c echo.Context) string {
	id, ok := c.Get(contextKeyUserID).(string)
	if !ok {
		return ""
	}
	return id
}

// --- Helpers ---

// isAPIRequest returns true if the request targets the /api/ path.
func isAPIRequest(c echo.Context) bool {
	path := c.Request().URL.Path
	return len(path) >= 4 && path[:4] == "/api"
}

// isHTMXRequest returns true if the request was made by HTMX.
func isHTMXRequest(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

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
//
// The JSON message is deliberately action-oriented — a widget showing the
// server's message field needs a string the end-user can act on, not the
// bare word "unauthorized". In-app widgets usually hit this when their
// session quietly expires mid-session, so "reload" is the correct next
// step for the user.
func handleUnauthenticated(c echo.Context) error {
	// API requests get a JSON 401 response.
	if isAPIRequest(c) {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error":   "unauthorized",
			"message": "Your session has ended. Please reload the page and sign in again.",
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

// OptionalAuth returns middleware that loads the session if a valid cookie
// exists, but does NOT reject unauthenticated requests. Use this on routes
// that should work both with and without authentication (e.g., public campaign
// pages where logged-in users see more features than guests).
func OptionalAuth(service AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := getSessionToken(c)
			if token != "" {
				session, err := service.ValidateSession(c.Request().Context(), token)
				if err == nil {
					c.Set(contextKeySession, session)
					c.Set(contextKeyUserID, session.UserID)
				} else {
					clearSessionCookie(c)
				}
			}
			// Proceed regardless of auth state.
			return next(c)
		}
	}
}

// --- Exported helpers for other plugins ---

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

// GetSessionTokenFromCookie reads the raw session token from the cookie on
// the incoming request. Exported so that other plugins which implement
// their own session-aware middleware (e.g. the syncapi multi-auth
// middleware that lets in-app widgets authenticate with a cookie instead
// of a Bearer token) can read the cookie without reaching into unexported
// helpers.
func GetSessionTokenFromCookie(c echo.Context) string {
	return getSessionToken(c)
}

// SetSession stores an authenticated session on the Echo context, using the
// same keys that RequireAuth populates. Exported so other plugins that
// run their own session validation (see syncapi.RequireAuthOrAPIKey) can
// advertise the session to downstream code — anything calling GetSession /
// GetUserID afterwards sees a uniform result regardless of which middleware
// did the validation.
func SetSession(c echo.Context, session *Session) {
	c.Set(contextKeySession, session)
	c.Set(contextKeyUserID, session.UserID)
}

// RequireSiteAdmin returns middleware that ensures the user has the site-wide
// is_admin flag set. Must be applied AFTER RequireAuth.
//
// Used by: admin plugin routes, SMTP settings routes.
func RequireSiteAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			session := GetSession(c)
			if session == nil {
				return handleUnauthenticated(c)
			}
			if !session.IsAdmin {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": "admin access required",
				})
			}
			return next(c)
		}
	}
}

// RequireReauth returns middleware that ensures the admin has recently confirmed
// their password (within the 5-minute reauth window). Must be applied AFTER
// RequireAuth. If reauth has not been confirmed, returns 403 with an
// HX-Trigger header that tells the client JS to show the password modal.
func RequireReauth(service AuthService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			session := GetSession(c)
			if session == nil {
				return handleUnauthenticated(c)
			}

			valid, err := service.IsReauthValid(c.Request().Context(), session.UserID)
			if err != nil {
				return err
			}
			if !valid {
				// Signal to HTMX that the reauth modal should be shown.
				c.Response().Header().Set("HX-Trigger", "reauth-required")
				return c.JSON(http.StatusForbidden, map[string]string{
					"error":   "reauth_required",
					"message": "please confirm your password to continue",
				})
			}

			return next(c)
		}
	}
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

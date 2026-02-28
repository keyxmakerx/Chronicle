package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// csrfTokenLength is the number of random bytes in a CSRF token (32 bytes = 64 hex chars).
const csrfTokenLength = 32

// csrfCookieName is the name of the cookie that stores the CSRF token.
const csrfCookieName = "chronicle_csrf"

// csrfHeaderName is the header that HTMX sends the CSRF token in.
const csrfHeaderName = "X-CSRF-Token"

// csrfFormField is the hidden form field name for non-HTMX form submissions.
const csrfFormField = "csrf_token"

// CSRF returns middleware that implements the double-submit cookie pattern
// for CSRF protection on all state-changing requests (POST, PUT, PATCH, DELETE).
//
// How it works:
//  1. On every request, if no CSRF cookie exists, generate one and set it.
//  2. On mutating requests, compare the cookie value with either:
//     - The X-CSRF-Token header (for HTMX/AJAX requests)
//     - The csrf_token form field (for traditional form submissions)
//  3. If they don't match, reject with 403 Forbidden.
//
// HTMX integration: Configure HTMX to send the cookie value as a header:
//
//	document.addEventListener('htmx:configRequest', function(evt) {
//	    const token = getCookie('chronicle_csrf');
//	    if (token) evt.detail.headers['X-CSRF-Token'] = token;
//	});
func CSRF() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Skip CSRF for API routes. They use Bearer token authentication
			// (not cookies), so they are not vulnerable to CSRF attacks. External
			// clients (Foundry VTT) cannot obtain a CSRF cookie.
			if strings.HasPrefix(req.URL.Path, "/api/") {
				return next(c)
			}

			// Ensure a CSRF token cookie exists.
			cookie, err := req.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				// Generate a new CSRF token and set it as a cookie.
				token, genErr := generateCSRFToken()
				if genErr != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate CSRF token")
				}

				c.SetCookie(&http.Cookie{
					Name:     csrfCookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: false, // Must be readable by JS for HTMX to send it.
					Secure:   req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https",
					SameSite: http.SameSiteLaxMode,
				})

				// Store token in context for templates to access.
				c.Set("csrf_token", token)
			} else {
				c.Set("csrf_token", cookie.Value)
			}

			// Skip validation for safe (non-mutating) HTTP methods.
			if isSafeMethod(req.Method) {
				return next(c)
			}

			// Validate CSRF token on mutating requests.
			cookieToken := ""
			if cookie != nil {
				cookieToken = cookie.Value
			} else {
				// We just set the cookie above, use the generated value.
				if ct, ok := c.Get("csrf_token").(string); ok {
					cookieToken = ct
				}
			}

			// Check header first (HTMX/AJAX), then form field (traditional forms).
			submittedToken := req.Header.Get(csrfHeaderName)
			if submittedToken == "" {
				submittedToken = req.FormValue(csrfFormField)
			}

			// Use constant-time comparison to prevent timing side-channel attacks
			// that could allow an attacker to deduce the token byte-by-byte.
			if submittedToken == "" || subtle.ConstantTimeCompare([]byte(submittedToken), []byte(cookieToken)) != 1 {
				return echo.NewHTTPError(http.StatusForbidden, "invalid or missing CSRF token")
			}

			return next(c)
		}
	}
}

// isSafeMethod returns true for HTTP methods that should not change state.
func isSafeMethod(method string) bool {
	return method == http.MethodGet ||
		method == http.MethodHead ||
		method == http.MethodOptions
}

// generateCSRFToken generates a cryptographically random hex-encoded token.
func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GetCSRFToken retrieves the CSRF token from the Echo context.
// Use this in Templ templates to inject the token into forms.
func GetCSRFToken(c echo.Context) string {
	if token, ok := c.Get("csrf_token").(string); ok {
		return token
	}
	return ""
}

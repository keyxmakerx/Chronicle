package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// csrfTokenLength is the number of random bytes in a CSRF token (32 bytes = 64 hex chars).
const csrfTokenLength = 32

// csrfCookieBaseName is the base name of the CSRF cookie. When served over
// HTTPS the cookie uses the __Host- prefix to prevent subdomain cookie
// injection attacks. Over plain HTTP (development) the base name is used
// without the prefix.
const csrfCookieBaseName = "chronicle_csrf"

// csrfCookieSecureName is the prefixed name used over HTTPS.
const csrfCookieSecureName = "__Host-chronicle_csrf"

// csrfHeaderName is the header that HTMX sends the CSRF token in.
const csrfHeaderName = "X-CSRF-Token"

// csrfFormField is the hidden form field name for non-HTMX form submissions.
const csrfFormField = "csrf_token"

// CSRFFriendlyMessage is the human-readable text shown to users when CSRF
// validation fails. It deliberately avoids the "CSRF" jargon (the precise
// reason is logged, not surfaced) and points at the real fix: reload. Shared
// by the middleware (the 403 body) and the login page's recovery banner.
const CSRFFriendlyMessage = "Your session expired or this page was open too long. Please reload and sign in again."

// schemeIsSecure reports whether the ORIGINAL client connection was HTTPS,
// tolerant of a TLS-terminating proxy. req.TLS is set only when Go itself
// terminated TLS; behind a proxy the signal is X-Forwarded-Proto, which can be
// a comma-separated list ("https, http" — left-most is the client) and is
// matched case-insensitively. Getting this wrong is the root cause of the
// login CSRF failure: an inconsistent reading flips the cookie name between
// the GET that sets the token and the POST that validates it.
func schemeIsSecure(req *http.Request) bool {
	if req.TLS != nil {
		return true
	}
	if xfp := req.Header.Get("X-Forwarded-Proto"); xfp != "" {
		first := xfp
		if i := strings.IndexByte(xfp, ','); i >= 0 {
			first = xfp[:i]
		}
		if strings.EqualFold(strings.TrimSpace(first), "https") {
			return true
		}
	}
	return false
}

// readExistingCSRF returns the value of whichever CSRF cookie the browser
// actually sent, checking BOTH the __Host- (HTTPS) and bare (HTTP) names. This
// is the load-bearing fix: behind a proxy the scheme we derive on the POST can
// differ from the one on the GET that set the cookie, so picking a single name
// by the current scheme can miss a cookie that's right there under the other
// name — which made the double-submit compare the form token against a
// freshly-generated value and 403 every fresh login. Prefer the prefixed name
// (the more-secure one) when both are somehow present.
func readExistingCSRF(req *http.Request) string {
	for _, name := range []string{csrfCookieSecureName, csrfCookieBaseName} {
		if ck, err := req.Cookie(name); err == nil && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

// isCSRFRecoverablePath reports whether a failed mutating request can
// self-heal by bouncing the user back to a GET that re-issues a fresh token.
// The login page is the one that matters (operator's flow): its GET re-sets
// the cookie and renders a matching hidden field, so a stale token reloads
// into a working form instead of dead-ending on an error page.
func isCSRFRecoverablePath(p string) bool {
	return p == "/login"
}

// csrfCookieName returns the appropriate cookie name based on whether the
// connection is secure. The __Host- prefix enforces Secure, no Domain, Path=/
// at the browser level, preventing subdomain cookie injection.
func csrfCookieName(isSecure bool) string {
	if isSecure {
		return csrfCookieSecureName
	}
	return csrfCookieBaseName
}

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
// The cookie name uses the __Host- prefix over HTTPS for defense against
// subdomain cookie injection. Over plain HTTP (dev only) the prefix is omitted.
func CSRF() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Skip CSRF for API routes and WebSocket upgrades. They use Bearer
			// token / API key authentication (not cookies), so they are not
			// vulnerable to CSRF attacks. External clients (Foundry VTT) cannot
			// obtain a CSRF cookie.
			if strings.HasPrefix(req.URL.Path, "/api/") || req.URL.Path == "/ws" {
				return next(c)
			}

			isSecure := schemeIsSecure(req)

			// The authoritative cookie value is whatever the browser actually
			// sent under EITHER name (see readExistingCSRF) — not the value
			// under the single name the current scheme would pick, which can
			// differ from the GET that set it behind a proxy.
			cookieToken := readExistingCSRF(req)
			if cookieToken == "" {
				// First visit (or genuinely no cookie): mint one and set it
				// under the scheme-appropriate name (__Host- over HTTPS).
				token, genErr := generateCSRFToken()
				if genErr != nil {
					return apperror.NewInternal(fmt.Errorf("failed to generate CSRF token"))
				}
				c.SetCookie(&http.Cookie{
					Name:     csrfCookieName(isSecure),
					Value:    token,
					Path:     "/", // __Host- requires Path=/ and no Domain (omitted).
					HttpOnly: false, // Must be readable by JS for HTMX to send it.
					Secure:   isSecure,
					SameSite: http.SameSiteLaxMode,
				})
				cookieToken = token
			}
			// Store the token for templates to render into the hidden field.
			c.Set("csrf_token", cookieToken)

			// Skip validation for safe (non-mutating) HTTP methods.
			if isSafeMethod(req.Method) {
				return next(c)
			}

			// Check header first (HTMX/AJAX), then form field (traditional forms).
			submittedToken := req.Header.Get(csrfHeaderName)
			if submittedToken == "" {
				submittedToken = req.FormValue(csrfFormField)
			}

			// Use constant-time comparison to prevent timing side-channel attacks
			// that could allow an attacker to deduce the token byte-by-byte.
			if submittedToken == "" || subtle.ConstantTimeCompare([]byte(submittedToken), []byte(cookieToken)) != 1 {
				// Keep the precise reason in logs (not in the user message).
				slog.Warn("csrf validation failed",
					slog.String("path", req.URL.Path),
					slog.String("method", req.Method),
					slog.Bool("secure", isSecure),
					slog.Bool("had_cookie", cookieToken != ""),
					slog.Bool("had_submitted_token", submittedToken != ""),
				)

				// Self-heal on the login page: bounce to its GET, which
				// re-issues a fresh cookie + matching hidden field, instead of
				// dead-ending on an error page. HTMX posts need HX-Redirect to
				// trigger a real navigation (a body swap wouldn't reload).
				if isCSRFRecoverablePath(req.URL.Path) {
					target := req.URL.Path + "?expired=1"
					if req.Header.Get("HX-Request") == "true" {
						c.Response().Header().Set("HX-Redirect", target)
						return c.NoContent(http.StatusOK)
					}
					return c.Redirect(http.StatusSeeOther, target)
				}

				// Everywhere else: a friendly 403 (no "CSRF" jargon) rendered
				// through the central error surface.
				return apperror.NewForbidden(CSRFFriendlyMessage)
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

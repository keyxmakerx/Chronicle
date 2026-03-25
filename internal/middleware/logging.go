// Package middleware provides HTTP middleware for the Chronicle Echo server.
// Middleware is applied globally (all routes) or per-route group depending
// on the middleware type. See internal/app/routes.go for registration.
package middleware

import (
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// sensitiveParams are query parameter names whose values should be redacted
// from request logs to prevent leaking secrets (e.g. password reset tokens).
var sensitiveParams = []string{
	"token", "key", "password", "secret", "api_key",
	"access_token", "refresh_token", "client_secret", "session",
}

// RequestLogger returns middleware that logs every HTTP request with
// structured fields: method, path, status, latency, and remote IP.
// Uses Go's built-in slog for structured logging.
//
// Sensitive query parameters (token, key, password, secret) are redacted
// to prevent credential leakage in log files.
func RequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)

			// Log after the request completes so we have the status code.
			latency := time.Since(start)
			req := c.Request()
			res := c.Response()

			// Build structured log fields.
			attrs := []slog.Attr{
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.Int("status", res.Status),
				slog.Duration("latency", latency),
				slog.String("remote_ip", c.RealIP()),
			}

			// Include query string if present, redacting sensitive values.
			if req.URL.RawQuery != "" {
				attrs = append(attrs, slog.String("query", redactQuery(req.URL.RawQuery)))
			}

			// Log at different levels based on status code.
			level := slog.LevelInfo
			if res.Status >= 500 {
				level = slog.LevelError
			} else if res.Status >= 400 {
				level = slog.LevelWarn
			}

			slog.LogAttrs(req.Context(), level, "request",
				attrs...,
			)

			return err
		}
	}
}

// redactQuery replaces values of sensitive query parameters with [REDACTED].
func redactQuery(rawQuery string) string {
	parsed, parseErr := url.ParseQuery(rawQuery)
	if parseErr != nil {
		return rawQuery // Don't fail on malformed queries.
	}

	redacted := false
	for _, param := range sensitiveParams {
		for key := range parsed {
			if strings.EqualFold(key, param) {
				parsed.Set(key, "[REDACTED]")
				redacted = true
			}
		}
	}

	if !redacted {
		return rawQuery // Avoid re-encoding when nothing changed.
	}
	return parsed.Encode()
}

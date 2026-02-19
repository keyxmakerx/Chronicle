// Package middleware provides HTTP middleware for the Chronicle Echo server.
// Middleware is applied globally (all routes) or per-route group depending
// on the middleware type. See internal/app/routes.go for registration.
package middleware

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
)

// RequestLogger returns middleware that logs every HTTP request with
// structured fields: method, path, status, latency, and remote IP.
// Uses Go's built-in slog for structured logging.
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

			// Include query string if present.
			if req.URL.RawQuery != "" {
				attrs = append(attrs, slog.String("query", req.URL.RawQuery))
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

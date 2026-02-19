// Package app is the application bootstrap and dependency injection root.
// It creates and holds all shared infrastructure (DB pool, Redis client,
// Echo instance) and wires together all plugins, modules, and widgets.
package app

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/templates/pages"
)

// App holds all shared dependencies and the Echo HTTP server instance.
// Created once at startup in main.go and used to register all routes.
type App struct {
	// Config holds the loaded application configuration.
	Config *config.Config

	// DB is the MariaDB connection pool shared by all plugins.
	DB *sql.DB

	// Redis is the Redis client shared for sessions, caching, rate limiting.
	Redis *redis.Client

	// Echo is the HTTP server instance.
	Echo *echo.Echo
}

// New creates a new App instance with the given dependencies and configures
// the Echo server with global middleware and error handling.
func New(cfg *config.Config, db *sql.DB, rdb *redis.Client) *App {
	e := echo.New()

	// Disable Echo's default banner and startup message -- we log our own.
	e.HideBanner = true
	e.HidePort = true

	// Configure trusted reverse proxy IPs so c.RealIP() returns the actual
	// client IP instead of the proxy's IP. Critical for rate limiting, audit
	// logging, and abuse detection. Cosmos Cloud routes through Docker networks.
	middleware.TrustedProxies(e, []string{
		"127.0.0.0/8",    // Localhost
		"10.0.0.0/8",     // Docker default bridge
		"172.16.0.0/12",  // Docker bridge (alternate range)
		"192.168.0.0/16", // Common LAN
		"fd00::/8",       // IPv6 private
	})

	app := &App{
		Config: cfg,
		DB:     db,
		Redis:  rdb,
		Echo:   e,
	}

	// Register global middleware in order of execution.
	app.setupMiddleware()

	// Register the custom error handler that maps AppErrors to HTTP responses.
	e.HTTPErrorHandler = app.errorHandler

	// Serve static files (CSS, JS, vendor libs, fonts, images).
	e.Static("/static", "static")

	return app
}

// setupMiddleware registers global middleware on the Echo instance.
// Order matters: outermost (recovery) runs first, innermost (CSRF) runs last.
func (a *App) setupMiddleware() {
	// Panic recovery -- must be outermost to catch panics from all other middleware.
	a.Echo.Use(middleware.Recovery())

	// Request logging -- log every request with method, path, status, latency.
	a.Echo.Use(middleware.RequestLogger())

	// Security headers -- CSP, X-Frame-Options, X-Content-Type-Options, etc.
	a.Echo.Use(middleware.SecurityHeaders())

	// CORS -- allow cross-origin requests for the REST API.
	// Only relevant for external clients (Foundry VTT module, etc.).
	a.Echo.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins:   []string{a.Config.BaseURL},
		AllowCredentials: true,
	}))

	// CSRF -- double-submit cookie pattern on all state-changing requests.
	a.Echo.Use(middleware.CSRF())
}

// errorHandler is the custom Echo error handler. It maps domain errors
// (AppError) to appropriate HTTP responses, and renders error pages for
// browser requests or JSON for API requests.
//
// For HTMX partial requests that hit errors, we set HX-Retarget and
// HX-Reswap headers so the error page replaces the full body instead of
// being swapped into a partial target.
//
// For 401 errors on browser requests, we redirect to the login page.
func (a *App) errorHandler(err error, c echo.Context) {
	// Don't double-write if response is already committed.
	if c.Response().Committed {
		return
	}

	code := http.StatusInternalServerError
	message := "An unexpected error occurred"

	// Check if it's our domain error type.
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		code = appErr.Code
		message = appErr.Message

		// Log internal errors with the underlying cause.
		if appErr.Internal != nil {
			slog.Error("internal error",
				slog.String("type", appErr.Type),
				slog.String("message", appErr.Message),
				slog.Any("internal", appErr.Internal),
				slog.String("path", c.Request().URL.Path),
			)
		}
	} else {
		// Check for Echo's built-in HTTP errors (e.g., 404 from router).
		var echoErr *echo.HTTPError
		if errors.As(err, &echoErr) {
			code = echoErr.Code
			if msg, ok := echoErr.Message.(string); ok {
				message = msg
			} else {
				message = defaultErrorMessage(code)
			}
		} else {
			// Truly unexpected error -- log it.
			slog.Error("unhandled error",
				slog.Any("error", err),
				slog.String("path", c.Request().URL.Path),
			)
		}
	}

	// API requests always get JSON.
	if isAPIRequest(c) {
		c.JSON(code, map[string]string{
			"error":   http.StatusText(code),
			"message": message,
		})
		return
	}

	// For HTMX requests, redirect to login on 401 so the browser navigates
	// instead of swapping error HTML into a fragment target.
	if isHTMXRequest(c) {
		if code == http.StatusUnauthorized {
			c.Response().Header().Set("HX-Redirect", "/login")
			c.NoContent(http.StatusNoContent)
			return
		}
		// For other HTMX errors, retarget to body so the full error page
		// replaces the entire page instead of landing in a partial target.
		c.Response().Header().Set("HX-Retarget", "body")
		c.Response().Header().Set("HX-Reswap", "innerHTML")
	}

	// Regular browser 401 — redirect to login page.
	if code == http.StatusUnauthorized {
		c.Redirect(http.StatusSeeOther, "/login")
		return
	}

	middleware.Render(c, code, pages.ErrorPage(code, message))
}

// defaultErrorMessage returns a user-friendly message for common HTTP status codes
// when no specific message was provided by the error.
func defaultErrorMessage(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "The request was invalid or cannot be processed."
	case http.StatusUnauthorized:
		return "You need to log in to access this page."
	case http.StatusForbidden:
		return "You don't have permission to access this resource."
	case http.StatusNotFound:
		return "The page you're looking for doesn't exist or has been moved."
	case http.StatusMethodNotAllowed:
		return "This action is not allowed."
	case http.StatusConflict:
		return "This action conflicts with the current state."
	case http.StatusUnprocessableEntity:
		return "The submitted data could not be processed."
	case http.StatusTooManyRequests:
		return "You're making too many requests. Please slow down."
	case http.StatusInternalServerError:
		return "Something went wrong on our end. Please try again."
	case http.StatusBadGateway:
		return "The server received an invalid response."
	case http.StatusServiceUnavailable:
		return "The service is temporarily unavailable. Please try again later."
	default:
		return "An unexpected error occurred."
	}
}

// isAPIRequest returns true if the request is targeting the API (JSON response expected).
func isAPIRequest(c echo.Context) bool {
	return len(c.Request().URL.Path) >= 4 && c.Request().URL.Path[:4] == "/api"
}

// isHTMXRequest returns true if the request was initiated by HTMX.
func isHTMXRequest(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// Start begins listening for HTTP requests on the configured port.
func (a *App) Start() error {
	addr := fmt.Sprintf(":%d", a.Config.Port)
	slog.Info("starting Chronicle server",
		slog.String("addr", addr),
		slog.String("env", a.Config.Env),
	)
	return a.Echo.Start(addr)
}

// Package app is the application bootstrap and dependency injection root.
// It creates and holds all shared infrastructure (DB pool, Redis client,
// Echo instance) and wires together all plugins, modules, and widgets.
package app

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
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

	// WASMPluginManager manages loaded WASM logic extension plugins.
	// Set during route registration; nil until then.
	WASMPluginManager *extensions.PluginManager

	// WASMHookDispatcher dispatches events to WASM plugins.
	// Set during route registration; nil until then.
	WASMHookDispatcher *extensions.HookDispatcher

	// PluginHealth tracks which built-in plugins have healthy schemas.
	// Used during route registration to skip degraded plugins.
	PluginHealth *database.PluginHealthRegistry

	// PluginSchemas holds the registered plugin migration configurations.
	// Used by the database explorer to re-run migrations on demand.
	PluginSchemas []database.PluginSchema

	// pkgService is the package manager service, used for Foundry module
	// path resolution and system loading from external repos.
	pkgService packages.PackageService
}

// New creates a new App instance with the given dependencies and configures
// the Echo server with global middleware and error handling.
func New(cfg *config.Config, db *sql.DB, rdb *redis.Client, pluginHealth *database.PluginHealthRegistry, pluginSchemas []database.PluginSchema) *App {
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
		Config:       cfg,
		DB:           db,
		Redis:        rdb,
		Echo:         e,
		PluginHealth:  pluginHealth,
		PluginSchemas: pluginSchemas,
	}

	// Register global middleware in order of execution.
	app.setupMiddleware()

	// Register the custom error handler that maps AppErrors to HTTP responses.
	e.HTTPErrorHandler = app.errorHandler

	// Serve static files (CSS, JS, vendor libs, fonts, images).
	e.Static("/static", "static")

	// Serve the Foundry VTT module directory for easy installation.
	// module.json is served with dynamic URL injection; zip built on-the-fly.
	e.GET("/foundry-module/module.json", app.serveFoundryModuleManifest)
	e.GET("/foundry-module/chronicle-sync.zip", app.serveFoundryModuleZip)
	e.Static("/foundry-module", "foundry-module")

	return app
}

// setupMiddleware registers global middleware on the Echo instance.
// Order matters: outermost (recovery) runs first, innermost (CSRF) runs last.
func (a *App) setupMiddleware() {
	// Panic recovery -- must be outermost to catch panics from all other middleware.
	a.Echo.Use(middleware.Recovery())

	// Global request body size limit -- prevents memory exhaustion from
	// oversized payloads on non-upload endpoints. The media upload endpoint
	// has its own per-route body limit based on the configured max upload size,
	// so we skip this global limit for that path.
	a.Echo.Use(echomw.BodyLimitWithConfig(echomw.BodyLimitConfig{
		Limit: "2M",
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/media/upload") || path == "/ws"
		},
	}))

	// Request logging -- log every request with method, path, status, latency.
	a.Echo.Use(middleware.RequestLogger())

	// Security headers -- CSP, X-Frame-Options, X-Content-Type-Options, etc.
	a.Echo.Use(middleware.SecurityHeaders())

	// CORS -- allow cross-origin requests for the REST API.
	// Only relevant for external clients (Foundry VTT module, etc.).
	// BaseURL is always allowed. Additional origins are loaded dynamically
	// from site_settings (managed by admin via /admin/api/cors).
	settingsRepo := settings.NewSettingsRepository(a.DB)
	settingsSvc := settings.NewSettingsService(settingsRepo)
	a.Echo.Use(middleware.CORS(middleware.CORSConfig{
		AllowedOrigins:   []string{a.Config.BaseURL},
		AllowCredentials: true,
		DynamicOrigins: func() []string {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			origins, err := settingsSvc.GetCORSOrigins(ctx)
			if err != nil {
				slog.Warn("failed to load dynamic CORS origins", slog.Any("error", err))
				return nil
			}
			return origins
		},
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
	} else {
		// Check for Echo's built-in HTTP errors (e.g., 404 from router,
		// panic recovery).
		var echoErr *echo.HTTPError
		if errors.As(err, &echoErr) {
			code = echoErr.Code
			if msg, ok := echoErr.Message.(string); ok {
				message = msg
			} else {
				message = defaultErrorMessage(code)
			}
		}
	}

	// Always log server errors (5xx) so silent failures are visible.
	// Previously, AppError with Internal==nil and echo.HTTPError from
	// panic recovery were both swallowed with no log output.
	if code >= http.StatusInternalServerError {
		slog.Error("server error",
			slog.Int("code", code),
			slog.String("message", message),
			slog.Any("error", err),
			slog.String("path", c.Request().URL.Path),
			slog.String("method", c.Request().Method),
		)
	}

	// API requests always get JSON.
	if isAPIRequest(c) {
		_ = c.JSON(code, map[string]string{
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
			_ = c.NoContent(http.StatusNoContent)
			return
		}
		// For other HTMX errors, retarget to body so the full error page
		// replaces the entire page instead of landing in a partial target.
		c.Response().Header().Set("HX-Retarget", "body")
		c.Response().Header().Set("HX-Reswap", "innerHTML")
	}

	// Regular browser 401 — redirect to login page.
	if code == http.StatusUnauthorized {
		_ = c.Redirect(http.StatusSeeOther, "/login")
		return
	}

	_ = middleware.Render(c, code, pages.ErrorPage(code, message))
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

// isAPIRequest returns true if the request expects a JSON response.
// Matches /api/* paths and fetch requests with JSON content type (e.g.,
// calendar/maps/timeline endpoints that use fetch + JSON but live under
// /campaigns/* rather than /api/*).
func isAPIRequest(c echo.Context) bool {
	path := c.Request().URL.Path
	if len(path) >= 4 && path[:4] == "/api" {
		return true
	}
	ct := c.Request().Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return true
	}
	// Check Accept header so that fetch() callers (e.g. image upload widget)
	// receive JSON error responses instead of HTML error pages.
	accept := c.Request().Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// isHTMXRequest returns true if the request was initiated by HTMX.
func isHTMXRequest(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// foundryModuleDir returns the directory to serve the Foundry module from.
// Prefers the package manager install path if available, falls back to the
// bundled foundry-module/ directory.
func (a *App) foundryModuleDir() string {
	if a.pkgService != nil {
		if path := a.pkgService.FoundryModulePath(); path != "" {
			if _, err := os.Stat(filepath.Join(path, "module.json")); err == nil {
				return path
			}
		}
	}
	return "foundry-module"
}

// serveFoundryModuleManifest serves foundry-module/module.json with the
// manifest and download URLs rewritten to use the Chronicle instance's BaseURL.
// This allows Foundry VTT to install the module directly from any Chronicle
// instance without needing GitHub releases.
func (a *App) serveFoundryModuleManifest(c echo.Context) error {
	moduleDir := a.foundryModuleDir()
	data, err := os.ReadFile(filepath.Join(moduleDir, "module.json"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "module.json not found")
	}

	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid module.json")
	}

	// Always rewrite manifest and download URLs to point at this instance.
	baseURL := strings.TrimRight(a.Config.BaseURL, "/")
	manifest["manifest"] = baseURL + "/foundry-module/module.json"
	manifest["download"] = baseURL + "/foundry-module/chronicle-sync.zip"

	return c.JSON(http.StatusOK, manifest)
}

// serveFoundryModuleZip dynamically zips the foundry-module/ directory and
// serves it as chronicle-sync.zip. Foundry VTT downloads this during module
// installation. The zip contains all files under a chronicle-sync/ root
// directory, which is the expected structure for Foundry module archives.
// The module.json inside the zip gets its manifest/download URLs rewritten
// to point at this Chronicle instance.
func (a *App) serveFoundryModuleZip(c echo.Context) error {
	moduleDir := a.foundryModuleDir()
	if _, err := os.Stat(moduleDir); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "foundry module directory not found")
	}

	baseURL := strings.TrimRight(a.Config.BaseURL, "/")

	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=chronicle-sync.zip")
	c.Response().WriteHeader(http.StatusOK)

	zw := zip.NewWriter(c.Response().Writer)

	walkErr := filepath.WalkDir(moduleDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip non-distributable files.
		name := d.Name()
		if name == ".ai.md" || name == "TESTING.md" {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Create zip entry under chronicle-sync/ root.
		relPath, _ := filepath.Rel(moduleDir, path)
		zipPath := filepath.Join("chronicle-sync", relPath)

		w, err := zw.Create(filepath.ToSlash(zipPath))
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Rewrite module.json URLs inside the zip so the installed module
		// points back at this Chronicle instance for updates.
		if name == "module.json" {
			data, err = a.rewriteModuleManifest(data, baseURL)
			if err != nil {
				return err
			}
		}

		_, err = w.Write(data)
		return err
	})
	if walkErr != nil {
		return walkErr
	}
	return zw.Close()
}

// rewriteModuleManifest parses module.json and rewrites the manifest and
// download URLs to point at the given baseURL.
func (a *App) rewriteModuleManifest(data []byte, baseURL string) ([]byte, error) {
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		return data, err
	}
	manifest["manifest"] = baseURL + "/foundry-module/module.json"
	manifest["download"] = baseURL + "/foundry-module/chronicle-sync.zip"
	return json.MarshalIndent(manifest, "", "  ")
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

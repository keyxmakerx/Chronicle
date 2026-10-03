// Package app is the application bootstrap and dependency injection root.
// It creates and holds all shared infrastructure (DB pool, Redis client,
// Echo instance) and wires together all plugins, modules, and widgets.
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
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
	"github.com/keyxmakerx/chronicle/internal/observability"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
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

	// ShutdownCtx is canceled by ShutdownCancel when the server begins
	// graceful shutdown (cmd/server/main.go's signal handler). Long-running
	// startup background jobs (e.g. the media content-hash backfill) select
	// on it instead of context.Background() so they stop instead of working
	// against a closing DB connection (#711).
	ShutdownCtx context.Context

	// ShutdownCancel cancels ShutdownCtx. Called once, from the signal
	// handler in cmd/server/main.go.
	ShutdownCancel context.CancelFunc

	// pkgService is the package manager service, used for Foundry module
	// path resolution and system loading from external repos.
	pkgService packages.PackageService

	// registeredPlugins is the metadata registry of plugins contributing
	// to this App. Populated inline from RegisterRoutes at each plugin's
	// setup point.
	registeredPlugins []PluginRegistration
}

// New creates a new App instance with the given dependencies and configures
// the Echo server with global middleware and error handling.
func New(cfg *config.Config, db *sql.DB, rdb *redis.Client, pluginHealth *database.PluginHealthRegistry, pluginSchemas []database.PluginSchema) (*App, error) {
	e := echo.New()

	// Disable Echo's default banner and startup message -- we log our own.
	e.HideBanner = true
	e.HidePort = true

	// Configure trusted reverse proxy IPs so c.RealIP() returns the actual
	// client IP instead of the proxy's IP — needed for rate limiting, audit
	// logging, and abuse detection to see the real visitor. The list is
	// configuration, not a literal, because a proxy on a non-private address
	// would otherwise never be recognized. A bad entry fails startup rather
	// than being skipped, so it never yields a silently wrong client IP.
	if err := middleware.TrustedProxies(e, cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxy configuration: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	app := &App{
		Config:         cfg,
		DB:             db,
		Redis:          rdb,
		Echo:           e,
		PluginHealth:   pluginHealth,
		PluginSchemas:  pluginSchemas,
		ShutdownCtx:    shutdownCtx,
		ShutdownCancel: shutdownCancel,
	}

	// Register global middleware in order of execution.
	app.setupMiddleware()

	// Register the custom error handler that maps AppErrors to HTTP responses.
	e.HTTPErrorHandler = app.errorHandler

	// Serve static files (CSS, JS, vendor libs, fonts, images).
	//
	// StaticCache turns the `?v=<digest>` tokens that layouts.AssetURL stamps
	// onto every template-emitted asset URL into a caching policy — immutable
	// for versioned requests, forced revalidation for bare ones. Registered
	// as global middleware, not on a /static group, so it also covers every
	// plugin embed mount registered later in mountPluginStatic; it is a no-op
	// for non-/static paths.
	e.Use(middleware.StaticCache(layouts.StaticURLPrefix))
	e.Static("/static", "static")

	return app, nil
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
	//
	// The calendar import routes (the wizard's drag-and-drop preview and its
	// review-step create, plus the equivalent JSON API pair) advertise an
	// ~10MB cap of their own, enforced via an http.LimitReader in the
	// calendar handler — this global 2MB limit would otherwise be the real
	// (and silent, since it fails before the handler's own check ever runs)
	// ceiling for every one of them. The Site look form carries a logo and a
	// sign-in picture, so it skips the global limit and takes a 5M cap of its
	// own at the route.
	a.Echo.Use(echomw.BodyLimitWithConfig(echomw.BodyLimitConfig{
		Limit: "2M",
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			if strings.HasPrefix(path, "/media/upload") || path == "/ws" || path == "/admin/site-look" {
				return true
			}
			return isCalendarImportPath(path)
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

// calendarImportPathPattern matches the calendar plugin's upload-heavy
// routes (its own routes.go): the wizard's drag-and-drop import preview, its
// "Build your own" structure preview (the browser's hand-built structure is
// just as large as an uploaded file's, and re-validated the same way —
// parseWizardImportJSON), its review-step create, and their JSON-API
// equivalents. Matched against the raw incoming URL path, so it doesn't need
// an *echo.Route lookup this early in the middleware chain — :id is any
// single path segment, exactly as permissive as Echo's own routing is for
// that position.
var calendarImportPathPattern = regexp.MustCompile(
	`^/campaigns/[^/]+/calendars/(wizard/import/preview|wizard/build/preview|wizard/create|import/preview|import)$`)

// isCalendarImportPath reports whether path is one of the calendar import
// routes the global body limit above must not apply to (see its own
// comment): they enforce their own ~10MB cap via an http.LimitReader in the
// calendar handler, so the global 2MB limit would otherwise silently be the
// real ceiling instead.
func isCalendarImportPath(path string) bool {
	return calendarImportPathPattern.MatchString(path)
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

	// kind records which branch below claimed the error, for the in-memory
	// error ring: a deliberate AppError/HTTPError vs. a raw error that
	// escaped a handler (almost always a bug). Defaults to raw.
	kind := observability.KindRaw

	// Check if it's our domain error type.
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		kind = observability.KindApp
		code = appErr.Code
		message = appErr.Message
	} else {
		// Check for Echo's built-in HTTP errors (e.g., 404 from router,
		// panic recovery).
		var echoErr *echo.HTTPError
		if errors.As(err, &echoErr) {
			kind = observability.KindHTTP
			code = echoErr.Code
			if msg, ok := echoErr.Message.(string); ok {
				message = msg
			} else {
				message = defaultErrorMessage(code)
			}
		}
	}

	// Always log server errors (5xx) so silent failures are visible.
	if code >= http.StatusInternalServerError {
		slog.Error("server error",
			slog.Int("code", code),
			slog.String("message", message),
			slog.Any("error", err),
			slog.String("path", c.Request().URL.Path),
			slog.String("method", c.Request().Method),
		)
	}

	// Record the error into the in-memory ring the host.errors diagnostics
	// read, so it's visible without shell access. RecordHTTPError applies
	// its own policy (5xx only) and stores the route TEMPLATE (c.Path()),
	// not the requested path, because a concrete path like /rsvp/:token
	// would carry a live credential into pasteable output. Placed here,
	// after the status is known and before anything is written, so it
	// observes exactly the code and error the client is about to receive.
	_ = observability.RecordHTTPError(code, c.Request().Method, c.Path(), c.Request().URL.Path, kind, err)

	// API requests always get JSON. `error` carries the MACHINE-READABLE
	// condition (e.g. `sync_api_disabled`, `calendar_rebuilding`), `message`
	// the prose — callers like the Foundry module's api-client.mjs read
	// `err.code` separately from `err.serverMessage`, so an AppError's Type
	// must reach the wire distinctly from its message. StatusText is the
	// fallback for errors with no type to offer (router 404s, panic recovery).
	if isAPIRequest(c) {
		errorField := http.StatusText(code)
		if appErr != nil && appErr.Type != "" {
			errorField = appErr.Type
		}
		_ = c.JSON(code, map[string]string{
			"error":   errorField,
			"message": message,
		})
		return
	}

	// For HTMX requests, redirect to login on 401 — but ONLY for boosted
	// navigations (real page moves, where landing on /login is what the user
	// needs). A lazily-loaded FRAGMENT that 401s must NOT hijack the page
	// (e.g. an anonymous visitor's stray authed widget call on a public
	// campaign): fragment 401s fall through to the 4xx toast branch instead.
	if isHTMXRequest(c) {
		if code == http.StatusUnauthorized && c.Request().Header.Get("HX-Boosted") == "true" {
			c.Response().Header().Set("HX-Redirect", "/login")
			_ = c.NoContent(http.StatusNoContent)
			return
		}
		// For 4xx client errors, show a toast notification instead of
		// replacing the entire page — much better UX for inline actions.
		if code >= 400 && code < 500 {
			trigger, _ := json.Marshal(map[string]any{
				"chronicle:notify": map[string]string{
					"message": message,
					"type":    "error",
				},
			})
			c.Response().Header().Set("HX-Trigger", string(trigger))
			c.Response().Header().Set("HX-Reswap", "none")
			_ = c.NoContent(code)
			return
		}
		// For 5xx server errors, retarget to body so the full error page
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
		return "Your session has ended. Please reload the page and sign in again."
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

// Start begins listening for HTTP requests on the configured port.
func (a *App) Start() error {
	addr := fmt.Sprintf(":%d", a.Config.Port)
	slog.Info("starting Chronicle server",
		slog.String("addr", addr),
		slog.String("env", a.Config.Env),
	)
	return a.Echo.Start(addr)
}

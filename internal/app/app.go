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
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/maintenance"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/observability"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
	"github.com/keyxmakerx/chronicle/internal/plugins/vault_import"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
	"github.com/keyxmakerx/chronicle/internal/templates/pages"
	"github.com/keyxmakerx/chronicle/internal/updatehealth"
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

	// Echo is the router and middleware chain that serves every request.
	Echo *echo.Echo

	// server is the HTTP listener in front of Echo. Echo v5 no longer owns
	// one, so Start and Shutdown drive it here.
	server *http.Server

	// routeHandlers maps "METHOD path" to the registered handler's function
	// name. Echo v5's route table names a route by method and path only, and
	// the operator diagnostics compare live handlers by function name.
	routeHandlersMu sync.Mutex
	routeHandlers   map[string]string

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

	// UploadCeilingFn, when set, replaces the settings-backed media upload
	// ceiling used by the pre-CSRF size cap. Tests set it; production leaves
	// it nil.
	UploadCeilingFn func() int64

	// Maintenance is the site-wide "back in a few minutes" state a restore
	// drives. The global gate middleware and /healthz read it; it is built
	// from the backup directory so a restart mid-restore comes back closed.
	Maintenance *maintenance.State

	// PostRestoreSchema brings a freshly restored database up to this build
	// the way a normal start does. Set by cmd/server before RegisterRoutes;
	// nil skips it (tests).
	PostRestoreSchema func(ctx context.Context) error

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

	// Configure trusted reverse proxy IPs so c.RealIP() returns the actual
	// client IP instead of the proxy's IP — needed for rate limiting, audit
	// logging, and abuse detection to see the real visitor. The list is
	// configuration, not a literal, because a proxy on a non-private address
	// would otherwise never be recognized. A bad entry fails startup rather
	// than being skipped, so it never yields a silently wrong client IP.
	if err := middleware.TrustedProxies(e, cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxy configuration: %w", err)
	}
	// Which header those proxies are believed on is an admin choice (Security >
	// Visitor addresses). The extractor reads it from memory on every request;
	// it is loaded here and refreshed when the admin saves.
	loadClientIPHeader(db)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	app := &App{
		Config:         cfg,
		DB:             db,
		Redis:          rdb,
		Echo:           e,
		server:         &http.Server{Handler: e},
		PluginHealth:   pluginHealth,
		PluginSchemas:  pluginSchemas,
		ShutdownCtx:    shutdownCtx,
		ShutdownCancel: shutdownCancel,
		Maintenance:    maintenance.NewState(filepath.Join(cfg.BackupDir, maintenance.MarkerName)),
	}

	app.routeHandlers = make(map[string]string)
	e.OnAddRoute = func(r echo.Route) error {
		if r.Handler == nil {
			return nil
		}
		app.routeHandlersMu.Lock()
		app.routeHandlers[r.Method+" "+r.Path] = echo.HandlerName(r.Handler)
		app.routeHandlersMu.Unlock()
		return nil
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
	// Every response names the running build, so a tab opened before a deploy
	// reloads instead of mixing new pages with its old scripts (boot.js).
	e.Use(middleware.BuildHeader(layouts.BuildToken()))
	e.Use(middleware.StaticGzip(layouts.StaticURLPrefix))
	e.Static("/static", "static")

	return app, nil
}

// setupMiddleware registers global middleware on the Echo instance.
// Order matters: outermost (recovery) runs first, innermost (CSRF) runs last.
func (a *App) setupMiddleware() {
	// Update health counts finished requests against the changed features.
	// It sits outside panic recovery so a recovered panic, which leaves only
	// a written 500 behind, is still counted as a failure.
	a.Echo.Use(updateHealthCounter())

	// Panic recovery -- must be outermost to catch panics from all other middleware.
	a.Echo.Use(middleware.Recovery())

	// Global request body size limit -- prevents memory exhaustion from
	// oversized payloads on non-upload endpoints. The media upload endpoint
	// has its own per-route body limit based on the configured max upload size,
	// so we skip this global limit for that path (see registerUploadCaps for
	// the cap that runs ahead of CSRF).
	//
	// The calendar import routes (the wizard's drag-and-drop preview and its
	// review-step create, plus the equivalent JSON API pair) advertise an
	// ~10MB cap of their own, enforced via an http.LimitReader in the
	// calendar handler — this global 2MB limit would otherwise be the real
	// (and silent, since it fails before the handler's own check ever runs)
	// ceiling for every one of them.
	a.Echo.Use(echomw.BodyLimitWithConfig(echomw.BodyLimitConfig{
		LimitBytes: 2 << 20,
		Skipper: func(c *echo.Context) bool {
			path := c.Request().URL.Path
			if strings.HasPrefix(path, "/media/upload") || path == "/ws" || path == siteLookPath {
				return true
			}
			return isCalendarImportPath(path) || isVaultImportUploadPath(path)
		},
	}))
	// The Site look form carries a logo and a sign-in picture, so it gets a
	// larger cap of its own. It is a global middleware, ahead of CSRF, because
	// the CSRF check reads the form field and so parses the multipart body
	// before any route-level middleware runs; a limit set only at the route
	// would come too late.
	a.Echo.Use(echomw.BodyLimitWithConfig(echomw.BodyLimitConfig{
		LimitBytes: siteLookBodyLimit,
		Skipper:    func(c *echo.Context) bool { return c.Request().URL.Path != siteLookPath },
	}))

	// Settings > Import's zip upload is exempt from the 2 MB limit above but gets
	// its own cap here, for the same reason as the Site look form: the CSRF
	// check below parses the multipart body before authentication, so an
	// unauthenticated client could otherwise make the server spool any size.
	a.Echo.Use(echomw.BodyLimitWithConfig(echomw.BodyLimitConfig{
		LimitBytes: vaultImportBodyLimit(),
		Skipper:    func(c *echo.Context) bool { return !isVaultImportUploadPath(c.Request().URL.Path) },
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
		AllowedOrigins: []string{a.Config.BaseURL},
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

	// Upload size caps for the media and calendar import routes. These sit
	// just ahead of CSRF for the reason given on the Site look cap above:
	// the CSRF check parses the multipart body before sign-in is checked.
	ceiling := a.UploadCeilingFn
	if ceiling == nil {
		ceiling = uploadCeiling(settingsSvc, a.Config.Upload.MaxSize, uploadCeilingTTL)
	}
	registerUploadCaps(a.Echo, ceiling)

	// Maintenance gate -- while a restore replaces the database every request
	// except health probes, static files and the restoring admin's own pages
	// gets a plain 503. It sits after CORS so API and Foundry callers can read
	// the JSON refusal, and before CSRF so a refused request never touches
	// anything else.
	a.Echo.Use(a.Maintenance.Gate())

	// CSRF -- double-submit cookie pattern on all state-changing requests.
	a.Echo.Use(middleware.CSRF())
}

// loadClientIPHeader reads the saved visitor-address header choice into the
// extractor. A read failure leaves the automatic default in force.
func loadClientIPHeader(db *sql.DB) {
	if db == nil {
		return
	}
	svc := settings.NewSettingsService(settings.NewSettingsRepository(db))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mode, err := svc.GetClientIPHeader(ctx)
	if err != nil {
		slog.Warn("could not read the visitor address setting; using automatic", slog.Any("error", err))
	}
	middleware.SetClientIPHeader(mode)
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

// vaultImportUploadPattern matches Settings > Import's zip upload. The handler
// caps the body itself (a vault is far larger than the global 2MB limit), so
// the global limit must not apply here.
var vaultImportUploadPattern = regexp.MustCompile(`^/campaigns/[^/]+/import/markdown/preview$`)

// vaultImportBodyLimit is the upload route's size cap in bytes, rounded up to
// a whole KiB.
func vaultImportBodyLimit() int64 {
	return (vault_import.RequestLimit(vault_import.DefaultLimits) + 1023) / 1024 * 1024
}

// isVaultImportUploadPath reports whether path is that upload route.
func isVaultImportUploadPath(path string) bool {
	return vaultImportUploadPattern.MatchString(path)
}

// siteLookPath is the Site look form's URL, and siteLookBodyLimit its request
// size cap: room for a 1 MB logo and a 3 MB picture plus the other fields.
const (
	siteLookPath      = "/admin/site-look"
	siteLookBodyLimit = 5 << 20
)

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
func (a *App) errorHandler(c *echo.Context, err error) {
	// Don't double-write if response is already committed.
	if middleware.ResponseState(c).Committed {
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
		// Echo's own errors: an *echo.HTTPError a handler returned, or one
		// of Echo's fixed sentinels (the router's 404 and 405, the body
		// limit's 413). The sentinels carry no message, so they get the
		// status text, as they did when they were HTTPErrors.
		if sc := echo.StatusCode(err); sc != 0 {
			kind = observability.KindHTTP
			code = sc
			message = http.StatusText(code)
			var echoErr *echo.HTTPError
			if errors.As(err, &echoErr) {
				message = echoErr.Message
				if message == "" {
					message = defaultErrorMessage(code)
				}
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
	// Update health stores only the status, never the error text, which can
	// carry values from the request or the database.
	if code >= http.StatusInternalServerError {
		updatehealth.Fail(c.Request().Method, c.Path(), fmt.Sprintf("%d %s", code, http.StatusText(code)))
	}

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
func isAPIRequest(c *echo.Context) bool {
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
func isHTMXRequest(c *echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// Start begins listening for HTTP requests on the configured port.
func (a *App) Start() error {
	addr := fmt.Sprintf(":%d", a.Config.Port)
	slog.Info("starting Chronicle server",
		slog.String("addr", addr),
		slog.String("env", a.Config.Env),
	)
	a.server.Addr = addr
	return a.server.ListenAndServe()
}

// routeHandler returns the function name of the handler registered for
// method and path, or "" when none was recorded.
func (a *App) routeHandler(method, path string) string {
	a.routeHandlersMu.Lock()
	defer a.routeHandlersMu.Unlock()
	return a.routeHandlers[method+" "+path]
}

// Shutdown stops accepting connections and waits, until ctx ends, for
// in-flight requests to finish. Start then returns http.ErrServerClosed.
func (a *App) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

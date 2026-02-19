package app

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/templates/pages"
)

// RegisterRoutes sets up all application routes. It registers public routes
// directly and delegates to each plugin's route registration function.
//
// This is the single place where all routes are aggregated. When a new
// plugin is added, its routes are registered here.
func (a *App) RegisterRoutes() {
	e := a.Echo

	// --- Public Routes (no auth required) ---

	// Landing page.
	e.GET("/", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, pages.Landing())
	})

	// Health check endpoint for Docker/Cosmos health monitoring.
	e.GET("/healthz", func(c echo.Context) error {
		// TODO: Check DB and Redis connectivity for a real health check.
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// --- Plugin Routes ---
	// Each plugin registers its own routes on a sub-group.
	// Uncomment as plugins are implemented:

	// auth plugin (public: login, register, logout)
	// authPlugin.RegisterRoutes(e)

	// Authenticated route group -- all routes below require a valid session.
	// authed := e.Group("", authMiddleware)

	// campaigns plugin
	// campaignPlugin.RegisterRoutes(authed)

	// entities plugin (scoped to campaign)
	// entityPlugin.RegisterRoutes(authed)

	// --- Module Routes ---
	// Game system reference pages and tooltip APIs.
	// ref := e.Group("/ref")
	// dnd5eModule.RegisterRoutes(ref)

	// --- API Routes ---
	// REST API for external clients (Foundry VTT, etc.).
	// api := e.Group("/api/v1")
	// apiPlugin.RegisterRoutes(api)
}

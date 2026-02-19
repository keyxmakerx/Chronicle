package app

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
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

	// Auth plugin: login, register, logout (public routes).
	authRepo := auth.NewUserRepository(a.DB)
	authService := auth.NewAuthService(authRepo, a.Redis, a.Config.Auth.SessionTTL)
	authHandler := auth.NewHandler(authService)
	auth.RegisterRoutes(e, authHandler)

	// Authenticated route group -- all routes below require a valid session.
	_ = e.Group("", auth.RequireAuth(authService))

	// TODO: campaigns plugin
	// campaignPlugin.RegisterRoutes(authed)

	// TODO: entities plugin (scoped to campaign)
	// entityPlugin.RegisterRoutes(authed)

	// Dashboard placeholder for authenticated users.
	e.GET("/dashboard", func(c echo.Context) error {
		return middleware.Render(c, http.StatusOK, pages.Landing())
	}, auth.RequireAuth(authService))

	// --- Module Routes ---
	// Game system reference pages and tooltip APIs.
	// ref := e.Group("/ref")
	// dnd5eModule.RegisterRoutes(ref)

	// --- API Routes ---
	// REST API for external clients (Foundry VTT, etc.).
	// api := e.Group("/api/v1")
	// apiPlugin.RegisterRoutes(api)
}

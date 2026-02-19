package app

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/smtp"
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

	// SMTP plugin: outbound email for transfers, password resets.
	smtpRepo := smtp.NewSMTPRepository(a.DB)
	smtpService := smtp.NewSMTPService(smtpRepo, a.Config.Auth.SecretKey)
	smtpHandler := smtp.NewHandler(smtpService)

	// Campaigns plugin: CRUD, membership, ownership transfer.
	userFinder := campaigns.NewUserFinderAdapter(authRepo)
	campaignRepo := campaigns.NewCampaignRepository(a.DB)
	campaignService := campaigns.NewCampaignService(campaignRepo, userFinder, smtpService, a.Config.BaseURL)
	campaignHandler := campaigns.NewHandler(campaignService)
	campaigns.RegisterRoutes(e, campaignHandler, campaignService, authService)

	// Admin plugin: site-wide management (users, campaigns, SMTP settings).
	adminHandler := admin.NewHandler(authRepo, campaignService, smtpService)
	admin.RegisterRoutes(e, adminHandler, authService, smtpHandler)

	// TODO: entities plugin (scoped to campaign)
	// entityPlugin.RegisterRoutes(authed)

	// Dashboard redirects to campaigns list for authenticated users.
	e.GET("/dashboard", func(c echo.Context) error {
		return c.Redirect(http.StatusSeeOther, "/campaigns")
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

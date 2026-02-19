package media

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// RegisterRoutes sets up all media-related routes on the given Echo instance.
func RegisterRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService) {
	// Public route: serve media files with cache headers.
	e.GET("/media/:id", h.Serve)
	e.GET("/media/:id/thumb/:size", h.ServeThumbnail)

	// Authenticated routes.
	authMw := auth.RequireAuth(authSvc)

	// Rate limit uploads: 30 per minute per IP.
	uploadRateLimit := middleware.RateLimit(30, time.Minute)

	e.POST("/media/upload", h.Upload, authMw, uploadRateLimit)
	e.GET("/media/:fileID/info", h.Info, authMw)
	e.DELETE("/media/:fileID", h.Delete, authMw)
}

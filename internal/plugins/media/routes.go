package media

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterRoutes sets up all media-related routes on the given Echo instance.
// maxUploadSize is used to limit request body size on the upload endpoint so
// oversized payloads are rejected before being read into memory.
// serveRateLimit controls the max media serve requests per minute per IP (0 = 300 default).
func RegisterRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService, maxUploadSize int64, serveRateLimit int) {
	// Serve routes are protected by:
	// 1. HMAC-signed URLs (handler-level, verifies cryptographic signature)
	// 2. Campaign membership check for private campaigns (handler-level)
	// 3. Per-IP rate limiting (middleware-level, prevents scraping/DoS)
	// 4. OptionalAuth for session-based fallback access during migration
	if serveRateLimit <= 0 {
		serveRateLimit = 300
	}
	serveRL := middleware.RateLimit(serveRateLimit, time.Minute)
	authOptional := auth.OptionalAuth(authSvc)
	e.GET("/media/:id", h.Serve, authOptional, serveRL)
	e.GET("/media/:id/thumb/:size", h.ServeThumbnail, authOptional, serveRL)

	// Authenticated routes.
	authMw := auth.RequireAuth(authSvc)

	// Rate limit uploads: 30 per minute per IP.
	uploadRateLimit := middleware.RateLimit(30, time.Minute)

	// Limit upload body size to prevent memory exhaustion from oversized payloads.
	// Uses a 10% margin above maxUploadSize to account for multipart encoding overhead.
	bodyLimit := bodyLimitMiddleware(maxUploadSize + maxUploadSize/10)

	e.POST("/media/upload", h.Upload, authMw, uploadRateLimit, bodyLimit)
	e.GET("/media/:fileID/info", h.Info, authMw)
	e.DELETE("/media/:fileID", h.Delete, authMw)
}

// RegisterCampaignRoutes sets up campaign-scoped media management routes.
// The media browser is Owner-only -- allows browsing, deleting, and
// checking which entities reference each file.
func RegisterCampaignRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)

	cg.GET("/media", h.CampaignMedia, campaigns.RequireRole(campaigns.RoleOwner))
	cg.DELETE("/media/:mid", h.CampaignDeleteMedia, campaigns.RequireRole(campaigns.RoleOwner))
	cg.GET("/media/:mid/refs", h.CampaignMediaRefs, campaigns.RequireRole(campaigns.RoleOwner))
}

// bodyLimitMiddleware returns middleware that rejects request bodies exceeding
// the given size in bytes. Applied before the handler reads the body into memory.
func bodyLimitMiddleware(maxBytes int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().ContentLength > maxBytes {
				return &apperror.AppError{
					Code:    http.StatusRequestEntityTooLarge,
					Type:    "request_too_large",
					Message: fmt.Sprintf("request body too large; maximum is %d MB", maxBytes/(1024*1024)),
				}
			}
			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBytes)
			return next(c)
		}
	}
}

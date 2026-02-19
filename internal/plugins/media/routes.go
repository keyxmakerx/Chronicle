package media

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// RegisterRoutes sets up all media-related routes on the given Echo instance.
// maxUploadSize is used to limit request body size on the upload endpoint so
// oversized payloads are rejected before being read into memory.
func RegisterRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService, maxUploadSize int64) {
	// Public route: serve media files with cache headers.
	e.GET("/media/:id", h.Serve)
	e.GET("/media/:id/thumb/:size", h.ServeThumbnail)

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

// bodyLimitMiddleware returns middleware that rejects request bodies exceeding
// the given size in bytes. Applied before the handler reads the body into memory.
func bodyLimitMiddleware(maxBytes int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().ContentLength > maxBytes {
				return echo.NewHTTPError(http.StatusRequestEntityTooLarge,
					fmt.Sprintf("request body too large; maximum is %d MB", maxBytes/(1024*1024)))
			}
			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBytes)
			return next(c)
		}
	}
}

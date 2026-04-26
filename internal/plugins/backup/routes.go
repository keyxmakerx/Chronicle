package backup

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// RegisterRoutes mounts the admin backup routes on the given group. The
// caller is responsible for the auth gate; this package assumes the
// group already requires site-admin.
//
// Rate limits are deliberately tight: backup is heavy, downloads are
// large, and the only legitimate caller is a human admin clicking
// occasionally. Lower bounds prevent click-flooding from spawning
// parallel mysqldumps even with the in-process single-flight lock as
// a backstop.
func RegisterRoutes(admin *echo.Group, h *Handler) {
	g := admin.Group("/backup")
	g.GET("", h.Page)
	g.POST("/run", h.Run, middleware.RateLimit(2, 1*time.Hour))
	g.GET("/files/:name", h.Download, middleware.RateLimit(20, 1*time.Hour))
}

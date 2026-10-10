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
//
// reauth guards the link route: the download itself is a plain GET a browser
// follows, so the password re-confirmation happens when the short-lived link
// is requested, and the GET only honours a link minted for that admin and file.
func RegisterRoutes(admin *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	g := admin.Group("/backup")
	g.GET("", h.Page)
	g.POST("/run", h.Run, middleware.RateLimit(2, 1*time.Hour))
	g.POST("/schedule", h.SaveSchedule, middleware.RateLimit(30, 1*time.Hour))
	g.POST("/files/:name/link", h.DownloadLink, reauth, middleware.RateLimit(20, 1*time.Hour))
	g.GET("/files/:name", h.Download, middleware.RateLimit(20, 1*time.Hour))
}

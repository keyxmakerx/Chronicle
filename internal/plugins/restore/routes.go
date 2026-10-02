package restore

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// RegisterRoutes mounts the admin restore routes on the given group.
// The caller is responsible for the auth gate; this package assumes the
// group already requires site-admin.
//
// Restore is rate-limited tighter than backup (1/hour) because each
// invocation is a hard reset of every shared resource. Combined with
// the in-process single-flight, click-flooding cannot start a second
// restore — at worst, the operator sees 409s.
//
// reauth runs before the rate limit so an unconfirmed attempt does not use
// up the hour's single restore.
func RegisterRoutes(admin *echo.Group, h *Handler, reauth echo.MiddlewareFunc) {
	g := admin.Group("/restore")
	g.GET("", h.Page)
	g.POST("/run", h.Run, reauth, middleware.RateLimit(1, 1*time.Hour))
}

package restore

import (
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/maintenance"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// RegisterRoutes mounts the admin restore routes on the given group.
// The caller is responsible for the auth gate; this package assumes the
// group already requires site-admin.
//
// Restore is rate-limited (3/hour) because each invocation replaces every
// shared resource; a failed attempt must still leave room for a retry while
// the site is closed. The single-flight in the maintenance state means
// click-flooding cannot start a second restore: at worst the operator sees
// 409s.
//
// reauth runs before the rate limit so an unconfirmed attempt does not use
// up the hour's allowance.
//
// state is the shared maintenance state: while it is closed the global gate
// serves the restoring admin through h.OwnerView, so these routes are only
// reached while the site is open.
func RegisterRoutes(admin *echo.Group, h *Handler, reauth echo.MiddlewareFunc, state *maintenance.State) {
	if state != nil {
		state.SetOwnerHandler(h.OwnerView)
	}
	g := admin.Group("/restore")
	g.GET("", h.Page)
	g.GET("/status", h.Status)
	g.POST("/run", h.Run, reauth, middleware.RateLimit(3, 1*time.Hour))
}

package calendar

import (
	"github.com/labstack/echo/v4"
)

// RegisterPublicAPIRoutes mounts the public Foundry-facing
// calendar endpoints under /api/v1/campaigns/:cid/calendar.
//
// Authentication is per-handler via the per-campaign signed token
// on the ?token= query parameter (same scheme as /foundry-vtt/
// module.json). No campaign-membership middleware — the token
// itself is the access control.
//
// Rate-limit middleware is applied by the caller, mirroring the
// foundry_vtt and packages plugin contracts. Without it an
// abusive client can DoS the calendar endpoints into the DB.
//
// URL shape is pinned in
// cordinator/decisions/2026-05-17-calendar-sync-wire-contract.md.
// Don't move the routes without amending that decision.
func RegisterPublicAPIRoutes(e *echo.Echo, h *APIHandler, rateLimit echo.MiddlewareFunc) {
	g := e.Group("/api/v1/campaigns/:cid/calendar")
	if rateLimit != nil {
		g.Use(rateLimit)
	}
	g.GET("", h.GetCalendar)
	// POST creates a calendar from a Calendaria-shaped import payload
	// (C-CAL-CREATE-ENDPOINT, 2026-05-19). Closes the empty-state
	// dead-end the operator hit when Sync Calendar found no Chronicle
	// calendar to sync against. Payload contract pinned in
	// cordinator/decisions/2026-05-19-calendar-create-wire.md.
	g.POST("", h.CreateCalendar)
	g.PUT("/date", h.PutDate)
	g.GET("/events", h.ListEvents)
	g.POST("/events", h.CreateEvent)
	g.PUT("/events/:eventId", h.UpdateEvent)
	g.DELETE("/events/:eventId", h.DeleteEvent)
}

// Package designlab provides the admin-only "Design Lab" page. It hosts the
// dynamic-surface demo: a live character sheet assembled by the frame
// (Chronicle.surface) from a declarative schema, used to exercise the
// expand/collapse box, overlay, and mini->full launch primitives without
// touching real campaign data. It also
// shows the crafting bench kit in build mode, on demo recipes.
package designlab

import (
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/labstack/echo/v4"
)

// Handler serves the Design Lab admin page.
type Handler struct{}

// NewHandler creates a new Design Lab handler.
func NewHandler() *Handler {
	return &Handler{}
}

// DesignLab renders the demo page. ?campaign_id=&calendar_id= point the crafting
// bench's window at that calendar's sky.
func (h *Handler) DesignLab(c echo.Context) error {
	sky := NewBenchSky(c.QueryParam("campaign_id"), c.QueryParam("calendar_id"))
	return middleware.Render(c, http.StatusOK, SurfaceDemoPage(sky))
}

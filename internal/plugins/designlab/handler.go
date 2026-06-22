// Package designlab provides the admin-only "Design Lab" page. It hosts the
// dynamic-surface demo: a live character sheet assembled by the frame
// (Chronicle.surface) from a declarative schema, used to exercise the
// expand/collapse box, overlay, and mini->full launch primitives without
// touching real campaign data. (It formerly hosted a static component
// catalogue; the moving dynamic-surface engine is the demo now.)
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

// DesignLab renders the dynamic-surface demo page.
func (h *Handler) DesignLab(c echo.Context) error {
	return middleware.Render(c, http.StatusOK, SurfaceDemoPage())
}

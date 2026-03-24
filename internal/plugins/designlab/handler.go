// Package designlab provides an admin-only UI component showcase ("Design Lab")
// for previewing button variants, cards, badges, alerts, form inputs, typography,
// and other design system components without affecting the live site.
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

// DesignLab renders the component showcase page.
func (h *Handler) DesignLab(c echo.Context) error {
	return middleware.Render(c, http.StatusOK, DesignLabPage())
}

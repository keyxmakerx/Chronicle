package designlab

import "github.com/labstack/echo/v4"

// RegisterRoutes adds the Design Lab page to the admin route group.
func RegisterRoutes(adminGroup *echo.Group, h *Handler) {
	adminGroup.GET("/design-lab", h.DesignLab)
}

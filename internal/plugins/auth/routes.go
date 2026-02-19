package auth

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes sets up all auth-related routes on the given Echo instance.
// Auth routes are public (no session required) -- the middleware is exported
// separately for other plugins to use on their route groups.
func RegisterRoutes(e *echo.Echo, h *Handler) {
	// Public routes -- no auth required.
	e.GET("/login", h.LoginForm)
	e.POST("/login", h.Login)
	e.GET("/register", h.RegisterForm)
	e.POST("/register", h.Register)

	// Logout requires an active session.
	e.POST("/logout", h.Logout)
}

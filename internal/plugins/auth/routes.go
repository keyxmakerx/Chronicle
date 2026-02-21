package auth

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// RegisterRoutes sets up all auth-related routes on the given Echo instance.
// Auth routes are public (no session required) -- the middleware is exported
// separately for other plugins to use on their route groups.
//
// POST endpoints are rate-limited to prevent brute-force and credential
// stuffing attacks: 10 attempts per IP per minute for login, 5 for register.
func RegisterRoutes(e *echo.Echo, h *Handler) {
	// Public routes -- no auth required.
	e.GET("/login", h.LoginForm)
	e.POST("/login", h.Login, middleware.RateLimit(10, time.Minute))
	e.GET("/register", h.RegisterForm)
	e.POST("/register", h.Register, middleware.RateLimit(5, time.Minute))

	// Password reset (public, rate-limited to prevent abuse).
	e.GET("/forgot-password", h.ForgotPasswordForm)
	e.POST("/forgot-password", h.ForgotPassword, middleware.RateLimit(3, time.Minute))
	e.GET("/reset-password", h.ResetPasswordForm)
	e.POST("/reset-password", h.ResetPassword, middleware.RateLimit(3, time.Minute))

	// Logout requires an active session.
	e.POST("/logout", h.Logout)
}

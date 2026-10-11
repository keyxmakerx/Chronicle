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
	e.POST("/login/two-factor", h.LoginTwoFactor, middleware.RateLimit(10, time.Minute))
	// Provider sign-in: the start and the provider's return.
	e.GET("/login/oidc", h.LoginOIDC, middleware.RateLimit(10, time.Minute))
	e.GET("/login/oidc/callback", h.OIDCCallback, middleware.RateLimit(10, time.Minute))
	e.GET("/register", h.RegisterForm)
	e.POST("/register", h.Register, middleware.RateLimit(5, time.Minute))

	// Password reset (public, rate-limited to prevent abuse).
	e.GET("/forgot-password", h.ForgotPasswordForm)
	e.POST("/forgot-password", h.ForgotPassword, middleware.RateLimit(3, time.Minute))
	e.GET("/reset-password", h.ResetPasswordForm)
	e.POST("/reset-password", h.ResetPassword, middleware.RateLimit(3, time.Minute))

	// Guest codes: anyone may join with one; a signed-in person joins with
	// their own account. Each try checks a code, so it is throttled.
	e.GET("/join", h.JoinPage, OptionalAuth(h.service))
	e.POST("/join", h.Join, OptionalAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.GET("/account/guest", h.GuestPanelFragment, RequireAuth(h.service))
	e.POST("/account/guest/keep", h.KeepGuestAccount, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.POST("/account/guest/merge", h.MergeGuest, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))

	// Logout requires an active session.
	e.POST("/logout", h.Logout)

	// Account settings (requires auth).
	e.GET("/account", h.AccountPage, RequireAuth(h.service))
	e.PUT("/account/timezone", h.UpdateTimezoneAPI, RequireAuth(h.service))
	e.PUT("/account/view-prefs", h.UpdateViewPrefsAPI, RequireAuth(h.service))
	e.PUT("/account/notifications", h.UpdateNotifyPrefsAPI, RequireAuth(h.service))
	// Deleting your account checks your password, so it is rate-limited like
	// the other password checks.
	e.POST("/account/delete", h.DeleteAccountAPI, RequireAuth(h.service), middleware.RateLimit(5, time.Minute))
	// Linking a provider account to your own.
	e.POST("/account/sign-in/link", h.LinkOIDC, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.POST("/account/sign-in/unlink", h.UnlinkOIDCAPI, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	// Two-factor: each call checks a password or a code, so each is throttled.
	e.POST("/account/two-factor/setup", h.TwoFactorSetupAPI, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.POST("/account/two-factor/enable", h.TwoFactorEnableAPI, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.POST("/account/two-factor/disable", h.TwoFactorDisableAPI, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.POST("/account/two-factor/recovery-codes", h.TwoFactorRecoveryCodesAPI, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	// Checking a password is a deliberately expensive hash, so every route
	// that checks one is throttled like login.
	e.PUT("/account/password", h.ChangePasswordAPI, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
	e.PUT("/account/display-name", h.UpdateDisplayNameAPI, RequireAuth(h.service))
	// Same rate limit as the general /media/upload route (media/routes.go):
	// avatar uploads skip mediaService's per-campaign quota (there is no
	// campaign), so this is the only throttle standing between a signed-in
	// user and looping uploads against the shared disk-space floor.
	e.POST("/account/avatar", h.UploadAvatarAPI, RequireAuth(h.service), middleware.RateLimit(30, time.Minute))
	e.DELETE("/account/avatar", h.ClearAvatarAPI, RequireAuth(h.service), middleware.RateLimit(30, time.Minute))

	// Email change (requires auth for request, public for verification link).
	e.PUT("/account/email", h.RequestEmailChangeAPI, RequireAuth(h.service))
	e.GET("/account/email/verify", h.ConfirmEmailChange)

	// Re-authentication for sensitive operations (requires auth).
	e.POST("/account/reauth", h.ReauthConfirm, RequireAuth(h.service), middleware.RateLimit(10, time.Minute))
}

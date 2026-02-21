package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// sessionCookieName is the HTTP cookie used to store the session token.
const sessionCookieName = "chronicle_session"

// Handler handles HTTP requests for authentication (login, register, logout).
// Handlers are thin: they bind the request, call the service, and render the
// response. No business logic lives here.
type Handler struct {
	service AuthService
}

// NewHandler creates a new auth handler with the given service.
func NewHandler(service AuthService) *Handler {
	return &Handler{service: service}
}

// LoginForm renders the login page (GET /login).
func (h *Handler) LoginForm(c echo.Context) error {
	// If the user already has a valid session, redirect to dashboard.
	if token := getSessionToken(c); token != "" {
		if _, err := h.service.ValidateSession(c.Request().Context(), token); err == nil {
			return c.Redirect(http.StatusSeeOther, "/dashboard")
		}
	}

	csrfToken := middleware.GetCSRFToken(c)

	// Show success banner after password reset.
	var successMsg string
	if c.QueryParam("reset") == "success" {
		successMsg = "Your password has been reset. You can now sign in."
	}

	return middleware.Render(c, http.StatusOK, LoginPage(csrfToken, "", "", successMsg))
}

// Login processes the login form submission (POST /login).
func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	input := LoginInput{
		Email:    req.Email,
		Password: req.Password,
	}

	token, _, err := h.service.Login(c.Request().Context(), input)
	if err != nil {
		// On failure, re-render the login form with the error message.
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "invalid email or password"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}

		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, LoginForm_(csrfToken, req.Email, errMsg))
		}
		return middleware.Render(c, http.StatusOK, LoginPage(csrfToken, req.Email, errMsg, ""))
	}

	// Set the session cookie.
	setSessionCookie(c, token)

	// HTMX requests get a redirect header; browser forms get a 303 redirect.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/dashboard")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/dashboard")
}

// RegisterForm renders the registration page (GET /register).
func (h *Handler) RegisterForm(c echo.Context) error {
	// If the user already has a valid session, redirect to dashboard.
	if token := getSessionToken(c); token != "" {
		if _, err := h.service.ValidateSession(c.Request().Context(), token); err == nil {
			return c.Redirect(http.StatusSeeOther, "/dashboard")
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, nil, ""))
}

// Register processes the registration form submission (POST /register).
func (h *Handler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Basic server-side validation.
	if validationErr := validateRegisterRequest(&req); validationErr != "" {
		csrfToken := middleware.GetCSRFToken(c)
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, RegisterFormComponent(csrfToken, &req, validationErr))
		}
		return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, &req, validationErr))
	}

	input := RegisterInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
	}

	_, err := h.service.Register(c.Request().Context(), input)
	if err != nil {
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "registration failed"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}

		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, RegisterFormComponent(csrfToken, &req, errMsg))
		}
		return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, &req, errMsg))
	}

	// Auto-login after successful registration.
	loginInput := LoginInput{
		Email:    req.Email,
		Password: req.Password,
	}

	token, _, err := h.service.Login(c.Request().Context(), loginInput)
	if err != nil {
		// Registration succeeded but auto-login failed -- redirect to login.
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	setSessionCookie(c, token)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/dashboard")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/dashboard")
}

// Logout destroys the session and clears the cookie (POST /logout).
func (h *Handler) Logout(c echo.Context) error {
	token := getSessionToken(c)
	if token != "" {
		// Destroy the session in Redis. Ignore errors -- the cookie
		// will be cleared regardless.
		_ = h.service.DestroySession(c.Request().Context(), token)
	}

	// Clear the session cookie.
	clearSessionCookie(c)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/login")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/login")
}

// --- Password Reset ---

// ForgotPasswordForm renders the forgot password page (GET /forgot-password).
func (h *Handler) ForgotPasswordForm(c echo.Context) error {
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, ForgotPasswordPage(csrfToken, "", ""))
}

// ForgotPassword processes the forgot password form (POST /forgot-password).
// Always shows a success message to avoid leaking whether the email exists.
func (h *Handler) ForgotPassword(c echo.Context) error {
	email := c.FormValue("email")
	if email == "" {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, ForgotPasswordPage(csrfToken, "", "email is required"))
	}

	// Initiate reset (fire-and-forget — always returns nil to avoid leaking info).
	_ = h.service.InitiatePasswordReset(c.Request().Context(), email)

	csrfToken := middleware.GetCSRFToken(c)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ForgotPasswordSent(csrfToken, email))
	}
	return middleware.Render(c, http.StatusOK, ForgotPasswordSentPage(csrfToken, email))
}

// ResetPasswordForm renders the reset password page (GET /reset-password?token=...).
func (h *Handler) ResetPasswordForm(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.Redirect(http.StatusSeeOther, "/forgot-password")
	}

	// Validate the token to show an error early if it's invalid/expired.
	email, err := h.service.ValidateResetToken(c.Request().Context(), token)
	if err != nil {
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "invalid or expired reset link"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, email, errMsg))
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, email, ""))
}

// ResetPassword processes the new password form (POST /reset-password).
func (h *Handler) ResetPassword(c echo.Context) error {
	token := c.FormValue("token")
	password := c.FormValue("password")
	confirm := c.FormValue("confirm")

	if token == "" {
		return c.Redirect(http.StatusSeeOther, "/forgot-password")
	}

	// Validate passwords.
	if password == "" {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, "", "password is required"))
	}
	if len(password) < 8 {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, "", "password must be at least 8 characters"))
	}
	if len(password) > 128 {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, "", "password must be at most 128 characters"))
	}
	if password != confirm {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, "", "passwords do not match"))
	}

	if err := h.service.ResetPassword(c.Request().Context(), token, password); err != nil {
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "failed to reset password"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, "", errMsg))
	}

	// Success — redirect to login with a flash message.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/login?reset=success")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/login?reset=success")
}

// --- Cookie helpers ---

// getSessionToken reads the session token from the cookie.
func getSessionToken(c echo.Context) string {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return ""
	}
	return cookie.Value
}

// setSessionCookie sets the session cookie on the response. The cookie is
// HttpOnly (JS can't read it), Secure if behind TLS, and SameSite=Lax.
func setSessionCookie(c echo.Context, token string) {
	req := c.Request()
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 60 * 60, // 30 days in seconds
	})
}

// clearSessionCookie removes the session cookie by setting MaxAge to -1.
func clearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// --- Validation helpers ---

// validateRegisterRequest performs basic server-side validation on the
// registration form. Returns an error message or empty string.
func validateRegisterRequest(req *RegisterRequest) string {
	if req.Email == "" {
		return "email is required"
	}
	if req.DisplayName == "" {
		return "display name is required"
	}
	if len(req.DisplayName) < 2 {
		return "display name must be at least 2 characters"
	}
	if len(req.DisplayName) > 100 {
		return "display name must be at most 100 characters"
	}
	if req.Password == "" {
		return "password is required"
	}
	if len(req.Password) < 8 {
		return "password must be at least 8 characters"
	}
	if len(req.Password) > 128 {
		return "password must be at most 128 characters"
	}
	if req.Confirm != req.Password {
		return "passwords do not match"
	}
	return ""
}

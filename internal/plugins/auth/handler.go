package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// sessionCookieName is the HTTP cookie used to store the session token.
const sessionCookieName = "chronicle_session"

// SecurityEventLogger records security events for the admin security dashboard.
// Implemented by the admin security service; wired after both are initialized.
type SecurityEventLogger interface {
	LogEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) error
}

// Handler handles HTTP requests for authentication (login, register, logout).
// Handlers are thin: they bind the request, call the service, and render the
// response. No business logic lives here.
type Handler struct {
	service        AuthService
	securityLogger SecurityEventLogger
	sessionTTL     time.Duration // Cookie MaxAge matches Redis session TTL.
}

// NewHandler creates a new auth handler with the given service and session TTL.
func NewHandler(service AuthService, sessionTTL time.Duration) *Handler {
	return &Handler{service: service, sessionTTL: sessionTTL}
}

// SetSecurityLogger wires a security event logger for recording auth events.
func (h *Handler) SetSecurityLogger(logger SecurityEventLogger) {
	h.securityLogger = logger
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

	ip := c.RealIP()
	ua := c.Request().UserAgent()

	input := LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		IP:        ip,
		UserAgent: ua,
	}

	token, user, err := h.service.Login(c.Request().Context(), input)
	if err != nil {
		// Log failed login attempt as a security event.
		h.logSecurityEvent(c.Request().Context(), "login.failed", "", "", ip, ua, map[string]any{"email": req.Email})

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

	// Log successful login as a security event.
	h.logSecurityEvent(c.Request().Context(), "login.success", user.ID, "", ip, ua, nil)

	// Set the session cookie.
	setSessionCookie(c, token, h.sessionTTL)

	// Redirect to the requested page (e.g., invite accept), or dashboard.
	redirectTo := "/dashboard"
	if redir := c.QueryParam("redirect"); redir != "" && strings.HasPrefix(redir, "/") {
		redirectTo = redir
	}

	// HTMX requests get a redirect header; browser forms get a 303 redirect.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectTo)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectTo)
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
		Email:     req.Email,
		Password:  req.Password,
		IP:        c.RealIP(),
		UserAgent: c.Request().UserAgent(),
	}

	token, _, err := h.service.Login(c.Request().Context(), loginInput)
	if err != nil {
		// Registration succeeded but auto-login failed -- redirect to login.
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	setSessionCookie(c, token, h.sessionTTL)

	// Redirect to the requested page (e.g., invite accept), or dashboard.
	redirectTo := "/dashboard"
	if redir := c.QueryParam("redirect"); redir != "" && strings.HasPrefix(redir, "/") {
		redirectTo = redir
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", redirectTo)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, redirectTo)
}

// Logout destroys the session and clears the cookie (POST /logout).
func (h *Handler) Logout(c echo.Context) error {
	token := getSessionToken(c)
	if token != "" {
		// Capture session info before destroying for the security log.
		if session, err := h.service.ValidateSession(c.Request().Context(), token); err == nil {
			h.logSecurityEvent(c.Request().Context(), "logout", session.UserID, "", c.RealIP(), c.Request().UserAgent(), nil)
		}
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

	h.logSecurityEvent(c.Request().Context(), "password.reset_initiated", "", "", c.RealIP(), c.Request().UserAgent(), map[string]any{"email": email})

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

	h.logSecurityEvent(c.Request().Context(), "password.reset_completed", "", "", c.RealIP(), c.Request().UserAgent(), nil)

	// Success — redirect to login with a flash message.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/login?reset=success")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/login?reset=success")
}

// ChangePasswordAPI changes the authenticated user's password (PUT /account/password).
func (h *Handler) ChangePasswordAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		ConfirmPassword string `json:"confirmPassword"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if req.NewPassword != req.ConfirmPassword {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "passwords do not match"})
	}

	if err := h.service.ChangePassword(c.Request().Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to change password"})
	}

	h.logSecurityEvent(c.Request().Context(), "password.changed", userID, "", c.RealIP(), c.Request().UserAgent(), nil)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateDisplayNameAPI updates the authenticated user's display name (PUT /account/display-name).
func (h *Handler) UpdateDisplayNameAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	var req struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.service.UpdateDisplayName(c.Request().Context(), userID, req.DisplayName); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update display name"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UploadAvatarAPI handles avatar image upload for the current user.
// POST /account/avatar
func (h *Handler) UploadAvatarAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no file provided"})
	}

	// Validate file size (max 2MB).
	if file.Size > 2*1024*1024 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "avatar must be under 2MB"})
	}

	// Read file bytes for content-based MIME detection.
	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("opening uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(io.LimitReader(src, 2*1024*1024+1))
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("reading uploaded file: %w", err))
	}

	// Validate MIME type using magic bytes, not client-provided Content-Type.
	contentType := http.DetectContentType(fileBytes)
	allowedAvatarTypes := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
	}
	ext, ok := allowedAvatarTypes[contentType]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file must be a JPEG, PNG, GIF, or WebP image"})
	}

	// Use file extension from filename if it matches the detected type.
	if fileExt := filepath.Ext(file.Filename); fileExt != "" {
		for _, allowed := range allowedAvatarTypes {
			if strings.EqualFold(fileExt, allowed) {
				ext = allowed
				break
			}
		}
	}

	// Generate random filename.
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return apperror.NewInternal(fmt.Errorf("generating filename: %w", err))
	}
	filename := hex.EncodeToString(randBytes) + ext

	// Ensure avatars directory exists.
	avatarDir := filepath.Join("uploads", "avatars")
	if err := os.MkdirAll(avatarDir, 0o755); err != nil {
		return apperror.NewInternal(fmt.Errorf("creating avatar directory: %w", err))
	}

	// Save file.
	destPath := filepath.Join(avatarDir, filename)
	if err := os.WriteFile(destPath, fileBytes, 0o644); err != nil {
		return apperror.NewInternal(fmt.Errorf("saving avatar file: %w", err))
	}

	// Update user's avatar path.
	webPath := "/uploads/avatars/" + filename
	if err := h.service.UpdateAvatarPath(c.Request().Context(), userID, &webPath); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update avatar"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "avatar_path": webPath})
}

// RequestEmailChangeAPI initiates an email change (PUT /account/email).
// Requires the user's current password for security.
func (h *Handler) RequestEmailChangeAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	var req struct {
		NewEmail        string `json:"newEmail"`
		CurrentPassword string `json:"currentPassword"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.service.RequestEmailChange(c.Request().Context(), userID, req.NewEmail, req.CurrentPassword); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to initiate email change"})
	}

	h.logSecurityEvent(c.Request().Context(), "email.change_requested", userID, "", c.RealIP(), c.Request().UserAgent(), map[string]any{"new_email": req.NewEmail})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "message": "Verification email sent to " + req.NewEmail})
}

// ConfirmEmailChange handles the verification link click (GET /account/email/verify?token=...).
// On success, redirects to login since all sessions are invalidated.
func (h *Handler) ConfirmEmailChange(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.Redirect(http.StatusSeeOther, "/account")
	}

	if err := h.service.ConfirmEmailChange(c.Request().Context(), token); err != nil {
		// Render a simple error page for invalid/expired tokens.
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := "invalid or expired verification link"
		if appErr, ok := err.(*apperror.AppError); ok {
			errMsg = appErr.Message
		}
		return middleware.Render(c, http.StatusOK, EmailVerifyResultPage(false, errMsg, csrfToken))
	}

	h.logSecurityEvent(c.Request().Context(), "email.change_confirmed", "", "", c.RealIP(), c.Request().UserAgent(), nil)

	return middleware.Render(c, http.StatusOK, EmailVerifyResultPage(true, "", ""))
}

// logSecurityEvent fires a security event if a logger is wired. Fire-and-forget
// so auth operations are never blocked by logging failures.
func (h *Handler) logSecurityEvent(ctx context.Context, eventType, userID, actorID, ip, userAgent string, details map[string]any) {
	if h.securityLogger != nil {
		_ = h.securityLogger.LogEvent(ctx, eventType, userID, actorID, ip, userAgent, details)
	}
}

// --- Account Settings ---

// AccountPage renders the user account settings page (GET /account).
func (h *Handler) AccountPage(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return c.Redirect(http.StatusSeeOther, "/login")
	}

	user, err := h.service.GetUser(c.Request().Context(), userID)
	if err != nil {
		return apperror.NewInternal(err)
	}

	csrfToken := middleware.GetCSRFToken(c)
	timezones := commonTimezones()

	return middleware.Render(c, http.StatusOK, AccountPage(user, csrfToken, timezones))
}

// UpdateTimezoneAPI updates the user's timezone preference (PUT /account/timezone).
func (h *Handler) UpdateTimezoneAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
	}

	var req struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	if err := h.service.UpdateTimezone(c.Request().Context(), userID, req.Timezone); err != nil {
		if appErr, ok := err.(*apperror.AppError); ok {
			return c.JSON(appErr.Code, map[string]string{"error": appErr.Message})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update timezone"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// commonTimezones returns a curated list of IANA timezones for the dropdown.
// Covers all major regions without overwhelming the user with obscure entries.
func commonTimezones() []string {
	zones := []string{}
	regions := []string{
		"Africa/Cairo", "Africa/Johannesburg", "Africa/Lagos", "Africa/Nairobi",
		"America/Anchorage", "America/Argentina/Buenos_Aires", "America/Bogota",
		"America/Chicago", "America/Denver", "America/Halifax", "America/Los_Angeles",
		"America/Mexico_City", "America/New_York", "America/Phoenix",
		"America/Santiago", "America/Sao_Paulo", "America/St_Johns", "America/Toronto",
		"America/Vancouver",
		"Asia/Baghdad", "Asia/Bangkok", "Asia/Colombo", "Asia/Dubai", "Asia/Hong_Kong",
		"Asia/Istanbul", "Asia/Jakarta", "Asia/Karachi", "Asia/Kolkata", "Asia/Manila",
		"Asia/Seoul", "Asia/Shanghai", "Asia/Singapore", "Asia/Taipei", "Asia/Tehran",
		"Asia/Tokyo",
		"Atlantic/Reykjavik",
		"Australia/Adelaide", "Australia/Brisbane", "Australia/Melbourne",
		"Australia/Perth", "Australia/Sydney",
		"Europe/Amsterdam", "Europe/Athens", "Europe/Berlin", "Europe/Brussels",
		"Europe/Dublin", "Europe/Helsinki", "Europe/Lisbon", "Europe/London",
		"Europe/Madrid", "Europe/Moscow", "Europe/Oslo", "Europe/Paris",
		"Europe/Prague", "Europe/Rome", "Europe/Stockholm", "Europe/Vienna",
		"Europe/Warsaw", "Europe/Zurich",
		"Pacific/Auckland", "Pacific/Fiji", "Pacific/Guam", "Pacific/Honolulu",
	}
	// Validate each timezone to ensure it's loadable.
	for _, tz := range regions {
		if _, err := time.LoadLocation(tz); err == nil {
			zones = append(zones, tz)
		}
	}
	return zones
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
func setSessionCookie(c echo.Context, token string, ttl time.Duration) {
	req := c.Request()
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
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

// ReauthConfirm handles password re-confirmation for sensitive admin operations
// (POST /account/reauth). It validates the admin's password and, if correct,
// sets a short-lived reauth token in Redis that allows sensitive operations
// for 5 minutes without re-prompting.
func (h *Handler) ReauthConfirm(c echo.Context) error {
	session := GetSession(c)
	if session == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	password := c.FormValue("password")
	if password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "password is required",
		})
	}

	if err := h.service.ConfirmReauth(c.Request().Context(), session.UserID, password); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "incorrect password",
		})
	}

	// Log the reauth event for the security dashboard.
	if h.securityLogger != nil {
		_ = h.securityLogger.LogEvent(
			c.Request().Context(),
			"reauth_confirmed",
			session.UserID, session.UserID,
			c.RealIP(), c.Request().UserAgent(),
			nil,
		)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "confirmed",
	})
}

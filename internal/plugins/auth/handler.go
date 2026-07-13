package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// sanitizeRedirect returns raw only if it is a safe same-site path (starts with
// a single "/", not "//" or "/\" which are protocol-relative open-redirect
// vectors). Anything else becomes "" so the caller falls back to a default.
func sanitizeRedirect(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") {
		return ""
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return ""
	}
	return raw
}

// extractInviteToken pulls a campaign invite token out of a post-register
// redirect that targets the invite-accept page. Returns "" for any other
// destination, so only genuine invite-flow registrations carry a token into the
// gate.
func extractInviteToken(redirect string) string {
	if redirect == "" {
		return ""
	}
	u, err := url.Parse(redirect)
	if err != nil || u.Path != "/invites/accept" {
		return ""
	}
	return u.Query().Get("token")
}

// sessionCookieName is the bare session cookie name, used over plain HTTP (dev).
const sessionCookieName = "chronicle_session"

// sessionCookieSecureName is the __Host--prefixed name used over HTTPS. The
// prefix is a browser-enforced guarantee that the cookie was set Secure, with
// Path=/ and no Domain — so no subdomain can inject or overwrite the session
// (mirrors the CSRF cookie, middleware/csrf.go).
const sessionCookieSecureName = "__Host-chronicle_session"

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

	// Auto-recovery banner: the CSRF middleware bounces a stale/missing-token
	// login POST here with ?expired=1. This GET already re-issued a fresh token
	// (above), so the reloaded form works — we just explain why they're back.
	var errMsg string
	if c.QueryParam("expired") == "1" {
		errMsg = middleware.CSRFFriendlyMessage
	}

	return middleware.Render(c, http.StatusOK, LoginPage(csrfToken, "", errMsg, successMsg))
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
		errMsg := apperror.UserMessage(err, "invalid email or password")

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
	return middleware.HTMXRedirect(c, redirectTo)
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
	redirect := sanitizeRedirect(c.QueryParam("redirect"))
	inviteToken := extractInviteToken(redirect)

	// Render the friendly gated panel instead of the form when the site
	// registration mode blocks this visitor (invite-only without a valid invite,
	// or closed). The first-user bootstrap is reported allowed, so a fresh
	// install always shows the form.
	mode, allowed, err := h.service.RegistrationStatus(c.Request().Context(), inviteToken)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, nil, "", redirect, !allowed, mode))
}

// Register processes the registration form submission (POST /register).
func (h *Handler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	// Post-register destination (carried as a hidden form field) + any invite
	// token it embeds, which the service uses to satisfy invite-only mode.
	redirect := sanitizeRedirect(c.FormValue("redirect"))
	inviteToken := extractInviteToken(redirect)

	// Basic server-side validation.
	if validationErr := validateRegisterRequest(&req); validationErr != "" {
		csrfToken := middleware.GetCSRFToken(c)
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, RegisterFormComponent(csrfToken, &req, validationErr, redirect))
		}
		return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, &req, validationErr, redirect, false, ""))
	}

	input := RegisterInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		InviteToken: inviteToken,
	}

	_, err := h.service.Register(c.Request().Context(), input)
	if err != nil {
		csrfToken := middleware.GetCSRFToken(c)

		// A blocked registration gate (403) renders the friendly gated panel, not
		// a form-level error — the visitor can't fix it by editing the form.
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusForbidden {
			mode, _, _ := h.service.RegistrationStatus(c.Request().Context(), inviteToken)
			if middleware.IsHTMX(c) {
				return middleware.Render(c, http.StatusOK, registrationGatedPanel(mode, redirect))
			}
			return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, &req, "", redirect, true, mode))
		}

		errMsg := apperror.UserMessage(err, "registration failed")
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, RegisterFormComponent(csrfToken, &req, errMsg, redirect))
		}
		return middleware.Render(c, http.StatusOK, RegisterPage(csrfToken, &req, errMsg, redirect, false, ""))
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

	// Redirect to the requested page (e.g., invite accept, so the invite is
	// consumed right after signup), or dashboard. Uses the sanitized hidden-field
	// redirect — the query param is not present on the HTMX form POST.
	redirectTo := "/dashboard"
	if redirect != "" {
		redirectTo = redirect
	}

	return middleware.HTMXRedirect(c, redirectTo)
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

	return middleware.HTMXRedirect(c, "/login")
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
		errMsg := apperror.UserMessage(err, "invalid or expired reset link")
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
		errMsg := apperror.UserMessage(err, "failed to reset password")
		return middleware.Render(c, http.StatusOK, ResetPasswordPage(csrfToken, token, "", errMsg))
	}

	h.logSecurityEvent(c.Request().Context(), "password.reset_completed", "", "", c.RealIP(), c.Request().UserAgent(), nil)

	// Success — redirect to login with a flash message.
	return middleware.HTMXRedirect(c, "/login?reset=success")
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
		return apperror.NewBadRequest("invalid request body")
	}

	if req.NewPassword != req.ConfirmPassword {
		return apperror.NewBadRequest("new password and confirmation do not match")
	}

	if err := h.service.ChangePassword(c.Request().Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		return err
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
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateDisplayName(c.Request().Context(), userID, req.DisplayName); err != nil {
		return err
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
		return apperror.NewBadRequest("no avatar file provided")
	}

	// Validate file size (max 2MB).
	if file.Size > 2*1024*1024 {
		return apperror.NewBadRequest("avatar must be under 2MB")
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
		return apperror.NewBadRequest("avatar must be a JPEG, PNG, GIF, or WebP image")
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
		return apperror.NewInternal(fmt.Errorf("updating avatar path: %w", err))
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
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.RequestEmailChange(c.Request().Context(), userID, req.NewEmail, req.CurrentPassword); err != nil {
		return err
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
		errMsg := apperror.UserMessage(err, "invalid or expired verification link")
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
		return apperror.NewUnauthorized("not authenticated")
	}

	var req struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.service.UpdateTimezone(c.Request().Context(), userID, req.Timezone); err != nil {
		return err
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

// sessionCookieNameFor returns the cookie name appropriate for the request's
// scheme: the __Host--prefixed name over HTTPS (the browser then guarantees the
// cookie was set Secure, with Path=/ and no Domain — no subdomain can forge or
// overwrite it), the bare name over plain HTTP so local dev over http:// still
// works. Mirrors the CSRF cookie's scheme detection (one shared implementation).
func sessionCookieNameFor(req *http.Request) string {
	if middleware.SchemeIsSecure(req) {
		return sessionCookieSecureName
	}
	return sessionCookieName
}

// getSessionToken reads the session token, preferring the __Host- cookie over
// HTTPS and falling back to the bare name only when no __Host- cookie is present.
// This mirrors the CSRF cookie's dual-read (middleware/csrf.go): behind a
// TLS-terminating proxy the scheme this codebase derives can differ between the
// request that SET the cookie and a later request that READS it (the documented
// C-AUTH-LOGIN-CSRF-FIX root cause), so a single-name read would silently drop
// the session on a scheme flip. Preferring __Host- keeps the anti-forgery
// property for logged-in users (their __Host- cookie always wins over a
// subdomain-injected bare cookie); the bare fallback only applies pre-login or
// to a pre-upgrade session, and login always rotates the token, so it does not
// enable session fixation. Over plain HTTP only the bare name is read.
func getSessionToken(c echo.Context) string {
	return readSessionToken(c.Request())
}

// readSessionToken is the raw-request form of getSessionToken, shared with
// callers outside echo (the WebSocket handshake). Over HTTPS it tries the
// __Host- name first, then the bare name; over HTTP only the bare name.
func readSessionToken(req *http.Request) string {
	names := []string{sessionCookieName}
	if middleware.SchemeIsSecure(req) {
		names = []string{sessionCookieSecureName, sessionCookieName}
	}
	for _, name := range names {
		if cookie, err := req.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

// ReadSessionToken reads the session token from an http.Request using the same
// scheme-aware dual-read as the web handlers. For callers outside this package
// (the WebSocket handshake) that don't have an echo.Context.
func ReadSessionToken(req *http.Request) string {
	return readSessionToken(req)
}

// setSessionCookie sets the session cookie on the response. The cookie is
// HttpOnly (JS can't read it), Secure + __Host--prefixed behind TLS, and
// SameSite=Lax. The __Host- prefix requires Secure=true, Path=/, and no Domain.
func setSessionCookie(c echo.Context, token string, ttl time.Duration) {
	req := c.Request()
	secure := middleware.SchemeIsSecure(req)
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieNameFor(req),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// clearSessionCookie removes the session cookie by setting MaxAge to -1. It
// clears BOTH the scheme-appropriate name AND the bare legacy name, so a stale
// pre-upgrade cookie can't linger in the browser after logout.
func clearSessionCookie(c echo.Context) {
	req := c.Request()
	names := []string{sessionCookieNameFor(req)}
	if n := sessionCookieName; names[0] != n {
		names = append(names, n)
	}
	for _, name := range names {
		c.SetCookie(&http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   name == sessionCookieSecureName,
			MaxAge:   -1,
		})
	}
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
		return apperror.NewUnauthorized("authentication required")
	}

	password := c.FormValue("password")
	if password == "" {
		return apperror.NewBadRequest("password is required")
	}

	if err := h.service.ConfirmReauth(c.Request().Context(), session.UserID, password); err != nil {
		return apperror.NewUnauthorized("incorrect password")
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

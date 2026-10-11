package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
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
	if c.QueryParam("deleted") == "1" {
		successMsg = "Your account has been deleted."
	}

	// Auto-recovery banner: the CSRF middleware bounces a stale/missing-token
	// login POST here with ?expired=1. This GET already re-issued a fresh token
	// (above), so the reloaded form works — we just explain why they're back.
	var errMsg string
	if c.QueryParam("expired") == "1" {
		errMsg = middleware.CSRFFriendlyMessage
	}

	// Post-login destination, carried into the form as a hidden field because
	// the query param does not survive the HTMX form POST.
	redirect := sanitizeRedirect(c.QueryParam("redirect"))

	return middleware.Render(c, http.StatusOK, LoginPage(csrfToken, "", errMsg, successMsg, redirect, h.service.LoginOptions(c.Request().Context())))
}

// Login processes the login form submission (POST /login).
func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}

	ip := c.RealIP()
	ua := c.Request().UserAgent()

	// Post-login destination. The form field is the live path; the query param
	// is a fallback for a plain GET-then-POST. Both MUST go through
	// sanitizeRedirect so a protocol-relative open-redirect ("//evil.example")
	// can never reach a Location header.
	redirect := sanitizeRedirect(c.FormValue("redirect"))
	if redirect == "" {
		redirect = sanitizeRedirect(c.QueryParam("redirect"))
	}

	input := LoginInput{
		Email:         req.Email,
		Password:      req.Password,
		IP:            ip,
		UserAgent:     ua,
		TrustedDevice: readTrustedDevice(c.Request()),
	}

	token, user, err := h.service.Login(c.Request().Context(), input)
	var need *TwoFactorRequired
	if errors.As(err, &need) {
		csrfToken := middleware.GetCSRFToken(c)
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, TwoFactorForm_(csrfToken, need.Challenge, redirect, ""))
		}
		return middleware.Render(c, http.StatusOK, TwoFactorPage(csrfToken, need.Challenge, redirect, ""))
	}
	if err != nil {
		// Log failed login attempt as a security event.
		h.logSecurityEvent(c.Request().Context(), "login.failed", "", "", ip, ua, map[string]any{"email": req.Email})

		// On failure, re-render the login form with the error message.
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "invalid email or password")

		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, LoginForm_(csrfToken, req.Email, errMsg, redirect))
		}
		return middleware.Render(c, http.StatusOK, LoginPage(csrfToken, req.Email, errMsg, "", redirect, h.service.LoginOptions(c.Request().Context())))
	}

	// Log successful login as a security event.
	h.logSecurityEvent(c.Request().Context(), "login.success", user.ID, "", ip, ua, nil)

	// Set the session cookie.
	setSessionCookie(c, token, h.sessionTTL)

	// Redirect to the requested page (e.g., invite accept, or the availability
	// grid a solicitation email deep-linked to), or dashboard. The value is
	// already sanitized above.
	redirectTo := "/dashboard"
	if redirect != "" {
		redirectTo = redirect
	}

	// HTMX requests get a redirect header; browser forms get a 303 redirect.
	return middleware.HTMXRedirect(c, redirectTo)
}

// LoginTwoFactor is the code step of a sign-in (POST /login/two-factor).
// The challenge from the password step stands in for the email and
// password, so neither is sent twice.
func (h *Handler) LoginTwoFactor(c echo.Context) error {
	ip := c.RealIP()
	ua := c.Request().UserAgent()
	redirect := sanitizeRedirect(c.FormValue("redirect"))
	challenge := c.FormValue("challenge")

	res, err := h.service.CompleteTwoFactorLogin(c.Request().Context(), TwoFactorLoginInput{
		Challenge: challenge,
		Code:      c.FormValue("code"),
		Remember:  c.FormValue("remember") != "",
		IP:        ip,
		UserAgent: ua,
	})
	if err != nil {
		h.logSecurityEvent(c.Request().Context(), "login.two_factor_failed", "", "", ip, ua, nil)
		csrfToken := middleware.GetCSRFToken(c)
		errMsg := apperror.UserMessage(err, "that code isn't right")
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, TwoFactorForm_(csrfToken, challenge, redirect, errMsg))
		}
		return middleware.Render(c, http.StatusOK, TwoFactorPage(csrfToken, challenge, redirect, errMsg))
	}

	h.logSecurityEvent(c.Request().Context(), "login.success", res.User.ID, "", ip, ua, map[string]any{"two_factor": true})
	setSessionCookie(c, res.SessionToken, h.sessionTTL)
	if res.TrustedDevice != "" {
		setTrustedDeviceCookie(c, res.TrustedDevice)
	}
	redirectTo := "/dashboard"
	if redirect != "" {
		redirectTo = redirect
	}
	return middleware.HTMXRedirect(c, redirectTo)
}

// oidcStateCookie names the cookie that ties a provider round trip to the
// browser that started it, so a callback link can't be replayed into
// someone else's browser.
const (
	oidcStateCookieName       = "chronicle_oidc_state"
	oidcStateCookieSecureName = "__Host-chronicle_oidc_state"
)

// SetOIDCStateCookie remembers the round trip's state in this browser. It
// is Lax so it comes back on the provider's top-level redirect.
func SetOIDCStateCookie(c echo.Context, state string) {
	secure := middleware.SchemeIsSecure(c.Request())
	name := oidcStateCookieName
	if secure {
		name = oidcStateCookieSecureName
	}
	c.SetCookie(&http.Cookie{Name: name, Value: state, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(oidcStateTTL.Seconds())})
}

func readOIDCStateCookie(c echo.Context) string {
	names := []string{oidcStateCookieName}
	if middleware.SchemeIsSecure(c.Request()) {
		names = []string{oidcStateCookieSecureName, oidcStateCookieName}
	}
	for _, n := range names {
		if ck, err := c.Cookie(n); err == nil && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

func clearOIDCStateCookie(c echo.Context) {
	for _, n := range []string{oidcStateCookieName, oidcStateCookieSecureName} {
		c.SetCookie(&http.Cookie{Name: n, Value: "", Path: "/", HttpOnly: true,
			Secure: n == oidcStateCookieSecureName, MaxAge: -1})
	}
}

// loginError shows the sign-in page with a message, for provider failures.
func (h *Handler) loginError(c echo.Context, msg string) error {
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, LoginPage(csrfToken, "", msg, "", "", h.service.LoginOptions(c.Request().Context())))
}

// LoginOIDC sends the browser to the provider (GET /login/oidc).
func (h *Handler) LoginOIDC(c echo.Context) error {
	redirect := sanitizeRedirect(c.QueryParam("redirect"))
	authURL, state, err := h.service.BeginOIDC(c.Request().Context(), OIDCModeLogin, "", redirect)
	if err != nil {
		return h.loginError(c, apperror.UserMessage(err, "sign-in with your provider isn't working right now"))
	}
	SetOIDCStateCookie(c, state)
	return c.Redirect(http.StatusSeeOther, authURL)
}

// LinkOIDC starts linking the signed-in person's provider account
// (POST /account/sign-in/link). It's a POST so another site can't start it.
func (h *Handler) LinkOIDC(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}
	authURL, state, err := h.service.BeginOIDC(c.Request().Context(), OIDCModeLink, userID, "")
	if err != nil {
		return err
	}
	SetOIDCStateCookie(c, state)
	return middleware.HTMXRedirect(c, authURL)
}

// UnlinkOIDCAPI removes the link (POST /account/sign-in/unlink).
func (h *Handler) UnlinkOIDCAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}
	if err := h.service.UnlinkOIDC(c.Request().Context(), userID); err != nil {
		return err
	}
	h.logSecurityEvent(c.Request().Context(), "sign_in.unlinked", userID, userID, c.RealIP(), c.Request().UserAgent(), nil)
	return c.NoContent(http.StatusNoContent)
}

// OIDCCallback is where the provider sends the browser back
// (GET /login/oidc/callback). The state must match this browser's cookie
// before anything else is looked at.
func (h *Handler) OIDCCallback(c echo.Context) error {
	ctx := c.Request().Context()
	ip, ua := c.RealIP(), c.Request().UserAgent()
	state := c.QueryParam("state")
	cookie := readOIDCStateCookie(c)
	clearOIDCStateCookie(c)
	if state == "" || cookie == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookie)) != 1 {
		return h.loginError(c, "That sign-in didn't start in this browser, or took too long. Start again.")
	}
	sessionUserID := ""
	if token := getSessionToken(c); token != "" {
		if sess, err := h.service.ValidateSession(ctx, token); err == nil {
			sessionUserID = sess.UserID
		}
	}
	res, err := h.service.FinishOIDC(ctx, OIDCCallbackInput{
		State: state, Code: c.QueryParam("code"), ProviderError: c.QueryParam("error"),
		SessionUserID: sessionUserID, IP: ip, UserAgent: ua,
		TrustedDevice: readTrustedDevice(c.Request()),
	})
	mode := OIDCModeLogin
	if res != nil {
		mode = res.Mode
	}
	switch mode {
	case OIDCModeTest:
		return c.Redirect(http.StatusSeeOther, "/admin/security?tab=provider")
	case OIDCModeLink:
		outcome := "linked"
		if err != nil {
			outcome = "failed"
			var appErr *apperror.AppError
			if errors.As(err, &appErr) && appErr.Code == http.StatusConflict {
				outcome = "taken"
			}
		} else {
			h.logSecurityEvent(ctx, "sign_in.linked", sessionUserID, sessionUserID, ip, ua, nil)
		}
		return c.Redirect(http.StatusSeeOther, "/account?signin="+outcome+"#sign-in")
	}
	var need *TwoFactorRequired
	if errors.As(err, &need) {
		return middleware.Render(c, http.StatusOK, TwoFactorPage(middleware.GetCSRFToken(c), need.Challenge, sanitizeRedirect(res.Redirect), ""))
	}
	if err != nil {
		h.logSecurityEvent(ctx, "login.failed", "", "", ip, ua, map[string]any{"provider": true})
		return h.loginError(c, apperror.UserMessage(err, "sign-in with your provider didn't work"))
	}
	h.logSecurityEvent(ctx, "login.success", res.User.ID, "", ip, ua, map[string]any{"provider": true})
	setSessionCookie(c, res.SessionToken, h.sessionTTL)
	redirectTo := "/dashboard"
	if r := sanitizeRedirect(res.Redirect); r != "" {
		redirectTo = r
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
	return middleware.Render(c, http.StatusOK, ForgotPasswordPage(csrfToken, "", "", h.service.CanEmailResetLinks(c.Request().Context())))
}

// ForgotPassword processes the forgot password form (POST /forgot-password).
// Always shows a success message to avoid leaking whether the email exists.
func (h *Handler) ForgotPassword(c echo.Context) error {
	email := c.FormValue("email")
	if email == "" {
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, ForgotPasswordPage(csrfToken, "", "email is required", true))
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

// avatarMaxUploadBytes caps the avatar upload before it ever reaches the
// media service. media.MediaService.Upload applies its own quota/size
// checks, but reading an unbounded body into memory here would let a
// caller exhaust memory before those checks ever run.
const avatarMaxUploadBytes = 2 * 1024 * 1024

// UploadAvatarAPI handles avatar image upload for the current user
// (POST /account/avatar). Delegates entirely to mediaService.Upload (via
// AuthService.UploadAvatar): magic-byte validation, EXIF stripping/
// re-encode, the per-file size limit, disk-space check and 0640
// permissions all happen there — this handler only binds the multipart
// form and renders the result. The per-campaign storage/file-count quota
// does NOT apply (there is no campaign to scope it to); the route-level
// rate limit (routes.go) is the only throttle on repeated avatar uploads.
func (h *Handler) UploadAvatarAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	file, err := c.FormFile("avatar")
	if err != nil {
		return apperror.NewBadRequest("no avatar file provided")
	}
	if file.Size > avatarMaxUploadBytes {
		return apperror.NewBadRequest("avatar must be under 2MB")
	}

	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("opening uploaded file: %w", err))
	}
	defer func() { _ = src.Close() }()

	fileBytes, err := io.ReadAll(io.LimitReader(src, avatarMaxUploadBytes+1))
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("reading uploaded file: %w", err))
	}

	// The declared Content-Type header is client-supplied and untrusted;
	// sniff the actual bytes instead (mirrors the boot reconciler's legacy
	// avatar migration). An avatar must be an image -- the media service's
	// own MIME allowlist also accepts audio, which has no business being a
	// profile picture.
	mimeType := http.DetectContentType(fileBytes)
	if !strings.HasPrefix(mimeType, "image/") {
		return apperror.NewBadRequest("avatar must be an image")
	}

	_, url, err := h.service.UploadAvatar(c.Request().Context(), userID, fileBytes, file.Filename, mimeType)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "avatar_path": url})
}

// ClearAvatarAPI removes the current user's avatar (DELETE /account/avatar).
// A user may only clear their own avatar: userID comes from the session,
// never from a request parameter.
func (h *Handler) ClearAvatarAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	if err := h.service.ClearAvatar(c.Request().Context(), userID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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
	timezones := timeutil.CommonZones()

	// A failed read shows the defaults: the page still works and a save fixes it.
	prefs, err := h.service.GetViewPrefs(c.Request().Context(), userID)
	if err != nil {
		slog.Warn("reading view prefs", slog.String("user_id", userID), slog.Any("error", err))
	}

	notify, err := h.service.GetNotifyPrefs(c.Request().Context(), userID)
	if err != nil {
		slog.Warn("reading notification choices", slog.String("user_id", userID), slog.Any("error", err))
	}

	owned, err := h.service.OwnedCampaigns(c.Request().Context(), userID)
	if err != nil {
		slog.Warn("listing owned campaigns", slog.String("user_id", userID), slog.Any("error", err))
	}

	twoFactor, err := h.service.TwoFactorStatus(c.Request().Context(), userID)
	if err != nil {
		slog.Warn("reading two-factor status", slog.String("user_id", userID), slog.Any("error", err))
	}

	methods, err := h.service.SignInMethods(c.Request().Context(), userID)
	if err != nil {
		slog.Warn("reading sign-in methods", slog.String("user_id", userID), slog.Any("error", err))
	}

	return middleware.Render(c, http.StatusOK, AccountPage(user, csrfToken, timezones, prefs, notify, owned, twoFactor,
		methods, signInOutcome(c.QueryParam("signin"))))
}

// DeleteAccountAPI deletes the signed-in person's own account
// (POST /account/delete) after checking their password and the typed
// confirmation, then signs this browser out.
func (h *Handler) DeleteAccountAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}
	var req DeleteAccountInput
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.service.DeleteOwnAccount(c.Request().Context(), userID, req); err != nil {
		return err
	}
	h.logSecurityEvent(c.Request().Context(), "account.deleted", userID, "", c.RealIP(), c.Request().UserAgent(), nil)
	clearSessionCookie(c)
	return c.JSON(http.StatusOK, map[string]string{"redirect": "/login?deleted=1"})
}

// twoFactorRequest is the body of the account page's two-factor calls; each
// call reads only the field it needs.
type twoFactorRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func bindTwoFactorRequest(c echo.Context) (string, twoFactorRequest, error) {
	userID := GetUserID(c)
	if userID == "" {
		return "", twoFactorRequest{}, apperror.NewUnauthorized("not authenticated")
	}
	var req twoFactorRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return "", twoFactorRequest{}, apperror.NewBadRequest("invalid request body")
	}
	return userID, req, nil
}

// TwoFactorSetupAPI starts turning two-factor on (POST /account/two-factor/setup).
func (h *Handler) TwoFactorSetupAPI(c echo.Context) error {
	userID, req, err := bindTwoFactorRequest(c)
	if err != nil {
		return err
	}
	setup, err := h.service.BeginTwoFactor(c.Request().Context(), userID, req.Password)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, setup)
}

// TwoFactorEnableAPI finishes turning two-factor on and returns the recovery
// codes (POST /account/two-factor/enable).
func (h *Handler) TwoFactorEnableAPI(c echo.Context) error {
	userID, req, err := bindTwoFactorRequest(c)
	if err != nil {
		return err
	}
	codes, err := h.service.EnableTwoFactor(c.Request().Context(), userID, req.Code)
	if err != nil {
		return err
	}
	h.logSecurityEvent(c.Request().Context(), "two_factor.enabled", userID, userID, c.RealIP(), c.Request().UserAgent(), nil)
	return c.JSON(http.StatusOK, map[string]any{"recoveryCodes": codes})
}

// TwoFactorDisableAPI turns two-factor off (POST /account/two-factor/disable).
func (h *Handler) TwoFactorDisableAPI(c echo.Context) error {
	userID, req, err := bindTwoFactorRequest(c)
	if err != nil {
		return err
	}
	if err := h.service.DisableTwoFactor(c.Request().Context(), userID, req.Password, req.Code); err != nil {
		return err
	}
	h.logSecurityEvent(c.Request().Context(), "two_factor.disabled", userID, userID, c.RealIP(), c.Request().UserAgent(), nil)
	return c.NoContent(http.StatusNoContent)
}

// TwoFactorRecoveryCodesAPI replaces the recovery codes
// (POST /account/two-factor/recovery-codes).
func (h *Handler) TwoFactorRecoveryCodesAPI(c echo.Context) error {
	userID, req, err := bindTwoFactorRequest(c)
	if err != nil {
		return err
	}
	codes, err := h.service.RegenerateRecoveryCodes(c.Request().Context(), userID, req.Password, req.Code)
	if err != nil {
		return err
	}
	h.logSecurityEvent(c.Request().Context(), "two_factor.codes_replaced", userID, userID, c.RealIP(), c.Request().UserAgent(), nil)
	return c.JSON(http.StatusOK, map[string]any{"recoveryCodes": codes})
}

// UpdateNotifyPrefsAPI saves the signed-in person's notification choices
// (PUT /account/notifications). The body is partial: only the switches sent change.
func (h *Handler) UpdateNotifyPrefsAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	var req notifyprefs.Update
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	prefs, err := h.service.UpdateNotifyPrefs(c.Request().Context(), userID, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, prefs)
}

// UpdateViewPrefsAPI saves the signed-in person's own viewing choices
// (PUT /account/view-prefs). The body is partial: only the keys sent change.
func (h *Handler) UpdateViewPrefsAPI(c echo.Context) error {
	userID := GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("not authenticated")
	}

	var req UpdateViewPrefsInput
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	prefs, err := h.service.UpdateViewPrefs(c.Request().Context(), userID, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, prefs)
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
// HTTPS and falling back to the bare name only when no __Host- cookie is
// present. This mirrors the CSRF cookie's dual-read (middleware/csrf.go):
// behind a TLS-terminating proxy the derived scheme can differ between the
// request that set the cookie and one that reads it, so a single-name read
// can silently drop the session on a scheme flip. Preferring __Host- keeps
// its anti-forgery property (it always wins over a subdomain-injected bare
// cookie); the bare fallback does not enable session fixation because login
// always rotates the token. Over plain HTTP only the bare name is read.
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

// trustedDeviceCookieName / trustedDeviceCookieSecureName name the
// remembered-device cookie, __Host- prefixed over HTTPS like the session.
const (
	trustedDeviceCookieName       = "chronicle_trusted_device"
	trustedDeviceCookieSecureName = "__Host-chronicle_trusted_device"
)

// setTrustedDeviceCookie remembers this device for the code step. It only
// skips the code; the password is still asked every time.
func setTrustedDeviceCookie(c echo.Context, token string) {
	req := c.Request()
	secure := middleware.SchemeIsSecure(req)
	name := trustedDeviceCookieName
	if secure {
		name = trustedDeviceCookieSecureName
	}
	c.SetCookie(&http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(trustedDeviceTTL.Seconds()),
	})
}

// readTrustedDevice reads the remembered-device cookie, preferring the
// __Host- name over HTTPS as readSessionToken does.
func readTrustedDevice(req *http.Request) string {
	names := []string{trustedDeviceCookieName}
	if middleware.SchemeIsSecure(req) {
		names = []string{trustedDeviceCookieSecureName, trustedDeviceCookieName}
	}
	for _, name := range names {
		if cookie, err := req.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
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
		// A wrong password here is someone with a signed-in session who
		// doesn't know its password, so the security dashboard sees it.
		if h.securityLogger != nil {
			_ = h.securityLogger.LogEvent(
				c.Request().Context(),
				"reauth_failed",
				session.UserID, session.UserID,
				c.RealIP(), c.Request().UserAgent(),
				nil,
			)
		}
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

// signInOutcome turns the fixed ?signin= codes from a link round trip into
// the account page's message. Only these words are ever shown, so the
// address bar can't put text of its own on the page.
func signInOutcome(code string) string {
	switch code {
	case "linked":
		return "Linked. You can now sign in with either."
	case "taken":
		return "That account is already linked to someone else's Chronicle account."
	case "failed":
		return "Linking didn't work. Try again, or ask your site admin to check the provider setting."
	}
	return ""
}

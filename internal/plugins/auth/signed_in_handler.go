package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// Security event types for the account page's sign-in list. They match the
// admin plugin's constants, which label them on the security page.
const (
	eventSessionSignedOut    = "account.session_signed_out"
	eventSignedOutEverywhere = "account.signed_out_everywhere"
	eventAppDisconnected     = "account.app_disconnected"
)

// SetConnectedApps wires the apps list into the account page. Unwired, the
// page lists browsers only.
func (h *Handler) SetConnectedApps(apps ConnectedApps) {
	h.apps = apps
}

// signedInView loads what the Where you're signed in card shows. A failed
// read is logged and shown as a message on the card, not a broken page.
func (h *Handler) signedInView(ctx context.Context, userID, token, csrfToken string) SignedInView {
	v := SignedInView{CSRFToken: csrfToken}
	sessions, err := h.service.ListMySessions(ctx, userID, token)
	if err != nil {
		slog.Warn("listing own sessions", slog.String("user_id", userID), slog.Any("error", err))
		v.LoadFailed = true
	}
	v.Sessions = sessions
	if h.apps != nil {
		apps, err := h.apps.ListForUser(ctx, userID)
		if err != nil {
			slog.Warn("listing connected apps", slog.String("user_id", userID), slog.Any("error", err))
			v.LoadFailed = true
		}
		v.Apps = apps
	}
	return v
}

// renderSignedIn answers an action on the card with the card itself.
func (h *Handler) renderSignedIn(c echo.Context, userID, msg string) error {
	v := h.signedInView(c.Request().Context(), userID, getSessionToken(c), middleware.GetCSRFToken(c))
	v.Message = msg
	return middleware.Render(c, http.StatusOK, signedInCard(v))
}

// SignOutSessionAPI ends another of the user's browsers
// (POST /account/signed-in/sessions/:id/sign-out).
func (h *Handler) SignOutSessionAPI(c echo.Context) error {
	userID := GetUserID(c)
	if err := h.service.SignOutMySession(c.Request().Context(), userID, c.Param("id"), getSessionToken(c)); err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return h.renderSignedIn(c, userID, "That browser was already signed out.")
		}
		return err
	}
	h.logSecurityEvent(c.Request().Context(), eventSessionSignedOut, userID, userID, c.RealIP(), c.Request().UserAgent(), nil)
	return h.renderSignedIn(c, userID, "Signed out.")
}

// DisconnectAppAPI ends one of the user's connected apps
// (POST /account/signed-in/apps/:id/disconnect).
func (h *Handler) DisconnectAppAPI(c echo.Context) error {
	if h.apps == nil {
		return apperror.NewNotFound("app not found")
	}
	userID := GetUserID(c)
	if err := h.apps.DisconnectForUser(c.Request().Context(), userID, c.Param("id")); err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return h.renderSignedIn(c, userID, "That app was already disconnected.")
		}
		return err
	}
	h.logSecurityEvent(c.Request().Context(), eventAppDisconnected, userID, userID, c.RealIP(), c.Request().UserAgent(), nil)
	return h.renderSignedIn(c, userID, "Disconnected.")
}

// SignOutEverywhereAPI ends every session the user has, this one included,
// and every connected app with them, then sends the browser to sign in
// (POST /account/signed-in/sign-out-everywhere).
func (h *Handler) SignOutEverywhereAPI(c echo.Context) error {
	userID := GetUserID(c)
	n, err := h.service.DestroyAllUserSessions(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	h.logSecurityEvent(c.Request().Context(), eventSignedOutEverywhere, userID, userID, c.RealIP(), c.Request().UserAgent(),
		map[string]any{"sessions": n})
	clearSessionCookie(c)
	return middleware.HTMXRedirect(c, "/login")
}

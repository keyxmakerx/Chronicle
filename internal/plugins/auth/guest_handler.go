package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// JoinPage shows the guest code form (GET /join). A guest already belongs
// to a campaign, so they go back to it.
func (h *Handler) JoinPage(c echo.Context) error {
	sess := GetSession(c)
	if sess != nil && sess.GuestCampaignID != "" {
		return c.Redirect(http.StatusSeeOther, "/campaigns/"+sess.GuestCampaignID)
	}
	signedInAs := ""
	if sess != nil {
		signedInAs = sess.Name
	}
	return middleware.Render(c, http.StatusOK, JoinPage(middleware.GetCSRFToken(c), c.QueryParam("code"), "", "", signedInAs))
}

// Join takes a guest code (POST /join): a new guest is made and signed in,
// or someone already signed in is added to the campaign.
func (h *Handler) Join(c echo.Context) error {
	ctx := c.Request().Context()
	ip, ua := c.RealIP(), c.Request().UserAgent()
	sess := GetSession(c)
	in := JoinInput{Code: c.FormValue("code"), Name: c.FormValue("name"), IP: ip, UserAgent: ua}
	signedInAs := ""
	if sess != nil {
		in.SessionUserID, signedInAs = sess.UserID, sess.Name
	}
	res, err := h.service.JoinWithGuestCode(ctx, in)
	if err != nil {
		msg := apperror.UserMessage(err, "that code didn't work")
		csrf := middleware.GetCSRFToken(c)
		if middleware.IsHTMX(c) {
			return middleware.Render(c, http.StatusOK, JoinForm_(csrf, in.Code, in.Name, msg, signedInAs))
		}
		return middleware.Render(c, http.StatusOK, JoinPage(csrf, in.Code, in.Name, msg, signedInAs))
	}
	if res.SessionToken != "" {
		setSessionCookie(c, res.SessionToken, h.sessionTTL)
		h.logSecurityEvent(ctx, "guest.joined", res.User.ID, "", ip, ua, map[string]any{"campaign_id": res.CampaignID})
	}
	return middleware.HTMXRedirect(c, "/campaigns/"+res.CampaignID)
}

// GuestPanelFragment loads the keep-your-account panel under a guest's
// banner (GET /account/guest). tab=close empties it.
func (h *Handler) GuestPanelFragment(c echo.Context) error {
	sess := GetSession(c)
	if sess == nil || sess.GuestCampaignID == "" {
		return c.NoContent(http.StatusNoContent)
	}
	canKeep := h.service.GuestCanKeep(c.Request().Context())
	tab := c.QueryParam("tab")
	switch tab {
	case "close":
		return c.HTML(http.StatusOK, "")
	case guestTabMerge:
	case guestTabKeep:
	default:
		tab = guestTabKeep
		if !canKeep {
			tab = guestTabMerge
		}
	}
	return middleware.Render(c, http.StatusOK, GuestPanel(middleware.GetCSRFToken(c), tab, "", "", canKeep))
}

// KeepGuestAccount adds an email and password to a guest's account (POST
// /account/guest/keep).
func (h *Handler) KeepGuestAccount(c echo.Context) error {
	ctx := c.Request().Context()
	sess := GetSession(c)
	if sess == nil || sess.GuestCampaignID == "" {
		return apperror.NewBadRequest("this is already a full account")
	}
	ip, ua := c.RealIP(), c.Request().UserAgent()
	email := c.FormValue("email")
	token, err := h.service.KeepGuestAccount(ctx, sess.UserID, KeepGuestInput{Email: email, Password: c.FormValue("password")}, ip, ua)
	if err != nil {
		return middleware.Render(c, http.StatusOK, GuestPanel(middleware.GetCSRFToken(c), guestTabKeep, email, apperror.UserMessage(err, "your account couldn't be kept"), h.service.GuestCanKeep(ctx)))
	}
	setSessionCookie(c, token, h.sessionTTL)
	h.logSecurityEvent(ctx, "guest.kept", sess.UserID, sess.UserID, ip, ua, nil)
	return middleware.HTMXRedirect(c, "/campaigns/"+sess.GuestCampaignID)
}

// MergeGuest moves a guest into an account they already have (POST
// /account/guest/merge).
func (h *Handler) MergeGuest(c echo.Context) error {
	ctx := c.Request().Context()
	sess := GetSession(c)
	if sess == nil || sess.GuestCampaignID == "" {
		return apperror.NewBadRequest("this is already a full account")
	}
	ip, ua := c.RealIP(), c.Request().UserAgent()
	email := c.FormValue("email")
	token, user, err := h.service.MergeGuest(ctx, sess.UserID, MergeGuestInput{
		Email: email, Password: c.FormValue("password"), Code: c.FormValue("code"),
	}, ip, ua)
	if err != nil {
		return middleware.Render(c, http.StatusOK, GuestPanel(middleware.GetCSRFToken(c), guestTabMerge, email, apperror.UserMessage(err, "the accounts couldn't be merged"), h.service.GuestCanKeep(ctx)))
	}
	setSessionCookie(c, token, h.sessionTTL)
	h.logSecurityEvent(ctx, "guest.merged", user.ID, user.ID, ip, ua, map[string]any{"guest_id": sess.UserID})
	return middleware.HTMXRedirect(c, "/campaigns/"+sess.GuestCampaignID)
}

package foundry_vtt

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// AdminVersionCampaignsHandler serves the "Campaigns Using v0.1.5"
// HTMX fragment. Embedded into the packages plugin's /admin/packages
// page on each foundry-module typed package's version row.
//
// GET /admin/foundry-vtt/version/:version/campaigns
//
// Returns the campaignUsageList + version action row (Force-update
// older / Notify older). No empty-state error — an empty pinned list
// is the normal case for newly-installed versions.
func (h *Handler) AdminVersionCampaignsHandler(c echo.Context) error {
	version := c.Param("version")
	usage, err := h.svc.CampaignsUsingVersion(c.Request().Context(), version)
	if err != nil {
		return h.respondError(c, err)
	}
	return middleware.Render(c, http.StatusOK,
		AdminVersionCampaignsBlock(version, usage, middleware.GetCSRFToken(c)))
}

// AdminNotifyCampaignHandler logs the audit event + fires the SMTP
// courtesy email for a single campaign. Does NOT change the pin.
//
// POST /admin/foundry-vtt/version/:version/notify/:cid
func (h *Handler) AdminNotifyCampaignHandler(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}
	version := c.Param("version")
	campaignID := c.Param("cid")
	if err := h.svc.NotifyCampaignOfUpdate(c.Request().Context(),
		campaignID, version, session.UserID, c.RealIP(), c.Request().UserAgent()); err != nil {
		return h.respondError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AdminForcePinCampaignHandler directly mutates a campaign's
// FoundryModulePin to the target version. Requires admin password
// re-auth (applied at the route level via auth.RequireReauth).
//
// POST /admin/foundry-vtt/version/:version/force-pin/:cid
func (h *Handler) AdminForcePinCampaignHandler(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}
	version := c.Param("version")
	campaignID := c.Param("cid")
	if err := h.svc.ForcePinCampaign(c.Request().Context(),
		campaignID, version, session.UserID, c.RealIP(), c.Request().UserAgent()); err != nil {
		return h.respondError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AdminNotifyOlderHandler fans out NotifyCampaignOfUpdate to every
// campaign whose pin is strictly older than the target version.
//
// POST /admin/foundry-vtt/version/:version/notify-older
//
// Returns {notified: N}. Partial failures don't abort — N reflects
// successes only.
func (h *Handler) AdminNotifyOlderHandler(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}
	version := c.Param("version")
	notified, err := h.svc.NotifyOlderCampaigns(c.Request().Context(),
		version, session.UserID, c.RealIP(), c.Request().UserAgent())
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"notified": notified})
}

// AdminForcePinOlderHandler fans out ForcePinCampaign to every
// campaign whose pin is strictly older than the target version.
// Requires admin password re-auth (applied at the route level).
//
// POST /admin/foundry-vtt/version/:version/force-pin-older
//
// Returns {pinned: N}.
func (h *Handler) AdminForcePinOlderHandler(c echo.Context) error {
	session := auth.GetSession(c)
	if session == nil {
		return apperror.NewUnauthorized("not authenticated")
	}
	version := c.Param("version")
	pinned, err := h.svc.ForcePinAllToVersion(c.Request().Context(),
		version, session.UserID, c.RealIP(), c.Request().UserAgent())
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"pinned": pinned})
}

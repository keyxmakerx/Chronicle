package foundry_vtt

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
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

// AdminAutoPinBannerHandler returns the auto-pin notification banner
// fragment if an unread summary exists, or empty if not. Embedded
// into /admin/packages via an hx-get block — packages.templ stays
// foundry-agnostic; this handler is the only place that knows the
// banner shape.
//
// GET /admin/foundry-vtt/autopin-banner
//
// Always returns 200 OK; the response is either the banner HTML or
// an empty body. Empty body = HTMX swaps nothing visible into the
// target div, so the banner gracefully absents itself.
func (h *Handler) AdminAutoPinBannerHandler(c echo.Context) error {
	summary, err := h.svc.GetUnreadAutoPinSummary(c.Request().Context())
	if err != nil {
		// Banner is supplementary; never fail the admin page load
		// over a banner-read issue. Log via the framework + emit
		// an empty body.
		return c.NoContent(http.StatusOK)
	}
	if summary == nil || summary.Affected == 0 {
		// No unread summary, or the install affected zero campaigns
		// (nothing meaningful to surface). Empty body.
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, AutoPinBanner(*summary))
}

// AdminAutoPinBannerDismissHandler stamps the dismissal timestamp.
// The next AdminAutoPinBannerHandler request will return empty.
//
// POST /admin/foundry-vtt/autopin-banner/dismiss
func (h *Handler) AdminAutoPinBannerDismissHandler(c echo.Context) error {
	if err := h.svc.DismissAutoPinBanner(c.Request().Context()); err != nil {
		return h.respondError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AdminPackageActionsFragmentHandler returns the per-row foundry-module
// action UI (API monitor link + Versions trigger) as an HTMX fragment
// for the /admin/packages page. packages.templ stays foundry-agnostic;
// this handler is the only place that knows the foundry-module per-row
// shape.
//
// GET /admin/foundry-vtt/packages/:id/actions-fragment
//
// Guards:
//   - 404 if the package ID doesn't exist
//   - 404 if the package isn't a foundry-module typed package (defensive
//     — packages.templ should only lazy-load this URL for foundry-module
//     rows, but a direct request from a poking admin shouldn't render
//     foundry UI for system packages)
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md + NW-2.2
// Chunk G.
func (h *Handler) AdminPackageActionsFragmentHandler(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return apperror.NewNotFound("package")
	}

	pkg, err := h.svc.GetPackageByID(c.Request().Context(), id)
	if err != nil || pkg == nil {
		// Package not found (or lookup error treated as not-found from
		// the admin UI's perspective — fragment 404s silently and the
		// row's lazy-load slot stays empty).
		return apperror.NewNotFound("package")
	}
	if pkg.Type != packages.PackageTypeFoundryModule {
		// Defensive: a system package's ID can't render the foundry
		// fragment. Return 404 so packages.templ's lazy-load swap
		// inserts an empty body for the wrong-type case.
		return apperror.NewNotFound("package")
	}

	// csrfToken passed through for parity with packages.templ's
	// per-row signature; current fragment contents are read-only but
	// future state-changing per-row actions would use it.
	csrfToken := middleware.GetCSRFToken(c)

	return middleware.Render(c, http.StatusOK, AdminPackageActionsFragment(*pkg, csrfToken))
}

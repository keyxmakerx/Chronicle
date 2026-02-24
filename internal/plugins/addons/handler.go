package addons

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles addon-related HTTP requests. Admin routes manage the global
// registry; campaign-scoped routes manage per-campaign toggles.
type Handler struct {
	service AddonService
}

// NewHandler creates a new addon handler.
func NewHandler(service AddonService) *Handler {
	return &Handler{service: service}
}

// --- Admin Routes (site admin) ---

// AdminAddonsPage renders the admin addon management page (GET /admin/addons).
func (h *Handler) AdminAddonsPage(c echo.Context) error {
	addons, err := h.service.List(c.Request().Context())
	if err != nil {
		return err
	}
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, AdminAddonsPageTempl(addons, csrfToken))
}

// CreateAddon handles POST /admin/addons to register a new addon.
func (h *Handler) CreateAddon(c echo.Context) error {
	input := CreateAddonInput{
		Slug:        c.FormValue("slug"),
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		Version:     c.FormValue("version"),
		Category:    AddonCategory(c.FormValue("category")),
		Icon:        c.FormValue("icon"),
		Author:      c.FormValue("author"),
	}

	_, err := h.service.Create(c.Request().Context(), input)
	if err != nil {
		return err
	}

	// HTMX: reload the addons list.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/addons")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/addons")
}

// UpdateAddonStatus handles PUT /admin/addons/:addonID/status.
func (h *Handler) UpdateAddonStatus(c echo.Context) error {
	addonID, err := strconv.Atoi(c.Param("addonID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid addon ID")
	}

	status := AddonStatus(c.FormValue("status"))
	if err := h.service.UpdateStatus(c.Request().Context(), addonID, status); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/addons")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/addons")
}

// DeleteAddon handles DELETE /admin/addons/:addonID.
func (h *Handler) DeleteAddon(c echo.Context) error {
	addonID, err := strconv.Atoi(c.Param("addonID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid addon ID")
	}

	if err := h.service.Delete(c.Request().Context(), addonID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/addons")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/addons")
}

// --- Campaign-scoped Routes (campaign owner) ---

// CampaignAddonsAPI returns the addon list with per-campaign enabled state (GET /campaigns/:id/addons).
func (h *Handler) CampaignAddonsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "campaign context required")
	}

	addons, err := h.service.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, addons)
}

// CampaignAddonsPage renders the campaign addons settings section (GET /campaigns/:id/addons/settings).
func (h *Handler) CampaignAddonsPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "campaign context required")
	}

	addons, err := h.service.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignAddonsPageTempl(cc.Campaign.ID, addons, csrfToken))
}

// CampaignAddonsFragment returns the addons list fragment for embedding in the
// Customization Hub Extensions tab (GET /campaigns/:id/addons/fragment).
func (h *Handler) CampaignAddonsFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "campaign context required")
	}

	addons, err := h.service.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, CampaignAddonsListFragment(cc.Campaign.ID, addons, csrfToken))
}

// ToggleCampaignAddon handles PUT /campaigns/:id/addons/:addonID/toggle.
func (h *Handler) ToggleCampaignAddon(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "campaign context required")
	}

	addonID, err := strconv.Atoi(c.Param("addonID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid addon ID")
	}

	userID := auth.GetUserID(c)
	action := c.FormValue("action") // "enable" or "disable"

	ctx := c.Request().Context()
	if action == "enable" {
		if err := h.service.EnableForCampaign(ctx, cc.Campaign.ID, addonID, userID); err != nil {
			return err
		}
	} else {
		if err := h.service.DisableForCampaign(ctx, cc.Campaign.ID, addonID); err != nil {
			return err
		}
	}

	// Return updated addon list for HTMX swap.
	if middleware.IsHTMX(c) {
		addons, err := h.service.ListForCampaign(ctx, cc.Campaign.ID)
		if err != nil {
			return err
		}
		csrfToken := middleware.GetCSRFToken(c)
		return middleware.Render(c, http.StatusOK, CampaignAddonsListFragment(cc.Campaign.ID, addons, csrfToken))
	}

	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/addons/settings")
}
